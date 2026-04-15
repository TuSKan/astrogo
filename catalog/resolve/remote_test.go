package resolve_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestClientRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}))
	defer server.Close()

	client := resolve.NewClient()
	client.MaxRetries = 5 // Ensures it has enough headroom
	// Use small timeouts to prevent long test runs if using arbitrary backoff
	// The cenkalti backoff implicitly uses default Backoff which starts at 500ms
	// We'll just run it as it's not strictly 500ms but we'll accept the brief pause for robust testing.

	req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)

	resp, err := client.Do(req)
	testutil.AssertNoError(t, err)
	defer resp.Body.Close()

	testutil.AssertEqual(t, "Status Code", resp.StatusCode, http.StatusOK)
	testutil.AssertEqual(t, "Attempts", attempts, 3)
}

func TestClientPermanentFailure(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound) // 404 is NOT transient by DefaultRetryPolicy
	}))
	defer server.Close()

	client := resolve.NewClient()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", server.URL, nil)

	_, err := client.Do(req)
	if err == nil {
		t.Fatalf("Expected permanent failure HTTP error")
	}

	var httpErr *resolve.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("Expected resolve.HTTPError type, got: %T", err)
	}

	testutil.AssertEqual(t, "Status Code", httpErr.StatusCode, http.StatusNotFound)
	testutil.AssertEqual(t, "Attempts", attempts, 1) // Did not retry
}

func TestClientContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond) // artificially hold response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := resolve.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	_, err := client.Do(req)

	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)) {
		t.Fatalf("Expected DeadlineExceeded or Canceled, got: %v", err)
	}
}

func TestSliceSeqIteration(t *testing.T) {
	targets := []resolve.Target{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
	}

	iter := resolve.SliceSeq(targets)

	count := 0
	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)
		count++
		return true // Continue iteration
	})
	testutil.AssertEqual(t, "Full iteration", count, 3)

	// Early Abort
	abortCount := 0
	iter(func(tar resolve.Target, err error) bool {
		abortCount++
		return false // Abort iteration
	})
	testutil.AssertEqual(t, "Early abort", abortCount, 1)
}
