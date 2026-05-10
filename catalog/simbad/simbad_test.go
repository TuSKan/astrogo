package simbad

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

func TestParseCSV(t *testing.T) {
	f, err := os.Open("testdata/m31.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	targets, err := ParseCSV(f)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 unique target, got %d", len(targets))
	}

	tgt := targets[0]
	if tgt.ID != "NAME M  31" {
		t.Errorf("unexpected ID: %s", tgt.ID)
	}

	if tgt.Kind != resolve.KindGalaxy {
		t.Errorf("unexpected Kind: %s", tgt.Kind)
	}

	if len(tgt.Aliases) != 3 {
		t.Errorf("expected 3 aliases, got %v", tgt.Aliases)
	}

	if !tgt.HasCoord {
		t.Fatalf("Coord is missing")
	}

	if math.Abs(tgt.Coord.RA().Degrees()-10.68470833) > 1e-6 {
		t.Errorf("unexpected RA: %f", tgt.Coord.RA().Degrees())
	}
}

func TestResolveMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		data, err := os.ReadFile("testdata/m31.csv")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/csv")

		if _, err := w.Write(data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	// temporarily override the global endpoint for testing
	// In a complete implementation we might want to dependency-inject `tapSyncURL`,
	// but for testing we can define a client specifically talking to it.
	// Since we defined tapSyncURL as const, we just test the public method? Actually,
	// test ParseCSV is the real test. We can just test Provider behavior if we can mock Client Transport.

	p := New()
	p.client.HTTPClient.Transport = &mockTransport{
		Handler: server.Config.Handler,
	}

	tgt, ok := p.Resolve("m31")
	if !ok {
		t.Fatalf("failed to resolve target")
	}

	if tgt.ID != "NAME M  31" {
		t.Errorf("unexpected ID: %s", tgt.ID)
	}
}

type mockTransport struct {
	Handler http.Handler
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	// Mock executing the request.
	m.Handler.ServeHTTP(rec, req)
	resp := rec.Result()
	// Set the dummy Request so it doesn't fail downstream context tracking
	resp.Request = req

	return resp, nil
}

func TestRetryTimeout(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++

		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	p := New()
	p.client.HTTPClient.Timeout = 100 * time.Millisecond
	p.client.UserAgent = "TestUserAgent"

	// Create mock transport directly to our server
	p.client.HTTPClient.Transport = &mockTransport{
		Handler: server.Config.Handler,
	}

	ctx := context.Background()
	req := resolve.ObjectRequest{Query: "test"}
	iter := p.ResolveObject(ctx, req)

	iter(func(_ resolve.Target, err error) bool {
		if err == nil {
			t.Errorf("expected error, got nil")
		}

		return false
	})

	if attempts == 0 {
		t.Errorf("expected multiple attempts")
	}
}

func TestParseEmptyCSV(t *testing.T) {
	f, err := os.Open("testdata/empty.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	targets, err := ParseCSV(f)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(targets) != 0 {
		t.Fatalf("Expected 0 targets for empty, got %d", len(targets))
	}
}

func TestParseMalformedCSV(t *testing.T) {
	f, err := os.Open("testdata/malformed.csv")
	if err != nil {
		t.Fatalf("failed to open test fixture: %v", err)
	}

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	_, err = ParseCSV(f)
	if err == nil {
		t.Fatalf("Expected ParseCSV to fail on malformed data")
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "simbad" {
		t.Errorf("expected simbad, got %s", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapObjectResolution {
		t.Errorf("expected CapObjectResolution, got %v", caps)
	}

	// Triggers internal error paths since we didn't mock
	_, _ = p.Resolve("non_existent_body")
	_ = p.Search("non_existent_body")
}
