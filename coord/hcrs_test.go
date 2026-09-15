package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// solarRadiiPerAU converts an AU to solar radii, which is the unit the Sun's
// barycentric wander is worth reading in: the interesting fact about it is
// that it is comparable to the size of the Sun.
const solarRadiiPerAU = 215.032

// TestTheSunOrbitsTheBarycentreByAboutASolarRadius is the physical check, and
// it is the one that would catch the whole quantity being wrong rather than
// slightly off.
//
// The barycentre is the mass-weighted centre of the solar system, and Jupiter
// alone is a thousandth of the Sun's mass at five AU — so the Sun's excursion
// is of order the solar radius, not of order the Earth's orbit and not of order
// zero. Sampling a Jupiter period has to show both: a distance that is
// sometimes under half a solar radius and sometimes near two, because the giant
// planets move in and out of alignment.
func TestTheSunOrbitsTheBarycentreByAboutASolarRadius(t *testing.T) {
	t.Parallel()

	var minR, maxR = math.Inf(1), 0.0

	// A Jupiter period, sampled finely enough to catch the extremes.
	for year := 2020; year <= 2032; year++ {
		for month := 1; month <= 12; month += 3 {
			ep := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.LocationUTC)

			v, err := coord.SunBarycentric(ep)
			if err != nil {
				t.Fatalf("SunBarycentric(%v): %v", ep, err)
			}

			minR = math.Min(minR, v.Norm())
			maxR = math.Max(maxR, v.Norm())
		}
	}

	t.Logf("over 2020-2032 the Sun's barycentric distance ran %.6f to %.6f AU (%.2f to %.2f solar radii)",
		minR, maxR, minR*solarRadiiPerAU, maxR*solarRadiiPerAU)

	// Nothing here is a fitted tolerance: these are the bounds the two-body
	// arithmetic gives for the Sun and the giant planets.
	if maxR > 0.01 {
		t.Errorf("the Sun reached %.6f AU from the barycentre, more than the ~0.009 AU "+
			"all the giant planets aligned can produce", maxR)
	}

	if maxR < 0.006 {
		t.Errorf("the Sun never got further than %.6f AU from the barycentre; Jupiter "+
			"alone puts it past 0.005 AU, so this is too small to be the real quantity",
			maxR)
	}

	if minR > 0.003 {
		t.Errorf("the Sun never came closer than %.6f AU to the barycentre over a full "+
			"Jupiter period, which it should — the giant planets do fall out of "+
			"alignment", minR)
	}
}

// TestTheSunsBarycentricPositionAndVelocityAgree differentiates the position
// and checks it against the velocity, which is the other half of the same SOFA
// call and was already being used elsewhere in this package.
//
// This is what catches an extraction mistake rather than a physics mistake:
// taking pvh from pvb instead of the other way round, or reading the velocity
// row where the position row was wanted, all produce a vector of a plausible
// size that does not differentiate into the right thing.
func TestTheSunsBarycentricPositionAndVelocityAgree(t *testing.T) {
	t.Parallel()

	const (
		epochYear = 2026
		// Half a day either side. Small enough that the curvature of a
		// twelve-year orbit is negligible, large enough that the difference is
		// far above float noise on a 0.006 AU vector.
		hDays = 0.5
	)

	mid := time.Date(epochYear, 6, 1, 12, 0, 0, 0, time.LocationUTC)

	before, err := coord.SunBarycentric(mid.AddDays(-hDays))
	if err != nil {
		t.Fatalf("SunBarycentric: %v", err)
	}

	after, err := coord.SunBarycentric(mid.AddDays(hDays))
	if err != nil {
		t.Fatalf("SunBarycentric: %v", err)
	}

	// The numerical derivative, in AU/day.
	got := after.Sub(before).DivScalar(2 * hDays)

	// The analytic velocity, from the other half of Epv00's answer.
	d1, d2 := mid.TDB().JDParts()

	pvh, pvb, status := gofaext.Epv00(d1, d2)
	if status < 0 {
		t.Fatalf("Epv00 status %d", status)
	}

	want := vector.V3(
		pvb[1][0]-pvh[1][0],
		pvb[1][1]-pvh[1][1],
		pvb[1][2]-pvh[1][2],
	)

	t.Logf("differentiated %v AU/day", got)
	t.Logf("analytic       %v AU/day", want)

	// A central difference over one day on a smooth twelve-year orbit is good
	// to about its own second-order term, which is parts in 1e8 of the speed.
	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"vx", got.X, want.X},
		{"vy", got.Y, want.Y},
		{"vz", got.Z, want.Z},
	} {
		testutil.AssertNear(t, c.name+" (AU/day)", c.got, c.want, 1e-11)
	}
}

