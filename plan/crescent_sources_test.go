package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Each criterion's convention is held to numbers its own source computed
// (#503). A convention in the wrong frame or at the wrong instant misses
// these by most of a degree: the Moon's parallax is near 1°, and it sinks
// about 1° in the five minutes between geometric and almanac sunset.

// Fotheringham (1910), pp. 528–530: the Moon's "true altitude at sunset",
// computed without parallax, and the difference in azimuth, for J. Schmidt's
// evening observations at Athens. Computed by hand in 1910, they agree with
// the geocentric altitude at geometric sunset to 0.25°.
func TestFotheringhamTableIsGeocentricAtGeometricSunset(t *testing.T) {
	t.Parallel()

	athens := crescentSite(t, 37.972, 23.718)
	prov := eph.Default()

	for _, o := range []struct {
		no       int
		y, m, d  int
		alt, daz float64
	}{
		{1, 1859, 7, 1, 12.7, 10.0}, {2, 1859, 10, 27, 6.1, 20.5}, {3, 1860, 1, 23, 6.0, 3.2},
		{4, 1860, 2, 23, 20.2, 2.4}, {5, 1860, 6, 20, 15.4, 12.5}, {6, 1861, 3, 12, 12.8, 1.8},
		{7, 1861, 8, 7, 5.2, 15.2}, {9, 1861, 9, 7, 12.9, 36.4}, {10, 1861, 10, 5, 5.3, 19.4},
		{12, 1861, 12, 3, 13.6, 16.6}, {14, 1862, 3, 31, 15.9, 0.4}, {15, 1862, 4, 29, 8.6, 0.1},
		{17, 1864, 1, 10, 18.4, 6.5}, {18, 1864, 3, 9, 21.1, 2.3},
	} {
		r, err := CrescentVisibility(time.Date(o.y, time.Month(o.m), o.d, 10, 0, 0, 0, time.LocationUTC), athens, prov)
		if err != nil {
			t.Fatalf("No. %d: %v", o.no, err)
		}

		p := r.Fotheringham.Params
		if math.Abs(p.MAlt-o.alt) > 0.25 || math.Abs(p.DAZ-o.daz) > 0.25 {
			t.Errorf("No. %d: altitude %.2f°, DAZ %.2f°; Fotheringham has %.1f°, %.1f°", o.no, p.MAlt, p.DAZ, o.alt, o.daz)
		}

		for _, v := range []CrescentVerdict{r.Maunder, r.Ilyas1988, r.KraussAthenian, r.Ilyas1983} {
			if v.Params != p {
				t.Errorf("No. %d: the azimuth–altitude criteria read different quantities: %+v, %+v", o.no, v.Params, p)
			}
		}
	}
}

// Fatoohi, Stephenson & Al-Dargazelli (1998), Table I: elongations at sunset
// "allowing for parallax". They agree with the topocentric elongation to
// 0.07°; the geocentric one is 0.8°–1.0° larger.
func TestFatoohiTableIsTopocentricAtSunset(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, o := range []struct {
		row          int
		y, m, d      int
		lonW, lat, h float64
		elong        float64
	}{
		{3, 1989, 6, 3, 155.5, 19.8, 4255, 6.1}, {6, 1988, 4, 16, 84.1, 37.2, 305, 6.6},
		{11, 1990, 2, 25, 83.5, 35.6, 1524, 7.5}, {16, 1985, 4, 20, 84.1, 37.2, 305, 7.8},
		{22, 1995, 1, 1, 106.0, 33.0, 1219, 7.9}, {23, 1989, 5, 5, 84.8, 42.7, 259, 8.1},
	} {
		loc, err := coord.NewGeodetic(angle.Deg(-o.lonW), angle.Deg(o.lat), unit.Meters(o.h))
		if err != nil {
			t.Fatalf("NewGeodetic: %v", err)
		}

		site, err := NewSite("fatoohi", loc)
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}

		r, err := CrescentVisibility(time.Date(o.y, time.Month(o.m), o.d, 18, 0, 0, 0, time.LocationUTC), site, prov)
		if err != nil {
			t.Fatalf("row %d: %v", o.row, err)
		}

		if got := r.Fatoohi1998.Params.ArcL; math.Abs(got-o.elong) > 0.07 {
			t.Errorf("row %d: elongation %.3f°, Fatoohi's Table I has %.1f°", o.row, got, o.elong)
		}

		if r.Danjon.Params != r.Fatoohi1998.Params {
			t.Errorf("row %d: Danjon and Fatoohi read different quantities", o.row)
		}
	}
}

