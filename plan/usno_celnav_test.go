package plan_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// usnoCelNavToleranceArcsec bounds the difference from USNO's computed
// altitude and azimuth, on the sky.
//
// The largest term known in advance is USNO's own: its Greenwich hour angle
// runs 0.662 arcseconds from Horizons' while astrogo's matches Horizons to a
// milliarcsecond (#256), and an hour-angle offset moves azimuth by up to that
// much. USNO prints to 1e-6 degrees, and the Sun's ephemeris here, SOFA's
// analytical one, is good to a few hundredths of an arcsecond. Two arcseconds
// is three times the largest term. What it is built to catch is far larger:
// refraction applied on one side and not the other is 0.002 degrees, seven
// arcseconds, even at Sirius's 83 degrees; aberration applied twice is twenty.
const usnoCelNavToleranceArcsec = 2.0

// TestUSNO_CelNav holds astrogo's horizontal coordinates to USNO's
// celestial-navigation service.
//
// USNO's hc and zn are the navigator's computed altitude and azimuth:
// airless and geocentric, with refraction and parallax listed as separate
// corrections in the same response. So astrogo is asked the same question: an
// airless Context, a star through ICRSToAltAz, and the Sun through the
// moving-body path, its apparent geocentric vector to GeocentricToObserved,
// with the parallax that path applies returned to put it back at the
// geocenter. The instant is UT1, which celestial navigation is computed on.
//
// The values are USNO's API 4.0.1 /api/celnav, transcribed from its decimal
// output and fetched 2026-10-08: São Paulo, then the four extreme places this
// file's integration test used to query live, near both poles, on the equator
// at an equinox, and at Everest's coordinates.
//
// Until #674 the comparison ran refracted against airless and passed the Sun's
// apparent place to ICRSToAltAz, which applied aberration a second time:
// 0.57 degrees off at the horizon and 21 arcseconds off everywhere, inside
// tolerances of 0.1 to 1.5 degrees.
func TestUSNO_CelNav(t *testing.T) {
	sirius := coord.NewICRSWithKinematics(
		angle.Deg(101.2871553333), angle.Deg(-16.7161158611),
		angle.Arcsec(-0.54601), angle.Arcsec(-1.22307),
		angle.Arcsec(0.37921), unit.KmPerSec(-5.5))

	sun := plan.NewSun(eph.Default())

	cases := []struct {
		place     string
		lat, lon  float64
		date, ut1 string
		object    string
		hc, zn    float64
	}{
		{"São Paulo", -23.600833, -46.6525, "2026-04-06", "21:00:00", "Sun", -0.657359, 277.028747},
		{"São Paulo", -23.600833, -46.6525, "2026-04-06", "21:00:00", "Sirius", 82.918811, 344.8198},
		{"north pole", 89.99, 0, "2026-06-21", "12:00:00", "Sun", 23.44785, 179.545527},
		{"south pole", -89.99, 0, "2026-12-21", "00:00:00", "Sun", 23.424475, 179.454439},
		{"south pole", -89.99, 0, "2026-12-21", "00:00:00", "Sirius", 16.762883, 12.010286},
		{"equator", 0, 0, "2026-03-20", "12:00:00", "Sun", 88.140231, 91.400374},
		{"Everest", 27.9881, 86.925, "2026-06-21", "06:00:00", "Sun", 84.456647, 144.377081},
		{"Everest", 27.9881, 86.925, "2026-06-21", "06:00:00", "Sirius", 42.901411, 159.963771},
	}

	for _, c := range cases {
		t.Run(c.place+" "+c.object, func(t *testing.T) {
			epoch := celNavEpoch(t, c.date, c.ut1)

			site, err := coord.NewGeodetic(angle.Deg(c.lon), angle.Deg(c.lat), 0)
			if err != nil {
				t.Fatalf("NewGeodetic: %v", err)
			}

			ctx := coord.NewContext(epoch, site, atmosphere.Refraction{})

			var alt, az float64

			switch c.object {
			case "Sirius":
				aa, err := ctx.ICRSToAltAz(sirius)
				if err != nil {
					t.Fatalf("ICRSToAltAz: %v", err)
				}

				alt, az = aa.Alt().Degrees(), aa.Az().Degrees()
			case "Sun":
				vec, err := sun.GeocentricVec(epoch)
				if err != nil {
					t.Fatalf("GeocentricVec: %v", err)
				}

				aa := ctx.GeocentricToObserved(vec)
				alt, az = aa.Alt().Degrees()+geocentricParallaxDeg(vec.Norm(), aa.Alt()), aa.Az().Degrees()
			}

			dAlt := (alt - c.hc) * 3600
			dAz := math.Remainder(az-c.zn, 360) * 3600 * math.Cos(alt*math.Pi/180)

			t.Logf("%s %s: alt %+.3f\", azimuth %+.3f\" across", c.date, c.object, dAlt, dAz)

			if math.Abs(dAlt) > usnoCelNavToleranceArcsec || math.Abs(dAz) > usnoCelNavToleranceArcsec {
				t.Errorf("%s at %s %s UT1: altitude %+.3f\" and azimuth %+.3f\" across from USNO's (%.6f, %.6f), beyond %.1f\"",
					c.object, c.date, c.ut1, dAlt, dAz, c.hc, c.zn, usnoCelNavToleranceArcsec)
			}
		})
	}
}

// celNavEpoch returns the instant USNO was asked about. Its date and time are
// UT1, so the calendar reading becomes a Julian Date labeled UT1 rather than
// one converted from UTC; none of these dates holds a leap second, so the
// calendar arithmetic is the same on both scales.
func celNavEpoch(t *testing.T, date, clock string) time.Time {
	t.Helper()

	cal, err := time.Parse("2006-01-02 15:04:05", date+" "+clock)
	if err != nil {
		t.Fatalf("parse %s %s: %v", date, clock, err)
	}

	jd1, jd2 := time.FromGo(cal).JDParts()

	return time.FromJDParts(jd1, jd2, time.UT1)
}

// geocentricParallaxDeg is how much higher a body at distAU stands from the
// geocenter than from a site where it is at altitude alt: the horizontal
// parallax, scaled by the cosine of the altitude. The Earth's equatorial
// radius stands in for the site's distance from the geocenter, which differs
// from it by at most a third of a per cent, a few hundredths of an arcsecond
// of the Sun's 8.8.
func geocentricParallaxDeg(distAU float64, alt angle.Angle) float64 {
	const earthRadiusAU = 6378.137 / 149597870.7

	return math.Asin(earthRadiusAU/distAU) * alt.Cos() * 180 / math.Pi
}
