package plan

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// TestEclipseGeometryReproducesNASA2026 evaluates the geometry at the instants
// of greatest eclipse NASA's Five Millennium Canons give for 2026
// (LEcat5/LE2001-2100.html, SEcat5/SE2001-2100.html) and compares it with the
// quantities the canons print: penumbral magnitude for the lunar eclipses,
// gamma — the shadow axis's distance from Earth's center, in Earth radii — for
// the solar ones.
//
// It also finds each eclipse and checks greatest eclipse against the canon's
// time. That check is the one that sees which Sun the shadow axis is drawn
// from: the geometric Sun instead of the apparent one moves greatest eclipse
// by about 0.8 minutes, the time the Moon takes to cross the Sun's 20″ of
// aberration.
//
// On eph.Default, the analytical ephemeris, not DE441, which is what the
// tolerances allow for. Measured here: magnitude 0.0006, gamma 0.0001, time
// 0.2 minutes at worst. Against DE441 over six centuries the same geometry
// agrees to 0.0005 in magnitude and 0.0002 in gamma, and greatest eclipse to
// 0.55 minutes.
func TestEclipseGeometryReproducesNASA2026(t *testing.T) {
	const (
		tolMagnitude = 0.002
		tolGamma     = 0.0005
		tolMinutes   = 0.4
	)

	prov := eph.Default()

	lunar := []struct {
		name      string
		td        time.Time
		magnitude float64
	}{
		{"2026-03-03 total", time.Date(2026, 3, 3, 11, 34, 52, 0, time.LocationUTC), 2.1838},
		{"2026-08-28 partial", time.Date(2026, 8, 28, 4, 14, 4, 0, time.LocationUTC), 1.9645},
	}

	for _, c := range lunar {
		td := time.FromJD(c.td.JD(), time.TDB)

		g, err := newEclipseGeometry(td, prov)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		got, _ := g.lunarPenumbralMagnitude()
		if math.Abs(got-c.magnitude) > tolMagnitude {
			t.Errorf("%s: penumbral magnitude %.4f, NASA gives %.4f", c.name, got, c.magnitude)
		}

		checkGreatestEclipse(t, c.name, LunarEclipses, td, prov, tolMinutes)
	}

	solar := []struct {
		name  string
		td    time.Time
		gamma float64
	}{
		{"2026-02-17 annular", time.Date(2026, 2, 17, 12, 13, 6, 0, time.LocationUTC), 0.9743},
		{"2026-08-12 total", time.Date(2026, 8, 12, 17, 47, 6, 0, time.LocationUTC), 0.8977},
	}

	for _, c := range solar {
		td := time.FromJD(c.td.JD(), time.TDB)

		g, err := newEclipseGeometry(td, prov)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if got := g.solarAxisDistance(); math.Abs(got-c.gamma) > tolGamma {
			t.Errorf("%s: |gamma| %.4f, NASA gives %.4f", c.name, got, c.gamma)
		}

		checkGreatestEclipse(t, c.name, SolarEclipses, td, prov, tolMinutes)
	}
}

// checkGreatestEclipse finds the one eclipse within three days of td and
// checks its time against td.
func checkGreatestEclipse(t *testing.T, name string, find func(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error),
	td time.Time, prov eph.Provider, tolMinutes float64,
) {
	t.Helper()

	events, err := find(td.Add(unit.Days(-3)), td.Add(unit.Days(3)), prov)
	if err != nil || len(events) != 1 {
		t.Errorf("%s: %d eclipses within three days, err %v; want 1", name, len(events), err)

		return
	}

	if off := events[0].Time.Sub(td).Minutes(); math.Abs(off) > tolMinutes {
		t.Errorf("%s: greatest eclipse %+.2f minutes from NASA's, limit %.1f", name, off, tolMinutes)
	}
}

