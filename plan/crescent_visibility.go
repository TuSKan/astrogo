package plan

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// ── Evaluating an evening, each criterion in its own convention ─────────────
//
// The criteria are not stated in one convention, so no one set of parameters
// can feed all of them (#496). Each was fitted to quantities its author defined:
//
//   - Yallop (1997, NAO Technical Note 69): ARCV is the geocentric, airless
//     difference in altitude of the centers of Sun and Moon; ARCL is
//     geocentric; the crescent width is W′ = SD′(1 − cos ARCL) with
//     SD = 0.27245π and SD′ = SD(1 + sin h sin π), π the Moon's horizontal
//     parallax and h its geocentric altitude (his eqs. 3.8–3.10); all at the
//     best time Tb = Ts + (4/9)·Lag (his eq. 4.1).
//   - Odeh (2004, Exp. Astron. 18, 39): ARCV, ARCL and W topocentric and
//     airless, W = SD′(1 − cos ARCL), at the same best time.
//   - MABIMS 2021: the Moon's topocentric altitude at least 3° and the
//     geocentric elongation at least 6.4°, at sunset.
//   - MABIMS 1995 ("2-3-8"): the altitude at least 2°, and the elongation at
//     least 3° or the Moon at least 8 hours old, at sunset.
//
// The other criteria read what NewCrescentParams gave them: topocentric,
// airless altitudes and azimuths, and the geocentric elongation, at sunset —
// which happens to be MABIMS's convention. Their own conventions are not yet
// verified against their sources. CrescentResult.Params is that set.
//
// One evaluation builds one coord.Context, at sunset, and derives the best
// time's from it with AtTime: an hour of AtTime costs ≲0.1″, against these
// criteria's tenths of a degree. The Sun and Moon are each looked up once per
// instant, and every criterion reads the same two lookups.
//
// What an evaluation costs is the three searches before it: sunset, moonset
// and the new moon. Each searches only as far as it must — the rise and set
// a few hours at a time, the new moon in the bracket the elongation puts it
// in — which took an evening at Port of Spain from 52 ms to 16 ms
// (BenchmarkCrescentVisibility); TestCrescentSearchesMatchAWholeWindow holds
// them to what one search over the whole window finds.

// errNoSunset is returned when the Sun does not set within a day of the
// requested evening, as in polar summer.
var errNoSunset = errors.New("crescent: no sunset within a day of the evening")

// errNoMoonset is returned when the Moon neither set before sunset nor sets
// within a day after it.
var errNoMoonset = errors.New("crescent: no moonset near the evening's sunset")

// errNoNewMoon is returned when the new moon the elongation places before an
// instant is not found there, which would mean an ephemeris fault.
var errNoNewMoon = errors.New("crescent: no new moon where the elongation puts it")

// eventChunkHours is how much of a rise and set search one solver call covers.
//
// The solver samples every 15 minutes, so its cost grows with the window,
// and the event wanted is hours away, not a day: sunset is later the same
// day, and a crescent sets a few hours after the Sun. The solver looks for a
// grazing extremum in a window's first and last step, so a chunk boundary
// loses nothing.
const eventChunkHours = 6

// The Sun–Moon elongation over the Moon's age, measured over the 618
// lunations of 2000–2050 with eph.Default(), lies between 10.78 and 14.36°
// a day. moonAgeHours brackets the new moon with a margin on both bounds.
const (
	minElongationRate = 10.0 // degrees a day
	maxElongationRate = 15.0 // degrees a day
	newMoonMargin     = 0.25 // days
)

// CrescentVisibility evaluates the young crescent at site on one evening,
// every criterion in the convention it was defined in.
//
// The evening is the first sunset at or after evening, which is any instant
// earlier that day, local noon for instance. The lag runs to the first moonset
// after it, or back to the last one before it if the Moon had already set.
//
// The result's Params are the quantities at sunset, which MABIMS and the
// criteria without a verified convention read: topocentric, airless altitudes
// and azimuths, and the geocentric elongation. Geocentric and Topocentric are
// the best-time quantities Yallop and Odeh read. A nil provider means
// eph.Default().
//
// It fails when the Sun does not set within a day of evening, or the Moon
// neither sets within a day after sunset nor set within a day before it.
func CrescentVisibility(evening time.Time, site *Site, prov eph.Provider) (CrescentResult, error) {
	if prov == nil {
		prov = eph.Default()
	}

	sunEvent, found, err := firstEvent(SunEvents, evening, 1, site, prov, isSet)
	if err != nil {
		return CrescentResult{}, err
	}

	if !found {
		return CrescentResult{}, errNoSunset
	}

	sunset := sunEvent.Time

	moonset, err := moonsetFor(sunset, site, prov)
	if err != nil {
		return CrescentResult{}, err
	}

	lag := moonset.Sub(sunset)

	// Yallop's best time. A Moon that set before the Sun has no best time
	// after sunset, and is evaluated at sunset, where every criterion that
	// reads altitude already says no.
	best := sunset
	if lag > 0 {
		best = sunset.Add(lag * 4 / 9)
	}

	age, err := moonAgeHours(sunset, prov)
	if err != nil {
		return CrescentResult{}, err
	}

	ctx := coord.NewContext(sunset, site.Location(), atmosphere.Refraction{})

	atSunset, err := crescentGeometryAt(ctx, prov)
	if err != nil {
		return CrescentResult{}, err
	}

	atBest, err := crescentGeometryAt(ctx.AtTime(best), prov)
	if err != nil {
		return CrescentResult{}, err
	}

	lagMinutes := lag.Minutes()
	ageAtBest := age + best.Sub(sunset).Hours()

	params := atSunset.sunset(lagMinutes, age)
	geocentric := atBest.geocentric(lagMinutes, ageAtBest)
	topocentric := atBest.topocentric(lagMinutes, ageAtBest)

	r := params.EvaluateAll()
	r.Sunset, r.Moonset, r.BestTime = sunset, moonset, best
	r.Geocentric, r.Topocentric = geocentric, topocentric

	r.Yallop = geocentric.Yallop()
	r.Odeh = topocentric.Odeh()

	return r, nil
}

