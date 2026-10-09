package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
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

// TestMovingContextStaysInsideItsStatedBound measures what the reuse actually
// costs, rather than trusting SetTime's ≲0.1″ on its own.
//
// Every 30-second sample across six hours is computed twice — once from the
// moving Context, once from a full coord.NewContext — and compared as an
// angular separation. The bound is what SetTime's doc comment claims for an
// hour of drift; if a change to SetTime or its rebuild hour made the reuse worse,
// this is the test that has to be argued with.
func TestMovingContextStaysInsideItsStatedBound(t *testing.T) {
	t.Parallel()

	sat, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	ctxAt := movingContext(site, defaultAtm)

	var (
		worst   float64
		worstAt time.Time
		n       int
	)

	for i := range 6 * 60 * 2 { // six hours at 30 s
		at := start.Add(unit.Seconds(float64(i) * 30))

		// LookAngle answers below the horizon too — a negative altitude, not
		// an error — and no sample in these six hours errors, so one that
		// does is a defect, not a sample to leave out.
		cached, err := LookAngle(sat, 0, ctxAt(at))
		if err != nil {
			t.Fatalf("%v: LookAngle through the moving Context: %v", at, err)
		}

		exact, err := LookAngle(sat, 0, coord.NewContext(at, site, defaultAtm))
		if err != nil {
			t.Fatalf("%v: LookAngle through a fresh context: %v", at, err)
		}

		n++

		// coord.ICRS carries the pair here purely as a direction on a
		// sphere: Separation is the angle between two unit vectors and does
		// not care which frame labeled them.
		sep := coord.Separation(
			coord.NewICRS(cached.Az(), cached.Alt()),
			coord.NewICRS(exact.Az(), exact.Alt()),
		).Arcsec()

		if math.Abs(sep) > worst {
			worst, worstAt = math.Abs(sep), at
		}
	}

	if n == 0 {
		t.Fatal("no samples compared; the fixture produced no look angles")
	}

	t.Logf("compared %d samples, worst separation %.4f arcsec at %v", n, worst, worstAt)

	// 0.1″ is the per-hour figure SetTime documents, and its rebuild holds the
	// drift to one hour. Asserted at the documented bound rather than at the
	// measured worst case, so the test cannot silently ratify a regression it
	// merely happens to permit.
	const boundArcsec = 0.1

	if worst > boundArcsec {
		t.Errorf("context reuse cost %.4f arcsec at %v, over the %.2f arcsec the rebuild hour is chosen to hold",
			worst, worstAt, boundArcsec)
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

	passes, err := SatellitePasses(sat, "ISS", start, end, site, minEl)
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
			got, err := LookAngle(sat, 0, coord.NewContext(ev.at, site, defaultAtm))
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

// TestMovingContextRebuildsPastTheHour: the whole safety argument rests on
// the Context being rebuilt once the drift would exceed an hour's. A minute
// past the hour, the moving Context must be exactly what a fresh NewContext
// gives at that instant, where a stale epoch would be about the drift SetTime
// documents away.
//
// Without this, a Context that never rebuilt would pass every other test here:
// six hours of ISS look angles stay inside 0.1" anyway, because the
// geocentric-to-observed path rebuilds its rotation matrix exactly and only
// the ASTROM precession-nutation goes stale. A fixed star is what exposes it.
func TestMovingContextRebuildsPastTheHour(t *testing.T) {
	_, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	past := start.Add(unit.Hours(1) + unit.Minutes(1))

	ctxAt := movingContext(site, defaultAtm)
	ctxAt(start)

	// The Crab, as an ordinary catalogue position; any fixed direction does.
	fixed := coord.NewAstrometric(angle.Deg(83.633), angle.Deg(22.014))

	got := ctxAt(past).AstrometricToCIRS(fixed)
	want := coord.NewContext(past, site, defaultAtm).AstrometricToCIRS(fixed)

	if got.RA() != want.RA() || got.Dec() != want.Dec() {
		sep := coord.Separation(
			coord.NewICRS(got.RA(), got.Dec()),
			coord.NewICRS(want.RA(), want.Dec()),
		).Arcsec()

		t.Errorf("a minute past the hour, the moving Context is %.6f arcsec from a fresh one; "+
			"it was moved rather than rebuilt", sep)
	}
}

// TestMovingContextHandlesBackwardsSteps: SetTime's drift test is
// |t − epoch|, not t − epoch, and this is the caller that needs it.
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

	const boundArcsec = 0.1

	if sep > boundArcsec {
		t.Errorf("a backwards step was served from a stale base: %.4f arcsec, over the %.2f arcsec bound",
			sep, boundArcsec)
	}
}
