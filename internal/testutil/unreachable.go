package testutil

import (
	"context"
	"errors"
	"net"
	"syscall"
)

// Unreachable reports whether err is the network declining to carry a request,
// as opposed to a service answering and saying no.
//
// # Why a predicate on the error, when RequireReachable already exists
//
// Because they answer different questions at different moments.
// [RequireReachable] asks "is anything listening" before a test starts, which
// is the cheap check and the right one for a suite that would otherwise spend
// a minute discovering the same thing. It cannot cover a service that accepts
// the connection and then stalls, and it cannot cover a fetch buried several
// layers down inside the code under test.
//
// That is not hypothetical. TestSmallBodyEros reached NAIF, got through DNS,
// and timed out mid-transfer thirty seconds later:
//
//	jpl: SPK kernel planets/de440s.bsp: remote: fetch ...: dial tcp
//	137.79.133.14:443: i/o timeout
//
// The test's own doc comment said it "must not turn JPL's downtime into a red
// build" and it did exactly that, because its guard recognised only the two
// ways Horizons answers 200 and still fails. An unreachable host was not one of
// them.
//
// # What counts, and what deliberately does not
//
// Counted: a timeout at any layer, a DNS failure, and the connection being
// refused or the host or network being unreachable. Each of these means the
// request never reached a service that could have an opinion.
//
// Not counted: any status a server actually returned, including 500. A service
// that answers is a service under test, and astrogo's own rule is that a
// wrong answer from a reachable endpoint stays fatal. Nor a bare
// context.Canceled, which means the caller gave up and says nothing about the
// network.
//
// The context errors are excluded first, and that ordering is load-bearing:
// context.DeadlineExceeded satisfies net.Error with Timeout() true, so without
// the exclusion a deadline the caller set and blew through on its own
// arithmetic would launder itself into a skip. Written the other way round this
// predicate was wrong, and its own test caught it.
func Unreachable(err error) bool {
	if err == nil {
		return false
	}

	// A caller who gave up, or a budget a caller set. Neither says anything
	// about whether the network would have carried the request.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}

	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}

	// A dial that failed for any reason reached no service at all, whatever the
	// platform called the failure. This is the portable form of the errno list
	// below: Windows reports a refused connection as WSAECONNREFUSED, which is
	// a different value from syscall.ECONNREFUSED and which errors.Is does not
	// match — measured, a real refused dial on Windows fell through every errno
	// case and reported false.
	if op, ok := errors.AsType[*net.OpError](err); ok && op.Op == "dial" {
		return true
	}

	// The errno cases, for a failure that arrives without an OpError around it.
	for _, syscallErr := range []error{
		syscall.ECONNREFUSED,
		syscall.ECONNRESET,
		syscall.EHOSTUNREACH,
		syscall.ENETUNREACH,
		syscall.ETIMEDOUT,
	} {
		if errors.Is(err, syscallErr) {
			return true
		}
	}

	return false
}
