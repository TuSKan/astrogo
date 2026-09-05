package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestCulminationSkipsSearchIsNeverAFalsePositive is the safety property.
//
// Episode now answers "does this ever rise?" from two arcsines instead of a
// 366-day search, and a wrong "no" would be silent: the caller gets a nil rise
// and a nil set, which is exactly what a genuine never-rises target returns.
//
// # Why a brute-force check is possible here
//
// For a target at fixed declination, one day covers every hour angle. So
// sweeping a day at fine steps is not a sample — it is the whole domain, and
// it settles the question exactly. That is what makes this an independent
// check rather than the closed form compared against itself.
//
// The sweep deliberately uses the same refracted pipeline Episode's own
// isAboveHorizon does, while the closed form is pure geometry with no
// refraction. Those disagree near the horizon, which is exactly what
// episodeClosedFormMargin exists to cover — and this test is what says the
// margin is big enough.
func TestCulminationSkipsSearchIsNeverAFalsePositive(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)

	const (
		sweepSteps = 96 // 15-minute steps across a day: every hour angle
		dayMinutes = 24 * 60
	)

	checkedSkips := 0

	for latDeg := -80.0; latDeg <= 80.0; latDeg += 10 {
		loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(latDeg), 0)
		if err != nil {
			t.Fatalf("NewGeodetic(%g): %v", latDeg, err)
		}

		site, err := NewSite("sweep", loc)
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}

		// Contexts depend on time and site, never on the target, so build the
		// day's worth once and reuse them across every declination. Without
		// this the test would pay an Apco13 solve per (latitude, declination,
		// step) and take minutes.
		ctxs := make([]*coord.Context, sweepSteps)
		times := make([]time.Time, sweepSteps)

		for i := range ctxs {
			times[i] = base.Add(time.Duration(i*dayMinutes/sweepSteps) * time.Minute)
			ctxs[i] = coord.NewContext(times[i], site.Location(), site.Refraction())
		}

		for decDeg := -90.0; decDeg <= 90.0; decDeg += 5 {
			star := NewStar("sweep", angle.Deg(0), angle.Deg(decDeg))

			if !culminationSkipsSearch(star, site, base) {
				continue
			}

			checkedSkips++

			// It claims there is no rise. Over a full day of hour angles the
			// altitude must therefore stay wholly on one side of the
			// threshold — never crossing it.
			pos, err := star.Position(base)
			if err != nil {
				t.Fatalf("Position: %v", err)
			}

			above, below := 0, 0

			for i := range ctxs {
				aa, err := observedAltAz(star, times[i], ctxs[i], pos)
				if err != nil {
					t.Fatalf("observedAltAz: %v", err)
				}

				if aa.Alt() > site.RiseSetThreshold() {
					above++
				} else {
					below++
				}
			}

			if above != 0 && below != 0 {
				t.Errorf("lat %+.0f dec %+.0f: the closed form skipped the search, "+
					"but over a full day the target is above the horizon at %d of %d "+
					"steps and below at %d.\n"+
					"  It does rise, so Episode would wrongly report no rise. "+
					"episodeClosedFormMargin (%g deg) is too small.",
					latDeg, decDeg, above, sweepSteps, below, episodeClosedFormMargin)
			}
		}
	}

	// A margin so large that nothing ever short-circuits would pass the loop
	// above trivially, so check the optimisation still fires.
	if checkedSkips < 50 {
		t.Fatalf("the closed form short-circuited only %d times across the whole "+
			"latitude/declination grid; it is no longer doing anything", checkedSkips)
	}

	t.Logf("%d short-circuits, each confirmed against a full day of hour angles",
		checkedSkips)
}

// TestEpisodeStillSearchesNearTheHorizonBoundary pins the other direction: a
// target close enough to the threshold that refraction or parallax could decide
// the answer must go to the search, exactly as before.
//
// The dangerous change would be a short-circuit that creeps toward the
// boundary and starts answering cases the geometry cannot settle.
func TestEpisodeStillSearchesNearTheHorizonBoundary(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(50), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("boundary", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	base := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)

	// At latitude 50N the never-rises boundary is dec = -(90-50) = -40, and the
	// circumpolar boundary is dec = +40. Declinations within the margin of
	// either must not be answered from geometry alone.
	for _, decDeg := range []float64{-40.5, -40.0, -39.5, 39.5, 40.0, 40.5} {
		star := NewStar("edge", angle.Deg(0), angle.Deg(decDeg))

		if culminationSkipsSearch(star, site, base) {
			t.Errorf("dec %+.1f is within %g deg of a boundary at latitude 50, but "+
				"the closed form answered without searching. Refraction alone is "+
				"~34 arcmin here, so geometry cannot settle it.",
				decDeg, episodeClosedFormMargin)
		}
	}
}

