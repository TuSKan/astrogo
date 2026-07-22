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

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

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

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

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

type mockTransport struct {
	Handler http.Handler
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.Handler.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req

	return resp, nil
}

func TestProviderInterface(t *testing.T) {
	p := New()
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
