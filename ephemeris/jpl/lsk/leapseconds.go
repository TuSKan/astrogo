package lsk

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
)

// RegisterLeapSeconds installs the kernel's DELTA_AT block as the process-wide
// ΔAT table, via [time.RegisterLeapSeconds].
//
// # Why a kernel is a better source than a pinned dependency
//
// gofa's table is compiled into its source and reaches astrogo only through a
// go.mod bump and a release. A leapsecond kernel is a file NAIF republishes
// when IERS announces a change, and [Cache] already refreshes it. So a caller
// who has a current kernel has a current leap-second record, and this is what
// connects the two.
//
// # Not called by NewReader
//
// Process-wide state is not something opening a file should acquire. A caller
// parsing a kernel to inspect it gets a parse and nothing else;
// [github.com/TuSKan/astrogo/ephemeris/jpl.NewProvider] calls this because a
// provider is by then already committed to driving epochs from that kernel.
//
// Registration is superset-only: a kernel that contradicts the built-in record
// below its last step is rejected with [time.ErrLeapSecondConflict] rather
// than installed. See [time.RegisterLeapSeconds] for why that restriction is
// what makes late registration safe.
func RegisterLeapSeconds(r *Reader) error {
	table, err := leapTable(r)
	if err != nil {
		return err
	}

	if err := time.RegisterLeapSeconds(table, "kernel"); err != nil {
		return fmt.Errorf("lsk: %w", err)
	}

	return nil
}

// leapTable converts the kernel's DELTA_AT entries to calendar form.
//
// The kernel records each step as a Julian Date at midnight UTC, because that
// is the instant a leap second takes effect. time's table is in calendar form
// because that is the form IERS publishes and the form a reader can check by
// eye against Bulletin C.
func leapTable(r *Reader) ([]time.LeapSecond, error) {
	if len(r.DeltaAt) == 0 {
		return nil, ErrNoLeapseconds
	}

	table := make([]time.LeapSecond, 0, len(r.DeltaAt))

	for _, d := range r.DeltaAt {
		// Midnight, so the fraction is 0 and the day number is the .5 below
		// an integer. Rounding to the nearest midnight first keeps a parse
		// that lands an ULP low from being read as the previous day.
		jd := math.Floor(d.JD) + 0.5

		y, m, day, _, status := gofaext.JdToDate(jd, 0)
		if status != 0 {
			return nil, fmt.Errorf("%w: DELTA_AT entry at JD %.1f is outside the "+
				"representable calendar range", ErrInvalidDate, d.JD)
		}

		table = append(table, time.LeapSecond{Year: y, Month: m, Day: day, DeltaAT: d.N})
	}

	return table, nil
}