// Alrefay et al. (2018), Table I: Saudi sightings with the age and lag at
// sunset and the arcs. Their arcs of vision match the topocentric ones at
// sunset to 0.15°, and the age and lag match too, so the sunset and moonset
// they measured from are the ones used here.
func TestAlrefayTableIIsTopocentricAtSunset(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, o := range []struct {
		lat, lon  float64
		y, m, d   int
		age, lag  float64 // hours, minutes
		arcv, daz float64
	}{
		{23.55, 46.39, 1988, 3, 19, 37.05, 93, 20.34, 2.55},
		{19.52, 42.22, 1988, 4, 17, 27.49, 65, 14.15, 4.49},
		{28.40, 36.73, 1989, 3, 8, 21.31, 56, 11.90, 1.11},
		{28.40, 36.73, 1990, 2, 26, 30.61, 77, 16.36, 1.33},
		{28.40, 36.73, 2003, 9, 27, 36.26, 65, 13.67, 15.07},
		{24.54, 39.63, 2004, 6, 18, 19.76, 46, 9.07, 0.16},
		{21.42, 39.83, 2003, 9, 27, 36.06, 68, 15.30, 13.12},
	} {
		site := crescentSite(t, o.lat, o.lon)

		r, err := CrescentVisibility(time.Date(o.y, time.Month(o.m), o.d, 9, 0, 0, 0, time.LocationUTC), site, prov)
		if err != nil {
			t.Fatalf("%d-%02d-%02d: %v", o.y, o.m, o.d, err)
		}

		p := r.AlrefayNakedEye.Params
		if math.Abs(p.ArcV-o.arcv) > 0.16 || math.Abs(p.DAZ-o.daz) > 0.15 ||
			math.Abs(p.Age-o.age) > 0.02 || math.Abs(p.LT-o.lag) > 1.5 {
			t.Errorf("%d-%02d-%02d at %.2f° %.2f°: ARCV %.2f°, DAZ %.2f°, age %.2f h, lag %.1f min; Alrefay's Table I has %.2f°, %.2f°, %.2f h, %.0f min",
				o.y, o.m, o.d, o.lat, o.lon, p.ArcV, p.DAZ, p.Age, p.LT, o.arcv, o.daz, o.age, o.lag)
		}

		if want := 16 * (1 - math.Cos(p.ArcL*math.Pi/180)); math.Abs(p.W-want) > 1e-12 {
			t.Errorf("Alrefay's W = %.4f′, want 16′(1 − cos ARCL) = %.4f′", p.W, want)
		}
	}
}

// Geometric sunset is the Sun's center on the horizon, to the event solver's
// 1 s, in which the Sun sinks 0.004°, and the solar parallax, 8.8″ or
// 0.0024°: a hundredth of a degree in all, where the Moon's altitude is read
// to tenths.
func TestGeometricSunsetIsTheSunOnTheHorizon(t *testing.T) {
	t.Parallel()

	site, evening := portOfSpainEvening(t)
	prov := eph.Default()

	r, err := CrescentVisibility(evening, site, prov)
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	g, err := crescentGeometryAt(coord.NewContext(r.GeometricSunset, site.Location(), atmosphere.Refraction{}), prov)
	if err != nil {
		t.Fatalf("crescentGeometryAt: %v", err)
	}

	if alt := g.sunGeo.Alt().Degrees(); math.Abs(alt) > 0.01 {
		t.Errorf("the Sun's geocentric altitude at geometric sunset is %.4f°", alt)
	}

	if early := r.Sunset.Sub(r.GeometricSunset).Minutes(); early < 2 || early > 6 {
		t.Errorf("geometric sunset %.2f min before sunset, want a few minutes", early)
	}
}

// Where the Sun sinks too slowly for Newton's method from sunset, the event
// solver finds geometric sunset instead: at Longyearbyen on 2025-04-17, two
// days before the midnight Sun, it comes an hour before the almanac's, the
// Sun's center 0.83° above the horizon at sunset falling 0.0001° a second.
// At midday under the midnight Sun nothing set the day before, and that is
// reported.
func TestGeometricSunsetWhereTheSunSetsSlowly(t *testing.T) {
	t.Parallel()

	site := crescentSite(t, 78.22, 15.65)
	prov := eph.Default()

	e, found, err := firstEvent(SunEvents, time.Date(2025, 4, 17, 10, 0, 0, 0, time.LocationUTC), 1, site, prov, isSet)
	if err != nil || !found {
		t.Fatalf("sunset: found %v, err %v", found, err)
	}

	ctx := coord.NewContext(e.Time, site.Location(), atmosphere.Refraction{})

	geometric, err := geometricSunset(ctx, e.Time, site, prov)
	if err != nil {
		t.Fatalf("geometricSunset: %v", err)
	}

	g, err := crescentGeometryAt(ctx.AtTime(geometric), prov)
	if err != nil {
		t.Fatalf("crescentGeometryAt: %v", err)
	}

	if alt, early := g.sunGeo.Alt().Degrees(), e.Time.Sub(geometric).Minutes(); math.Abs(alt) > 0.01 || early < 50 || early > 70 {
		t.Errorf("geometric sunset %.1f min before sunset, the Sun's center at %.4f°", early, alt)
	}

	noon := time.Date(2025, 6, 21, 11, 0, 0, 0, time.LocationUTC)
	if _, err := geometricSunset(coord.NewContext(noon, site.Location(), atmosphere.Refraction{}), noon, site, prov); !errors.Is(err, errNoSunset) {
		t.Errorf("under the midnight Sun: err = %v, want errNoSunset", err)
	}
}

