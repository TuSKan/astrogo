package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(t *testing.T, id EndpointID, opts ...ClientOption) *Client {
	t.Helper()

	c, err := NewClientFor(id, opts...)
	if err != nil {
		t.Fatalf("NewClientFor(%s): %v", id, err)
	}

	return c
}

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

	c := newTestClient(t, SIMBAD)

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

	c := newTestClient(t, SIMBAD)

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

	c := newTestClient(t, SIMBAD)

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

	c := newTestClient(t, SIMBAD)

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

	var calls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)

		time.Sleep(200 * time.Millisecond)

		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	c := newTestClient(t, SIMBAD)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	if resp, err := c.Do(req); err == nil {
		_ = resp.Body.Close()

		t.Fatal("expected context deadline error")
	}

	// The handler goroutine may still be mid-sleep when Do returns (the
	// context deadline fires client-side; it doesn't cancel the handler),
	// so give it a moment to finish before reading the final count —
	// otherwise this check itself races with the handler's write.
	time.Sleep(250 * time.Millisecond)

	if n := calls.Load(); n > 1 {
		t.Errorf("context cancellation must not retry; got %d attempts", n)
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

	c := newTestClient(t, SIMBAD)

	q := url.Values{}
	q.Set("q", "mars")

	body, err := c.Get(context.Background(), SIMBAD, "", q)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_ = body.Close()

	// Disabled endpoint short-circuits before any request.
	Disable(SIMBAD)

	if body2, err := c.Get(context.Background(), SIMBAD, "", nil); !errors.Is(err, ErrEndpointDisabled) {
		if body2 != nil {
			_ = body2.Close()
		}

		t.Errorf("expected ErrEndpointDisabled, got %v", err)
	}

	// Offline mode too.
	Enable(SIMBAD)
	SetOffline(true)

	if body3, err := c.Get(context.Background(), SIMBAD, "", nil); !errors.Is(err, ErrOffline) {
		if body3 != nil {
			_ = body3.Close()
		}

		t.Errorf("expected ErrOffline, got %v", err)
	}
}

func TestNewClientForUsesEndpointTimeout(t *testing.T) {
	t.Cleanup(Reset)

	c := newTestClient(t, FINK)

	if c.HTTPClient.Timeout != 120*time.Second {
		t.Errorf("Timeout = %s, want %s (FINK's registered Timeout)", c.HTTPClient.Timeout, 120*time.Second)
	}
}

func TestNewClientForZeroFallsBackToDefault(t *testing.T) {
	t.Cleanup(Reset)

	// NAIFSPK is a KindFile endpoint: it has no registered Timeout (only
	// DownloadTimeout), so NewClientFor must fall back to DefaultAPITimeout.
	c := newTestClient(t, NAIFSPK)

	if c.HTTPClient.Timeout != DefaultAPITimeout {
		t.Errorf("Timeout = %s, want %s (DefaultAPITimeout)", c.HTTPClient.Timeout, DefaultAPITimeout)
	}
}

func TestNewClientForUnknownEndpoint(t *testing.T) {
	t.Cleanup(Reset)

	if _, err := NewClientFor("no.such.endpoint"); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("expected ErrUnknownEndpoint, got %v", err)
	}
}

func TestNewClientForExplicitOptionWins(t *testing.T) {
	t.Cleanup(Reset)

	c := newTestClient(t, SIMBAD, WithTimeout(5*time.Second))

	if c.HTTPClient.Timeout != 5*time.Second {
		t.Errorf("explicit WithTimeout not applied: got %s", c.HTTPClient.Timeout)
	}
}

func TestClientGetReturnsBodyReader(t *testing.T) {
	t.Cleanup(Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("plain body"))
	}))
	defer srv.Close()

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, SIMBAD)

	body, err := c.Get(context.Background(), SIMBAD, "", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer body.Close() //nolint:errcheck // test

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != "plain body" {
		t.Errorf("body = %q, want %q", got, "plain body")
	}
}

func TestClientGetJSONDecodesAndCloses(t *testing.T) {
	t.Cleanup(Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"foo":"bar"}`))
	}))
	defer srv.Close()

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, SIMBAD)

	var out struct {
		Foo string `json:"foo"`
	}

	if err := c.GetJSON(context.Background(), SIMBAD, "", nil, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	if out.Foo != "bar" {
		t.Errorf("decoded Foo = %q, want %q", out.Foo, "bar")
	}
}

func TestClientPostForm(t *testing.T) {
	t.Cleanup(Reset)

	var gotContentType, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte("ack"))
	}))
	defer srv.Close()

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, SIMBAD)

	v := url.Values{}
	v.Set("query", "SELECT ra, dec FROM basic")

	body, err := c.PostForm(context.Background(), SIMBAD, "", v)
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}
	defer body.Close() //nolint:errcheck // test

	if gotContentType != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q, want form-urlencoded", gotContentType)
	}

	if gotBody != v.Encode() {
		t.Errorf("body = %q, want %q", gotBody, v.Encode())
	}
}

func TestClientPostJSON(t *testing.T) {
	t.Cleanup(Reset)

	var gotContentType string

	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte("ack"))
	}))
	defer srv.Close()

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatal(err)
	}

	c := newTestClient(t, SIMBAD)

	body, err := c.PostJSON(context.Background(), SIMBAD, "", map[string]any{"target": "Mars"})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}
	defer body.Close() //nolint:errcheck // test

	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}

	if gotBody["target"] != "Mars" {
		t.Errorf("decoded body = %+v, want target=Mars", gotBody)
	}
}
