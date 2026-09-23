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
// build" and it did exactly that, because its guard recognized only the two
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
// # The ordering, which has now been wrong in both directions
//
// context.DeadlineExceeded satisfies net.Error with Timeout() true, so a
// generic timeout check placed ahead of the context exclusion lets a deadline
// the caller set and blew through on its own arithmetic launder itself into a
// skip. That was the first version, and this predicate's own test caught it.
//
// Excluding the context errors before *everything* was the overcorrection, and
// #348 is what it cost. A download runs under a context carrying the endpoint's
// timeout, so a host that stops answering can leave both the dial failure and
// context.DeadlineExceeded in one chain — and the exclusion swallowed the pair,
// reporting a genuinely unreachable host as reachable while the skip guard that
// depended on this sat right there and did not fire.
//
// What is true is narrower than either: the exclusion belongs ahead of the
// check that cannot distinguish a network timeout from a caller's deadline, and
// behind the ones that can. A DNS failure, a failed dial and ECONNREFUSED are
// not things a context error produces, so they are decided first and a deadline
// alongside them changes nothing.
func Unreachable(err error) bool {
	if err == nil {
		return false
	}

	// The unambiguous network signals come first, and the ordering is the fix
	// for #348.
	//
	// A download runs under a context carrying the endpoint's timeout, so when
	// a host stops answering the chain can end up holding both the dial failure
	// and context.DeadlineExceeded. The context exclusion below used to run
	// first and swallowed the whole thing: a genuinely unreachable host
	// reported false, and ephemeris/jpl's kernel tests failed on NAIF's
	// downtime with the skip guard sitting right there and not firing.
	//
	// None of the three checks in this block can be produced by a context error
	// on its own — a canceled context is not a *net.DNSError, is not a dial
	// OpError, and is not ECONNREFUSED — so hoisting them past the exclusion
	// costs that exclusion nothing. What must stay below it is the generic
	// net.Error timeout check, for the reason given there.
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}

	// A TLS handshake that failed on the service's own certificate. The
	// connection opened and nothing was exchanged, so the request was never
	// carried — the same thing the checks around this one report, arriving one
	// layer up. A context error cannot produce it either, so it belongs in
	// this block. certificateFailure says which certificate errors count.
	if _, ok := certificateFailure(err); ok {
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

	// A caller who gave up, or a budget a caller set, with nothing above
	// saying the network was at fault. Neither says anything about whether the
	// network would have carried the request.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	// Last, and only once the context errors are out of the way: this is the
	// check that cannot tell them apart. context.DeadlineExceeded satisfies
	// net.Error with Timeout() true, so a deadline the caller set and blew
	// through on its own arithmetic would launder itself into a skip if this
	// ran first. Written the other way round this predicate was wrong, and its
	// own test caught it.
	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return true
	}

	return false
}
