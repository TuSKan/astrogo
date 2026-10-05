package plan_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Work contracts for plan's hot paths, counted as coord/work_test.go counts
// NewContext: evaluations of the IAU 2006/2000A precession-nutation series,
// through gofaext.SeriesEvaluations. A coord.Context is one evaluation, so
// these say where a path builds a Context and where it must reuse one.
//
// Not parallel: the counter is process-wide.

// seriesIn returns how many times f evaluates the precession-nutation series.
func seriesIn(f func()) int64 {
	before := gofaext.SeriesEvaluations()

	f()

	return gofaext.SeriesEvaluations() - before
}

// TestConstraintChecksEvaluateNoSeries holds the scheduler's per-sample unit
// to zero: given the Context of its time step, every built-in constraint
// evaluates the series not at all, for a star and for the solar-system bodies
// whose positions it looks up. The scheduler shares one Context across all of
// them for exactly this reason (ConstraintCtx); a constraint that built its
// own would cost a full Context per target per step.
func TestConstraintChecksEvaluateNoSeries(t *testing.T) {
	prov := eph.Default()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := plan.NewSite("work contract", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	epoch := time.Date(2026, time.June, 15, 3, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(epoch, site.Location(), site.Refraction())

	targets := []struct {
		name string
		obj  plan.Observable
	}{
		{"a star", plan.NewStar("Antares", angle.Hour(16.49), angle.Deg(-26.43))},
		{"the Moon", plan.NewMoon(prov)},
		{"Mars", plan.NewPlanet("Mars", eph.Mars, prov)},
	}

	constraints := []struct {
		name string
		c    plan.ConstraintCtx
	}{
		{"Altitude", plan.Altitude{Threshold: angle.Deg(30)}},
		{"Airmass", plan.Airmass{}},
		{"Sun", plan.Sun{}},
		{"MoonSep", plan.MoonSep{}},
		{"MoonIllum", plan.MoonIllum{}},
	}

	for _, target := range targets {
		for _, constraint := range constraints {
			got := seriesIn(func() { _, _ = constraint.c.CheckCtx(target.obj, epoch, site, ctx) })
			if got != 0 {
				t.Errorf("%s.CheckCtx for %s evaluated the precession-nutation series %d times, want 0: "+
					"it has a Context and should not build another", constraint.name, target.name, got)
			}
		}
	}
}

// TestRiseSetBuildsAContextAnHourNotASample holds the rise/set solver to the
// cadence #10 gave it: one full Context per hour of window, every sample and
// bisection step in between derived with Context.AtTime. Before #10 it built
// one per sample, which was 65% of a fortnight's forecast.
//
// A day measures 31 evaluations: the hourly Contexts and the event
// refinements. The bound is two an hour, which that is well inside and a
// Context per ten-minute sample (more than a hundred and forty) is far
// outside.
func TestRiseSetBuildsAContextAnHourNotASample(t *testing.T) {
	prov := eph.Default()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := plan.NewSite("work contract", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	start := time.Date(2026, time.June, 15, 3, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Hours(24))

	const bound = 2 * 24

	for _, c := range []struct {
		name  string
		solve func()
	}{
		{"SunriseSunset", func() { _, _, _ = plan.SunriseSunset(start, end, site, prov) }},
		{"MoonriseMoonset", func() { _, _, _ = plan.MoonriseMoonset(start, end, site, prov) }},
	} {
		if got := seriesIn(c.solve); got > bound {
			t.Errorf("%s over one day evaluated the precession-nutation series %d times, want at most %d: "+
				"it is building a Context per sample rather than deriving them with AtTime", c.name, got, bound)
		}
	}
}
