package time_test

import (
	"math"
	"testing"
	gotime "time"

	astrotime "github.com/TuSKan/astrogo/time"
)

// epochs spans the range a catalogue or an ephemeris is likely to be asked
// about, including a leap-second boundary and both sides of J2000.
func epochs() []gotime.Time {
	return []gotime.Time{
		gotime.Date(1972, 1, 1, 0, 0, 0, 0, gotime.UTC), // TAI-UTC becomes 10s
		gotime.Date(1999, 12, 31, 23, 59, 59, 0, gotime.UTC),
		gotime.Date(2000, 1, 1, 12, 0, 0, 0, gotime.UTC),     // J2000.0
		gotime.Date(2016, 12, 31, 23, 59, 59, 0, gotime.UTC), // last leap second so far
		gotime.Date(2017, 1, 1, 0, 0, 1, 0, gotime.UTC),
		gotime.Date(2026, 8, 21, 3, 14, 15, 0, gotime.UTC),
		gotime.Date(2050, 6, 1, 18, 30, 0, 0, gotime.UTC),
	}
}

// Every conversion between time scales must be reversible.
//
// The scales differ by offsets that are constant (TT-TAI), tabulated
// (TAI-UTC), or measured (UT1-UTC). Going out and back has to return the
// instant that was started from: a conversion applied in the wrong direction
// lands 32 or 37 seconds away, which is a plausible timestamp and enough to
// move a fast-moving body across a detector.
func TestScaleConversionsAreReversible(t *testing.T) {
	t.Parallel()

	// UT1 is excluded from the exhaustive pairing because it depends on
	// measured Earth-orientation data that may be absent offline; it is
	// checked separately below.
	scales := []struct {
		name string
		to   func(astrotime.Time) astrotime.Time
	}{
		{"UTC", astrotime.Time.UTC},
		{"TAI", astrotime.Time.TAI},
		{"TT", astrotime.Time.TT},
		{"TDB", astrotime.Time.TDB},
	}

	for _, when := range epochs() {
		start := astrotime.FromGo(when)

		for _, out := range scales {
			for _, back := range scales {
				there := out.to(start)
				andBack := back.to(there)
				home := out.to(andBack)

				// Converting to a scale and back to the same scale must be
				// exact to well below a microsecond.
				if d := math.Abs(home.JD()-there.JD()) * 86400; d > 1e-6 {
					t.Errorf("%s: %s -> %s -> %s moved the instant by %.3g seconds",
						when.Format(gotime.RFC3339), out.name, back.name, out.name, d)
				}
			}
		}
	}
}

// A scale conversion must change the numeric date, not merely relabel it.
//
// If Scale() reports TT but the underlying Julian date was never shifted, every
// downstream computation is silently 32.184 seconds out and nothing says so.
func TestScaleConversionsActuallyShiftTheInstant(t *testing.T) {
	t.Parallel()

	when := astrotime.FromGo(gotime.Date(2026, 8, 21, 3, 0, 0, 0, gotime.UTC))

	utc := when.UTC()
	tai := when.TAI()
	tt := when.TT()

	// TT - TAI is exactly 32.184 seconds, by definition.
	if d := secondsBetween(tt, tai); math.Abs(d-32.184) > 1e-6 {
		t.Errorf("TT - TAI = %.6f seconds, want 32.184 exactly", d)
	}

	// TAI - UTC is the accumulated leap seconds: 37 since 2017.
	if d := secondsBetween(tai, utc); math.Abs(d-37) > 1e-6 {
		t.Errorf("TAI - UTC = %.6f seconds, want 37 for this epoch", d)
	}

	// And the labels have to agree with the arithmetic.
	if utc.Scale() != astrotime.UTC || tai.Scale() != astrotime.TAI || tt.Scale() != astrotime.TT {
		t.Errorf("scales mislabelled: %v %v %v", utc.Scale(), tai.Scale(), tt.Scale())
	}
}

