package gaia

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/remote"
)

func TestGaiaOfflineConeSearch(t *testing.T) {
	csvData := `source_id,ra,dec,pmra,pmdec,parallax
123456789,10.684,41.269,1.1,-2.2,5.5
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")

		if _, err := fmt.Fprint(w, csvData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(5),
	}

	iter := prov.ConeSearch(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "ID", targets[0].ID, "123456789")
	testutil.AssertEqual(t, "Kind", string(targets[0].Kind), string(resolve.KindStar))
	testutil.AssertEqual(t, "Catalog", targets[0].Catalog, "Gaia DR3")
}

// TestGaiaOfflineConeSearch_SkipsUnparseableRow is a regression test: a row
// with a malformed RA/Dec must be skipped entirely, never silently become a
// fake (0,0) position reported as HasCoord=true (the bug class this
// provider used to have — see catalog/catalog.go's trustworthyCoord, which
// exists as defense in depth against exactly this).
func TestGaiaOfflineConeSearch_SkipsUnparseableRow(t *testing.T) {
	csvData := `source_id,ra,dec,pmra,pmdec,parallax
111111111,not-a-number,41.269,1.1,-2.2,5.5
222222222,10.684,41.269,1.1,-2.2,5.5
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")

		if _, err := fmt.Fprint(w, csvData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(5),
	}

	iter := prov.ConeSearch(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("expected the unparseable row to be skipped, leaving 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "ID", targets[0].ID, "222222222")

	if !targets[0].HasCoord || targets[0].Coord.IsZero() {
		t.Errorf("expected a real, non-zero coordinate, got HasCoord=%v Coord=%v", targets[0].HasCoord, targets[0].Coord)
	}
}

func TestProviderInterface(t *testing.T) {
	p := newForTest(t)
	testutil.AssertEqual(t, "Name", p.Name(), "gaia")

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapConeSearch {
		t.Errorf("expected CapConeSearch, got %v", caps)
	}

	_, ok := p.Resolve(context.Background(), "foo")
	if ok {
		t.Error("expected Resolve to return false")
	}

	if p.Search(context.Background(), "foo") != nil {
		t.Error("expected Search to return nil")
	}
}

// redirect points endpoint id at a test server for the duration of one
// test. It replaces the old http.RoundTripper injection: remote/api's
// Client is opaque by design, and every request resolves its URL through
// remote.URL(id) anyway, so the registry is the natural seam.
// newForTest builds a provider against the default archive.
//
// The tests redirect [DefaultEndpoint] to a local server, so what they
// exercise is this package's request building and CSV parsing rather than any
// archive. Naming the constant rather than an endpoint keeps the two in step:
// were the default to move, a test redirecting the old one would quietly
// exercise nothing.
func newForTest(t *testing.T) *Provider {
	t.Helper()

	p, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return p
}

func redirect(t *testing.T, url string) {
	t.Helper()

	scope := remote.Capture(DefaultEndpoint)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(DefaultEndpoint, url); err != nil {
		t.Fatalf("SetURL(%s): %v", DefaultEndpoint, err)
	}
}
