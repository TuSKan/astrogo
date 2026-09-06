package api

import (
	"net/http"

	"resty.dev/v3"
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
// # What it does not decide
//
// How long to wait. That is already handled better than a policy would: resty
// honours a 429 or 503 response's own Retry-After header — in both the seconds
// and the HTTP-date forms — and otherwise backs off exponentially with jitter,
// bounded by the client's retry count. A server that says when to come back is
// obeyed, and one that does not gets a well-behaved client anyway, so there is
// nothing left for a caller to improve by hand.
//
// A policy is installed per [Client], with [WithRetryPolicy]. Varying retry
// behaviour by endpoint means constructing a client per endpoint, which is what
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

// retryCondition adapts a [RetryPolicy] to the shape resty wants.
//
// This is the only place a resty type meets the policy, which is what keeps
// the promise that no resty type escapes this package: a caller writing a
// policy never sees *resty.Response.
func retryCondition(p RetryPolicy) resty.RetryConditionFunc {
	return func(res *resty.Response, err error) bool { return p(attemptOf(res, err)) }
}

// attemptOf reads an [Attempt] out of a resty exchange.
//
// Shared by the retry condition and by Client.body, so the question "would this
// have been retried" is answered by one reading of the response in both places
// rather than by two that can drift.
//
// A nil response means the request never produced one, which resty reports
// separately from the error — so status and attempt number both fall back
// rather than being read off it.
func attemptOf(res *resty.Response, err error) Attempt {
	a := Attempt{Number: 1, Err: err}

	if res == nil {
		return a
	}

	a.StatusCode = res.StatusCode()

	if res.Request != nil {
		a.Number = res.Request.Attempt
	}

	return a
}
