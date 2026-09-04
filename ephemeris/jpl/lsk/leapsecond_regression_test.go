package lsk_test

import (
	"io"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// NAIF writes the closing parenthesis of DELTET/DELTA_AT on the same line as
// the final leap second. naif0012.tls — the kernel this package actually
// loads — ends exactly like this:
//
//	36,   @2015-JUL-1
//	37,   @2017-JAN-1 )
//
// The parser used to clear its in-block flag on seeing ")" and only then
// test it, so the remainder ("37,   @2017-JAN-1") matched neither arm of the
// guard and the entry was dropped without an error. The table silently ended
// at 36 leap seconds.
const lskParenOnLastEntry = `KPL/LSK

\begindata

DELTET/DELTA_AT        = ( 10,   @1972-JAN-1
                           36,   @2015-JUL-1
                           37,   @2017-JAN-1 )

\begintext
`

// Some kernels close the block on a line of their own. Both shapes have to
// parse to the same table, which is the reason this fixture exists next to
// the one above rather than instead of it.
const lskParenOnOwnLine = `KPL/LSK

\begindata

DELTET/DELTA_AT        = ( 10,   @1972-JAN-1
                           36,   @2015-JUL-1
                           37,   @2017-JAN-1
                         )

\begintext
`

// TestFinalLeapSecondIsParsed pins the last row of the leap-second table.
//
// # Why this is worth its own test
//
// Because losing the final row is invisible at the point it happens and
// expensive everywhere else. The parser reported no error, the kernel loaded,
// and every date before 2017 stayed correct — so nothing looked wrong. What
// changed was that every UTC epoch from 2017-01-01 onward converted one
// second early, and one second of Earth's orbital motion put the geocentric
// Sun about 30 km from where DE440 has it. That is three times Epv00's own
// published worst-case error, which is how it was eventually noticed: a
// DE440-against-SOFA comparison that printed its measured difference instead
// of only asserting a bound.
//
// The failure mode also renews itself. The next leap second appended to the
// table becomes the new last row, so without this test the bug returns the
// day IERS announces one.
func TestFinalLeapSecondIsParsed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		fixture string
	}{
		{"paren on last entry", lskParenOnLastEntry},
		{"paren on own line", lskParenOnOwnLine},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r, err := lsk.NewReader(io.NopCloser(strings.NewReader(tc.fixture)))
			testutil.AssertNoError(t, err)

			t.Cleanup(func() {
				if err := r.Close(); err != nil {
					t.Errorf("close: %v", err)
				}
			})

			// TDB - UTC is the accumulated leap seconds plus 32.184 s. The
			// two epochs straddle the 2017 entry, so a dropped final row
			// shows up as the later one returning the earlier one's answer
			// rather than as a wrong number in isolation.
			for _, want := range []struct {
				label   string
				year    int
				offsetS float64
			}{
				{"2016 (36 leap seconds)", 2016, 36 + 32.184},
				{"2020 (37 leap seconds)", 2020, 37 + 32.184},
			} {
				utc := time.Date(want.year, 1, 1, 0, 0, 0, 0, time.LocationUTC)
				got := (lsk.UTCToTDB(utc, r) - utc.JD()) * 86400

				// 3 ms, not the 1 ms this once asserted. The old bound
				// encoded a bug: it required TDB-UTC to be *exactly*
				// leap + 32.184, which is the formula for TT-UTC. True TDB
				// carries a periodic term of about ±1.7 ms amplitude, so the
				// old assertion could only pass while the conversion was
				// silently returning TT.
				//
				// Widening does not weaken what this test is for. A dropped
				// leap-second row moves the answer by a whole second — three
				// hundred times the slack — so the 2016/2020 pair still
				// straddles the 2017 entry as tightly as before.
				testutil.AssertNear(t, "TDB-UTC at "+want.label, got, want.offsetS, 3e-3)
			}
		})
	}
}
