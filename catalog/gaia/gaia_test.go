package gaia

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/provider"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestGaiaOfflineConeSearch(t *testing.T) {
	csvData := `source_id,ra,dec,pmra,pmdec,parallax
123456789,10.684,41.269,1.1,-2.2,5.5
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		fmt.Fprint(w, csvData)
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	req := provider.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(5),
	}

	iter := prov.ConeSearch(context.Background(), req)

	var targets []provider.Target
	iter(func(tar provider.Target, err error) bool {
		testutil.AssertNoError(t, err)
		targets = append(targets, tar)
		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "ID", targets[0].ID, "123456789")
	testutil.AssertEqual(t, "Kind", string(targets[0].Kind), string(provider.KindStar))
	testutil.AssertEqual(t, "Catalog", targets[0].Catalog, "Gaia DR3")
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
	if len(caps) != 1 || caps[0] != provider.CapConeSearch {
		t.Errorf("expected CapConeSearch, got %v", caps)
	}
	_, ok := p.Resolve("foo")
	if ok {
		t.Error("expected Resolve to return false")
	}
	if p.Search("foo") != nil {
		t.Error("expected Search to return nil")
	}
}
