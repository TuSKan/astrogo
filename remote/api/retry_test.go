package api_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/remote/api"
)

// countingServer answers with status for the first n requests and 200 after
// that, counting every request it receives.
//
// The count is what these tests actually assert on: a retry policy is a claim
// about how many times the client goes back, and nothing else observes that.
func countingServer(t *testing.T, status, failures int) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if int(hits.Add(1)) <= failures {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("service says no"))

			return
		}

		_, _ = w.Write([]byte("ok"))
	}))

	t.Cleanup(srv.Close)

	return srv, &hits
}

// TestDefaultRetryPolicyDecides pins the rule that used to be three of resty's
// own predicates wired directly into the client.
//
// Naming it, exporting it and giving it a signature a caller can implement is
// the whole point of the change; this is the table that says the behaviour did
// not move while the shape did.
func TestDefaultRetryPolicyDecides(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		want   bool
		why    string
	}{
		{"429 rate limit", http.StatusTooManyRequests, true, "the server is asking us to slow down, not refusing"},
		{"500 internal", http.StatusInternalServerError, true, "a server-side fault is usually transient"},
		{"503 unavailable", http.StatusServiceUnavailable, true, "explicitly temporary"},
		{"501 not implemented", http.StatusNotImplemented, false, "the server will not start implementing it"},
		{"no response at all", 0, true, "a dial failure or timeout, which may not recur"},
		{"404 not found", http.StatusNotFound, false, "the request describes something that is not there"},
		{"400 bad request", http.StatusBadRequest, false, "repeating an identical request cannot fix it"},
		{"401 unauthorized", http.StatusUnauthorized, false, "the credentials will be the same next time"},
		{"200 ok", http.StatusOK, false, "not a failure"},
	} {
		if got := api.DefaultRetryPolicy(api.Attempt{StatusCode: tc.status}); got != tc.want {
			t.Errorf("%s: DefaultRetryPolicy = %v, want %v — %s", tc.name, got, tc.want, tc.why)
		}
	}
}

// TestClientRetriesAccordingToTheDefaultPolicy is the end-to-end half: the
// policy is worth nothing if the client does not consult it.
func TestClientRetriesAccordingToTheDefaultPolicy(t *testing.T) {
	// Not parallel: redirects a process-wide endpoint URL.
	srv, hits := countingServer(t, http.StatusServiceUnavailable, 2)
	base := srv.URL

	c := newClient(t, api.WithRetries(3))

	r, err := c.Get(context.Background(), base, "", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_ = r.Close()

	// Two failures then a success: three requests in total.
	if got := hits.Load(); got != 3 {
		t.Errorf("server saw %d requests, want 3 (two 503s retried, then a 200)", got)
	}
}

// TestClientDoesNotRetryAStatusThePolicyRejects is the other side, and the one
// that matters more: retrying a 404 wastes a service's time and the caller's.
func TestClientDoesNotRetryAStatusThePolicyRejects(t *testing.T) {
	srv, hits := countingServer(t, http.StatusNotFound, 10)
	base := srv.URL

	c := newClient(t, api.WithRetries(3))

	if _, err := c.Get(context.Background(), base, "", nil); err == nil {
		t.Fatal("Get accepted a 404")
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("server saw %d requests for a 404, want 1 — a 4xx describes the request", got)
	}
}

// TestWithRetryPolicyReplacesTheDefault is the extension point itself.
//
// The policy here retries a 409, which the default does not, and refuses a 503,
// which the default does — so passing it changes the behaviour in both
// directions and neither result could come from the default still being in
// force.
func TestWithRetryPolicyReplacesTheDefault(t *testing.T) {
	t.Run("retries what the default would not", func(t *testing.T) {
		srv, hits := countingServer(t, http.StatusConflict, 1)
		base := srv.URL

		c := newClient(t,
			api.WithRetries(3),
			api.WithRetryPolicy(func(a api.Attempt) bool {
				return a.StatusCode == http.StatusConflict
			}),
		)

		r, err := c.Get(context.Background(), base, "", nil)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}

		_ = r.Close()

		if got := hits.Load(); got != 2 {
			t.Errorf("server saw %d requests, want 2 — the custom policy retried a 409 the default would not", got)
		}
	})

	t.Run("refuses what the default would retry", func(t *testing.T) {
		srv, hits := countingServer(t, http.StatusServiceUnavailable, 10)
		base := srv.URL

		c := newClient(t,
			api.WithRetries(3),
			api.WithRetryPolicy(func(api.Attempt) bool { return false }),
		)

		if _, err := c.Get(context.Background(), base, "", nil); err == nil {
			t.Fatal("Get accepted a 503")
		}

		if got := hits.Load(); got != 1 {
			t.Errorf("server saw %d requests, want 1 — the custom policy refused to retry", got)
		}
	})
}

