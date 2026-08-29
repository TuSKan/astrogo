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

		// We sent a bad request: exactly what these tests exist to catch.
		{"400 bad request", &httpStatusError{400}, false},
		{"404 not found", &httpStatusError{404}, false},
		{"401 unauthorized", &httpStatusError{401}, false},
		{"403 forbidden", &httpStatusError{403}, false},
		{"parse failure", errStaticParse, false},
		{"connection refused", &net.DNSError{}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, got := upstreamFailure(c.err); got != c.want {
				t.Fatalf("upstreamFailure(%v) = %v, want %v", c.err, got, c.want)
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

// fakeTB records whether Skipf was called instead of skipping the real test.
type fakeTB struct {
	testing.TB

	skipped bool
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Skipf(string, ...any) { f.skipped = true }
