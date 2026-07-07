package jpl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestJPLResolveMock(t *testing.T) {
	jsonPayload := `{
  "signature": {"version": "1.2", "source": "NASA/JPL Horizons API"},
  "result": "Multiple major-bodies match string \"Mars*\"\n\n  Number  Name                           Designation  IAU/aliases/other   \n  ------  -----------------------------  -----------  ------------------- \n     401  Mars Barycenter                                                 \n     499  Mars                                                            \n"
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, jsonPayload); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()
	// Override transport
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	// The provider does not parse Horizons' free-text result block (see
	// ErrNotImplemented) — a successful, decodable API response with data it
	// can't parse must surface as an explicit error, not a fabricated Target.
	_, ok := prov.Resolve("Mars")
	testutil.AssertEqual(t, "Resolve Ok", ok, false)
}

func TestJPLResolveObjectNotImplemented(t *testing.T) {
	jsonPayload := `{
  "signature": {"version": "1.2", "source": "NASA/JPL Horizons API"},
  "result": "Multiple major-bodies match string \"Mars*\"\n\n  Number  Name                           Designation  IAU/aliases/other   \n  ------  -----------------------------  -----------  ------------------- \n     401  Mars Barycenter                                                 \n     499  Mars                                                            \n"
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, jsonPayload); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	iter := prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "Mars"})
	iter(func(_ resolve.Target, err error) bool {
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("expected ErrNotImplemented, got %v", err)
		}

		return false
	})
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, jsonPayload); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	req := resolve.ObjectRequest{Query: "!!!ERROR!!!"}
	iter := prov.ResolveObject(context.Background(), req)
	iter(func(_ resolve.Target, err error) bool {
		if err == nil {
			t.Fatalf("Expected explicit json payload error")
		}

		if !errors.Is(err, ErrAPIError) {
			t.Fatalf("Expected ErrAPIError, got: %v", err)
		}

		if !strings.Contains(err.Error(), "unrecognized command") {
			t.Fatalf("Unexpected error mapping: %v", err)
		}

		return false
	})
}

// errTransport fails every request locally, so exercising the miss path
// below never reaches the real network (this is a default, non-network-tagged
// test — see CLAUDE.md's build-tag convention).
type errTransport struct{}

var errNoTransport = errors.New("errTransport: no network access in this test")

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errNoTransport
}

func TestProviderInterface(t *testing.T) {
	p := New()
	testutil.AssertEqual(t, "Name", p.Name(), "jpl")

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapObjectResolution {
		t.Errorf("expected CapObjectResolution, got %v", caps)
	}

	p.client.HTTPClient.Transport = errTransport{}

	// Fast fail search / resolve without any real network call.
	// This hits the missing coverage lines.
	_, ok := p.Resolve("non_existent_body_to_trigger_miss")
	if ok {
		t.Error("expected Resolve to fail with no transport")
	}
}
