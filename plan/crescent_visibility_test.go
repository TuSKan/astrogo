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
)

// crescentSite is a sea-level site for the crescent tests.
func crescentSite(tb testing.TB, latDeg, lonDeg float64) *Site {
	tb.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(lonDeg), angle.Deg(latDeg), 0)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("crescent", loc)
	if err != nil {
		tb.Fatalf("NewSite: %v", err)
	}

	return site
}

// portOfSpainEvening is the evening after the new moon of 2025-03-29 10:58 UT,
// at Port of Spain: a crescent whose geocentric elongation at sunset, 6.87°,
// clears MABIMS 2021's 6.4° while its topocentric one, 5.98°, does not, with
// the Moon 4.3° up. Each margin is at least 0.4°.
func portOfSpainEvening(tb testing.TB) (*Site, time.Time) {
	tb.Helper()

	return crescentSite(tb, 10.65, -61.52), time.Date(2025, 3, 29, 16, 0, 0, 0, time.LocationUTC)
}

func TestCrescentVisibilityReadsTheMABIMSElongationGeocentric(t *testing.T) {
	t.Parallel()

	site, evening := portOfSpainEvening(t)
	prov := eph.Default()

	r, err := CrescentVisibility(evening, site, prov)
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	g, err := crescentGeometryAt(coord.NewContext(r.Sunset, site.Location(), atmosphere.Refraction{}), prov)
	if err != nil {
		t.Fatalf("crescentGeometryAt: %v", err)
	}

	p := r.MABIMS2021.Params

	// The fixture must still straddle the limit for the test to mean anything.
	if p.MAlt < 3.4 || g.arclGeo < 6.8 || g.arclTopo > 6.0 {
		t.Fatalf("fixture moved: altitude %.3f°, geocentric elongation %.3f°, topocentric %.3f°",
			p.MAlt, g.arclGeo, g.arclTopo)
	}

	if math.Abs(p.ArcL-g.arclGeo) > 1e-9 {
		t.Errorf("MABIMS read ArcL = %.6f°, want the geocentric elongation %.6f°, not the topocentric %.6f°",
			p.ArcL, g.arclGeo, g.arclTopo)
	}

	if !r.MABIMS2021.Visible {
		t.Errorf("MABIMS2021 = false at a geocentric elongation of %.3f°", g.arclGeo)
	}

	if !r.MABIMS1995.Visible {
		t.Error("MABIMS1995 = false where MABIMS 2021, the stricter criterion, holds")
	}
}

func TestCrescentVisibilityYallopAndOdehReadTheirOwnConventions(t *testing.T) {
	t.Parallel()

	site, evening := portOfSpainEvening(t)
	prov := eph.Default()

	r, err := CrescentVisibility(evening, site, prov)
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	geocentric, topocentric, atSunset := r.Yallop.Params, r.Odeh.Params, r.MABIMS2021.Params

	if !r.Moonset.After(r.Sunset) {
		t.Fatalf("moonset %v not after sunset %v", r.Moonset, r.Sunset)
	}

	// Yallop's eq. 4.1, Tb = Ts + (4/9)·Lag.
	wantBest := r.Moonset.Sub(r.Sunset).Seconds() * 4 / 9
	if got := r.BestTime.Sub(r.Sunset).Seconds(); math.Abs(got-wantBest) > 1e-3 {
		t.Errorf("best time %.3f s after sunset, want 4/9 of the lag, %.3f s", got, wantBest)
	}

	if got, want := geocentric.Age-atSunset.Age, r.BestTime.Sub(r.Sunset).Hours(); math.Abs(got-want) > 1e-9 {
		t.Errorf("best-time age is %.6f h past the sunset age, want %.6f h", got, want)
	}

	if atSunset.LT != geocentric.LT || atSunset.LT != r.Moonset.Sub(r.Sunset).Minutes() {
		t.Errorf("lag %.3f / %.3f min, want the sunset-to-moonset %.3f min", atSunset.LT, geocentric.LT, r.Moonset.Sub(r.Sunset).Minutes())
	}

	// Qureshi's eq. 6, Tb = Ts + (4.3/9.3)·Lag.
	wantQureshi := r.Moonset.Sub(r.Sunset).Seconds() * 4.3 / 9.3
	if got := r.QureshiBestTime.Sub(r.Sunset).Seconds(); math.Abs(got-wantQureshi) > 1e-3 {
		t.Errorf("Qureshi's best time %.3f s after sunset, want 4.3/9.3 of the lag, %.3f s", got, wantQureshi)
	}

	// The two best-time sets differ by the Moon's parallax in altitude,
	// asin(sin π cos h) with h geocentric, for a spherical Earth. The site is
	// on the ellipsoid, nearer the center than its equatorial radius by up to
	// 0.34% and with its vertical off the geocentric by up to 0.19°, which
	// moves this by under 0.004°.
	g, err := crescentGeometryAt(coord.NewContext(r.BestTime, site.Location(), atmosphere.Refraction{}), prov)
	if err != nil {
		t.Fatalf("crescentGeometryAt: %v", err)
	}

	sinPi := math.Sin(g.parallax * math.Pi / 180)
	want := math.Asin(sinPi*math.Cos(geocentric.MAlt*math.Pi/180)) * 180 / math.Pi

	if got := geocentric.MAlt - topocentric.MAlt; math.Abs(got-want) > 0.005 {
		t.Errorf("geocentric − topocentric altitude = %.4f°, the parallax gives %.4f°", got, want)
	}
}