// eventsFunc finds a body's events in a window, as SunEvents and MoonEvents do.
type eventsFunc func(start, end time.Time, site *Site, prov eph.Provider) ([]Event, error)

func isSet(e Event) bool { return e.Kind == EventSet }

func isRiseOrSet(e Event) bool { return e.Kind == EventRise || e.Kind == EventSet }

// firstEvent is the first event matching match in the days after start,
// searched forward eventChunkHours at a time.
func firstEvent(events eventsFunc, start time.Time, days float64, site *Site, prov eph.Provider, match func(Event) bool) (Event, bool, error) {
	end := start.Add(unit.Days(days))
	chunk := unit.Hours(eventChunkHours)

	for from := start; from.Before(end); from = from.Add(chunk) {
		to := from.Add(chunk)
		if to.After(end) {
			to = end
		}

		evs, err := events(from, to, site, prov)
		if err != nil {
			return Event{}, false, fmt.Errorf("crescent: %w", err)
		}

		for _, e := range evs {
			if match(e) {
				return e, true, nil
			}
		}
	}

	return Event{}, false, nil
}

// lastEvent is the last event matching match in the days before end,
// searched backward eventChunkHours at a time.
func lastEvent(events eventsFunc, end time.Time, days float64, site *Site, prov eph.Provider, match func(Event) bool) (Event, bool, error) {
	start := end.Add(unit.Days(-days))
	chunk := unit.Hours(eventChunkHours)

	for to := end; to.After(start); to = to.Add(-chunk) {
		from := to.Add(-chunk)
		if from.Before(start) {
			from = start
		}

		evs, err := events(from, to, site, prov)
		if err != nil {
			return Event{}, false, fmt.Errorf("crescent: %w", err)
		}

		for _, e := range slices.Backward(evs) {
			if match(e) {
				return e, true, nil
			}
		}
	}

	return Event{}, false, nil
}

// moonsetFor is the moonset Yallop's lag is measured to: the first after
// sunset if the Moon was up at sunset, else the last before it.
//
// Up or down is read from the events themselves, so that it uses the same
// horizon the moonset does: the Moon was up if the first rise or set after
// sunset is a set, and down if it is a rise, or if there is none and the last
// one before sunset is a set. Only a Moon that was down looks back.
func moonsetFor(sunset time.Time, site *Site, prov eph.Provider) (time.Time, error) {
	next, found, err := firstEvent(MoonEvents, sunset, 1, site, prov, isRiseOrSet)
	if err != nil {
		return time.Time{}, err
	}

	if found && next.Kind == EventSet {
		return next.Time, nil
	}

	prev, found, err := lastEvent(MoonEvents, sunset, 1, site, prov, isRiseOrSet)
	if err != nil {
		return time.Time{}, err
	}

	if found && prev.Kind == EventSet {
		return prev.Time, nil
	}

	return time.Time{}, errNoMoonset
}

// moonAgeHours is the time since the last new moon before t, in hours.
//
// The elongation at t brackets that new moon to a window E/30 + 0.5 days
// wide, where a lunation's search would sample the whole month: a day and a
// half for a two-day crescent.
func moonAgeHours(t time.Time, prov eph.Provider) (float64, error) {
	elong, err := moonElongation(t, prov)
	if err != nil {
		return 0, fmt.Errorf("crescent: %w", err)
	}

	from := t.Add(unit.Days(-(elong/minElongationRate + newMoonMargin)))

	to := t.Add(unit.Days(-(elong/maxElongationRate - newMoonMargin)))
	if to.After(t) {
		to = t
	}

	phases, err := MoonPhases(from, to, prov)
	if err != nil {
		return 0, fmt.Errorf("crescent: moon phases: %w", err)
	}

	for _, p := range slices.Backward(phases) {
		if p.Phase == PhaseNewMoon {
			return t.Sub(p.Time).Hours(), nil
		}
	}

	return 0, errNoNewMoon
}

