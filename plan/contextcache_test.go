package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
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

// TestContextCacheStaysInsideItsStatedBound measures what the reuse actually
// costs, rather than trusting AtTime's ≲0.1″/hour on its own.
//
// Every 30-second sample across six hours is computed twice — once from the
// cache, once from a full coord.NewContext — and compared as an angular
// separation. The bound is what the cache's own doc comment claims for one
// hour of drift; if a change to AtTime or to ctxRefresh made the reuse worse,
// this is the test that has to be argued with.
func TestContextCacheStaysInsideItsStatedBound(t *testing.T) {
	sat, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	ctxAt := newContextCache(site, defaultAtm)

	var (
		worst   float64
		worstAt time.Time
		n       int
	)

	for i := range 6 * 60 * 2 { // six hours at 30 s
		at := start.Add(time.Duration(i) * 30 * time.Second)

		cached, err := LookAngle(sat, 0, ctxAt(at))
		if err != nil {
			continue // below the horizon geometry the provider rejects; not this test's subject
		}

		exact, err := LookAngle(sat, 0, coord.NewContext(at, site, defaultAtm))
		if err != nil {
			continue
		}

		n++

		// coord.ICRS carries the pair here purely as a direction on a
		// sphere: Separation is the angle between two unit vectors and does
		// not care which frame labelled them.
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

	// 0.1″ is the per-hour figure AtTime documents, and ctxRefresh holds the
	// drift to one hour. Asserted at the documented bound rather than at the
	// measured worst case, so the test cannot silently ratify a regression it
	// merely happens to permit.
	const boundArcsec = 0.1

	if worst > boundArcsec {
		t.Errorf("context reuse cost %.4f arcsec at %v, over the %.2f arcsec ctxRefresh is chosen to hold",
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
	end := start.Add(6 * time.Hour)
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

// TestContextCacheRebuildsPastItsWindow: the whole safety argument rests on
// the base being rebuilt once the drift would exceed ctxRefresh. Nothing about
// a returned Context says which base it came from, so the check is
// differential — the same instant is asked of the cache and of a deliberately
// stale base, and the two must disagree by about the drift AtTime documents.
//
// Without this, a cache that never rebuilt would pass every other test here:
// six hours of ISS look angles stay inside 0.1" anyway, because the
// geocentric-to-observed path rebuilds its rotation matrix exactly and only
// the ASTROM precession-nutation goes stale. A fixed star is what exposes it.
func TestContextCacheRebuildsPastItsWindow(t *testing.T) {
	_, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	past := start.Add(ctxRefresh + time.Minute)

	ctxAt := newContextCache(site, defaultAtm)

	base := ctxAt(start)
	rebuilt := ctxAt(past)
	stale := base.AtTime(past)

	// The Crab, as an ordinary catalogue position; any fixed direction does.
	fixed := coord.NewAstrometric(angle.Deg(83.633), angle.Deg(22.014))

	fresh := rebuilt.AstrometricToApparent(fixed)
	drifted := stale.AstrometricToApparent(fixed)

	sep := coord.Separation(
		coord.NewICRS(fresh.RA(), fresh.Dec()),
		coord.NewICRS(drifted.RA(), drifted.Dec()),
	).Arcsec()

	t.Logf("rebuilt vs stale derivation at ctxRefresh+1m: %.4f arcsec", sep)

	// An order of magnitude below the ~0.1"/hour AtTime documents, so the
	// assertion is that a rebuild happened at all rather than that it produced
	// any particular number.
	if sep < 0.01 {
		t.Errorf("a Context past the reuse window agrees with the stale base to %.4f arcsec; "+
			"it was derived rather than rebuilt", sep)
	}
}

// TestContextCacheHandlesBackwardsSteps: the drift test is |t − base|, not
// t − base, and this is the caller that needs it.
//
// SatellitePasses samples the whole window forward and only then refines its
// crossings, so the first refinement asks for an instant hours *behind* the
// base the sweep left behind. A signed comparison never trips on that and
// serves a stale derivation instead — silently, since the answer is still
// plausible. Found by mutation: dropping the .Abs() passes every other test
// in this file.
func TestContextCacheHandlesBackwardsSteps(t *testing.T) {
	_, site := testISS(t)

	start := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.LocationUTC)
	ctxAt := newContextCache(site, defaultAtm)

	// Sweep forward far enough to move the base several windows on, the way a
	// six-hour sample loop does.
	for h := range 6 {
		ctxAt(start.Add(time.Duration(h) * time.Hour))
	}

	// Now refine a crossing back near the beginning.
	back := start.Add(10 * time.Minute)

	fixed := coord.NewAstrometric(angle.Deg(83.633), angle.Deg(22.014))

	got := ctxAt(back).AstrometricToApparent(fixed)
	want := coord.NewContext(back, site, defaultAtm).AstrometricToApparent(fixed)

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