// TestSolarPenumbraMarginSeesTheFlattenedLimb puts the shadow axis the same
// distance from Earth's center twice, once toward the pole and once along the
// equator, and requires the margins to differ by Earth's flattening: the limb
// toward the pole is b/a of the way out. Over the six centuries compared with
// NASA's canon, a spherical Earth reports five New Moons as eclipses that the
// canon, and the spheroid, do not have: penumbrae grazing past a polar limb
// that is not there.
func TestSolarPenumbraMarginSeesTheFlattenedLimb(t *testing.T) {
	const moonKm, offsetKm = 384400.0, 9000.0

	// At J2000 the true pole is GCRS z to within the frame bias and a few
	// arcseconds of nutation, so the axis along x has declination ~0 and the
	// outline's short axis is z.
	at := time.J2000()
	sun := vector.Vec3{X: -auKm}

	toPole, _ := eclipseGeometry{sun: sun, moon: vector.Vec3{X: -moonKm, Z: offsetKm}}.solarPenumbraMargin(at)
	toEquator, _ := eclipseGeometry{sun: sun, moon: vector.Vec3{X: -moonKm, Y: offsetKm}}.solarPenumbraMargin(at)

	f := constants.Derived.WGS84Flattening.Value
	if got := toEquator - toPole; math.Abs(got-f) > 1e-6 {
		t.Errorf("margin toward the equator exceeds the one toward the pole by %.7f Earth radii, want the flattening %.7f",
			got, f)
	}
}

// TestEclipseLatitudeScreenCoversTheWidestShadow checks the screen against
// the widest limit either kind of eclipse can reach: the Moon at its closest
// perigee, 356,400 km, and Earth at perihelion, 0.98329 AU, which make the
// parallax, and so the shadow, the Moon and the penumbra's reach, their
// largest. The screen applies to latitude at the syzygy, which is the Moon's
// closest approach to the axis over the cosine of its path's tilt to the
// ecliptic: 5.145° of orbital inclination, steepened a little by Earth's own
// motion, and taken here as 6°.
//
// A screen inside that limit drops eclipses before the geometry ever sees
// them, which is the fixed limit's failure over again, by another name.
func TestEclipseLatitudeScreenCoversTheWidestShadow(t *testing.T) {
	const (
		perigeeKm   = 356400.0
		perihelion  = 0.98329 // AU
		maxTiltDeg  = 6.0
		radToDegree = 180 / math.Pi
	)

	moonParallax := math.Asin(earthEquatorialRadiusKm / perigeeKm)
	sunParallax := math.Asin(earthEquatorialRadiusKm / (perihelion * auKm))
	sunSemiDiameter := math.Asin(math.Sin(sunSemiDiameterAt1AU) / perihelion)
	moonSemiDiameter := math.Asin(moonRadiusRatio * earthEquatorialRadiusKm / perigeeKm)

	widest := map[string]float64{
		"lunar": danjonParallaxFactor*moonParallax + sunSemiDiameter + sunParallax + moonSemiDiameter,
		"solar": moonParallax - sunParallax + sunSemiDiameter + moonSemiDiameter,
	}

	for kind, limit := range widest {
		atSyzygy := limit / math.Cos(maxTiltDeg/radToDegree) * radToDegree
		if eclipseLatitudeScreen <= atSyzygy {
			t.Errorf("%s: the screen, %.2f°, is inside the widest possible limit, %.3f° at the syzygy",
				kind, float64(eclipseLatitudeScreen), atSyzygy)
		}
	}
}