// crescentGeometry is the Sun and Moon as seen from a site at one instant,
// from the geocenter and from the observer.
type crescentGeometry struct {
	sunTopo, moonTopo coord.AltAz
	sunGeo, moonGeo   coord.AltAz

	arclTopo, arclGeo float64 // degrees

	// parallax is the Moon's equatorial horizontal parallax, π, in degrees.
	parallax float64
}

// crescentGeometryAt evaluates the geometry through ctx, which must be
// airless: every criterion here is stated without refraction.
//
// The geocentric directions use the same Context as the topocentric ones,
// with the observer put back on the vector: GeocentricToObserved subtracts the
// observer, so v + observer arrives as v, the direction from the geocenter,
// turned into the site's horizon.
func crescentGeometryAt(ctx *coord.Context, prov eph.Provider) (crescentGeometry, error) {
	t := ctx.Time()

	sun, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return crescentGeometry{}, fmt.Errorf("crescent: sun position: %w", err)
	}

	moon, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return crescentGeometry{}, fmt.Errorf("crescent: moon position: %w", err)
	}

	obs := ctx.ObsVec()

	return crescentGeometry{
		sunTopo:  ctx.GeocentricToObserved(sun),
		moonTopo: ctx.GeocentricToObserved(moon),
		sunGeo:   ctx.GeocentricToObserved(sun.Add(obs)),
		moonGeo:  ctx.GeocentricToObserved(moon.Add(obs)),
		arclTopo: vectorAngleDeg(sun.Sub(obs), moon.Sub(obs)),
		arclGeo:  vectorAngleDeg(sun, moon),
		parallax: math.Asin(constants.WGS84.SemiMajorAxis.Value/1e3/(moon.Norm()*kmPerAU)) * 180 / math.Pi,
	}, nil
}

// topocentricSemiDiameterArcmin is Yallop's SD′ = SD(1 + sin h sin π), with
// SD = 0.27245π and h the Moon's geocentric altitude, in arcminutes.
func (g crescentGeometry) topocentricSemiDiameterArcmin() float64 {
	sd := 0.27245 * g.parallax * 60
	h := g.moonGeo.Alt().Radians()

	return sd * (1 + math.Sin(h)*math.Sin(g.parallax*math.Pi/180))
}

// geocentric is Yallop's convention: geocentric ARCV and ARCL, and W′ from
// the geocentric ARCL.
func (g crescentGeometry) geocentric(lagMinutes, age float64) CrescentParams {
	return CrescentParams{
		ArcV: g.moonGeo.Alt().Degrees() - g.sunGeo.Alt().Degrees(),
		ArcL: g.arclGeo,
		DAZ:  azimuthGapDeg(g.moonGeo, g.sunGeo),
		MAlt: g.moonGeo.Alt().Degrees(),
		W:    g.topocentricSemiDiameterArcmin() * (1 - math.Cos(g.arclGeo*math.Pi/180)),
		LT:   lagMinutes,
		Age:  age,
	}
}

// topocentric is Odeh's convention: everything from the observer.
func (g crescentGeometry) topocentric(lagMinutes, age float64) CrescentParams {
	return CrescentParams{
		ArcV: g.moonTopo.Alt().Degrees() - g.sunTopo.Alt().Degrees(),
		ArcL: g.arclTopo,
		DAZ:  azimuthGapDeg(g.moonTopo, g.sunTopo),
		MAlt: g.moonTopo.Alt().Degrees(),
		W:    g.topocentricSemiDiameterArcmin() * (1 - math.Cos(g.arclTopo*math.Pi/180)),
		LT:   lagMinutes,
		Age:  age,
	}
}

// sunset is the set read at sunset: topocentric altitudes and azimuths, and
// the geocentric elongation, with W from it.
//
// That is MABIMS's convention, whose altitude is topocentric and elongation
// geocentric. It is also what NewCrescentParams gave every criterion, so the
// criteria whose conventions are not yet verified read what they always did,
// but for W, which takes the Moon's semi-diameter at the time rather than a
// constant 15.5′.
func (g crescentGeometry) sunset(lagMinutes, age float64) CrescentParams {
	p := g.topocentric(lagMinutes, age)
	p.ArcL = g.arclGeo
	p.W = g.topocentricSemiDiameterArcmin() * (1 - math.Cos(g.arclGeo*math.Pi/180))

	return p
}

// azimuthGapDeg is the unsigned difference in azimuth, in [0, 180].
func azimuthGapDeg(a, b coord.AltAz) float64 {
	d := math.Abs(a.Az().Degrees() - b.Az().Degrees())
	if d > 180 {
		d = 360 - d
	}

	return d
}

// vectorAngleDeg is the angle between two vectors, by atan2 so that it stays
// accurate near 0° and 180° where acos of a dot product does not.
func vectorAngleDeg(a, b vector.Vec3) float64 {
	return math.Atan2(a.Cross(b).Norm(), a.Dot(b)) * 180 / math.Pi
}
