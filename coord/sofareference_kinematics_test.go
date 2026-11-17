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

// TestKinematicStarIsSOFAsAtco13 holds a star's observed place through a
// Context to SOFA's iauAtco13 given the same proper motion, parallax and
// radial velocity.
//
// TestTopocentricPathIsSOFAsAtco13 passes zero for all four, so a wrong unit
// on any of them reached SOFA as zero on both sides and agreed: Context passed
// parallax in radians where Atciq takes arcseconds, which dropped the annual
// parallax and the perspective term, 0.78″ for Barnard's Star, and nothing
// failed (#1173). These stars are the nearby, fast-moving ones where those
// terms are largest, and the test first requires that the kinematics move
// each one by more than 0.1″, so it cannot pass on inputs that were zeroed.
func TestKinematicStarIsSOFAsAtco13(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	t.Cleanup(time.ResetEOP)

	const (
		dut1Seconds = 0.1237
		xpArcsec    = 0.1834
		ypArcsec    = 0.4021
		// The step-by-step path and Atco13 are the same SOFA routines; what
		// separates them is floating-point order.
		toleranceArcsec = 1e-5
		// The least the kinematics move any of these stars here.
		kinematicFloorArcsec = 0.1
	)

	time.RegisterModel(fixedEOP{
		dut1: dut1Seconds,
		xp:   angle.Arcsec(xpArcsec).Radians(),
		yp:   angle.Arcsec(ypArcsec).Radians(),
	})

	atm := atmosphere.StandardRefraction()
	atm.Model = atmosphere.RefractionNone{}

	// Positions, proper motions (μα* including cos δ), parallaxes and radial
	// velocities from SIMBAD.
	stars := []struct {
		name                     string
		raDeg, decDeg            float64
		pmRAmas, pmDecmas, pxmas float64
		rvKmS                    float64
	}{
		{"Barnard's Star", 269.452075, 4.693391, -801.551, 10362.394, 548.31, -110.11},
		{"alpha Cen A", 219.902066, -60.833975, -3608.0, 686.0, 750.0, -21.4},
		{"61 Cyg A", 316.724802, 38.749411, 4164.209, 3249.614, 285.995, -65.97},
	}

	sites := []struct {
		name              string
		lon, lat, heightM float64
	}{
		{"Greenwich", 0, 51.4778, 46},
		{"Paranal", -70.4042, -24.6272, 2635},
		{"Mauna Kea", -155.4681, 19.8207, 4205},
	}

	epochs := []time.Time{
		time.Date(2026, time.March, 20, 12, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.August, 21, 2, 17, 43, 250000000, time.LocationUTC),
	}

	for _, s := range sites {
		site, err := coord.NewGeodetic(angle.Deg(s.lon), angle.Deg(s.lat), unit.Meters(s.heightM))
		if err != nil {
			t.Fatalf("%s: NewGeodetic: %v", s.name, err)
		}

		for _, epoch := range epochs {
			ctx := coord.NewContext(epoch, site, atm)
			utc1, utc2 := epoch.UTC().JDParts()

			for _, st := range stars {
				ra, dec := angle.Deg(st.raDeg), angle.Deg(st.decDeg)
				pmRA, pmDec := angle.Arcsec(st.pmRAmas/1000), angle.Arcsec(st.pmDecmas/1000)
				px := angle.Arcsec(st.pxmas / 1000)

				got, err := ctx.ICRSToAltAz(coord.NewICRSWithKinematics(ra, dec, pmRA, pmDec, px, unit.KmPerSec(st.rvKmS)))
				if err != nil {
					t.Fatalf("%s: ICRSToAltAz: %v", st.name, err)
				}

				// SOFA's own units: dRA/dt in radians a year, so μα* over cos δ;
				// parallax in arcseconds; radial velocity in km/s.
				atco := func(pr, pd, pxArcsec, rv float64) coord.AltAz {
					aob, zob, _, _, _, _, status := gofaext.Atco13(
						ra.Radians(), dec.Radians(), pr, pd, pxArcsec, rv,
						utc1, utc2, dut1Seconds,
						site.Lon().Radians(), site.Lat().Radians(), site.Height().Meters(),
						angle.Arcsec(xpArcsec).Radians(), angle.Arcsec(ypArcsec).Radians(),
						0, atm.Temperature, atm.Humidity, atm.Wavelength,
					)
					if status < 0 {
						t.Fatalf("%s @ %s: Atco13 status %d", st.name, s.name, status)
					}

					return coord.NewAltAz(angle.Rad(math.Pi/2-zob), angle.Rad(aob).Wrap360())
				}

				want := atco(pmRA.Radians()/math.Cos(dec.Radians()), pmDec.Radians(), px.Arcseconds(), st.rvKmS)
				static := atco(0, 0, 0, 0)

				label := st.name + " @ " + s.name + " " + epoch.Format("2006-01-02")

				if moved := horizonSeparationArcsec(want, static); moved < kinematicFloorArcsec {
					t.Fatalf("%s: the kinematics move the star %.3f″ in SOFA, under %.1f″; the test would not see them",
						label, moved, kinematicFloorArcsec)
				}

				if sep := horizonSeparationArcsec(got, want); sep > toleranceArcsec {
					t.Errorf("%s: %.6f″ from SOFA's Atco13 with the same kinematics (%.3f″ from its place without them)",
						label, sep, horizonSeparationArcsec(got, static))
				}
			}
		}
	}
}
