package gaia

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestResolveIdentifiersAsksForTheSourcesNamed: of a SIMBAD record's
// identifiers, only a Gaia DR3 source ID that is a number goes into the
// query, once, and the row comes back with its designation.
func TestResolveIdentifiersAsksForTheSourcesNamed(t *testing.T) {
	var query testutil.Recorded[string]

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		query.Set(r.PostFormValue("QUERY"))

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, barnardRow)
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	ids := []resolve.Identifier{
		{Namespace: "hip", Value: "87937"},
		{Namespace: "gaia", Release: "DR2", Value: "4472832130931075840"},
		{Namespace: "gaia", Release: "DR3", Value: "4472832130942575872"},
		{Namespace: "gaia", Release: "DR3", Value: "4472832130942575872"},
		{Namespace: "gaia", Release: "DR3", Value: "1 OR 1=1"},
	}

	got, err := resolve.Drain(prov.ResolveIdentifiers(context.Background(), ids), 0)
	testutil.AssertNoError(t, err)

	adql := query.Get()
	if !strings.Contains(adql, "TOP 1 ") || !strings.Contains(adql, "WHERE source_id IN (4472832130942575872)") {
		t.Errorf("the query does not ask for exactly the one DR3 source:\n%s", adql)
	}

	if len(got) != 1 || got[0].Designation != "Gaia DR3 4472832130942575872" {
		t.Errorf("got %+v, want Barnard's star's row with its designation", got)
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

	ids := []resolve.Identifier{{Namespace: "hip", Value: "87937"}, {Namespace: "gaia", Release: "DR2", Value: "1"}}

	got, err := resolve.Drain(prov.ResolveIdentifiers(context.Background(), ids), 0)
	testutil.AssertNoError(t, err)

	if len(got) != 0 || requests.Load() != 0 {
		t.Errorf("%d targets from %d requests, want none from none", len(got), requests.Load())
	}
}