// TestEclipsesAreDecidedByTheShadow pins three syzygies the fixed 1.58°
// latitude limit decided wrongly (#401), each confirmed against NASA's
// canons: a penumbral eclipse it missed, and a Full Moon and a New Moon it
// reported as eclipses that NASA does not list and whose shadows, sized that
// month, miss by a clear margin.
func TestEclipsesAreDecidedByTheShadow(t *testing.T) {
	prov := eph.Default()

	cases := []struct {
		name string
		find func(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error)
		at   time.Time
		want bool
	}{
		// NASA: 09553, type Ne, penumbral magnitude 0.0135, greatest eclipse
		// 04:00:15 TD. Latitude at the syzygy 1.580°, just past the old limit.
		{"1958-04-04 penumbral lunar eclipse", LunarEclipses, time.Date(1958, 4, 4, 4, 0, 15, 0, time.LocationUTC), true},
		// Latitude 1.546°: inside the old limit, outside the penumbra by 0.12
		// of the Moon's diameter.
		{"1987-03-15 Full Moon", LunarEclipses, time.Date(1987, 3, 15, 13, 31, 0, 0, time.LocationUTC), false},
		// Latitude 1.533°: inside the old limit, the penumbra 0.11 Earth
		// radii short of the limb.
		{"1978-09-02 New Moon", SolarEclipses, time.Date(1978, 9, 2, 16, 28, 0, 0, time.LocationUTC), false},
	}

	for _, c := range cases {
		at := time.FromJD(c.at.JD(), time.TDB)

		events, err := c.find(at.Add(unit.Days(-10)), at.Add(unit.Days(10)), prov)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if got := len(events) == 1; got != c.want || len(events) > 1 {
			t.Errorf("%s: %d eclipses found within 10 days, want an eclipse: %v", c.name, len(events), c.want)

			continue
		}

		if !c.want {
			continue
		}

		e := events[0]
		if off := math.Abs(e.Time.Sub(at).Minutes()); off > 0.4 {
			t.Errorf("%s: greatest eclipse at %v, %.2f minutes from NASA's", c.name, e.Time, off)
		}

		if e.Gamma <= 0.9 || e.Gamma >= 1 {
			t.Errorf("%s: Gamma %.4f, want just under 1 for an eclipse of magnitude 0.0135", c.name, e.Gamma)
		}
	}
}

// TestEclipseSearchReturnsEveryProviderFailure fails the provider on each of
// its calls in turn, from the first one after the phase search to the last one
// the eclipse search makes, and requires the failure back every time.
//
// Before #401 a failure mid-search was not returned: a latitude that could not
// be computed skipped the syzygy, and a search that failed fell back to the
// syzygy's own time. Either way an eclipse could go missing with a nil error.
func TestEclipseSearchReturnsEveryProviderFailure(t *testing.T) {
	// Each window holds one eclipse, the first of 2026 of each kind.
	cases := []struct {
		name       string
		find       func(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error)
		start, end time.Time
	}{
		{"lunar", LunarEclipses, time.Date(2026, 2, 25, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 3, 10, 0, 0, 0, 0, time.LocationUTC)},
		{"solar", SolarEclipses, time.Date(2026, 2, 10, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 2, 24, 0, 0, 0, 0, time.LocationUTC)},
	}

	for _, c := range cases {
		counting := &failingAfterProvider{Provider: eph.Default(), limit: math.MaxInt}

		if _, err := MoonPhases(c.start, c.end, counting); err != nil {
			t.Fatalf("%s: MoonPhases: %v", c.name, err)
		}

		phaseCalls := counting.calls
		counting.calls = 0

		events, err := c.find(c.start, c.end, counting)
		if err != nil || len(events) != 1 {
			t.Fatalf("%s: %d eclipses, err %v; want the one eclipse in the window", c.name, len(events), err)
		}

		total := counting.calls

		for limit := phaseCalls; limit < total; limit++ {
			prov := &failingAfterProvider{Provider: eph.Default(), limit: limit}

			if _, err := c.find(c.start, c.end, prov); !errors.Is(err, errFailingAfter) {
				t.Errorf("%s: provider failing after %d of %d calls: err %v, want errFailingAfter",
					c.name, limit, total, err)
			}
		}
	}
}

var errFailingAfter = errors.New("failingAfterProvider: out of calls")

// failingAfterProvider answers its first limit State calls and fails every
// one after.
type failingAfterProvider struct {
	eph.Provider

	calls, limit int
}

func (p *failingAfterProvider) State(id eph.ID, t time.Time) (core.State, error) {
	p.calls++
	if p.calls > p.limit {
		return core.State{}, errFailingAfter
	}

	st, err := p.Provider.State(id, t)
	if err != nil {
		return core.State{}, fmt.Errorf("failingAfterProvider: %w", err)
	}

	return st, nil
}
