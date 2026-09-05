package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

// TestLookAngleUsesTheContextItIsGiven pins that the Context parameter is the
// one used, rather than three fields copied out of it and a fresh Context built
// behind the caller's back.
//
// # Why a derived Context is the fixture
//
// A Context built by Context.AtTime holds precession-nutation and aberration
// from its *base* epoch, deliberately, because recomputing them is the 145 us
// this exists to avoid. So it differs slightly from what NewContext would
// produce at the same instant — measured here at 0.0005 arcsec after an hour
// and 0.03 arcsec after twelve.
//
// That difference is the probe. The old implementation called
// coord.NewReducer(ctx.Site(), ctx.Time(), ctx.Refraction()), which rebuilt a
// Context from scratch and so returned the *direct* answer no matter which
// Context it was handed. Handing it a derived one and demanding the derived
// answer is what tells the two implementations apart.
//
// The assertion is exact equality rather than a tolerance, because there is
// nothing approximate about it: the value must come from this Context.
func TestLookAngleUsesTheContextItIsGiven(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	site, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	base := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)

	for _, dt := range []time.Duration{time.Hour, 6 * time.Hour, 12 * time.Hour} {
		at := base.Add(dt)
		derived := coord.NewContext(base, site, defaultAtm).AtTime(at)

		st, err := sat.State(0, at)
		if err != nil {
			t.Fatalf("State: %v", err)
		}

		want := derived.GeocentricToObserved(st.Pos)

		got, err := LookAngle(sat, 0, derived)
		if err != nil {
			t.Fatalf("LookAngle: %v", err)
		}

		if got.Alt() != want.Alt() || got.Az() != want.Az() {
			t.Errorf("dt=%v: LookAngle returned alt %v az %v, but the Context it "+
				"was given gives alt %v az %v (%.4f and %.4f arcsec apart).\n"+
				"  The Context is being rebuilt rather than used.",
				dt, got.Alt(), got.Az(), want.Alt(), want.Az(),
				(got.Alt() - want.Alt()).Arcseconds(),
				(got.Az() - want.Az()).Arcseconds())
		}

		// The range must come from the same Context's observer vector.
		const kmPerAU = 149597870.7

		wantDist := st.Pos.Sub(derived.ObsVec()).Norm() * kmPerAU
		if math.Abs(got.Dist()-wantDist) > 1e-9 {
			t.Errorf("dt=%v: range %.6f km, want %.6f km from this Context's "+
				"observer vector", dt, got.Dist(), wantDist)
		}

		// And it is a plausible ISS range, so the test is not comparing two
		// identically-wrong numbers.
		if got.Dist() < 300 || got.Dist() > 45000 {
			t.Errorf("dt=%v: range %.1f km is not a plausible ISS topocentric "+
				"distance", dt, got.Dist())
		}
	}
}

// TestLookAngleMatchesTheReducerItReplaced guards the other direction: the
// change was meant to remove a redundant computation, not alter the result.
//
// Given a Context built the ordinary way, coord.Reducer's pipeline and this
// function must agree exactly, since the Reducer's own Context is then
// constructed from the same three values.
func TestLookAngleMatchesTheReducerItReplaced(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	site, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	const kmPerAU = 149597870.7

	for _, hour := range []int{0, 3, 9, 15, 21} {
		at := time.Date(2026, time.April, 20, hour, 0, 0, 0, time.LocationUTC)
		ctx := coord.NewContext(at, site, defaultAtm)

		st, err := sat.State(0, at)
		if err != nil {
			t.Fatalf("State: %v", err)
		}

		reduction := coord.NewReducer(site, at, defaultAtm).Reduce(st.Pos)
		reduction.Observed.SetDist(reduction.Topocentric.Norm() * kmPerAU)

		got, err := LookAngle(sat, 0, ctx)
		if err != nil {
			t.Fatalf("LookAngle: %v", err)
		}

		if got.Alt() != reduction.Observed.Alt() || got.Az() != reduction.Observed.Az() {
			t.Errorf("%02d:00 — alt/az drifted from the Reducer pipeline: "+
				"%.6f\" alt, %.6f\" az", hour,
				(got.Alt() - reduction.Observed.Alt()).Arcseconds(),
				(got.Az() - reduction.Observed.Az()).Arcseconds())
		}

		if math.Abs(got.Dist()-reduction.Observed.Dist()) > 1e-9 {
			t.Errorf("%02d:00 — range drifted: %.9f km", hour,
				got.Dist()-reduction.Observed.Dist())
		}
	}
}
