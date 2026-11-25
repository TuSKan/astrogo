package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote/api"
)

// TestRetryAfterSetsTheWait: a 503 saying Retry-After: 1 is retried no
// sooner than a second later, and the retry's answer is the call's.
func TestRetryAfterSetsTheWait(t *testing.T) {
	t.Parallel()

	var (
		mu    sync.Mutex
		times []time.Time
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()

		times = append(times, time.Now())
		n := len(times)
		mu.Unlock()

		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	c := api.NewClient(10*time.Second, api.WithRetries(1))

	t.Cleanup(func() { _ = c.Close() })

	body, err := c.Get(context.Background(), srv.URL, "", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_ = body.Close()

	mu.Lock()
	defer mu.Unlock()

	if len(times) != 2 {
		t.Fatalf("%d attempts, want 2", len(times))
	}

	if gap := times[1].Sub(times[0]); gap < 900*time.Millisecond {
		t.Errorf("retried %v after a Retry-After of 1 s, want at least a second", gap)
	}
}

// TestAWaitPastTheBudgetEndsTheRetries: a Retry-After longer than what is
// left of the call's timeout is not waited for. The call returns the 503 at
// once, marked ErrRetriable since the policy would have retried it, rather
// than sleeping until its deadline.
func TestAWaitPastTheBudgetEndsTheRetries(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)

	c := api.NewClient(5*time.Second, api.WithRetries(3))

	t.Cleanup(func() { _ = c.Close() })

	start := time.Now()
	_, err := c.Get(context.Background(), srv.URL, "", nil)
	took := time.Since(start)

	if !errors.Is(err, api.ErrRetriable) {
		t.Errorf("err = %v, want it to wrap ErrRetriable", err)
	}

	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("err = %v, want the 503 as an *HTTPError", err)
	}

	if n := hits.Load(); n != 1 {
		t.Errorf("%d attempts, want 1: the wait it asked for outlasts the call", n)
	}

	if took > 2*time.Second {
		t.Errorf("the call took %v; it should end at once, not wait toward its 5 s deadline", took)
	}
}
