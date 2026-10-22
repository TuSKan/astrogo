package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// testISS builds the provider and site the context-reuse tests share — the
// same real ISS TLE and Paranal site the benchmarks use, so nothing here
// touches the network.
func testISS(t *testing.T) (*satellite.Satellite, *coord.Geodetic) {
	t.Helper()

	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	site, err := coord.NewGeodetic(angle.Deg(-70.4028), angle.Deg(-24.6251), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return sat, site
}

// TestMovingContextStaysInsideItsStatedBound measures what SatellitePasses'
// reuse of one moving Context costs, rather than trusting SetTime's own
// figure.
//
// Every 30-second sample across six hours is computed twice, once from the
// moving Context and once from a full coord.NewContext, and compared as an
// angular separation, along two routes:
//
//   - LookAngle as SatellitePasses calls it. Since #812 it places the
//     satellite with the Context's own precession-nutation, so the error of
//     the moving Context's interpolated series cancels, and the two agree to
//     rounding: held to 0.01 mas, measured under a microarcsecond.
//   - The same through a provider that offers only State, which places the
//     satellite with a precession-nutation of its own. The moving Context's
//     error then shows, and it is not SetTime's ≲0.007″ for its day-long
//     bracket (#822): that is the error in a direction, and here it also
//     turns the observer's 6,400 km geocentric vector, a few hundred km
//     from the satellite. Seen from the observer, the satellite moves by the
//     direction's error times 1 + |observer|/range, sixteen at the zenith of
//     a pass; that is what each sample is held to, measured at up to 0.034″.
//     It must also reach ten times the first route's bound, or the agreement
//     there would be a Context that never moved, not a cancellation.
//
// Until #826 this held one route to 0.1″, SetTime's figure when it rebuilt
// hourly, and the route had canceled the error since #812: it measured
// 0.0000″ and could not fail.
func TestMovingContextStaysInsideItsStatedBound(t *testing.T) {
	t.Parallel()

	const (
		canceledArcsec = 1e-5  // 0.01 mas
		setTimeArcsec  = 0.007 // coord.Context's bracket
	)

	sat, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)

	for _, route := range []struct {
		name string
		prov eph.Provider
		// bound is the most a sample may be off, at a geocentric observer
		// distance obs and a range rng; least is what the worst must reach.
		bound func(obs, rng float64) float64
		least float64
	}{
		{"the Context's precession-nutation", sat, func(_, _ float64) float64 { return canceledArcsec }, 0},
		{"the satellite's own precession-nutation", stateOnly{sat}, func(obs, rng float64) float64 {
			return setTimeArcsec * (1 + obs/rng)
		}, 10 * canceledArcsec},
	} {
		ctxAt := movingContext(site, defaultAtm)

		var (
			worst, worstOver float64
			worstAt          time.Time
		)

		for i := range 6 * 60 * 2 { // six hours at 30 s
			at := start.Add(unit.Seconds(float64(i) * 30))

			// LookAngle answers below the horizon too — a negative
			// altitude, not an error — and no sample in these six hours
			// errors, so one that does is a defect, not a sample to leave
			// out.
			moved := ctxAt(at)

			cached, err := LookAngle(route.prov, sat.ID(), moved)
			if err != nil {
				t.Fatalf("%v: LookAngle through the moving Context: %v", at, err)
			}

			exact, err := LookAngle(route.prov, sat.ID(), coord.NewContext(at, site, defaultAtm))
			if err != nil {
				t.Fatalf("%v: LookAngle through a fresh context: %v", at, err)
			}

			// coord.ICRS carries the pair here purely as a direction on a
			// sphere: Separation is the angle between two unit vectors and
			// does not care which frame labeled them.
			sep := coord.Separation(
				coord.NewICRS(cached.Az(), cached.Alt()),
				coord.NewICRS(exact.Az(), exact.Alt()),
			).Arcsec()

			const kmPerAU = 149597870.7

			bound := route.bound(moved.ObsVec().Norm()*kmPerAU, exact.Dist().Km())
			worst = math.Max(worst, math.Abs(sep))

			if over := math.Abs(sep) / bound; over > worstOver {
				worstOver, worstAt = over, at
			}
		}

		t.Logf("%s: worst separation %.3g arcsec, %.3g of its bound at %v", route.name, worst, worstOver, worstAt)

		if worstOver > 1 {
			t.Errorf("placed with %s, the moving Context's look angle is %.3g times its bound from a fresh one's at %v",
				route.name, worstOver, worstAt)
		}

		if worst < route.least {
			t.Errorf("placed with %s, the moving Context's look angle is at most %.3g arcsec from a fresh one's, want at least %.3g: the window this test relies on is not visible",
				route.name, worst, route.least)
		}
	}
}

