package starlight

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/remote"
)

// fetchHipparcos's request path, against a fake VizieR.
//
// It was covered only by a network-tagged test, so the query this package
// actually sends — the one thing a parser test cannot check — was never
// exercised in CI. The assertions are on what the service received: an ADQL
// statement naming the right table with the right magnitude cut, posted as a
// TAP form.

func TestFetchHipparcosPostsTheQueryAndParsesTheAnswer(t *testing.T) {
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotQuery = r.PostForm.Get("QUERY")

		_, _ = w.Write([]byte("HIP,RAICRS,DEICRS,Vmag,B-V,V-I,pmRA,pmDE,RAhms,DEdms\n" +
			"3,0.00500795,38.85928608,6.61,0.51,0.60,5.24,-2.91,00 00 01.20,+38 51 33.4\n"))
	}))
	defer srv.Close()

	scope := remote.Capture(remote.VizieR)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.VizieR, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	stars, pmRA, pmDec, err := fetchHipparcos(t.Context(), 6.5)
	if err != nil {
		t.Fatalf("fetchHipparcos: %v", err)
	}

	if len(stars) != 1 || len(pmRA) != 1 || len(pmDec) != 1 {
		t.Fatalf("got %d stars, %d pmRA, %d pmDec; want 1 of each", len(stars), len(pmRA), len(pmDec))
	}

	// The magnitude cut is the caller's argument, and getting it wrong would
	// silently change how much of the sky the map covers.
	if !strings.Contains(gotQuery, "Vmag < 6.5") {
		t.Errorf("query = %q, want the caller's magnitude cut", gotQuery)
	}

	if !strings.Contains(gotQuery, "I/239/hip_main") {
		t.Errorf("query = %q, want the Hipparcos main table", gotQuery)
	}

	// B-V and V-I are what make a multi-band map possible without a colour
	// fit, so their absence from the query would be a silent loss of colour.
	for _, col := range []string{"B-V", "V-I"} {
		if !strings.Contains(gotQuery, col) {
			t.Errorf("query = %q, want it to select %s", gotQuery, col)
		}
	}
}

// A service that declines must surface as an error rather than an empty star
// list, which would render as a sky with no stars in it.
func TestFetchHipparcosReportsAServiceFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "query rejected", http.StatusBadRequest)
	}))
	defer srv.Close()

	scope := remote.Capture(remote.VizieR)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.VizieR, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	if _, _, _, err := fetchHipparcos(t.Context(), 6.5); err == nil {
		t.Fatal("a rejected query produced a star list")
	}
}

// TestAddCousinsRQueriesTheBrightStarCatalogue covers the second VizieR query
// this package sends, for the same reason as the first: the ADQL is the part no
// parser test can check.
func TestAddCousinsRQueriesTheBrightStarCatalogue(t *testing.T) {
	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotQuery = r.PostForm.Get("QUERY")

		// One catalogue row at the same position as the star below, so the
		// positional match succeeds and R is filled in.
		_, _ = w.Write([]byte("RAJ2000,DEJ2000,Vmag,R-I\n" +
			"0.00500795,38.85928608,6.61,0.35\n"))
	}))
	defer srv.Close()

	scope := remote.Capture(remote.VizieR)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.VizieR, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	stars := []BrightStar{{
		RA:   angle.Deg(0.00500795),
		Dec:  angle.Deg(38.85928608),
		Vmag: 6.61,
		Mag:  map[string]float64{"V": 6.61},

		// Only a star already carrying V-I is a candidate for R: the
		// catalogue supplies R-I, and R needs both.
		hasVminusI: true,
	}}

	matched, err := AddCousinsR(t.Context(), stars)
	if err != nil {
		t.Fatalf("AddCousinsR: %v", err)
	}

	if !strings.Contains(gotQuery, "V/50/catalog") {
		t.Errorf("query = %q, want the Bright Star Catalogue table", gotQuery)
	}

	if !strings.Contains(gotQuery, "R-I") {
		t.Errorf("query = %q, want it to select R-I — the colour this whole "+
			"function exists to fetch", gotQuery)
	}

	if matched != 1 {
		t.Errorf("matched = %d, want 1 — the catalogue row is at the star's own position", matched)
	}
}
