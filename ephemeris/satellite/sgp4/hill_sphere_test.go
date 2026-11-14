package sgp4

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/vector"
)

// The ISS on 2026-04-19, CelesTrak's GP record as catalog/norad's tests carry
// it, written as a TLE by norad.GP.ToTLE. Vallado's 2008 ISS set does not
// show the defect: run back to JD 0, its mean eccentricity goes negative and
// ErrEccentricity fires first. This one's drag terms carry the orbit outward
// instead, which is what #1131 reported.
const (
	issLine1 = "1 25544U 98067A   26109.48996335  .00010082  00000+0  19194-3 0  9992"
	issLine2 = "2 25544  51.6329 230.6068 0006631 325.6576  34.3983 15.48833250562656"
)

// TestEvaluationBeyondTheHillSphereIsRefused: run thousands of years from
// its epoch, an element set's drag terms carry the orbit out without bound,
// and none of the model's own checks fire. The ISS evaluated at JD 0 came
// back 5.9e23 km away with no error (#1131). A position beyond the Earth's
// Hill sphere is outside what an Earth-orbit theory represents, and is now
// refused with both vectors zero. Days from epoch, nothing changes.
func TestEvaluationBeyondTheHillSphereIsRefused(t *testing.T) {
	t.Parallel()

	el, err := ParseTLE(issLine1, issLine2)
	if err != nil {
		t.Fatal(err)
	}

	p, err := New(el)
	if err != nil {
		t.Fatal(err)
	}

	// JD 0 is about 3.5e9 minutes before this epoch, 2026 day 109.48996335,
	// JD 2461149.98996335.
	const minutesFromJD0 = -2461149.98996335 * 1440

	pos, vel, err := p.At(minutesFromJD0)
	if !errors.Is(err, ErrBeyondHillSphere) {
		t.Fatalf("At(%g min) = %v km, err %v; want ErrBeyondHillSphere", minutesFromJD0, pos.Norm(), err)
	}

	if pos != (vector.Vec3{}) || vel != (vector.Vec3{}) {
		t.Errorf("refused state is not zero: pos %v, vel %v", pos, vel)
	}

	for _, days := range []float64{-3, 0, 3} {
		pos, _, err := p.At(days * 1440)
		if err != nil {
			t.Errorf("At(%g days) failed: %v", days, err)
		}

		if r := pos.Norm(); r < 6378 || r > 7000 {
			t.Errorf("At(%g days): |r| = %g km, not a low Earth orbit", days, r)
		}
	}
}

// TestEarthHillRadiusIsItsFormula holds the constant to the formula and the
// DE440 masses its doc comment states.
func TestEarthHillRadiusIsItsFormula(t *testing.T) {
	t.Parallel()

	const (
		au      = 149597870.7      // km
		gmEarth = 398600.435436    // km³/s², DE440
		gmSun   = 1.32712440041e11 // km³/s², DE440
	)

	want := au * math.Cbrt(gmEarth/(3*gmSun))
	if math.Abs(earthHillRadiusKM-want) > 5 {
		t.Errorf("earthHillRadiusKM = %g km, the formula gives %g km", float64(earthHillRadiusKM), want)
	}
}
