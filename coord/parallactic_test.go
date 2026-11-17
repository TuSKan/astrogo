package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestParallacticAngleIsSOFAs holds Context.ParallacticAngle to SOFA from end
// to end: the parallactic angle SOFA's iauHd2pa gives for the observed hour
// angle and declination iauAtco13 computes, for stars east and west of the
// meridian at three sites, with refraction. The angle is taken from the
// observed position ICRSToAltAz returns, so the test covers the recovery of
// the hour angle and declination as well as the formula, which
// TestHd2paAgainstSOFAsOwnVector checks against SOFA's own vector.
func TestParallacticAngleIsSOFAs(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	t.Cleanup(time.ResetEOP)

	const (
		dut1Seconds = 0.1237
		xpArcsec    = 0.1834
		ypArcsec    = 0.4021
		// One observed position, two orders of floating-point operations.
		toleranceArcsec = 1e-3
	)

	time.RegisterModel(fixedEOP{
		dut1: dut1Seconds,
		xp:   angle.Arcsec(xpArcsec).Radians(),
		yp:   angle.Arcsec(ypArcsec).Radians(),
	})

	atm := atmosphere.StandardRefraction()
	epoch := time.Date(2026, time.March, 20, 3, 0, 0, 0, time.LocationUTC)
	utc1, utc2 := epoch.UTC().JDParts()

	sites := []struct {
		name              string
		lon, lat, heightM float64
	}{
		{"Greenwich", 0, 51.4778, 46},
		{"Paranal", -70.4042, -24.6272, 2635},
		{"Mauna Kea", -155.4681, 19.8207, 4205},
	}

	// Bright stars spread in right ascension, so at each site some are east
	// of the meridian and some west.
	stars := []struct {
		name          string
		raDeg, decDeg float64
	}{
		{"Sirius", 101.287155, -16.716116},
		{"Arcturus", 213.915300, 19.182410},
		{"Vega", 279.234735, 38.783689},
		{"Canopus", 95.987958, -52.695661},
		{"Regulus", 152.092962, 11.967209},
		{"Antares", 247.351915, -26.432003},
	}

	var east, west int

	for _, s := range sites {
		site, err := coord.NewGeodetic(angle.Deg(s.lon), angle.Deg(s.lat), unit.Meters(s.heightM))
		if err != nil {
			t.Fatalf("%s: NewGeodetic: %v", s.name, err)
		}

		ctx := coord.NewContext(epoch, site, atm)

		for _, st := range stars {
			ra, dec := angle.Deg(st.raDeg), angle.Deg(st.decDeg)

			aob, zob, hob, dob, _, _, status := gofaext.Atco13(
				ra.Radians(), dec.Radians(), 0, 0, 0, 0,
				utc1, utc2, dut1Seconds,
				site.Lon().Radians(), site.Lat().Radians(), site.Height().Meters(),
				angle.Arcsec(xpArcsec).Radians(), angle.Arcsec(ypArcsec).Radians(),
				atm.Pressure, atm.Temperature, atm.Humidity, atm.Wavelength,
			)
			if status < 0 {
				t.Fatalf("%s @ %s: Atco13 status %d", st.name, s.name, status)
			}

			// SOFA refracts with its own series, which this Context also uses
			// above 10°; below, it hands over to Bennett. Keep to where both
			// refract alike.
			if 90-zob*180/math.Pi < 15 {
				continue
			}

			got, err := ctx.ICRSToAltAz(coord.NewICRS(ra, dec))
			if err != nil {
				t.Fatalf("%s: ICRSToAltAz: %v", st.name, err)
			}

			// The observed position itself agrees with SOFA's, or the angle
			// below would be compared at two different places.
			if dAz := math.Abs(math.Remainder(got.Az().Radians()-aob, 2*math.Pi)); angle.Rad(dAz).Arcseconds() > toleranceArcsec {
				t.Fatalf("%s @ %s: azimuth %.6f° against SOFA's %.6f°", st.name, s.name, got.Az().Degrees(), aob*180/math.Pi)
			}

			want := gofaext.Hd2pa(hob, dob, site.Lat().Radians())
			q := ctx.ParallacticAngle(got).Radians()

			if d := math.Abs(math.Remainder(q-want, 2*math.Pi)); angle.Rad(d).Arcseconds() > toleranceArcsec {
				t.Errorf("%s @ %s: parallactic angle %.6f°, SOFA's %.6f°", st.name, s.name, q*180/math.Pi, want*180/math.Pi)
			}

			// West of the meridian it is positive, east negative.
			if math.Remainder(hob, 2*math.Pi) > 0 {
				west++

				if q <= 0 {
					t.Errorf("%s @ %s: west of the meridian, parallactic angle %.4f°, want positive", st.name, s.name, q*180/math.Pi)
				}
			} else {
				east++

				if q >= 0 {
					t.Errorf("%s @ %s: east of the meridian, parallactic angle %.4f°, want negative", st.name, s.name, q*180/math.Pi)
				}
			}
		}
	}

	// The comparison has to have covered both sides of the sky.
	if east < 2 || west < 2 {
		t.Fatalf("compared %d positions east of the meridian and %d west, want at least two of each", east, west)
	}
}

// TestParallacticAngleOnTheMeridian: on the meridian the parallactic angle is
// zero for a star south of the zenith and 180° for one north of it, between
// the zenith and the pole, at a northern site.
func TestParallacticAngleOnTheMeridian(t *testing.T) {
	t.Parallel()

	site, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(51.4778), unit.Meters(46))
	if err != nil {
		t.Fatal(err)
	}

	ctx := coord.NewContext(time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC), site, atmosphere.Refraction{})

	for _, c := range []struct {
		name    string
		az, alt float64
		want    float64
	}{
		{"south of the zenith", 180, 40, 0},
		{"north of the zenith, above the pole", 0, 70, 180},
	} {
		q := ctx.ParallacticAngle(coord.NewAltAz(angle.Deg(c.alt), angle.Deg(c.az))).Degrees()
		if d := math.Abs(math.Remainder(q-c.want, 360)); d > 1e-9 {
			t.Errorf("%s: parallactic angle %.9f°, want %g°", c.name, q, c.want)
		}
	}
}
