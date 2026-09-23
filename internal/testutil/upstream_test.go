package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
)

// httpStatusError is any error carrying an HTTP status, matched structurally. Using a
// local type here rather than remote/api's keeps this package free of the
// cloud dependency tree, and proves the classifier works for any error that
// reports a status — not only remote/api's.
type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("http %d", e.code) }

func (e *httpStatusError) HTTPStatus() int { return e.code }

// errStaticParse stands in for a decode failure, which is astrogo's problem.
var errStaticParse = errors.New("invalid character 'x'")

func TestUpstreamFailureClassification(t *testing.T) {
	timeout := &net.DNSError{IsTimeout: true}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		// The upstream is broken or throttling: not our defect.
		{"500 server error", &httpStatusError{500}, true},
		{"502 bad gateway", &httpStatusError{502}, true},
		{"503 unavailable", &httpStatusError{503}, true},
		{"429 rate limited", &httpStatusError{429}, true},
		{"408 request timeout", &httpStatusError{408}, true},
		{"deadline exceeded", context.DeadlineExceeded, true},

		// Dropped mid-transfer: not a timeout, so it needs its own arm.
		// This is the case that failed CI on #51, which had not touched
		// the network at all.
		{"connection reset", &net.OpError{Err: syscall.ECONNRESET}, true},
		{"wrapped connection reset", fmt.Errorf("read: %w", &net.OpError{Err: syscall.ECONNRESET}), true},
		{"connection aborted", &net.OpError{Err: syscall.ECONNABORTED}, true},
		{"broken pipe", &net.OpError{Err: syscall.EPIPE}, true},
		{"truncated response", io.ErrUnexpectedEOF, true},
		{"network timeout", timeout, true},
		{"wrapped 500", fmt.Errorf("norad: fetch failed: %w", &httpStatusError{500}), true},

		// A service declining to serve a request that carried no credential.
		// CelesTrak answers a burst this way — an IIS "Forbidden: Access is
		// denied" page — and serves the same query normally a minute later.
		{"403 forbidden", &httpStatusError{403}, true},
		{"wrapped 403", fmt.Errorf("norad: fetch failed: %w", &httpStatusError{403}), true},

		// We sent a bad request: exactly what these tests exist to catch.
		{"400 bad request", &httpStatusError{400}, false},
		{"404 not found", &httpStatusError{404}, false},
		// 401 is the opposite of the 403 above and not a near-duplicate of it.
		// It says the service wants authentication, which for an endpoint
		// astrogo believes is public means astrogo's model of that endpoint is
		// wrong — the URL moved, or the service grew an auth requirement.
		{"401 unauthorized", &httpStatusError{401}, false},
		{"parse failure", errStaticParse, false},

		// The network never carried the request. Delegated to Unreachable
		// rather than restated here, and these cases are the reason a caller
		// no longer needs to consult both predicates — see #371.
		//
		// The DNS case used to be pinned to false, under the label "connection
		// refused", which it is not. That expectation was the gap: a name that
		// does not resolve is no more astrogo's doing than a 503.
		{"dns failure", &net.DNSError{IsNotFound: true}, true},
		{"refused dial", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, true},
		{"host unreachable", &net.OpError{Op: "dial", Err: syscall.EHOSTUNREACH}, true},

		// A caller that gave up on its own budget still says nothing about the
		// network, and Unreachable declines it — but the deadline arm above
		// claims it first, deliberately, because a service too slow to answer
		// within the endpoint's timeout is the service's problem.
		{"canceled by the caller", context.Canceled, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, got := UpstreamFailure(c.err); got != c.want {
				t.Fatalf("UpstreamFailure(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestSkipOnUpstreamFailureIgnoresNil(t *testing.T) {
	// A nil error must not skip: the call succeeded and the assertions
	// that follow are the point of the test.
	SkipOnUpstreamFailure(t, nil)
}

func TestSkipOnUpstreamFailureSkipsA500(t *testing.T) {
	fake := &fakeTB{TB: t}
	SkipOnUpstreamFailure(fake, &httpStatusError{503})

	if !fake.skipped {
		t.Fatal("a 503 must skip, not fail: the service is down, astrogo is not")
	}
}

func TestSkipOnUpstreamFailureKeepsA404(t *testing.T) {
	fake := &fakeTB{TB: t}
	SkipOnUpstreamFailure(fake, &httpStatusError{404})

	if fake.skipped {
		t.Fatal("a 404 must not skip: astrogo built a request the service rejected")
	}
}

// TestSkipOnUpstreamFailureAloneCoversAnUnreachableHost is the point of #371:
// one call, not two.
//
// Before this, a refused dial reached SkipOnUpstreamFailure and was declined,
// so a call site that wanted the whole question answered had to consult
// Unreachable first. Four did. The rest of the repository mostly did not, and
// reached for a skip on any error instead.
func TestSkipOnUpstreamFailureAloneCoversAnUnreachableHost(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"a refused dial", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}},
		{"a name that does not resolve", &net.DNSError{IsNotFound: true}},
	} {
		fake := &fakeTB{TB: t}
		SkipOnUpstreamFailure(fake, tc.err)

		if !fake.skipped {
			t.Errorf("%s must skip through SkipOnUpstreamFailure alone; a caller "+
				"should not have to ask Unreachable as well", tc.name)
		}
	}
}

// TestSkipOnUpstreamFailureKeepsAParseFailure guards the other direction: the
// widening must not have turned this into a predicate that skips on anything.
func TestSkipOnUpstreamFailureKeepsAParseFailure(t *testing.T) {
	fake := &fakeTB{TB: t}
	SkipOnUpstreamFailure(fake, errStaticParse)

	if fake.skipped {
		t.Fatal("a decode failure must not skip: it is astrogo reading a response wrong")
	}
}

// fakeTB records whether Skipf was called instead of skipping the real test.
type fakeTB struct {
	testing.TB

	skipped bool
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Skipf(string, ...any) { f.skipped = true }
