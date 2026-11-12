package plan

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// runSky is what one scheduling run has worked out about the sky at each
// instant it has asked about, held for the whole run and dropped with it.
//
// # Why a run keeps it
//
// None of it depends on the target, and a run tests target after target
// against the same instants. Greedy tries every candidate block at each step
// of the night, and the swap passes return to every placement on every pass.
// So the Sun and the Moon were computed again for each target, and they were
// most of what a schedule cost: with the Sun and Moon constraints, Epv00 alone
// was 53% of the CPU of twenty targets tested against one night (#913). The
// run knows its own instants, so it keeps them, as #959 already did for the
// score's Moon. A failure is returned and never kept.
//
// # What makes one record right for every target
//
// A run checks its constraints through one Context, the run's, so the place
// the site sees is the same whichever target asks. A run is also one
// goroutine, and these maps are never shared between goroutines.
//
// A nil *runSky keeps nothing, and every method computes its answer afresh.
// That is how a constraint checked outside a run answers.
type runSky struct {
	// moon is the Moon's apparent geocentric vector at each instant the run
	// scored. A greedy run leaves it nil, and its score uses the process-wide
	// cache, since it scores each placement once (#959).
	moon moonPlaces

	sunAlt    map[time.Time]angle.Angle
	moonTopo  map[time.Time]coord.ICRS
	moonIllum map[time.Time]float64
}

// newRunSky starts a run's record. With moon, it keeps the score's Moon as
// well as the constraints' sky.
func newRunSky(moon bool) *runSky {
	s := &runSky{
		sunAlt:    map[time.Time]angle.Angle{},
		moonTopo:  map[time.Time]coord.ICRS{},
		moonIllum: map[time.Time]float64{},
	}

	if moon {
		s.moon = moonPlaces{}
	}

	return s
}

// skyConstraint is a constraint whose sky term is the same for every target
// at an instant, and so can be answered from a run's record. It is the
// built-in Sun, MoonSep and MoonIllum. Their CheckCtx is checkSky with no
// record.
type skyConstraint interface {
	checkSky(obj Observable, t time.Time, ctx *coord.Context, sky *runSky) (Result, error)
}

// checkConstraint evaluates c for obj at t. It takes c's sky term from the
// run's record when c has one, gives c the Context when c takes one, and
// otherwise lets c build what it needs.
func checkConstraint(c Constraint, obj Observable, t time.Time, site *Site, ctx *coord.Context, sky *runSky) (Result, error) {
	switch c := c.(type) {
	case skyConstraint:
		return c.checkSky(obj, t, ctx, sky)
	case ConstraintCtx:
		return c.CheckCtx(obj, t, site, ctx) //nolint:wrapcheck // both callers wrap it, with the instant or as a constraint check
	default:
		return c.Check(obj, t, site) //nolint:wrapcheck // both callers wrap it, with the instant or as a constraint check
	}
}

// sunAltitude is the Sun's observed altitude at t, as ctx places it.
func (s *runSky) sunAltitude(t time.Time, ctx *coord.Context) (angle.Angle, error) {
	if s != nil {
		if alt, ok := s.sunAlt[t]; ok {
			return alt, nil
		}
	}

	aa, err := skyAltAzCtx(NewSun(eph.Default()), t, ctx)
	if err != nil {
		return angle.Zero(), err
	}

	alt := aa.Alt()

	if s != nil {
		s.sunAlt[t] = alt
	}

	return alt, nil
}

// moonTopocentric is the Moon at t as the site ctx describes sees it.
func (s *runSky) moonTopocentric(t time.Time, ctx *coord.Context) (coord.ICRS, error) {
	if s != nil {
		if pos, ok := s.moonTopo[t]; ok {
			return pos, nil
		}
	}

	pos, err := topocentricPosition(NewMoon(eph.Default()), t, ctx)
	if err != nil {
		return coord.ICRS{}, err
	}

	if s != nil {
		s.moonTopo[t] = pos
	}

	return pos, nil
}

// moonIllumination is the Moon's illuminated fraction at t, from the default
// ephemeris.
func (s *runSky) moonIllumination(t time.Time) (float64, error) {
	if s != nil {
		if frac, ok := s.moonIllum[t]; ok {
			return frac, nil
		}
	}

	frac, _, err := MoonIllumination(t, eph.Default())
	if err != nil {
		return 0, err
	}

	if s != nil {
		s.moonIllum[t] = frac
	}

	return frac, nil
}
