package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// epoch is the instant these tests work at: after the 2017 leap second, so
// ΔAT is 37 s and GPST − UTC is 37 − 19 = 18 s, the figure Levine, Tavella &
// Milton (2023) table 1 gives for GPS.
func epoch() time.Time {
	return time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC)
}

const secondsPerDay = 86400

// labelSeconds is the difference between two instants' LABELS, in seconds.
//
// It subtracts the two-part Julian Dates componentwise rather than through
// JD(). Collapsing jd1+jd2 into one float64 costs precision: at a modern epoch
// one ULP is about 4.7e-10 days, which is 40 microseconds, and that is enough
// to make an exactly-19-second offset read as 19.000018. The split exists
// precisely so the fractional part keeps its resolution, and a test that
// asserts an exact constant has to use it.
//
// Sub is not the answer here: it converts to a common scale first and would
// correctly report zero, since these are one instant. The label gap is the
// quantity under test.
func labelSeconds(a, b time.Time) float64 {
	a1, a2 := a.JDParts()
	b1, b2 := b.JDParts()

	return ((a1 - b1) + (a2 - b2)) * secondsPerDay
}

// TestGPSTOffsetFromTAIIsExactlyNineteenSeconds pins the definition.
//
// Everything else about this scale follows from it: GPS was set to UTC on
// 1980-01-06, when TAI−UTC was 19 s, and has run at the TAI rate ignoring leap
// seconds ever since. So TAI − GPST is 19 s at every epoch, not a table.
func TestGPSTOffsetFromTAIIsExactlyNineteenSeconds(t *testing.T) {
	t.Parallel()

	for _, when := range []time.Time{
		time.Date(1990, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2005, time.July, 4, 6, 30, 0, 0, time.LocationUTC),
		epoch(),
		time.Date(2040, time.December, 31, 23, 0, 0, 0, time.LocationUTC),
	} {
		tai := when.TAI()
		gps := when.GPST()

		// The label difference, which is what the offset is about. Sub would
		// be zero, since these are the same instant.
		got := labelSeconds(tai, gps)
		if math.Abs(got-19.0) > 1e-9 {
			t.Errorf("at %v: TAI − GPST = %.9f s, want exactly 19", when.Format(time.DateOnly), got)
		}
	}
}

// TestGPSTIsEighteenSecondsAheadOfUTCToday is the figure a GNSS user recognises,
// and the one that makes the scale worth having.
//
// Unlike the TAI offset it is not constant: it is ΔAT − 19, so it stepped to 18
// at the 2017 leap second and will step again at the next one. That is exactly
// why a caller cannot be asked to subtract a number themselves.
func TestGPSTIsEighteenSecondsAheadOfUTCToday(t *testing.T) {
	t.Parallel()

	utc := epoch()
	gps := utc.GPST()

	got := labelSeconds(gps, utc)
	if math.Abs(got-18.0) > 1e-9 {
		t.Errorf("GPST − UTC = %.9f s, want 18 (ΔAT 37 − 19)", got)
	}
}

// TestGPSTRoundTripsThroughEveryScale is the property a new scale most easily
// breaks: the conversion graph routes GPST through TAI, and every method that
// did not learn about it would either misread it as UT1 or recurse.
func TestGPSTRoundTripsThroughEveryScale(t *testing.T) {
	t.Parallel()

	start := epoch().GPST()

	// Tolerance is one microsecond of a day. The route through TDB carries the
	// Fairhead periodic term and back, which is not bit-exact.
	const tol = 1e-6 / secondsPerDay

	for _, tc := range []struct {
		name string
		trip func(time.Time) time.Time
	}{
		{"UTC", func(x time.Time) time.Time { return x.UTC().GPST() }},
		{"TAI", func(x time.Time) time.Time { return x.TAI().GPST() }},
		{"TT", func(x time.Time) time.Time { return x.TT().GPST() }},
		{"TDB", func(x time.Time) time.Time { return x.TDB().GPST() }},
	} {
		back := tc.trip(start)

		if back.Scale() != time.GPST {
			t.Errorf("via %s: came back on scale %v, want GPST", tc.name, back.Scale())
		}

		if d := math.Abs(back.JD() - start.JD()); d > tol {
			t.Errorf("via %s: round trip moved the instant by %.9f s",
				tc.name, d*secondsPerDay)
		}
	}
}

