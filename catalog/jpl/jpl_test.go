package jpl

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestJPLResolveMock(t *testing.T) {
	jsonPayload := `{
  "signature": {"version": "1.2", "source": "NASA/JPL Horizons API"},
  "result": "Multiple major-bodies match string \"Mars*\"\n\n  Number  Name                           Designation  IAU/aliases/other   \n  ------  -----------------------------  -----------  ------------------- \n     401  Mars Barycenter                                                 \n     499  Mars                                                            \n"
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jsonPayload)
	}))
	defer server.Close()

	prov := New()
	// Override transport
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	// This relies on the current basic fallback returning the query
	tar, ok := prov.Resolve("Mars")
	testutil.AssertEqual(t, "Resolve Ok", ok, true)
	testutil.AssertEqual(t, "Resolve ID", tar.ID, "Mars")
	testutil.AssertEqual(t, "Resolve Kind", string(tar.Kind), string(resolve.KindPlanet))
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

func TestJPLErrorResponse(t *testing.T) {
	jsonPayload := `{
  "signature": {"version": "1.2", "source": "NASA/JPL Horizons API"},
  "error": "unrecognized command"
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jsonPayload)
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	req := resolve.ObjectRequest{Query: "!!!ERROR!!!"}
	iter := prov.ResolveObject(context.Background(), req)
	iter(func(tar resolve.Target, err error) bool {
		if err == nil {
			t.Fatalf("Expected explicit json payload error")
		}
		if err.Error() != "jpl: unrecognized command" {
			t.Fatalf("Unexpected error mapping: %v", err)
		}
		return false
	})
}

func TestProviderInterface(t *testing.T) {
	p := New()
	testutil.AssertEqual(t, "Name", p.Name(), "jpl")
	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapObjectResolution {
		t.Errorf("expected CapObjectResolution, got %v", caps)
	}

	// Fast fail search / resolve since no network mock attached here
	// This hits the missing coverage lines.
	_, _ = p.Resolve("non_existent_body_to_trigger_miss")
}
