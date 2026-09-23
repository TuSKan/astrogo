package sgp4

import (
	"math"
	"testing"
)

// TestAFSPCOriginIsTheSameSiderealTime ties the package's two sidereal-time
// expressions together at the one point where they must agree.
//
// [gmst82] and [gstoAFSPC] are different definitions, not two approximations of
// one, and the package says so. But gstoAFSPC counts from a fixed value at a
// fixed origin, and that value turns out to be gmst82 evaluated at that origin:
// 1970 January 0.0, which is 1969-12-31 0h UT, JD 2440586.5.
//
// That is worth asserting rather than noting, for two reasons. It makes the
// magic constant thgr70 checkable — a transposed digit in a seventeen-digit
// literal is otherwise invisible — and it explains the measured behavior of
// [ModeAFSPC]: the two modes agree exactly at 1970 and drift apart only as each
// accumulates from there, which is why a 2024 epoch shows them 1.6e-10 km apart
// rather than kilometers.
func TestAFSPCOriginIsTheSameSiderealTime(t *testing.T) {
	t.Parallel()

	// 1970 January 0.0 in the AFSPC expression's own argument: days from 0
	// January 1950, which is what SGP4 carries an epoch in.
	const origin1950 = 7305.0

	fromGMST := gmst82(origin1950 + jd1950)

	if rel := math.Abs(fromGMST-1.7321343856509374) / 1.7321343856509374; rel > 1e-9 {
		t.Errorf("gmst82 at the AFSPC origin is %.17g rad and thgr70 is %.17g (%.3g relative).\n"+
			"  These are different definitions of sidereal time, but they are anchored to the "+
			"same instant, and a gap here means one of the two expressions has a typo.",
			fromGMST, 1.7321343856509374, rel)
	}

	// And the whole expression, not just its constant: gstoAFSPC evaluated at
	// its own origin must return that same angle, which additionally checks the
	// day-counting and the 1e-8 boundary nudge around ts70 = 0.
	if rel := math.Abs(gstoAFSPC(origin1950)-fromGMST) / fromGMST; rel > 1e-9 {
		t.Errorf("gstoAFSPC at its own origin is %.17g rad, gmst82 says %.17g (%.3g relative)",
			gstoAFSPC(origin1950), fromGMST, rel)
	}
}

// TestTheTwoSiderealTimesAreNearlyTheSameNumber bounds the gap between them,
// and the bound is the finding.
//
// They are presented as different definitions, and they are — different
// expressions, different arguments, different origins. But measured across the
// ninety years from the 1970 origin they never differ by more than 3.2e-10 rad,
// which is 0.0001 arcseconds. An arcminute was the first guess written here;
// it was six orders too loose to detect anything.
//
// So the honest description of ModeAFSPC is not "a materially different
// sidereal time". It is a different route to the same angle, and the reason to
// keep both is exact reproduction of output generated one way or the other —
// which is what the 1.6e-10 km position difference in
// TestAFSPCModeIsADifferentAnswer reflects, and why that test bounds the
// difference from above as well as requiring it to be non-zero.
//
// The bound here is 1e-8 rad: thirty times the measured drift, tight enough
// that a real change to either expression fails it.
func TestTheTwoSiderealTimesAreNearlyTheSameNumber(t *testing.T) {
	t.Parallel()

	const (
		origin1950  = 7305.0
		maxDriftRad = 1e-8
	)

	var worst float64

	// Every year from 1970 to 2060.
	for year := 0.0; year <= 90.0; year++ {
		days := origin1950 + year*365.25

		a := gstoAFSPC(days)
		g := gmst82(days + jd1950)

		// Both are reduced to [0, 2pi), so compare on the circle.
		d := math.Abs(a - g)
		if d > math.Pi {
			d = 2*math.Pi - d
		}

		if d > worst {
			worst = d
		}
	}

	if worst > maxDriftRad {
		t.Errorf("the two sidereal-time expressions differ by up to %.4g rad (%.5f arcsec) over "+
			"1970-2060, against a %.0e rad bound. They track each other to a ten-thousandth "+
			"of an arcsecond; a gap this size means one of them has changed.",
			worst, worst*180.0/math.Pi*3600.0, maxDriftRad)
	}

	t.Logf("gstoAFSPC and gmst82 stay within %.4g rad (%.5f arcsec) over 1970-2060",
		worst, worst*180.0/math.Pi*3600.0)
}

// TestGMSTStaysOnTheCircle is the property both expressions promise: an angle in
// [0, 2pi), for any epoch, including the negative-modulo region a pre-1950 date
// reaches.
func TestGMSTStaysOnTheCircle(t *testing.T) {
	t.Parallel()

	for _, days := range []float64{-40000, -7305, 0, 7305, 27000, 40000, 100000} {
		for name, got := range map[string]float64{
			"gmst82":    gmst82(days + jd1950),
			"gstoAFSPC": gstoAFSPC(days),
		} {
			if math.IsNaN(got) || got < 0 || got >= 2*math.Pi {
				t.Errorf("%s at %g days from 1950 returned %v, outside [0, 2pi)", name, days, got)
			}
		}
	}
}
