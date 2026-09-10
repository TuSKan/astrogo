package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestGeometricLeadsTopocentricByTheAberration measures the one gap between
// [coord.Reduction]'s two pairs of fields.
//
// # Why it needs measuring
//
// Reduction hands back two positions and two directions, and it is natural to
// read them as four refinements of one vector — to assume that rotating
// Topocentric into the horizon frame reproduces Geometric. It does not, and has
// not since #261 put diurnal aberration on that path.
//
// The difference is up to 0.32 arcseconds. That is small enough to look like
// rounding and large enough to matter for anything this library is for, and it
// is exactly the size of an error somebody would chase for an afternoon. So it
// is asserted here rather than described: the gap is the aberration, the whole
// aberration, and it scales with latitude the way the observer's own speed
// about the Earth's axis does.
//
// # What each assertion rules out
//
// That the gap is the aberration and not something else: it is compared
// against |v|/c·sin(theta) from the WGS84 ellipsoid and the sidereal rotation
// rate, per direction, not against a remembered number.
//
// That Geometric is the one carrying it, rather than the hand rotation being
// wrong: Observed with refraction switched off must equal Geometric exactly, so
// whatever Geometric has, the documented pipeline has too.
func TestGeometricLeadsTopocentricByTheAberration(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	t.Cleanup(time.ResetEOP)

	const (
		dut1Seconds = 0.1237
		xpArcsec    = 0.1834
		ypArcsec    = 0.4021

		// One per cent of the constant, the same bound and the same reasoning
		// as coord/diurnalaberration_test.go: it covers the second-order term
		// the sine law drops and nothing else.
		tolerance = 0.01
	)

	time.RegisterModel(fixedEOP{
		dut1: dut1Seconds,
		xp:   angle.Arcsec(xpArcsec).Radians(),
		yp:   angle.Arcsec(ypArcsec).Radians(),
	})

	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	epoch := diurnalEpoch
	tt1, tt2 := epoch.TT().JDParts()
	rc2i := gofaext.C2i06a(tt1, tt2)

	// The observer's rotation velocity is due east, so this is the direction
	// the aberration displaces towards.
	east := coord.NewAltAz(angle.Zero(), angle.Deg(90))

	for _, s := range diurnalSites {
		site, err := coord.NewGeodetic(angle.Deg(s.lon), angle.Deg(s.lat), s.heightM)
		if err != nil {
			t.Fatalf("%s: NewGeodetic: %v", s.name, err)
		}

		ctx := coord.NewContext(epoch, site, atm)
		reducer := coord.NewReducer(site, epoch, atm)
		diurabArcsec := expectedDiurnalArcsec(site.Lat().Radians(), site.Height())

		var worst float64

		for _, d := range diurnalDirections {
			ri, di := angle.Hour(d.raHr).Radians(), angle.Deg(d.decDeg).Radians()

			// Far enough away that the parallax step changes no direction, so
			// what separates the two answers is only the aberration.
			far := icrsFromCIRS(rc2i, ri, di).MulScalar(1e9)

			red := reducer.Reduce(far)

			// In a vacuum the refracted answer is the unrefracted one. If this
			// ever parts company, the gap measured below stops being a
			// statement about the documented pipeline.
			if sep := horizonSeparationArcsec(red.Observed, red.Geometric); sep > 1e-9 {
				t.Fatalf("%s at RA %gh dec %g: Observed and Geometric differ by %.4g arcsec "+
					"with refraction switched off.\n"+
					"  The pair exists to show what the atmosphere did; in a vacuum it did "+
					"nothing.", s.name, d.raHr, d.decDeg, sep)
			}

			// The direction of Topocentric, rotated into the horizon frame by
			// hand and stopped there — what a caller gets from the vector.
			plain := rotateOnly(t, ctx, far)

			gap := horizonSeparationArcsec(red.Geometric, plain)
			if gap > worst {
				worst = gap
			}

			theta := coord.Separation(
				coord.NewICRS(plain.Az(), plain.Alt()),
				coord.NewICRS(east.Az(), east.Alt()),
			).Radians()

			want := diurabArcsec * math.Sin(theta)

			if diff := math.Abs(gap - want); diff > tolerance*diurabArcsec {
				t.Errorf("%s at RA %gh dec %g: Geometric leads the direction of Topocentric "+
					"by %.4f arcsec, and |v|/c·sin(theta) with theta=%.1f° says %.4f.\n"+
					"  The gap between Reduction's positions and its directions is diurnal "+
					"aberration. A gap of another size means something else is in there, or "+
					"the aberration has moved.",
					s.name, d.raHr, d.decDeg, gap, theta*180/math.Pi, want)
			}
		}

		// The gap has to be real somewhere, or the test above passes on two
		// identical answers and says nothing.
		if worst < 0.5*diurabArcsec {
			t.Errorf("%s: the largest gap over the sweep is %.4f arcsec against a constant of "+
				"%.4f.\n"+
				"  Every sampled direction sits near the velocity, where the aberration "+
				"projects to nothing, so this site proves neither presence nor absence.",
				s.name, worst, diurabArcsec)
		}

		t.Logf("%-18s lat %+7.3f  |v|/c = %.4f arcsec, largest Geometric-minus-Topocentric %.4f",
			s.name, s.lat, diurabArcsec, worst)
	}
}

// TestReductionTopocentricIsParallaxOnly pins the other half of the contract:
// that Topocentric is the input less the observer and nothing more.
//
// It is the field a caller reaches for when they want a vector rather than a
// direction — to difference two bodies, to compute a range, to feed something
// of their own. Anything added to it silently would be wrong for all three, and
// the aberration in particular must not reach it, since aberration is a
// property of light rather than of where the body is.
func TestReductionTopocentricIsParallaxOnly(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	t.Cleanup(time.ResetEOP)

	time.RegisterModel(fixedEOP{
		dut1: 0.1237,
		xp:   angle.Arcsec(0.1834).Radians(),
		yp:   angle.Arcsec(0.4021).Radians(),
	})

	atm := atmosphere.StandardRefraction
	atm.Model = atmosphere.RefractionNone{}

	site, err := coord.NewGeodetic(angle.Deg(-70.4042), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	epoch := diurnalEpoch
	ctx := coord.NewContext(epoch, site, atm)
	red := coord.NewReducer(site, epoch, atm).Reduce(vector.V3(0.5, 0.3, 0.1))

	want := vector.V3(0.5, 0.3, 0.1).Sub(ctx.ObsVec())

	if red.Topocentric != want {
		t.Errorf("Topocentric = %+v, want %+v exactly.\n"+
			"  It is Geocentric less the observer's own position and nothing else. The "+
			"aberration belongs to Geometric, which is a direction; a position does not "+
			"carry it.", red.Topocentric, want)
	}

	if red.Geocentric != vector.V3(0.5, 0.3, 0.1) {
		t.Errorf("Geocentric = %+v, want the input %+v unchanged.",
			red.Geocentric, vector.V3(0.5, 0.3, 0.1))
	}
}
