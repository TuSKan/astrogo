package mast

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestMastOfflineResolve(t *testing.T) {
	jsonPayload := `{
	"resolvedCoordinate": [{
		"resolver": "NED",
		"ra": 10.684,
		"decl": 41.269,
		"canonicalName": "M31"
	}],
	"status": "COMPLETE"
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jsonPayload)
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	req := resolve.ObjectRequest{Query: "M31"}
	iter := prov.ResolveObject(context.Background(), req)

	var targets []resolve.Target
	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)
		targets = append(targets, tar)
		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "Name", targets[0].Name, "M31")
	testutil.AssertEqual(t, "Catalog", targets[0].Catalog, "NED")
	testutil.AssertEqual(t, "RA", targets[0].Coord.RA().Degrees(), 10.684)
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
	if p.Name() != "mast" {
		t.Errorf("expected mast, got %s", p.Name())
	}
	caps := p.Capabilities()
	if len(caps) != 2 || caps[0] != resolve.CapObjectResolution || caps[1] != resolve.CapConeSearch {
		t.Errorf("expected CapObjectResolution and CapConeSearch, got %v", caps)
	}
	_, _ = p.Resolve("non_existent_body")
	_ = p.Search("non_existent_body")
}