// TestRetryPolicySeesTheAttemptNumber covers the field a backoff-shaped policy
// is written around, and the one that is read off resty rather than computed.
func TestRetryPolicySeesTheAttemptNumber(t *testing.T) {
	srv, hits := countingServer(t, http.StatusServiceUnavailable, 10)
	base := srv.URL

	var seen []int

	c := newClient(t,
		api.WithRetries(5),
		api.WithRetryPolicy(func(a api.Attempt) bool {
			seen = append(seen, a.Number)

			// Give up after the second attempt, which the default would not.
			return a.Number < 2
		}),
	)

	if _, err := c.Get(context.Background(), base, "", nil); err == nil {
		t.Fatal("Get accepted a 503")
	}

	// Two attempts made: the policy said yes once and no once.
	if got := hits.Load(); got != 2 {
		t.Errorf("server saw %d requests, want 2 — the policy stopped at attempt 2", got)
	}

	if len(seen) < 2 {
		t.Fatalf("policy was consulted %d times, want at least 2", len(seen))
	}

	if seen[0] != 1 {
		t.Errorf("first attempt reported Number = %d, want 1 — the count is 1-based", seen[0])
	}

	if seen[1] != 2 {
		t.Errorf("second attempt reported Number = %d, want 2 — the count does not advance", seen[1])
	}
}

// TestExhaustedRetriesReportErrRetriable is what makes the sentinel worth
// exporting, and the reason this change is not only a refactor.
//
// A 503 that survived every retry and a 404 are both non-2xx. Only one is worth
// trying again later, and only one should be reported to an operator as an
// outage rather than as a mistake — but before this they were the same error.
func TestExhaustedRetriesReportErrRetriable(t *testing.T) {
	t.Run("a retried failure is marked", func(t *testing.T) {
		srv, _ := countingServer(t, http.StatusServiceUnavailable, 10)
		base := srv.URL

		c := newClient(t, api.WithRetries(1))

		_, err := c.Get(context.Background(), base, "", nil)
		if err == nil {
			t.Fatal("Get accepted a 503")
		}

		if !errors.Is(err, api.ErrRetriable) {
			t.Errorf("err = %v, want it to wrap ErrRetriable.\n"+
				"  Without it a caller cannot tell an exhausted retry from a request that "+
				"was simply wrong, and will not know to try again later.", err)
		}

		// The status and body must survive the wrapping, or every existing
		// caller that inspects them breaks.
		var httpErr *api.HTTPError
		if !errors.As(err, &httpErr) {
			t.Fatalf("err = %v; the *HTTPError underneath is no longer reachable", err)
		}

		if httpErr.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("StatusCode = %d, want 503", httpErr.StatusCode)
		}

		if httpErr.Body == "" {
			t.Error("the response body was lost; these services put the reason in it")
		}
	})

	t.Run("a hard failure is not marked", func(t *testing.T) {
		srv, _ := countingServer(t, http.StatusNotFound, 10)
		base := srv.URL

		c := newClient(t, api.WithRetries(3))

		_, err := c.Get(context.Background(), base, "", nil)
		if err == nil {
			t.Fatal("Get accepted a 404")
		}

		if errors.Is(err, api.ErrRetriable) {
			t.Errorf("a 404 was reported as retriable: %v.\n"+
				"  A signal that fires for every failure distinguishes nothing.", err)
		}

		var httpErr *api.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
			t.Errorf("err = %v, want an *HTTPError carrying 404", err)
		}
	})

	t.Run("a custom policy decides what counts", func(t *testing.T) {
		srv, _ := countingServer(t, http.StatusNotFound, 10)
		base := srv.URL

		// This service means "ask again" by 404. Absurd, and real services do
		// stranger; the point is that the mark follows the policy rather than
		// a second hard-coded list that could disagree with it.
		c := newClient(t,
			api.WithRetries(0),
			api.WithRetryPolicy(func(a api.Attempt) bool {
				return a.StatusCode == http.StatusNotFound
			}),
		)

		_, err := c.Get(context.Background(), base, "", nil)
		if !errors.Is(err, api.ErrRetriable) {
			t.Errorf("err = %v, want ErrRetriable — the installed policy calls a 404 retriable, "+
				"and the mark must follow the policy rather than a fixed list", err)
		}
	})
}

// TestWithRetryPolicyIgnoresNil keeps a computed-and-empty policy from silently
// disabling retrying.
//
// A caller building a policy conditionally can end up passing nil, and the
// difference between "no opinion" and "never retry" is a service outage that
// looks like a hard failure. Disabling retries has its own spelling.
func TestWithRetryPolicyIgnoresNil(t *testing.T) {
	srv, hits := countingServer(t, http.StatusServiceUnavailable, 2)
	base := srv.URL

	c := newClient(t, api.WithRetries(3), api.WithRetryPolicy(nil))

	r, err := c.Get(context.Background(), base, "", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_ = r.Close()

	if got := hits.Load(); got != 3 {
		t.Errorf("server saw %d requests, want 3 — a nil policy must leave the default in place", got)
	}
}
