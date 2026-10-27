// Package horizons recognizes JPL Horizons reporting a fault on its own server.
//
// Horizons answers that it cannot do the work the same way it answers that it
// will not: HTTP 200, a well-formed body, an explanation in the text. One is
// somebody else's outage, which fixes itself; the other is a fact about the
// request, which never will. astrogo reaches Horizons through several paths —
// the name lookup in catalog/jpl, kernel generation in ephemeris/jpl/spk, and
// the text tables the validation suites read — and until #858 only the kernel
// path told the two apart. On 2026-10-26 one fault on JPL's server failed 45
// live tests through the other three.
//
// So the recognition lives here, once, and every path wraps the same
// [ErrServerFault]. internal/testutil classifies it as an upstream failure,
// which is what lets a live test skip on it without knowing which path it took.
package horizons

import (
	"errors"
	"strings"
)

// ErrServerFault is a Horizons answer describing a fault on JPL's own server
// rather than anything about the request.
//
// Observed 2026-09-15 and again 2026-10-26, for every kind of request:
//
//	wldini(): missing required file LTKERNL
//	ERROR in VLRDC: Var not declared: IP_ADDR
//
// LTKERNL is a leap-second kernel on Horizons' server and IP_ADDR is one of its
// internal variables. Nothing a caller can send produces that, and the same
// requests succeeded again within hours.
var ErrServerFault = errors.New("horizons: a fault on JPL's own server")

// serverFaultMarkers are phrases that can only describe the state of Horizons'
// own server, never the request that reached it.
//
// That is the whole selection rule, and it is what keeps this from becoming a
// list of every error string anyone has seen. "missing required file" names a
// file on JPL's disk; "Var not declared" names a variable in JPL's code. A
// caller cannot cause either by asking for the wrong body, spelling a
// designation badly, or requesting an epoch outside coverage.
//
// Deliberately absent: "ERROR in ", which prefixes the SPICE-convention routine
// name in the same message. It is the obvious third marker and it is too
// generic — Horizons could reasonably use it for a complaint about the request,
// and a marker that might match a real refusal would turn a permanent problem
// into a skipped test.
var serverFaultMarkers = [...]string{
	"missing required file",
	"Var not declared",
}

// ServerFault reports whether text — Horizons' explanation, or a whole answer
// — describes a fault on its own server.
//
// Conservative by construction: text it does not recognize is not a fault, so
// a refusal or a format change it has never seen fails loudly, which is the
// right default for a signal nobody has seen before. Matching a third party's
// prose is fragile, and it is the only signal there is: the status is 200 and
// the shape is the same either way.
func ServerFault(text string) bool {
	for _, marker := range serverFaultMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	return false
}