// The best time's Context comes from sunset's through AtTime, which the coord
// contract holds to ≲0.1″ an hour. This evening's best time is 25 minutes
// after sunset; the criteria read tenths of a degree.
func TestCrescentVisibilityBestTimeContextMatchesAFreshOne(t *testing.T) {
	t.Parallel()

	site, evening := portOfSpainEvening(t)
	prov := eph.Default()

	r, err := CrescentVisibility(evening, site, prov)
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	sunsetCtx := coord.NewContext(r.Sunset, site.Location(), atmosphere.Refraction{})

	cached, err := crescentGeometryAt(sunsetCtx.AtTime(r.BestTime), prov)
	if err != nil {
		t.Fatalf("crescentGeometryAt: %v", err)
	}

	fresh, err := crescentGeometryAt(coord.NewContext(r.BestTime, site.Location(), atmosphere.Refraction{}), prov)
	if err != nil {
		t.Fatalf("crescentGeometryAt: %v", err)
	}

	for _, c := range []struct {
		name      string
		got, want coord.AltAz
	}{
		{"Moon, topocentric", cached.moonTopo, fresh.moonTopo},
		{"Moon, geocentric", cached.moonGeo, fresh.moonGeo},
		{"Sun, topocentric", cached.sunTopo, fresh.sunTopo},
		{"Sun, geocentric", cached.sunGeo, fresh.sunGeo},
	} {
		dAlt := math.Abs(c.got.Alt().Degrees()-c.want.Alt().Degrees()) * 3600
		dAz := math.Abs(c.got.Az().Degrees()-c.want.Az().Degrees()) * 3600

		if dAlt > 0.1 || dAz > 0.1 {
			t.Errorf("%s through AtTime is %.3f″ in altitude, %.3f″ in azimuth from a fresh Context", c.name, dAlt, dAz)
		}
	}
}

func TestCrescentVisibilityMoonSetBeforeTheSun(t *testing.T) {
	t.Parallel()

	// The evening before that new moon: the old Moon sets 35 minutes before
	// the Sun, so there is no best time after sunset.
	site := crescentSite(t, 10.65, -61.52)

	r, err := CrescentVisibility(time.Date(2025, 3, 28, 16, 0, 0, 0, time.LocationUTC), site, eph.Default())
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	if lag := r.MABIMS2021.Params.LT; !r.Moonset.Before(r.Sunset) || lag >= 0 {
		t.Fatalf("moonset %v, sunset %v, lag %.1f min: want the Moon set first", r.Moonset, r.Sunset, lag)
	}

	if !r.BestTime.Equal(r.Sunset) || !r.QureshiBestTime.Equal(r.Sunset) {
		t.Errorf("best times %v and %v, want sunset %v when the Moon set first", r.BestTime, r.QureshiBestTime, r.Sunset)
	}

	if r.Yallop.Code != "F" || r.MABIMS2021.Visible || r.MABIMS1995.Visible {
		t.Errorf("a Moon below the horizon at sunset: Yallop %s, MABIMS 2021 %v, 1995 %v", r.Yallop.Code, r.MABIMS2021.Visible, r.MABIMS1995.Visible)
	}
}

func TestCrescentVisibilityPolarEvenings(t *testing.T) {
	t.Parallel()

	// Longyearbyen: the midnight Sun, and in early March 2025, with the Moon
	// above +12° declination, a Moon that neither rises nor sets.
	site := crescentSite(t, 78.22, 15.65)

	for _, c := range []struct {
		name    string
		evening time.Time
		want    error
	}{
		{"midsummer", time.Date(2025, 6, 21, 11, 0, 0, 0, time.LocationUTC), errNoSunset},
		{"circumpolar Moon", time.Date(2025, 3, 7, 11, 0, 0, 0, time.LocationUTC), errNoMoonset},
	} {
		_, err := CrescentVisibility(c.evening, site, eph.Default())
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
	}
}

// CrescentVisibility looks the Sun and Moon up through eph.Position directly,
// not through NewSun and NewMoon, so it needs its own nil-provider default.
func TestCrescentVisibilityNilProviderIsTheDefault(t *testing.T) {
	t.Parallel()

	site, evening := portOfSpainEvening(t)

	withNil, err := CrescentVisibility(evening, site, nil)
	if err != nil {
		t.Fatalf("CrescentVisibility(nil): %v", err)
	}

	withDefault, err := CrescentVisibility(evening, site, eph.Default())
	if err != nil {
		t.Fatalf("CrescentVisibility(eph.Default()): %v", err)
	}

	if withNil.Yallop != withDefault.Yallop || !withNil.Sunset.Equal(withDefault.Sunset) {
		t.Errorf("nil provider gave Yallop %v at sunset %v, eph.Default() %v at %v",
			withNil.Yallop, withNil.Sunset, withDefault.Yallop, withDefault.Sunset)
	}
}

func BenchmarkCrescentVisibility(b *testing.B) {
	site, evening := portOfSpainEvening(b)
	prov := eph.Default()

	for b.Loop() {
		if _, err := CrescentVisibility(evening, site, prov); err != nil {
			b.Fatal(err)
		}
	}
}
