package horizons_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/horizons"
)

// horizonsCIFault is the explanation JPL Horizons returned for both 433 Eros
// and Apophis on 2026-09-15, with HTTP 200 and an otherwise well-formed body,
// failing astrogo's CI, and again for every kind of request on 2026-10-26.
//
// Quoted verbatim, double semicolon included, because the point is that
// astrogo recognizes the real string and not a tidied-up version of it.
const horizonsCIFault = "wldini(): missing required file LTKERNL; ; " +
	"ERROR in VLRDC: Var not declared: IP_ADDR"

// TestServerFaultSeparatesOutageFromRefusal is the discrimination
// this whole mechanism exists for, stated as a table.
//
// The two classes arrive identically — 200, well-formed JSON, an explanation in
// the "error" field — and mean opposite things. Getting it backwards in either
// direction is a real cost: a refusal misread as an outage is a permanently
// broken request that skips silently for ever, and an outage misread as a
// refusal is a red build nobody can fix.
func TestServerFaultSeparatesOutageFromRefusal(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		explanation string
		wantFault   bool
	}{
		{
			name:        "the fault that failed CI",
			explanation: horizonsCIFault,
			wantFault:   true,
		},
		{
			name:        "a missing file on JPL's own disk",
			explanation: "wldini(): missing required file LTKERNL",
			wantFault:   true,
		},
		{
			name:        "an undeclared variable in JPL's own code",
			explanation: "ERROR in VLRDC: Var not declared: IP_ADDR",
			wantFault:   true,
		},
		{
			// The canonical real refusal, quoted in ErrHorizonsRefused's doc
			// comment. It must stay loud: no retry will ever make it work.
			name: "a considered refusal about the request",
			explanation: "SPK creation is not available for pre-computed objects " +
				"in the major body index",
			wantFault: false,
		},
		{
			name:        "an out-of-range object number",
			explanation: "requested IOBJ=20000004 is out of bounds",
			wantFault:   false,
		},
		{
			// "ERROR in " is deliberately not a marker, precisely so that a
			// complaint about the request phrased that way is not swallowed.
			name:        "a request complaint wearing an ERROR in prefix",
			explanation: "ERROR in SPKSUB: Requested interval is outside object coverage",
			wantFault:   false,
		},
		{
			name:        "no explanation at all",
			explanation: "",
			wantFault:   false,
		},
	} {
		if got := horizons.ServerFault(tc.explanation); got != tc.wantFault {
			t.Errorf("%s: ServerFault(%q) = %v, want %v",
				tc.name, tc.explanation, got, tc.wantFault)
		}
	}
}
