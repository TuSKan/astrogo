package plan

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/TuSKan/astrogo/angle"
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
// can feed all of them (#496, #503). crescent.go's header tables each one's
// quantities and instant; this file computes them. Yallop's are the model for
// the rest (1997, NAO Technical Note 69): ARCV the geocentric, airless
// difference in altitude of the centers of Sun and Moon; ARCL geocentric; the
// crescent width W′ = SD′(1 − cos ARCL) with SD = 0.27245π and SD′ =
// SD(1 + sin h sin π), π the Moon's horizontal parallax and h its geocentric
// altitude (his eqs. 3.8–3.10); at the best time Tb = Ts + (4/9)·Lag (eq. 4.1).
//
// An evening is evaluated at four instants: geometric sunset, the almanac's
// sunset, Yallop's best time and Qureshi's. One coord.Context is built, at
// sunset, and moved to the others with SetTime, which holds it to ≲0.1″
// against these criteria's tenths of a degree. The Sun and Moon are
// each looked up once per instant, and every criterion at that instant reads
// the same two lookups.
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
// Each criterion's answer carries the quantities it read, in its own
// convention. A nil provider means eph.Default().
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

	ctx := coord.NewContext(sunset, site.Location(), atmosphere.Refraction{})

	geometric, err := geometricSunset(ctx, sunset, site, prov)
	if err != nil {
		return CrescentResult{}, err
	}

	moonset, err := moonsetFor(sunset, site, prov)
	if err != nil {
		return CrescentResult{}, err
	}

	lag := moonset.Sub(sunset)

	// The best times. A Moon that set before the Sun has no best time after
	// sunset, and is evaluated at sunset, where every criterion that reads
	// altitude already says no.
	best, qureshiBest := sunset, sunset
	if lag > 0 {
		best = sunset.Add(lag * 4 / 9)
		qureshiBest = sunset.Add(lag * 43 / 93)
	}

	age, err := moonAgeHours(sunset, prov)
	if err != nil {
		return CrescentResult{}, err
	}

	var gGeometric, gSunset, gBest, gQureshi crescentGeometry

	for _, at := range []struct {
		g *crescentGeometry
		t time.Time
	}{{&gGeometric, geometric}, {&gSunset, sunset}, {&gBest, best}, {&gQureshi, qureshiBest}} {
		ctx.SetTime(at.t)

		if *at.g, err = crescentGeometryAt(ctx, prov); err != nil {
			return CrescentResult{}, err
		}
	}

	lagMinutes := lag.Minutes()
	ageAt := func(t time.Time) float64 { return age + t.Sub(sunset).Hours() }

	atGeometric := gGeometric.geocentric(lagMinutes, ageAt(geometric))
	topocentric := gSunset.topocentric(lagMinutes, age)
	yallop := gBest.geocentric(lagMinutes, ageAt(best))

	r := CrescentResult{
		Sunset: sunset, Moonset: moonset, GeometricSunset: geometric,
		BestTime: best, QureshiBestTime: qureshiBest,
	}

	verdict := func(p CrescentParams, criterion func(*CrescentParams) bool) CrescentVerdict {
		return CrescentVerdict{Visible: criterion(&p), Params: p}
	}

	r.Fotheringham = verdict(atGeometric, (*CrescentParams).Fotheringham)
	r.Maunder = verdict(atGeometric, (*CrescentParams).Maunder)
	r.Ilyas1988 = verdict(atGeometric, (*CrescentParams).Ilyas1988)
	r.KraussAthenian = verdict(atGeometric, (*CrescentParams).KraussAthenian)
	r.Ilyas1983 = verdict(atGeometric, (*CrescentParams).Ilyas1983)

	r.Danjon = verdict(topocentric, (*CrescentParams).Danjon)
	r.Fatoohi1998 = verdict(topocentric, (*CrescentParams).Fatoohi1998)

	mabims := gSunset.sunset(lagMinutes, age)
	r.MABIMS1995 = verdict(mabims, (*CrescentParams).MABIMS1995)
	r.MABIMS2021 = verdict(mabims, (*CrescentParams).MABIMS2021)
	r.Istanbul2016 = verdict(mabims, (*CrescentParams).Istanbul2016)

	alrefay := topocentric
	alrefay.W = crescentWidth(16, alrefay.ArcL)
	r.AlrefayNakedEye = verdict(alrefay, (*CrescentParams).AlrefayNakedEye)
	r.AlrefayOpticalAid = verdict(alrefay, (*CrescentParams).AlrefayOpticalAid)

	saao := topocentric
	saao.MAlt = apparentLowerLimb(gSunset, site.Refraction())
	r.CaldwellNakedEye = verdict(saao, (*CrescentParams).CaldwellNakedEye)
	r.CaldwellOptical = verdict(saao, (*CrescentParams).CaldwellOptical)

	bruin := yallop
	bruin.W = crescentWidth(15, bruin.ArcL)
	r.Bruin = verdict(bruin, (*CrescentParams).Bruin)

	odeh := gBest.topocentric(lagMinutes, ageAt(best))
	qureshi := gQureshi.geocentric(lagMinutes, ageAt(qureshiBest))

	r.Yallop = yallop.Yallop()
	r.Odeh = odeh.Odeh()
	r.Qureshi = qureshi.Qureshi()

	return r, nil
}

