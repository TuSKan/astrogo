package gaia

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestResolveIdentifiersAsksForTheSourcesNamed: of a SIMBAD record's
// identifiers, a Gaia DR3 source ID goes into the query, once, and a
// Hipparcos-2 or Tycho-2 identifier goes in through Gaia's cross-match of
// that catalog. Another release's ID, and a value that is not an identifier
// of its scheme, do not; the row comes back with its designation.
func TestResolveIdentifiersAsksForTheSourcesNamed(t *testing.T) {
	var (
		mu      sync.Mutex
		queries []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mu.Lock()

		queries = append(queries, r.PostFormValue("QUERY"))

		mu.Unlock()

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, barnardRow)
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	ids := []resolve.Identifier{
		{Namespace: "hip", Value: "87937"},
		{Namespace: "tyc", Value: "425-2502-1"},
		{Namespace: "gaia", Release: "DR2", Value: "4472832130931075840"},
		{Namespace: "gaia", Release: "DR3", Value: "4472832130942575872"},
		{Namespace: "gaia", Release: "DR3", Value: "4472832130942575872"},
		{Namespace: "gaia", Release: "DR3", Value: "1 OR 1=1"},
		{Namespace: "hip", Value: "1 OR 1=1"},
		{Namespace: "tyc", Value: "1-2-3' OR '1"},
	}

	got, err := resolve.Drain(prov.ResolveIdentifiers(context.Background(), ids), 0)
	testutil.AssertNoError(t, err)

	mu.Lock()
	asked, adql := len(queries), strings.Join(queries, "\n")
	mu.Unlock()

	if asked != 3 {
		t.Errorf("%d queries, want one for each of the three schemes:\n%s", asked, adql)
	}

	for _, want := range []string{
		"WHERE g.source_id IN (4472832130942575872)",
		"WHERE hip.original_ext_source_id IN (87937)",
		"WHERE tyc.original_ext_source_id IN ('425-2502-1')",
	} {
		if !strings.Contains(adql, want) {
			t.Errorf("the queries lack %q:\n%s", want, adql)
		}
	}

	for _, unwanted := range []string{"4472832130931075840", "OR 1", "OR '1"} {
		if strings.Contains(adql, unwanted) {
			t.Errorf("the queries hold %q:\n%s", unwanted, adql)
		}
	}

	// Each query answers with Barnard's star; it comes back once.
	if len(got) != 1 || got[0].Designation != "Gaia DR3 4472832130942575872" {
		t.Errorf("got %+v, want Barnard's star's row once, with its designation", got)
	}
}

// TestResolveIdentifiersWithNoneOfItsOwnAsksNothing: identifiers in other
// naming schemes, or of another Gaia release, name no Gaia DR3 source and
// cost no request.
func TestResolveIdentifiersWithNoneOfItsOwnAsksNothing(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, barnardRow)
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	ids := []resolve.Identifier{{Namespace: "hd", Value: "164428"}, {Namespace: "gaia", Release: "DR2", Value: "1"}}

	got, err := resolve.Drain(prov.ResolveIdentifiers(context.Background(), ids), 0)
	testutil.AssertNoError(t, err)

	if len(got) != 0 || requests.Load() != 0 {
		t.Errorf("%d targets from %d requests, want none from none", len(got), requests.Load())
	}
}

// crossMatchRows are answer rows with the cross-match columns: source 1 with
// clean Hipparcos-2 and Tycho-2 matches, as Barnard's star's are (xm_flag 72);
// source 2 with a Hipparcos-2 object Gaia resolves into several sources (4);
// source 3 named by two Hipparcos-2 objects, one row for each.
const crossMatchRows = "source_id,ra,dec,hip_id,hip_xm_flag,tyc_id,tyc_xm_flag\n" +
	"1,10,20,87937,72,425-2502-1,72\n" +
	"2,11,21,100,12,1-2-1,8\n" +
	"3,12,22,200,8,3-4-1,8\n" +
	"3,12,22,201,8,3-4-1,8\n"

// TestARowCarriesItsUnambiguousCounterparts: a clean best neighbor becomes
// an identifier SIMBAD's cross-identification shares a key with; one Gaia
// resolves, or one of two that name the same source, does not; and a source
// with two rows is one target.
func TestARowCarriesItsUnambiguousCounterparts(t *testing.T) {
	t.Parallel()

	targets, _, err := parseResult(strings.NewReader(crossMatchRows))
	testutil.AssertNoError(t, err)

	got := make(map[string][]string, len(targets))
	for _, tg := range targets {
		got[tg.ID] = tg.Aliases
	}

	want := map[string][]string{
		"1": {"HIP 87937", "TYC 425-2502-1"},
		"2": {"TYC 1-2-1"},
		"3": {"TYC 3-4-1"},
	}

	if len(targets) != 3 {
		t.Fatalf("%d targets, want one for each of the 3 sources", len(targets))
	}

	for id, aliases := range want {
		if !slices.Equal(got[id], aliases) {
			t.Errorf("source %s: aliases %v, want %v", id, got[id], aliases)
		}
	}

	// The keys SIMBAD's cross-identifications of Barnard's star give.
	simbad := resolve.IdentityKeys(resolve.Target{Aliases: []string{"HIP 87937", "TYC 425-2502-1"}})
	for _, k := range simbad {
		if !slices.Contains(resolve.IdentityKeys(targets[0]), k) {
			t.Errorf("source 1's keys %v lack SIMBAD's %q", resolve.IdentityKeys(targets[0]), k)
		}
	}
}