// TestGPSTNamesTheSameInstantAsItsUTC is the contract #145 asks for, and the
// one that actually protects a satellite user.
//
// A GPS timestamp and the UTC timestamp of the same physical moment must be
// equal as instants, however differently they are labelled. If they are not,
// everything downstream — a look angle, a pass prediction — is computed for the
// wrong moment.
func TestGPSTNamesTheSameInstantAsItsUTC(t *testing.T) {
	t.Parallel()

	utc := epoch()
	gps := utc.GPST()

	if !gps.Equal(utc) {
		t.Error("a GPST instant and the UTC instant it denotes compare unequal")
	}

	if d := gps.Sub(utc); d != 0 {
		t.Errorf("gps.Sub(utc) = %v, want 0 — these are one moment on two scales", d)
	}

	// And the trap the scale exists to prevent: the same NUMBER read as UTC is
	// a different moment, by the 18 s that would otherwise go unnoticed.
	misread := time.FromJD(gps.JD(), time.UTC)
	if d := misread.Sub(utc).Seconds(); math.Abs(d-18.0) > 1e-3 {
		t.Errorf("mislabelling GPS time as UTC shifts the instant by %.3f s, want 18", d)
	}
}

// TestGPSTLabelsAdvanceInSISecondsAcrossALeapSecond is the property that makes
// this scale worth having for interval arithmetic, and the contrast with UTC is
// the whole of it.
//
// GPS ignores leap seconds, so a GPST label never repeats or skips: the gap
// between two labels IS the elapsed time. A UTC label does skip, so the same
// two instants differ by two hours of label and 7201 seconds of clock.
//
// # What this does not test
//
// Scale.uniform() returns true for GPST, and nothing here can check that.
// uniform() is consulted in exactly one place -- SubDays, to decide whether to
// convert to TT before subtracting -- and for GPST both routes give the same
// answer, because GPST-to-TT is a constant that cancels in a difference.
// Dropping GPST from uniform() therefore changes no result, only two needless
// conversions. Verified by mutation: removing it leaves every test here
// passing.
//
// It is still the correct marking. UTC and UT1 are non-uniform because their
// labels do not track SI seconds, and GPST's do; saying so keeps the predicate
// meaning one thing rather than becoming a list of scales that happen to want
// the fast path.
func TestGPSTLabelsAdvanceInSISecondsAcrossALeapSecond(t *testing.T) {
	t.Parallel()

	// Two hours of UTC label spanning the 2017-01-01 leap second.
	utcBefore := time.Date(2016, time.December, 31, 23, 0, 0, 0, time.LocationUTC)
	utcAfter := time.Date(2017, time.January, 1, 1, 0, 0, 0, time.LocationUTC)

	before, after := utcBefore.GPST(), utcAfter.GPST()

	labels := labelSeconds(after, before)
	elapsed := after.Sub(before).Seconds()

	if math.Abs(labels-elapsed) > 1e-6 {
		t.Errorf("GPST labels advanced %.6f s while %.6f s elapsed; on this scale they are the same thing",
			labels, elapsed)
	}

	// 7201, not 7200: a second was inserted inside the interval.
	if math.Abs(elapsed-7201) > 1e-6 {
		t.Errorf("elapsed = %.6f s, want 7201", elapsed)
	}

	// The contrast. Same two instants, UTC labels, and the gap is 7200 --
	// which is why subtracting UTC labels directly is a bug and subtracting
	// GPST labels is not.
	if utcLabels := labelSeconds(utcAfter, utcBefore); math.Abs(utcLabels-7200) > 1e-6 {
		t.Errorf("UTC labels advanced %.6f s, want 7200 -- the leap second is missing from the label", utcLabels)
	}
}

// TestGPSTStringIsItsName keeps the scale printable, which matters because a
// mislabelled instant is most often noticed by reading one.
func TestGPSTStringIsItsName(t *testing.T) {
	t.Parallel()

	if got := time.GPST.String(); got != "GPST" {
		t.Errorf("GPST.String() = %q", got)
	}

	if got := epoch().GPST().Scale().String(); got != "GPST" {
		t.Errorf("converted instant reports scale %q", got)
	}
}