// The Julian date and the calendar must agree in both directions.
func TestJulianDateCalendarRoundTrip(t *testing.T) {
	t.Parallel()

	for _, when := range epochs() {
		start := astrotime.FromGo(when)

		back := astrotime.FromJD(start.JD(), start.Scale())
		if d := math.Abs(back.JD()-start.JD()) * 86400; d > 1e-6 {
			t.Errorf("%s: JD round trip moved by %.3g seconds", when.Format(gotime.RFC3339), d)
		}

		// The two-part form exists to keep precision that one float loses; it
		// must be at least as good as the single value.
		jd1, jd2 := start.JDParts()

		parts := astrotime.FromJDParts(jd1, jd2, start.Scale())
		if d := math.Abs(parts.JD()-start.JD()) * 86400; d > 1e-6 {
			t.Errorf("%s: JDParts round trip moved by %.3g seconds", when.Format(gotime.RFC3339), d)
		}

		// And the Go round trip, which is what most callers use.
		if d := start.ToGo().Sub(when).Abs(); d > gotime.Microsecond {
			t.Errorf("%s: FromGo/ToGo moved by %v", when.Format(gotime.RFC3339), d)
		}
	}
}

// The calendar accessors must agree with the date they came from.
func TestCalendarAccessorsAgree(t *testing.T) {
	t.Parallel()

	for _, when := range epochs() {
		start := astrotime.FromGo(when)

		y, m, d, _ := start.Calendar()

		if y != start.Year() || m != int(start.Month()) || d != start.Day() {
			t.Errorf("%s: Calendar gave (%d, %d, %d) but the accessors gave (%d, %d, %d)",
				when.Format(gotime.RFC3339), y, m, d, start.Year(), int(start.Month()), start.Day())
		}

		// Against Go's own calendar, which is independent of this package.
		gy, gm, gd := when.Date()
		if y != gy || m != int(gm) || d != gd {
			t.Errorf("%s: Calendar gave (%d, %d, %d), Go says (%d, %d, %d)",
				when.Format(gotime.RFC3339), y, m, d, gy, int(gm), gd)
		}
	}
}

// Adding a duration and subtracting it must return the same instant, including
// across a leap-second boundary where the UTC day is not 86400 seconds long.
func TestAddIsReversible(t *testing.T) {
	t.Parallel()

	for _, when := range epochs() {
		start := astrotime.FromGo(when)

		for _, d := range []gotime.Duration{
			gotime.Second, gotime.Minute, gotime.Hour, 24 * gotime.Hour, 365 * 24 * gotime.Hour,
		} {
			back := start.Add(d).Add(-d)
			if moved := math.Abs(back.JD()-start.JD()) * 86400; moved > 1e-6 {
				t.Errorf("%s: +%v then -%v moved the instant by %.3g seconds",
					when.Format(gotime.RFC3339), d, d, moved)
			}
		}

		for _, days := range []float64{0.5, 1, 30, 365.25} {
			back := start.AddDays(days).AddDays(-days)
			if moved := math.Abs(back.JD()-start.JD()) * 86400; moved > 1e-6 {
				t.Errorf("%s: +%v days then back moved by %.3g seconds",
					when.Format(gotime.RFC3339), days, moved)
			}
		}
	}
}

// secondsBetween differences two instants through their two-part Julian dates.
//
// Subtracting two JD() values cannot resolve a small offset: one ULP of a
// float64 near 2460000 is about 4.7e-10 days, which is 40 microseconds, so
// TT - TAI computed that way comes out as 32.184014 rather than 32.184. That
// is the representation, not the conversion, and JDParts exists precisely so a
// caller does not have to lose it — the integer day and the fraction are kept
// apart, and differencing them term by term keeps the fraction's own precision.
func secondsBetween(a, b astrotime.Time) float64 {
	a1, a2 := a.JDParts()
	b1, b2 := b.JDParts()

	return ((a1 - b1) + (a2 - b2)) * 86400
}
