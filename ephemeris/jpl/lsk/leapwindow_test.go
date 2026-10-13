package lsk_test

import (
	"fmt"
	"io"
	"math"
	"strings"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/time"
)

// leapWindowLSK carries naif0012.tls's DELTET constants and the DELTA_AT
// entries around the three most recent leap seconds, as that kernel gives
// them.
const leapWindowLSK = `KPL/LSK

\begindata

DELTET/DELTA_T_A       =   32.184
DELTET/K               =    1.657D-3
DELTET/EB              =    1.671D-2
DELTET/M               = (  6.239996D0   1.99096871D-7 )

DELTET/DELTA_AT        = ( 10,   @1972-JAN-1
                           34,   @2009-JAN-1
                           35,   @2012-JUL-1
                           36,   @2015-JUL-1
                           37,   @2017-JAN-1 )

\begintext
`

// TestUTCToETHoldsTheOldCountUntilTheStep holds UTCToET to time's own TDB in
// the minutes either side of a leap second (#756).
//
// The kernel's DELTA_AT entries are midnight UTC, and the lookup added
// 69.184 s to the UTC date first, as if they were TDB. So from about 23:58:51
// to the end of 23:59:60 on every day ending in a leap second, the new count
// was applied a second early and ET came out a second late: measured
// +0.999979 s at 2016-12-31 23:59:00. Away from a step the two models agree
// to about 30 µs, Moyer's single sinusoid against Fairhead & Bretagnon, so a
// millisecond separates them from a whole second by three orders.
func TestUTCToETHoldsTheOldCountUntilTheStep(t *testing.T) {
	t.Parallel()

	r, err := lsk.NewReader(io.NopCloser(strings.NewReader(leapWindowLSK)))
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}

	const bound = 1e-3 // seconds

	steps := []gotime.Time{
		gotime.Date(2012, gotime.July, 1, 0, 0, 0, 0, gotime.UTC),
		gotime.Date(2015, gotime.July, 1, 0, 0, 0, 0, gotime.UTC),
		gotime.Date(2017, gotime.January, 1, 0, 0, 0, 0, gotime.UTC),
	}

	offsets := []gotime.Duration{
		-10 * gotime.Minute, -70 * gotime.Second, -69 * gotime.Second, -60 * gotime.Second,
		-gotime.Second, -gotime.Millisecond, gotime.Millisecond, gotime.Minute,
	}

	checked := 0

	for _, step := range steps {
		for _, off := range offsets {
			check(t, r, time.FromGo(step.Add(off)), step.Add(off).Format("2006-01-02 15:04:05.000"), bound)

			checked++
		}

		// The inserted second itself, which time builds as a real instant
		// on the day's 86401st second. Its last 10 µs are below the 40 µs
		// ULP of a summed Julian Date, which rounds them onto the next
		// midnight; the lookup compares the two parts to keep them.
		y, m, d := step.Add(-gotime.Hour).Date()

		for _, us := range []int{0, 500_000, 999_000, 999_990} {
			leap := time.Date(y, m, d, 23, 59, 60, us*1000, gotime.UTC)
			check(t, r, leap, fmt.Sprintf("%d-%02d-%02d 23:59:60.%06d", y, m, d, us), bound)

			checked++
		}
	}

	t.Logf("%d instants within ten minutes of three leap seconds", checked)
}

func check(t *testing.T, r *lsk.Reader, when time.Time, label string, bound float64) {
	t.Helper()

	d1, d2 := when.TDB().JDParts()
	want := ((d1 - 2451545.0) + d2) * 86400

	if got := lsk.UTCToET(when, r); math.Abs(got-want) > bound {
		t.Errorf("UTCToET(%s UTC) - TDB = %+.6f s, want within %g s", label, got-want, bound)
	}
}