// TestSatellitePassEventsSurviveAFullRebuild is the end-to-end half: the pass
// times SatellitePasses reports are re-evaluated against a freshly built
// Context, and each rise and set must still sit on the elevation threshold it
// was solved for.
//
// This is the property the reuse could actually break. A drifting Context
// would move the crossing, and the reported time would then be a time at which
// the satellite is not in fact at the minimum elevation.
func TestSatellitePassEventsSurviveAFullRebuild(t *testing.T) {
	sat, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Hours(6))
	minEl := angle.Deg(10)

	passes, err := SatellitePasses(sat, sat.ID(), "ISS", start, end, site, minEl)
	if err != nil {
		t.Fatalf("SatellitePasses: %v", err)
	}

	if len(passes) == 0 {
		t.Fatal("no passes found; the fixture cannot exercise anything")
	}

	// The solver refines crossings to 1 s. The ISS crosses 10° at roughly
	// 0.4°/s, so a residual of a few tenths of a degree is the refinement's
	// own tolerance rather than context drift — and a residual from drift
	// would be four orders of magnitude smaller than that. This catches a
	// reuse window opened wide enough to move a crossing perceptibly.
	const residualDeg = 0.5

	for _, p := range passes {
		for _, ev := range []struct {
			what string
			at   time.Time
		}{{"rise", p.Rise.Time}, {"set", p.Set.Time}} {
			got, err := LookAngle(sat, sat.ID(), coord.NewContext(ev.at, site, defaultAtm))
			if err != nil {
				t.Errorf("%s at %v: LookAngle: %v", ev.what, ev.at, err)
				continue
			}

			off := got.Alt().Degrees() - minEl.Degrees()
			if math.Abs(off) > residualDeg {
				t.Errorf("%s reported at %v, but a full rebuild puts the ISS at %.4f° — %.4f° off the %.1f° threshold",
					ev.what, ev.at, got.Alt().Degrees(), off, minEl.Degrees())
			}
		}
	}
}

// TestMovingContextKeepsAFixedStarInSetTimesBound: the whole safety argument
// rests on the moving Context's series being evaluated afresh as it moves. A
// fixed star's CIRS place through it must stay within coord.Context.SetTime's
// documented 0.007″ of a fresh NewContext's, across three days; a Context
// that kept its first evaluation would be about a second of arc off by then,
// and one rebuilt only hourly 0.07″ (#790).
//
// Without this, a Context that never refreshed would pass every other test
// here: six hours of ISS look angles stay inside 0.1" anyway. A fixed star is
// what exposes it.
func TestMovingContextKeepsAFixedStarInSetTimesBound(t *testing.T) {
	_, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	ctxAt := movingContext(site, defaultAtm)

	// The Crab, as an ordinary catalog position; any fixed direction does.
	fixed := coord.NewAstrometric(angle.Deg(83.633), angle.Deg(22.014))

	// coord.Context.SetTime's documented bound against a fresh Context.
	const boundArcsec = 0.007

	var worst float64

	for m := 0; m <= 3*24*60; m += 47 {
		at := start.Add(unit.Minutes(float64(m)))

		got := ctxAt(at).AstrometricToCIRS(fixed)
		want := coord.NewContext(at, site, defaultAtm).AstrometricToCIRS(fixed)

		worst = max(worst, coord.Separation(
			coord.NewICRS(got.RA(), got.Dec()),
			coord.NewICRS(want.RA(), want.Dec()),
		).Arcsec())
	}

	if worst > boundArcsec {
		t.Errorf("across three days the moving Context puts a fixed star %.4f arcsec from a fresh one, "+
			"over SetTime's %g", worst, boundArcsec)
	}
}

// TestMovingContextHandlesBackwardsSteps: SetTime finds its bracket either
// way from where it is, not only forward, and this is the caller that needs
// it.
//
// SatellitePasses samples the whole window forward and only then refines its
// crossings, so the first refinement asks for an instant hours *behind* the
// base the sweep left behind. A signed comparison never trips on that and
// serves a stale derivation instead — silently, since the answer is still
// plausible. Found by mutation, when the comparison lived in plan: dropping
// the .Abs() passed every other test in this file.
func TestMovingContextHandlesBackwardsSteps(t *testing.T) {
	_, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	ctxAt := movingContext(site, defaultAtm)

	// Sweep forward far enough to move the base several windows on, the way a
	// six-hour sample loop does.
	for h := range 6 {
		ctxAt(start.Add(unit.Hours(float64(h))))
	}

	// Now refine a crossing back near the beginning.
	back := start.Add(unit.Minutes(10))

	fixed := coord.NewAstrometric(angle.Deg(83.633), angle.Deg(22.014))

	got := ctxAt(back).AstrometricToCIRS(fixed)
	want := coord.NewContext(back, site, defaultAtm).AstrometricToCIRS(fixed)

	sep := coord.Separation(
		coord.NewICRS(got.RA(), got.Dec()),
		coord.NewICRS(want.RA(), want.Dec()),
	).Arcsec()

	t.Logf("backwards step of ~5h from the base: %.4f arcsec", sep)

	// coord.Context.SetTime's documented bound against a fresh Context.
	const boundArcsec = 0.007

	if sep > boundArcsec {
		t.Errorf("a backwards step was served from a stale base: %.4f arcsec, over the %g arcsec bound",
			sep, boundArcsec)
	}
}
