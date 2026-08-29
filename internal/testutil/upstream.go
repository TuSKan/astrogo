package testutil

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

// SkipOnUpstreamFailure skips tb when err is the remote service failing rather
// than astrogo misbehaving.
//
// [RequireReachable] answers whether a socket opens, which is not the same
// question as whether the service works. A host answering "500 Internal Server
// Error" passes the reachability probe and then fails the assertion, so an
// outage at CelesTrak or JPL is reported as a defect in this repository — and
// it blocks pull requests that never touched the package.
//
// The classification is deliberately narrow. A 5xx, a 429, a 408 or a request
// that never completed is the upstream's problem. Every other 4xx is not: a
// 400 or a 404 means astrogo built a request the service rejected, which is
// exactly the defect these tests exist to catch, and it stays a failure.
func SkipOnUpstreamFailure(tb testing.TB, err error) {
	tb.Helper()

	if err == nil {
		return
	}

	if reason, ok := upstreamFailure(err); ok {
		tb.Skipf("upstream service failure, not verified: %s (%v)", reason, err)
	}
}

// upstreamFailure reports whether err is the remote end failing, and why.
func upstreamFailure(err error) (string, bool) {
	// Matched through an interface, not remote/api's concrete type: testutil is
	// imported by nearly every test in the repository, and importing remote
	// here would both create a cycle with remote's own tests and pull the
	// cloud-storage dependency tree into every test binary.
	var status interface{ HTTPStatus() int }
	if errors.As(err, &status) {
		switch code := status.HTTPStatus(); {
		case code >= 500:
			return "server error", true
		case code == http.StatusTooManyRequests:
			return "rate limited", true
		case code == http.StatusRequestTimeout:
			return "request timeout", true
		default:
			// 4xx other than the two above means we sent a bad request.
			return "", false
		}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline exceeded", true
	}

	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return "network timeout", true
	}

	return "", false
}
