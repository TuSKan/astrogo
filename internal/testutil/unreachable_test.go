package testutil_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		{
			name: "a connection closed before any response, as NAIF did (#505)",
			err: fmt.Errorf("remote/file: HEAD %s: %w", naifKernel,
				&url.Error{Op: "Head", URL: naifKernel, Err: io.EOF}),
			want: true,
		},
		{
			name: "a status line cut short",
			err:  &url.Error{Op: "Get", URL: naifKernel, Err: io.ErrUnexpectedEOF},
			want: true,
		},
		{
			name: "a body cut short by a server that answered",
			err:  fmt.Errorf("read kernel: %w", io.ErrUnexpectedEOF),
			want: false,
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

// naifKernel is the URL NAIF closed the connection on in #505.
const naifKernel = "https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/planets/de440s.bsp"

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
			// Fatal. A loopback listener on port 0 is the least a machine can
			// offer, and this is the only test that exercises Unreachable
			// against a socket the operating system actually refused rather
			// than an error this repository built. Skipping it would retire
			// that coverage silently, on exactly the machines where something
			// is unusual enough to be worth knowing about.
			t.Fatalf("listen on 127.0.0.1:0: %v", err)
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

	t.Run("a connection closed before any response is unreachable", func(t *testing.T) {
		t.Parallel()

		// The request is read in full before the close, so the close is an
		// orderly one; closing on unread bytes would reset the connection
		// instead, which is the other case.
		addr := rawServer(t, func(net.Conn, *bufio.Reader) {})

		resp, err := get(t, addr)
		if err == nil {
			_ = resp.Body.Close()

			t.Fatal("a server that answered nothing produced a response")
		}

		if !testutil.Unreachable(err) {
			t.Errorf("Unreachable(%v) = false for a connection closed before any response", err)
		}
	})

	t.Run("a connection the server reset is unreachable", func(t *testing.T) {
		t.Parallel()

		// A zero linger makes the close a reset rather than an orderly
		// close: Linux and macOS report ECONNRESET, Windows WSAECONNRESET,
		// which only the platform's own number matches.
		addr := rawServer(t, func(conn net.Conn, _ *bufio.Reader) {
			if tcp, ok := conn.(*net.TCPConn); ok {
				_ = tcp.SetLinger(0)
			}
		})

		resp, err := get(t, addr)
		if err == nil {
			_ = resp.Body.Close()

			t.Fatal("a server that reset the connection produced a response")
		}

		if !testutil.Unreachable(err) {
			t.Errorf("Unreachable(%v) = false for a reset connection", err)
		}
	})

	t.Run("a body cut short by a server that answered is not unreachable", func(t *testing.T) {
		t.Parallel()

		addr := rawServer(t, func(conn net.Conn, _ *bufio.Reader) {
			_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nten bytes!")
		})

		resp, err := get(t, addr)
		if err != nil {
			t.Fatalf("the request itself failed: %v", err)
		}

		defer func() { _ = resp.Body.Close() }()

		_, err = io.ReadAll(resp.Body)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("reading a short body: err = %v, want io.ErrUnexpectedEOF", err)
		}

		if testutil.Unreachable(fmt.Errorf("read kernel: %w", err)) {
			t.Error("a body cut short by a server that answered reported as unreachable")
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

// rawServer serves every connection on a loopback port by reading the
// request's head, handing the connection to respond and closing it, and
// returns the address.
func rawServer(t *testing.T, respond func(net.Conn, *bufio.Reader)) string {
	t.Helper()

	var lc net.ListenConfig

	l, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on 127.0.0.1:0: %v", err)
	}

	t.Cleanup(func() { _ = l.Close() })

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}

			r := bufio.NewReader(conn)

			for {
				line, err := r.ReadString('\n')
				if err != nil || line == "\r\n" {
					break
				}
			}

			respond(conn, r)
			_ = conn.Close()
		}
	}()

	return l.Addr().String()
}

// get requests the root of addr.
func get(t *testing.T, addr string) (*http.Response, error) {
	t.Helper()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+addr+"/", nil)
	if err != nil {
		t.Fatal(err)
	}

	return http.DefaultClient.Do(req) //nolint:wrapcheck // the test reads the transport's own error
}

// TestANetworkFailureWinsOverAnAccompanyingDeadline is #348.
//
// A download runs under a context carrying the endpoint's timeout, so a host
// that stops answering can leave both the dial failure and
// context.DeadlineExceeded in one error chain. Excluding the context errors
// before everything else swallowed that pair and reported a genuinely
// unreachable host as reachable — which is how ephemeris/jpl's kernel tests
// failed on NAIF's downtime with their skip guard in place and not firing.
//
// The distinction the exclusion exists to protect is still asserted below: a
// deadline on its own, with no network signal anywhere in the chain, is still
// not the network's doing.
func TestANetworkFailureWinsOverAnAccompanyingDeadline(t *testing.T) {
	t.Parallel()

	dial := &net.OpError{Op: "dial", Net: "tcp", Err: timeoutError{}}

	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "a dial failure and the download's deadline, joined",
			err:  fmt.Errorf("remote: fetch de440s.bsp: %w: %w", context.DeadlineExceeded, dial),
			want: true,
		},
		{
			name: "the same pair the other way round",
			err:  fmt.Errorf("remote: fetch: %w: %w", dial, context.DeadlineExceeded),
			want: true,
		},
		{
			name: "DNS failed and the deadline fired while it did",
			err: fmt.Errorf("fetch: %w: %w", context.DeadlineExceeded,
				&net.DNSError{Err: "no such host", Name: "naif.jpl.nasa.gov"}),
			want: true,
		},
		{
			name: "connection refused under a canceled context",
			err:  fmt.Errorf("get: %w: %w", context.Canceled, syscall.ECONNREFUSED),
			want: true,
		},

		// The exclusion still does its job when nothing says the network was
		// involved. These are the cases that made it necessary.
		{
			name: "a deadline alone is still the caller's own",
			err:  fmt.Errorf("compute: %w", context.DeadlineExceeded),
			want: false,
		},
		{
			name: "cancellation alone is still the caller's own",
			err:  fmt.Errorf("fetch: %w", context.Canceled),
			want: false,
		},
		{
			name: "a deadline wrapping a served response is not the network",
			err:  fmt.Errorf("remote: %w: %w", context.DeadlineExceeded, errServed),
			want: false,
		},
	} {
		if got := testutil.Unreachable(tc.err); got != tc.want {
			t.Errorf("%s: Unreachable = %v, want %v\n  err: %v", tc.name, got, tc.want, tc.err)
		}
	}
}
