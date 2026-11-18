package api

import (
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/time"
)

// Attempt describes one HTTP attempt a [RetryPolicy] is asked to judge.
//
// It is deliberately small. Everything a policy could want that is not here is
// either a property of the client — which the caller constructed, and can give
// its own policy — or a property of the response body, which cannot be read
// without consuming the stream the caller is about to be handed.
type Attempt struct {
	// Number is which attempt this was, counting from 1. The first failure
	// arrives as Number 1, so a policy that wants "retry once" returns true
	// only for that.
	Number int

	// StatusCode is the response status, or 0 when no response arrived at all
	// — a dial failure, a timeout, a connection reset mid-exchange.
	StatusCode int

	// Err is the transport error, and is nil whenever a response arrived
	// however unwelcome its status. A non-nil Err with StatusCode 0 is the
	// ordinary "could not reach the service" shape.
	Err error
}

// RetryPolicy reports whether a failed attempt should be retried.
//
// It does not decide how long to wait. A 429 or 503 answer's Retry-After
// header, in seconds or as an HTTP date, says that; otherwise the wait
// doubles from 100 ms to at most 2 s, jittered. A wait that would end past
// the call's timeout ends the retries instead.
//
// A policy is installed per [Client], with [WithRetryPolicy]. Varying retry
// behavior by endpoint means constructing a client per endpoint, which is what
// a provider does anyway.
type RetryPolicy func(Attempt) bool

// DefaultRetryPolicy is what a client retries when no policy is installed:
//
//   - 429 Too Many Requests — a rate limit, and the one status where the server
//     usually says when to return.
//   - any 5xx except 501 Not Implemented — a server-side fault is generally
//     transient, but 501 says the server does not implement this and will not
//     start to.
//   - status 0 — no response arrived. A dial failure or a timeout.
//
// A 4xx is never retried. It describes the request, and repeating an identical
// request cannot change it.
//
// A POST reaches the policy only from a client built [WithIdempotentPost];
// from any other it is sent once, since repeating it might do something twice.
//
// Exported so a custom policy can defer to it rather than restate it:
//
//	api.WithRetryPolicy(func(a api.Attempt) bool {
//		if a.StatusCode == http.StatusConflict {
//			return a.Number <= 2 // this service means "busy" by 409
//		}
//		return api.DefaultRetryPolicy(a)
//	})
func DefaultRetryPolicy(a Attempt) bool {
	switch {
	case a.StatusCode == http.StatusTooManyRequests:
		return true
	case a.StatusCode >= 500 && a.StatusCode != http.StatusNotImplemented:
		return true
	case a.StatusCode == 0:
		return true
	default:
		return false
	}
}

// WithRetryPolicy replaces [DefaultRetryPolicy] for this client.
//
// A nil policy is ignored rather than installed, so a caller computing one
// cannot accidentally disable retrying by passing the zero value. To disable
// retrying, use WithRetries(0), which says so.
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *config) {
		if p != nil {
			c.retryPolicy = p
		}
	}
}

// The wait between attempts with no Retry-After to go by: doubling from
// minRetryWait to maxRetryWait.
const (
	minRetryWait = 100 * time.Millisecond
	maxRetryWait = 2 * time.Second
)

// retryWait is how long to wait after a failed attempt before the next: the
// answer's Retry-After for a 429 or 503 that carries one, otherwise the
// attempt's backoff, jittered across its upper half so clients that failed
// together do not retry together.
func retryWait(resp *http.Response, attempt int) time.Duration {
	if resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) {
		if d, ok := retryAfter(resp.Header.Get("Retry-After")); ok {
			return d
		}
	}

	backoff := maxRetryWait
	if attempt < 6 {
		backoff = min(maxRetryWait, minRetryWait<<(attempt-1))
	}

	half := backoff / 2

	return half + time.Duration(rand.Int64N(int64(half)+1)) //nolint:gosec // jitter spreads retries; it guards nothing
}

// retryAfter reads a Retry-After header: a number of seconds, or an HTTP
// date. A date already past is no wait.
func retryAfter(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}

	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}

	at, err := http.ParseTime(v)
	if err != nil {
		return 0, false
	}

	return max(0, time.Until(at)), true
}
