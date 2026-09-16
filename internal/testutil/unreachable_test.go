package testutil_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestUnreachableSeparatesTheNetworkFromTheService is the whole point of the
// predicate: a service that answers is under test, and one that cannot be
// reached says nothing about the code.
//
// Getting it wrong in one direction turns somebody else's outage into a red
// build; in the other it turns a real failure into a skip, and a skip reads as
// a pass.
func TestUnreachableSeparatesTheNetworkFromTheService(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},

		{
			name: "dial timeout, wrapped the way a fetch wraps it",
			err: fmt.Errorf("jpl: SPK kernel planets/de440s.bsp: %w",
				&net.OpError{Op: "dial", Net: "tcp", Err: timeoutError{}}),
			want: true,
		},
		{
			name: "DNS failure",
			err:  fmt.Errorf("fetch: %w", &net.DNSError{Err: "no such host", Name: "nope.invalid"}),
			want: true,
		},
		{"connection refused", fmt.Errorf("get: %w", syscall.ECONNREFUSED), true},
		{"connection reset", fmt.Errorf("read: %w", syscall.ECONNRESET), true},
		{"host unreachable", fmt.Errorf("dial: %w", syscall.EHOSTUNREACH), true},
		{"network unreachable", fmt.Errorf("dial: %w", syscall.ENETUNREACH), true},

		{
			name: "a plain error from a service that answered",
			err:  fmt.Errorf("remote: %w: 500 Internal Server Error", errServed),
			want: false,
		},
		{
			name: "the caller gave up, which is not the network's doing",
			err:  fmt.Errorf("fetch: %w", context.Canceled),
			want: false,
		},
		{
			name: "a deadline the caller set and blew on its own arithmetic",
			err:  fmt.Errorf("compute: %w", context.DeadlineExceeded),
			want: false,
		},
	} {
		if got := testutil.Unreachable(tc.err); got != tc.want {
			t.Errorf("%s: Unreachable(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

// errServed stands for a status a server actually returned.
var errServed = errors.New("unexpected HTTP status")

// timeoutError is a net.Error that reports a timeout, which is how the standard
// library signals a transport deadline.
type timeoutError struct{}

func (timeoutError) Error() string { return "i/o timeout" }
func (timeoutError) Timeout() bool { return true }
func (timeoutError) Temporary() bool {
	return true
}

// TestUnreachableAgainstARealSocket covers the two ends against an actual
// network rather than constructed errors, because the error a dial produces is
// the standard library's to shape and this predicate reads its insides.
func TestUnreachableAgainstARealSocket(t *testing.T) {
	t.Parallel()

	t.Run("a refused connection is unreachable", func(t *testing.T) {
		t.Parallel()

		// A listener closed immediately leaves a port nothing is on.
		var lc net.ListenConfig

		l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
		if err != nil {
			t.Skipf("cannot listen: %v", err)
		}

		addr := l.Addr().String()
		_ = l.Close()

		// One second, in nanoseconds: this package sits below astrogo/time and
		// cannot import it without closing a cycle, and the standard library's
		// time is barred everywhere outside it — see
		// TestNoStandardLibraryTimeOutsideTimePackage and the same reasoning on
		// ReachableTimeout. An untyped constant converts to a Duration here.
		const oneSecond = 1_000_000_000

		dialer := net.Dialer{Timeout: oneSecond}

		_, derr := dialer.DialContext(t.Context(), "tcp", addr)
		if derr == nil {
			t.Skip("something is listening on the port that was just released")
		}

		if !testutil.Unreachable(derr) {
			t.Errorf("Unreachable(%v) = false for a refused connection", derr)
		}
	})

	t.Run("a served 500 is not unreachable", func(t *testing.T) {
		t.Parallel()

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "the service is broken, but it is there", http.StatusInternalServerError)
		}))
		defer srv.Close()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("the request itself failed: %v", err)
		}

		defer func() { _ = resp.Body.Close() }()

		// The transport succeeded; whatever a caller makes of a 500 is not this
		// predicate's business, and must not read as unreachable.
		if testutil.Unreachable(err) {
			t.Error("a served response reported as unreachable")
		}
	})
}