// TestEpisodeDoesNotShortCircuitAMovingBody pins the restriction that makes
// the whole thing sound.
//
// The closed form reads one declination and treats it as fixed for the entire
// search window. That holds for a star and not for anything in the solar
// system: the Moon covers 28 degrees of declination in a fortnight, so a body
// below the horizon today may well rise next week.
//
// # The fixture is chosen so the guard is load-bearing
//
// A first version of this test used an arbitrary epoch and passed even with
// the MovingBody guard deleted — at that instant the Moon was near the horizon
// boundary, so the margin declined to answer anyway and the guard was never
// the reason. A mutation caught it.
//
// 2026-03-11 is picked instead because the Moon is then at declination -27.9,
// putting upper culmination 17.9 degrees below this site's horizon — far
// outside the margin, so a *fixed* target there would certainly be
// short-circuited. Ten days later the Moon is at +13.5 and plainly rises. Only
// the MovingBody check stops Episode answering "never rises" for a body that
// rises within the week.
func TestEpisodeDoesNotShortCircuitAMovingBody(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(80), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("arctic", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	base := time.Date(2026, time.March, 11, 0, 0, 0, 0, time.LocationUTC)
	moon := NewMoon(eph.Default())

	if _, isMoving := Observable(moon).(MovingBody); !isMoving {
		t.Fatal("precondition: the Moon is no longer a MovingBody, so this test " +
			"no longer covers the case it was written for")
	}

	// Confirm the fixture still puts the Moon clear of the margin, so that
	// only the MovingBody check can be preventing the short-circuit.
	pos, err := moon.Position(base)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	_, maxAlt := circumpolarExtremes(pos.Dec(), site)

	threshold := site.RiseSetThreshold().Radians()
	margin := angle.Deg(episodeClosedFormMargin).Radians()

	if maxAlt >= threshold-margin {
		t.Fatalf("fixture drifted: the Moon's declination %.2f puts upper "+
			"culmination at %.2f deg, inside the margin of the %.2f deg threshold. "+
			"The margin would decline to answer regardless, so this test would "+
			"pass with the MovingBody guard removed.",
			pos.Dec().Degrees(), angle.Rad(maxAlt).Degrees(),
			site.RiseSetThreshold().Degrees())
	}

	if culminationSkipsSearch(moon, site, base) {
		t.Error("a MovingBody was answered from a single declination; its " +
			"declination changes across the search window, so the culmination " +
			"bounds do not hold")
	}

	// And the Moon really does rise, so short-circuiting here would be wrong
	// rather than merely unjustified.
	rise, _, err := Episode(base, base.AddDays(1), moon, site)
	if err != nil {
		t.Fatalf("Episode: %v", err)
	}

	if rise == nil {
		t.Error("Episode found no moonrise within the search window, but the " +
			"Moon reaches declination +13.5 ten days later and plainly rises")
	}
}

// BenchmarkEpisodeNeverRises measures the case the closed form exists for.
//
// The search it replaces doubles a window out to 366 days and evaluates a
// position at every step, finding nothing each time and concluding only by
// exhaustion — measured at 2.84 s before this change.
func BenchmarkEpisodeNeverRises(b *testing.B) {
	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(80), 0)
	if err != nil {
		b.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Arctic", loc)
	if err != nil {
		b.Fatalf("NewSite: %v", err)
	}

	star := NewStar("NeverUp", angle.Deg(0), angle.Deg(-85))
	from := time.FromJD(2451544.5, time.UTC)
	to := from.AddDays(1)

	b.ResetTimer()

	for b.Loop() {
		if _, _, err := Episode(from, to, star, site); err != nil {
			b.Fatalf("Episode: %v", err)
		}
	}
}