// geometricSunset is the instant before sunset when the Sun's center was on
// the geometric horizon, its geocentric altitude 0 — the "sunset" of
// Fotheringham, Maunder, Ilyas and Krauss, a few minutes before the almanac's.
//
// Newton's method from sunset converges in three or four steps, each two
// lookups of the Sun through ctx, which it moves. Where the Sun sinks too
// slowly for it, near the pole, the event solver searches the day before
// sunset instead; it reads the Sun's topocentric altitude, off the geocentric
// by the solar parallax, 8.8″.
func geometricSunset(ctx *coord.Context, sunset time.Time, site *Site, prov eph.Provider) (time.Time, error) {
	alt := func(t time.Time) (float64, error) {
		sun, err := eph.Position(prov, eph.Sun, t)
		if err != nil {
			return 0, fmt.Errorf("crescent: sun position: %w", err)
		}

		ctx.SetTime(t)

		return ctx.GeocentricToObserved(sun.Add(ctx.ObsVec())).Alt().Degrees(), nil
	}

	t := sunset

	for range 8 {
		h, err := alt(t)
		if err != nil {
			return time.Time{}, err
		}

		later, err := alt(t.Add(unit.Seconds(30)))
		if err != nil {
			return time.Time{}, err
		}

		rate := (later - h) / 30 // degrees a second
		if rate > -1e-4 {
			break
		}

		step := -h / rate
		t = t.Add(unit.Seconds(step))

		if t.Sub(sunset).Hours() < -1 || t.After(sunset) {
			break
		}

		if math.Abs(step) < 0.05 {
			return t, nil
		}
	}

	solver := NewEventSolver(unit.Minutes(15), unit.Seconds(1))
	spec := EventSpec{Family: EventFamilyVisibility, Kind: EventSet, Target: NewSun(prov), Observer: site}

	sets := func(start, end time.Time, _ *Site, _ eph.Provider) ([]Event, error) {
		return solver.Find(spec, start, end)
	}

	e, found, err := lastEvent(sets, sunset, 1, site, prov, isSet)
	if err != nil {
		return time.Time{}, err
	}

	if !found {
		return time.Time{}, errNoSunset
	}

	return e.Time, nil
}

// apparentLowerLimb is the apparent altitude of the Moon's lower limb, in
// degrees: its airless topocentric altitude, less the topocentric
// semi-diameter SD′, raised by refraction at the site's pressure and
// temperature.
//
// The refraction is the site's own, which holds to the horizon, where these
// crescents are. Until #588 it had to be forced to Saemundsson's formula: the
// default then held SOFA's series at about 10′ below 3° of altitude, and
// would have put every lower limb near the horizon a third of a degree low.
func apparentLowerLimb(g crescentGeometry, site atmosphere.Refraction) float64 {
	limb := angle.Deg(g.moonTopo.Alt().Degrees() - g.topocentricSemiDiameterArcmin()/60)

	return limb.Degrees() + site.RefractFromTrue(limb).Degrees()
}

// crescentWidth is SD(1 − cos ARCL), in arcminutes, for a constant
// semi-diameter sd in arcminutes, as Bruin (15′) and Alrefay (16′) take it.
func crescentWidth(sd, arclDeg float64) float64 {
	return sd * (1 - math.Cos(arclDeg*math.Pi/180))
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
// geocentric, and Istanbul 2016's as this package reads it. W takes the
// Moon's semi-diameter at the time.
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
