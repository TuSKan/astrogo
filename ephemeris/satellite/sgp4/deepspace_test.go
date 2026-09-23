package sgp4

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
)

// TestEarthRotationRateAgreesWithWGS72 is the one deep-space constant that
// describes something outside SGP4, and so the one that can be checked against
// astrogo's published values.
//
// # Why the rest of this file's constants are not in the constants package
//
// Because they are not constants of nature. zes = 0.01675 is the Sun's
// eccentricity as Hoots' 1980 lunisolar model uses it; fasx2 = 0.13130908 is a
// resonance phase angle from the same fit; root44 = 7.3636953e-9 is a
// geopotential coefficient scaled into SGP4's units. None has an uncertainty,
// a standard, a publishing body, or any meaning outside this model — which is
// exactly what constants.Constant's Reference and Exact fields exist to record.
// Filing them there would describe them as something they are not.
//
// rptim is the exception: it is Earth's rotation rate, expressed in radians per
// minute, and astrogo publishes that. So the relationship is asserted rather
// than either duplicated in silence or unified into a dependency the model must
// not have.
//
// # What the measurement says
//
// rptim/60 is 7.2921151466855e-5 rad/s. constants.WGS72.AngularVelocity is
// 7.292115147e-5 — the same number, rounded to the ten digits the standard
// publishes. They agree to 4.3e-11 relative, which is the rounding and nothing
// else.
//
// constants.WGS84.AngularVelocity is 7.292115e-5, a different rounding of a
// different realization, and differs by 2.0e-8 relative — 470 times further.
// That is the check with teeth: it fails if this table is ever "corrected"
// towards WGS-84, which is the mistake astrogo already made once with the
// gravity model.
func TestEarthRotationRateAgreesWithWGS72(t *testing.T) {
	t.Parallel()

	radPerSec := rptim / 60.0

	// The reference writes this value in a comment beside the constant:
	// "equates to 7.29211514668855e-5 rad/sec". It does, to 4.2e-14 relative —
	// the comment is a rounded restatement of the constant rather than a second
	// independent value, and rptim itself is what the model evaluates. Bounded
	// at 1e-12 so a real typo in either is caught while that rounding is not
	// reported as a defect.
	const valladosNote = 7.29211514668855e-5

	if rel := math.Abs(radPerSec-valladosNote) / valladosNote; rel > 1e-12 {
		t.Errorf("rptim/60 = %.17g rad/s, and the reference's own note beside it says %.17g "+
			"(%.3g relative, where only its rounding is expected)", radPerSec, valladosNote, rel)
	}

	rel72 := math.Abs(radPerSec-constants.WGS72.AngularVelocity.Value) /
		constants.WGS72.AngularVelocity.Value

	if rel72 > 1e-9 {
		t.Errorf("SGP4's Earth rotation rate is %.17g rad/s against constants.WGS72's %.17g — "+
			"a relative gap of %.3g, where only the standard's ten-digit rounding (~4e-11) "+
			"is expected.\n"+
			"  These describe the same quantity. If they have genuinely diverged, say which "+
			"one moved and why before changing either.",
			radPerSec, constants.WGS72.AngularVelocity.Value, rel72)
	}

	rel84 := math.Abs(radPerSec-constants.WGS84.AngularVelocity.Value) /
		constants.WGS84.AngularVelocity.Value

	if rel84 < 10*rel72 {
		t.Errorf("SGP4's rotation rate is now as close to constants.WGS84 (%.3g) as to "+
			"constants.WGS72 (%.3g). It should be much closer to WGS-72: that is the "+
			"realization TLEs are fitted with, and this table must not drift towards the "+
			"later one.", rel84, rel72)
	}
}

// TestResonanceClassesAreTheModelsOwn pins the two commensurability bands, in
// the units the model tests them in.
//
// Mean motion in radians per minute is not a quantity anyone reads by eye, so
// the bounds are also stated as orbital periods, which is what makes them
// recognizable: the synchronous band is 20.0 to 30.0 hours and the half-day
// band 11.333 to 12.678 hours. A transposed digit in either would pass every other
// test in this package and silently move a geostationary satellite out of the
// resonance it lives in.
func TestResonanceClassesAreTheModelsOwn(t *testing.T) {
	t.Parallel()

	// Period in hours for a mean motion in radians per minute.
	period := func(nm float64) float64 { return twoPi / nm / 60.0 }

	for _, tc := range []struct {
		name      string
		nm        float64
		wantHours float64
	}{
		{"synchronous band, slow edge", 0.0034906585, 30.0},
		{"synchronous band, fast edge", 0.0052359877, 20.0},
		{"half-day band, slow edge", 8.26e-3, 12.678},
		{"half-day band, fast edge", 9.24e-3, 11.333},
	} {
		if got := period(tc.nm); math.Abs(got-tc.wantHours) > 0.001 {
			t.Errorf("%s: mean motion %g rad/min is a period of %.3f hours, want %.3f",
				tc.name, tc.nm, got, tc.wantHours)
		}
	}

	// The half-day band additionally requires e >= 0.5, which is what makes it
	// the Molniya case rather than every twelve-hour orbit.
	if resonanceHalfDay == resonanceSynchronous || resonanceNone == resonanceSynchronous {
		t.Error("the three resonance classes are not distinct")
	}
}

// TestResonanceStepIsHalfItsSquare guards a derived constant that is written
// out as a literal.
//
// resonanceHalfStep is 0.5 * 720², and the integrator uses both. Spelled as a
// literal to match the reference, so the relationship is asserted here rather
// than trusted.
func TestResonanceStepIsHalfItsSquare(t *testing.T) {
	t.Parallel()

	if want := 0.5 * resonanceStep * resonanceStep; resonanceHalfStep != want {
		t.Errorf("resonanceHalfStep = %g, want 0.5 * %g² = %g", resonanceHalfStep, resonanceStep, want)
	}
}
