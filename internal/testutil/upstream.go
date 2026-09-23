package testutil

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"syscall"
	"testing"
)

// SkipOnUpstreamFailure skips tb when err is somebody else's fault rather than
// astrogo misbehaving — the service failing, or the network never carrying the
// request at all.
//
// [RequireReachable] answers whether a socket opens, which is not the same
// question as whether the service works. A host answering "500 Internal Server
// Error" passes the reachability probe and then fails the assertion, so an
// outage at CelesTrak or JPL is reported as a defect in this repository — and
// it blocks pull requests that never touched the package.
//
// The classification is deliberately narrow. A 5xx, a 429, a 408, a 403, a
// request that never completed and a connection dropped mid-transfer are the
// upstream's problem. Every other 4xx is not: a 400 or a 404 means astrogo
// built a request the service rejected, which is exactly the defect these
// tests exist to catch, and it stays a failure.
//
// # Why this is the only one a caller needs
//
// [Unreachable] answers the other half — a DNS failure, a refused or unroutable
// dial — and is still exported, because it is a useful predicate on its own and
// some tests read it directly. But it is consulted here, so nothing has to ask
// twice.
//
// It used to. Four tests spelled out both checks before reaching their t.Fatal,
// and that friction is part of why a dozen others reached for
// t.Skipf("did not answer") on any error instead — a helper that answers half a
// question invites a call site to stop asking. The duplication had also become
// literal: teaching the classifiers about bad certificates meant teaching both,
// separately, because neither consulted the other.
//
// # Why 403 is on the upstream's side and 401 is not
//
// They look like the same case and are opposite ones. A 401 says "you must
// authenticate", which for an endpoint astrogo believes is public means
// astrogo's model of that endpoint is wrong — the URL moved, or the service
// grew an auth requirement — and that is a defect to see. A 403 says "I know
// who you are and I decline", and to a caller that sent no credential there is
// nothing to get right: it is the service's policy, not our request.
//
// That reasoning holds only while astrogo sends no credential the service
// requires, which remote.TokenEnv's contract states ("a token is an
// optimisation, never a requirement") and remote's
// TestNoAPIEndpointRequiresACredential enforces. Break that invariant and this
// arm starts hiding a real authorization failure — which is why the test that
// keeps it true names this function.
//
// Observed rather than assumed: CelesTrak answers a burst of requests with an
// IIS "403 - Forbidden: Access is denied" page and serves the same query
// normally a minute later. See #206.
func SkipOnUpstreamFailure(tb testing.TB, err error) {
	tb.Helper()

	if err == nil {
		return
	}

	if reason, ok := UpstreamFailure(err); ok {
		tb.Skipf("upstream service failure, not verified: %s (%v)", reason, err)
	}
}

// UpstreamFailure reports whether err is somebody else's fault rather than
// astrogo's, and why: the classification [SkipOnUpstreamFailure] acts on,
// available to a caller that must do something other than skip.
//
// Exported for the suites that record a result before they stop.
// metrology.NotVerified writes NOT VERIFIED into the accuracy report and then
// skips, and four suites called it on any error at all — so a regression that
// broke jpl.NewProvider outright was reported as an outage and could never
// fail. With the question separated from the action they record NOT VERIFIED
// for an outage and fail for everything else, which is the same line this
// package draws everywhere else.
func UpstreamFailure(err error) (string, bool) {
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
		case code == http.StatusForbidden:
			// The service declined to serve a request that carried no
			// credential, so there is nothing about the request to correct.
			// See this function's doc comment for why 401 is not here.
			return "refused by the service", true
		default:
			// 4xx other than those above means we sent a bad request.
			return "", false
		}
	}

	// Before the network checks below, because a handshake failure is not a
	// timeout and would otherwise fall through all of them. See
	// certificateFailure for which certificate errors count and which stay
	// fatal.
	if reason, ok := certificateFailure(err); ok {
		return reason, true
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline exceeded", true
	}

	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return "network timeout", true
	}

	// A connection dropped or truncated mid-transfer. Not a timeout, so the
	// check above misses it — AstroPixels reset one request out of sixteen
	// while the other fifteen parsed cleanly, which failed CI on a branch
	// that had not touched the network at all.
	switch {
	case errors.Is(err, syscall.ECONNRESET):
		return "connection reset by peer", true
	case errors.Is(err, syscall.ECONNABORTED):
		return "connection aborted", true
	case errors.Is(err, syscall.EPIPE):
		return "broken pipe", true
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "truncated response", true
	}

	// Last: a request the network never carried at all — a DNS failure, a
	// refused dial, an unroutable host. Not a verdict on astrogo any more than
	// a 503 is, and [Unreachable] already owns the question, so it is asked
	// here rather than left to every call site.
	//
	// Safe to put last rather than first. Every arm above decides a case
	// Unreachable declines or never sees: a status it cannot read, a bare
	// context.DeadlineExceeded it deliberately returns false for, and the
	// mid-transfer drops, which happen after a connection was carried. So
	// nothing above changes meaning by having this below it.
	if Unreachable(err) {
		return "the network did not carry the request", true
	}

	return "", false
}