// TestTheTranslationGoesTheRightWay pins the direction, which is the error this
// pair of functions exists to prevent.
//
// Subtracting when a caller should have added does not produce a small error.
// It produces one of twice the Sun's offset, in a position that still looks
// entirely reasonable.
func TestTheTranslationGoesTheRightWay(t *testing.T) {
	t.Parallel()

	ep := time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC)

	sun, err := coord.SunBarycentric(ep)
	if err != nil {
		t.Fatalf("SunBarycentric: %v", err)
	}

	// An arbitrary barycentric position, roughly where Mars might be.
	bary := vector.V3(0.5053, -1.7138, -0.9949)

	helio, err := coord.BarycentricToHeliocentric(bary, ep)
	if err != nil {
		t.Fatalf("BarycentricToHeliocentric: %v", err)
	}

	// Going to the Sun's frame moves the origin toward the Sun, so the
	// difference is exactly the Sun's own barycentric position.
	moved := bary.Sub(helio)

	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"x", moved.X, sun.X},
		{"y", moved.Y, sun.Y},
		{"z", moved.Z, sun.Z},
	} {
		testutil.AssertNear(t, "offset "+c.name+" (AU)", c.got, c.want, 1e-15)
	}

	back, err := coord.HeliocentricToBarycentric(helio, ep)
	if err != nil {
		t.Fatalf("HeliocentricToBarycentric: %v", err)
	}

	if diff := back.Sub(bary).Norm(); diff > 1e-15 {
		t.Errorf("round trip moved the position by %g AU", diff)
	}
}

// TestTheOffsetMattersForAPlanetAndNotForAStar states the size of the thing in
// the units a caller decides with, since "which origin" is a question worth
// ignoring for one kind of target and not the other.
func TestTheOffsetMattersForAPlanetAndNotForAStar(t *testing.T) {
	t.Parallel()

	ep := time.Date(2023, 1, 1, 0, 0, 0, 0, time.LocationUTC) // near the Sun's furthest excursion

	sun, err := coord.SunBarycentric(ep)
	if err != nil {
		t.Fatalf("SunBarycentric: %v", err)
	}

	const auPerParsec = 206264.806

	for _, tc := range []struct {
		name       string
		distanceAU float64
		wantAbove  float64 // arcseconds
		wantBelow  float64
	}{
		{"a body at 1 AU", 1, 1000, 3000},
		{"a body at 5 AU, Jupiter's distance", 5, 200, 600},
		{"a star at 1 parsec", auPerParsec, 1e-3, 1e-2},
		{"a star at 100 parsecs", 100 * auPerParsec, 1e-5, 1e-4},
	} {
		// The largest angle the origin shift can subtend at that distance.
		arcsec := math.Atan2(sun.Norm(), tc.distanceAU) * 180 / math.Pi * 3600

		t.Logf("%-34s origin shift subtends %11.5f arcsec", tc.name, arcsec)

		if arcsec < tc.wantAbove || arcsec > tc.wantBelow {
			t.Errorf("%s: origin shift subtends %g arcsec, want between %g and %g",
				tc.name, arcsec, tc.wantAbove, tc.wantBelow)
		}
	}
}

func BenchmarkSunBarycentric(b *testing.B) {
	ep := time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC)

	b.ReportAllocs()

	for range b.N {
		_, _ = coord.SunBarycentric(ep)
	}
}