// A failed lookup of the Sun in the search for geometric sunset is
// reported: either of Newton's two, and one in the solver's fallback.
func TestGeometricSunsetReportsAFailedLookup(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, c := range []struct {
		name string
		site *Site
		day  time.Time
		// fails picks the failing lookups, given the sunset.
		fails func(sunset, at time.Time) bool
	}{
		{"at sunset", crescentSite(t, 10.65, -61.52), time.Date(2025, 3, 29, 16, 0, 0, 0, time.LocationUTC),
			func(sunset, at time.Time) bool { return at.Equal(sunset) }},
		{"half a minute on", crescentSite(t, 10.65, -61.52), time.Date(2025, 3, 29, 16, 0, 0, 0, time.LocationUTC),
			func(sunset, at time.Time) bool { return at.Equal(sunset.Add(unit.Seconds(30))) }},
		{"in the fallback", crescentSite(t, 78.22, 15.65), time.Date(2025, 4, 17, 10, 0, 0, 0, time.LocationUTC),
			func(sunset, at time.Time) bool { return at.Before(sunset.Add(unit.Minutes(-70))) }},
	} {
		e, found, err := firstEvent(SunEvents, c.day, 1, c.site, prov, isSet)
		if err != nil || !found {
			t.Fatalf("%s: sunset: found %v, err %v", c.name, found, err)
		}

		failing := failingWhenProvider{Provider: prov, fails: func(id eph.ID, at time.Time) bool {
			return id == eph.Sun && c.fails(e.Time, at)
		}}

		ctx := coord.NewContext(e.Time, c.site.Location(), atmosphere.Refraction{})
		if _, err := geometricSunset(ctx, e.Time, c.site, failing); !errors.Is(err, errFailingWhen) {
			t.Errorf("%s: err = %v, want the lookup's error", c.name, err)
		}
	}

	// Through CrescentVisibility: the sunset search never looks half a minute
	// past the sunset it finds, Newton's method does.
	site, evening := portOfSpainEvening(t)

	r, err := CrescentVisibility(evening, site, prov)
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	failing := failingWhenProvider{Provider: prov, fails: func(id eph.ID, at time.Time) bool {
		return id == eph.Sun && at.Equal(r.Sunset.Add(unit.Seconds(30)))
	}}

	if _, err := CrescentVisibility(evening, site, failing); !errors.Is(err, errFailingWhen) {
		t.Errorf("CrescentVisibility: err = %v, want the lookup's error", err)
	}
}

// The SAAO criterion reads the apparent altitude of the lower limb: the
// airless topocentric limb raised by refraction. The site's refraction is
// 0.185° at Port of Spain's 4°, and 0.46° on the horizon at Quinta Calixto,
// 835 m up, where SOFA's series alone, clamped, gave 0.17° until #588 and
// would have put the limb 0.3° low.
func TestSAAOReadsTheApparentLowerLimb(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	portOfSpain, portOfSpainEvening := portOfSpainEvening(t)

	calixto, err := NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	for _, c := range []struct {
		name    string
		site    *Site
		evening time.Time
		lo, hi  float64 // refraction, degrees
	}{
		{"Port of Spain, 4° up", portOfSpain, portOfSpainEvening, 0.15, 0.22},
		{"Quinta Calixto, on the horizon", calixto, time.Date(2026, 10, 10, 16, 0, 0, 0, time.LocationUTC), 0.40, 0.50},
	} {
		r, err := CrescentVisibility(c.evening, c.site, prov)
		if err != nil {
			t.Fatalf("%s: CrescentVisibility: %v", c.name, err)
		}

		g, err := crescentGeometryAt(coord.NewContext(r.Sunset, c.site.Location(), atmosphere.Refraction{}), prov)
		if err != nil {
			t.Fatalf("%s: crescentGeometryAt: %v", c.name, err)
		}

		airlessLimb := g.moonTopo.Alt().Degrees() - g.topocentricSemiDiameterArcmin()/60
		limb := r.CaldwellNakedEye.Params.MAlt

		if refraction := limb - airlessLimb; refraction < c.lo || refraction > c.hi {
			t.Errorf("%s: apparent lower limb %.3f°, airless %.3f°: refraction %.3f°, want %.2f°–%.2f°",
				c.name, limb, airlessLimb, refraction, c.lo, c.hi)
		}
	}
}
