package remote

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestClientRetryOn500ThenSuccess(t *testing.T) {
	t.Cleanup(Reset)

	var calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	if calls != 3 {
		t.Errorf("expected 3 attempts, got %d", calls)
	}
}

func TestClientNoRetryOn400(t *testing.T) {
	t.Cleanup(Reset)

	var calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		w.WriteHeader(http.StatusBadRequest)

		_, _ = w.Write([]byte("bad input"))
	}))
	defer srv.Close()

	c := NewClient()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	resp, err := c.Do(req)
	if err == nil {
		defer resp.Body.Close() //nolint:errcheck // unreachable guard for the linter

		t.Fatal("expected error for 400")
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected *HTTPError, got %T: %v", err, err)
	}

	if httpErr.StatusCode != http.StatusBadRequest || httpErr.Body != "bad input" {
		t.Errorf("unexpected HTTPError: %+v", httpErr)
	}

	if calls != 1 {
		t.Errorf("400 must not retry; got %d attempts", calls)
	}
}

func TestClientPOSTBodyReplayOnRetry(t *testing.T) {
	t.Cleanup(Reset)

	var (
		calls  int
		bodies []string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++

		b, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(b))

		if calls < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, srv.URL,
		bytes.NewReader([]byte("payload-123")))

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	if len(bodies) != 2 || bodies[0] != "payload-123" || bodies[1] != "payload-123" {
		t.Errorf("POST body not replayed intact on retry: %q", bodies)
	}
}

func TestClientUserAgent(t *testing.T) {
	t.Cleanup(Reset)

	var gotUA string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)

	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}

	defer resp.Body.Close() //nolint:errcheck // test

	if gotUA != defaultUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, defaultUserAgent)
	}
}

func TestClientContextCancelNotRetried(t *testing.T) {
	t.Cleanup(Reset)

	var calls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		time.Sleep(200 * time.Millisecond)

		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	c := NewClient()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	if resp, err := c.Do(req); err == nil {
		_ = resp.Body.Close()

		t.Fatal("expected context deadline error")
	}

	if calls > 1 {
		t.Errorf("context cancellation must not retry; got %d attempts", calls)
	}
}

func TestClientGetGatesOnRegistry(t *testing.T) {
	t.Cleanup(Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "mars" {
			t.Errorf("query not forwarded: %s", r.URL.RawQuery)
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatal(err)
	}

	c := NewClient()

	q := url.Values{}
	q.Set("q", "mars")

	resp, err := c.Get(context.Background(), SIMBAD, "", q)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_ = resp.Body.Close()

	// Disabled endpoint short-circuits before any request.
	Disable(SIMBAD)

	if resp2, err := c.Get(context.Background(), SIMBAD, "", nil); !errors.Is(err, ErrEndpointDisabled) {
		if resp2 != nil {
			_ = resp2.Body.Close()
		}

		t.Errorf("expected ErrEndpointDisabled, got %v", err)
	}

	// Offline mode too.
	Enable(SIMBAD)
	SetOffline(true)

	if resp3, err := c.Get(context.Background(), SIMBAD, "", nil); !errors.Is(err, ErrOffline) {
		if resp3 != nil {
			_ = resp3.Body.Close()
		}

		t.Errorf("expected ErrOffline, got %v", err)
	}
}
