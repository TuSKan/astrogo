package resolve_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestClientRetries(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		w.WriteHeader(http.StatusOK)

		_, err := w.Write([]byte("OK"))
		if err != nil {
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

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)

	resp, err := client.Do(req)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		err := resp.Body.Close()
		if err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	})

	testutil.AssertEqual(t, "Status Code", resp.StatusCode, http.StatusOK)
	testutil.AssertEqual(t, "Attempts", attempts, 3)
}

// TestClientRetriesWithBody is a regression test: Do used to reuse the same
// *http.Request across retry attempts without rewinding req.Body via
// req.GetBody(), so a POST request that survived a transient failure and got
// retried would resend an empty (already-drained) body instead of replaying
// the original request — exactly the pattern SIMBAD/Gaia/VizieR/MAST use for
// their ADQL query bodies.
func TestClientRetriesWithBody(t *testing.T) {
	attempts := 0

	const wantBody = "query=select+*+from+targets"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		if string(body) != wantBody {
			t.Errorf("attempt %d: got body %q, want %q", attempts, body, wantBody)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := resolve.NewClient()
	client.MaxRetries = 5

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, server.URL, strings.NewReader(wantBody))
	testutil.AssertNoError(t, err)

	resp, err := client.Do(req)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		if cerr := resp.Body.Close(); cerr != nil {
			t.Errorf("failed to close response body: %v", cerr)
		}
	})

	testutil.AssertEqual(t, "Status Code", resp.StatusCode, http.StatusOK)
	testutil.AssertEqual(t, "Attempts", attempts, 3)
}

func TestClientPermanentFailure(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++

		w.WriteHeader(http.StatusNotFound) // 404 is NOT transient by DefaultRetryPolicy
	}))
	defer server.Close()

	client := resolve.NewClient()
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)

	resp, err := client.Do(req)
	if resp != nil {
		err := resp.Body.Close()
		if err != nil {
			t.Errorf("failed to close response body: %v", err)
		}
	}

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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond) // artificially hold response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := resolve.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)

	resp, err := client.Do(req)
	if resp != nil {
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				t.Logf("failed to close response body: %v", cerr)
			}
		}()
	}

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

	iter(func(_ resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		count++

		return true // Continue iteration
	})
	testutil.AssertEqual(t, "Full iteration", count, 3)

	// Early Abort
	abortCount := 0

	iter(func(_ resolve.Target, _ error) bool {
		abortCount++
		return false // Abort iteration
	})
	testutil.AssertEqual(t, "Early abort", abortCount, 1)
}
