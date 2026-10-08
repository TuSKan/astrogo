package sbdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// The bodies under testdata are SBDB's answers verbatim, fetched 2026-10-07
// with the parameters this package sends (phys-par, full-prec), and served
// with the status SBDB gave each: 200 for an object and for "not found", 300
// for a list.

// sbdbAnswer is one canned SBDB answer.
type sbdbAnswer struct {
	status  int
	fixture string
}

// serveSBDB points SBDB at a server answering by the request's key=value
// ("sstr=Vega", "des=73P"), and returns the keys it was asked, in order.
func serveSBDB(t *testing.T, answers map[string]sbdbAnswer) *[]string {
	t.Helper()
	t.Cleanup(remote.Reset)

	var asked []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		key := "sstr=" + q.Get("sstr")
		if q.Has("des") {
			key = "des=" + q.Get("des")
		}

		asked = append(asked, key)

		a, ok := answers[key]
		if !ok {
			t.Errorf("unexpected request %q", key)
			http.Error(w, "unexpected", http.StatusTeapot)

			return
		}

		body, err := os.ReadFile("testdata/" + a.fixture)
		if err != nil {
			t.Error(err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(a.status)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.JPLSBDB, srv.URL); err != nil {
		t.Fatal(err)
	}

	return &asked
}

// TestUnknownNameIsNotFound: SBDB's "specified object was not found" is an
// answer, not a failure. It used to be ErrAPIError, so a catalog.Resolver
// with SBDB registered could never report ErrNotFound.
func TestUnknownNameIsNotFound(t *testing.T) {
	serveSBDB(t, map[string]sbdbAnswer{"sstr=Vega": {http.StatusOK, "sstr-vega.json"}})

	_, err := New().Resolve(context.Background(), "Vega")
	if !errors.Is(err, resolve.ErrNotFound) {
		t.Fatalf("Resolve(Vega) = %v, want ErrNotFound", err)
	}
}

// TestNonUniqueNameIsAmbiguous: "Halley" matches 2688 Halley and 1P/Halley,
// and neither designation is the query, so there is nothing to pick.
func TestNonUniqueNameIsAmbiguous(t *testing.T) {
	serveSBDB(t, map[string]sbdbAnswer{"sstr=Halley": {http.StatusMultipleChoices, "sstr-halley.json"}})

	_, err := New().Resolve(context.Background(), "Halley")
	if !errors.Is(err, resolve.ErrAmbiguous) {
		t.Fatalf("Resolve(Halley) = %v, want ErrAmbiguous", err)
	}

	for _, name := range []string{"2688 Halley (1982 HG1)", "1P/Halley"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not name %q", err, name)
		}
	}
}

// TestDesignationAmongMatchesIsSelected: "73P" matches the comet and its 73
// fragments, and one entry's designation is "73P" itself. SBDB documents
// asking again with des= to select it.
func TestDesignationAmongMatchesIsSelected(t *testing.T) {
	asked := serveSBDB(t, map[string]sbdbAnswer{
		"sstr=73P": {http.StatusMultipleChoices, "sstr-73p.json"},
		"des=73P":  {http.StatusOK, "des-73p.json"},
	})

	got, err := New().Resolve(context.Background(), "73P")
	testutil.AssertNoError(t, err)

	testutil.AssertEqual(t, "SPKID", got.SPKID, "1000394")
	testutil.AssertEqual(t, "Name", got.Name, "73P/Schwassmann-Wachmann 3")
	testutil.AssertEqual(t, "requests", strings.Join(*asked, ", "), "sstr=73P, des=73P")

	if !got.HasElements {
		t.Error("HasElements = false: the des= answer carries the full orbit")
	}
}

// TestAnswerWithNothingIsAnError: a payload with no object, list or message
// used to come back as a zero Target with a nil error.
func TestAnswerWithNothingIsAnError(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"signature":{"version":"1.3","source":"NASA/JPL Small-Body Database (SBDB) API"}}`))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.JPLSBDB, srv.URL); err != nil {
		t.Fatal(err)
	}

	got, err := New().Resolve(context.Background(), "Eros")
	if !errors.Is(err, ErrAPIError) {
		t.Fatalf("Resolve = %+v, %v, want ErrAPIError", got, err)
	}
}

func TestExactDesignation(t *testing.T) {
	list := []lookupMatch{{PDes: "73P", Name: "73P/Schwassmann-Wachmann 3"}, {PDes: "73P-A", Name: "73P/Schwassmann-Wachmann 3-A"}}

	if got, ok := exactDesignation("73p", list); !ok || got != "73P" {
		t.Errorf("exactDesignation(73p) = %q, %v, want 73P, true", got, ok)
	}

	if _, ok := exactDesignation("Schwassmann", list); ok {
		t.Error("exactDesignation(Schwassmann) found a designation")
	}

	// Two entries with the query's designation leave nothing to pick.
	if _, ok := exactDesignation("73P", append(list, lookupMatch{PDes: "73 P"})); ok {
		t.Error("exactDesignation found one of two equal designations")
	}
}

func TestAmbiguousNamesTheFirstFew(t *testing.T) {
	list := make([]lookupMatch, 7)
	for i := range list {
		list[i] = lookupMatch{PDes: string(rune('A' + i)), Name: "n" + string(rune('A'+i))}
	}

	err := ambiguous("q", list)
	testutil.AssertEqual(t, "message", err.Error(),
		`ambiguous target name: "q" in sbdb matches 7 objects: nA; nB; nC; nD; nE and 2 more`)
}
