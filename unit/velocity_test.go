package unit_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

// TestVelocityRoundTripsThroughEveryUnit is [TestLengthRoundTripsThroughEveryUnit]
// for the other dimension.
func TestVelocityRoundTripsThroughEveryUnit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		make func(float64) unit.Velocity
		read func(unit.Velocity) float64
	}{
		{"metres per second", unit.MetresPerSec, unit.Velocity.MetresPerSec},
		{"kilometres per second", unit.KmPerSec, unit.Velocity.KmPerSec},
		{"astronomical units per day", unit.AUPerDay, unit.Velocity.AUPerDay},
	} {
		for _, v := range []float64{0, 1, -110.6, 29.78, 18.04, 247.3, 1e-6} {
			got := tc.read(tc.make(v))

			if v == 0 {
				if got != 0 {
					t.Errorf("%s: zero round-tripped to %g", tc.name, got)
				}

				continue
			}

			if rel := math.Abs(got-v) / math.Abs(v); rel > 1e-15 {
				t.Errorf("%s: %g round-tripped to %g (relative %.3g)", tc.name, v, got, rel)
			}
		}
	}
}

// TestVelocityConversionsAgreeWithTheKnownValues checks against numbers from
// outside this package, since a round trip passes on any self-consistent
// scale factor including a wrong one.
func TestVelocityConversionsAgreeWithTheKnownValues(t *testing.T) {
	t.Parallel()

	// One au per day in km/s: 149597870.7 km over 86400 s. Written as the
	// division rather than as a decimal, because a recalled decimal is how the
	// wrong value for the per-year form got into coord in the first place.
	const auPerDayInKmPerSec = 149597870.7 / 86400

	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"1 km/s in m/s", unit.KmPerSec(1).MetresPerSec(), 1000},
		{"1 au/day in km/s", unit.AUPerDay(1).KmPerSec(), auPerDayInKmPerSec},

		// The classical identity the coord tests use: one au per Julian year,
		// which is 149597870.7 km over 365.25 x 86400 s. The decimal is
		// 4.740470463533, not the 4.740470446 widely repeated and — until this
		// test — used by coord itself.
		{"1 au/year in km/s", unit.AUPerDay(1.0 / 365.25).KmPerSec(), 149597870.7 / (365.25 * 86400)},
	} {
		if rel := math.Abs(tc.got-tc.want) / math.Abs(tc.want); rel > 1e-9 {
			t.Errorf("%s = %.17g, want %.17g (relative %.3g)", tc.name, tc.got, tc.want, rel)
		}
	}
}

// TestVelocityAndQuantityCannotDisagree is the same no-drift contract
// [TestLengthAndQuantityCannotDisagree] asserts, and matters more here: the
// velocity units are *derived* from the length and time tables with Div rather
// than written down, so a mistake in that derivation would show up only as two
// representations quietly disagreeing.
func TestVelocityAndQuantityCannotDisagree(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		v    unit.Velocity
		u    unit.Unit
		read func(unit.Velocity) float64
	}{
		{"metres per second", unit.MetresPerSec(343), unit.MeterPerSecond, unit.Velocity.MetresPerSec},
		{"kilometres per second", unit.KmPerSec(-110.6), unit.KilometerPerSecond, unit.Velocity.KmPerSec},
		{"au per day", unit.AUPerDay(0.0172), unit.AstronomicalUnitPerDay, unit.Velocity.AUPerDay},
	} {
		q, err := tc.v.Quantity().In(tc.u)
		if err != nil {
			t.Errorf("%s: converting to %v: %v", tc.name, tc.u, err)
			continue
		}

		// Agreement to a few ulp rather than bit-for-bit, and the reason is
		// worth stating: the accessor divides by the scale factor while
		// Quantity multiplies by a precomputed reciprocal, and x/s is not
		// x*(1/s) in floating point. Measured, km/s differs in the last two
		// ulp. The accessor is the more accurate of the two, which is why it
		// divides rather than being made to match.
		//
		// What cannot drift is the definition: both read the same ScaleFactor,
		// so neither can come to hold a different idea of how fast 1 km/s is.
		if direct := tc.read(tc.v); math.Abs(q.Value-direct) > 1e-12*math.Abs(direct) {
			t.Errorf("%s: Quantity says %.17g and the accessor says %.17g — further "+
				"apart than the last few ulp, so the two representations have drifted",
				tc.name, q.Value, direct)
		}
	}
}

// TestVelocityUnitsCarryTheRightDimension checks the derivation itself.
//
// MeterPerSecond and its siblings are built with Unit.Div rather than declared,
// so this asserts the dimension that came out is the one the [unit.DimVelocity]
// table names — L¹T⁻¹. A Div that subtracted the wrong exponent would still
// produce a usable-looking Unit whose conversions were all wrong together.
func TestVelocityUnitsCarryTheRightDimension(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		u    unit.Unit
	}{
		{"MeterPerSecond", unit.MeterPerSecond},
		{"KilometerPerSecond", unit.KilometerPerSecond},
		{"AstronomicalUnitPerDay", unit.AstronomicalUnitPerDay},
	} {
		if tc.u.Dimension != unit.DimVelocity {
			t.Errorf("%s has dimension %+v, want DimVelocity %+v",
				tc.name, tc.u.Dimension, unit.DimVelocity)
		}

		// And it must not be compatible with a plain length, which is the
		// confusion a wrong exponent would produce.
		if tc.u.Compatible(unit.Meter) {
			t.Errorf("%s is compatible with Meter, so its dimension is wrong", tc.name)
		}
	}
}

// TestVelocityFromRejectsWhatIsNotAVelocity covers the dimensioned direction.
func TestVelocityFromRejectsWhatIsNotAVelocity(t *testing.T) {
	t.Parallel()

	v, err := unit.VelocityFrom(unit.New(-110.6, unit.KilometerPerSecond))
	if err != nil {
		t.Fatalf("a velocity was rejected: %v", err)
	}

	if rel := math.Abs(v.KmPerSec()+110.6) / 110.6; rel > 1e-15 {
		t.Errorf("-110.6 km/s came back as %v", v)
	}

	if _, err := unit.VelocityFrom(unit.New(1.5, unit.AstronomicalUnit)); err == nil {
		t.Error("a length was accepted as a velocity")
	}
}

// TestVelocityStringPicksTheUnitAReaderExpects pins the one threshold.
func TestVelocityStringPicksTheUnitAReaderExpects(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		v    unit.Velocity
		want string
	}{
		{"a perspective term", unit.MetresPerSec(7.5), "7.5 m/s"},
		{"the solar apex", unit.KmPerSec(18.04), "18.04 km/s"},
		{"Barnard's Star, approaching", unit.KmPerSec(-110.6), "-110.6 km/s"},
		{"the Sun around the Galaxy", unit.KmPerSec(247.3), "247.3 km/s"},
		{"zero", unit.MetresPerSec(0), "0 m/s"},
	} {
		if got := tc.v.String(); got != tc.want {
			t.Errorf("%s: String() = %q, want %q", tc.name, got, tc.want)
		}
	}
}
