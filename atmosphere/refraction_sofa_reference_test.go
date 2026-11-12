package atmosphere_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestRefractionSOFAIsAtioqAndAtoiqAboveTheHandOver holds RefractionSOFA to
// its doc: above 10° of observed altitude it is SOFA's series, "exactly as
// iauAtioq and iauAtoiq apply it".
//
// # Why against gofa, and why at the pole
//
// The series is transcribed in internal/refraction. That package's tests
// compare its blend with the transcription, so they cannot see a fault in the
// transcription itself, and before this test nothing could (#1103). gofa's
// Atioq and Atoiq are SOFA's own. For an observer on the pole, a CIRS
// declination is a true altitude, so the refraction SOFA applies can be read
// straight off the observed zenith distance, with nothing in between.
//
// # The bound
//
// Measured, the two agree to 1.4e-4 µas at worst over these four airs and
// every altitude from 10° to the zenith. That is float64 rounding of a
// refraction of order 1e-4 rad. 0.01 µas leaves seventy times that for fused
// multiply-add on arm64, while a wrong coefficient in the series moves the
// answer by milliarcseconds.
func TestRefractionSOFAIsAtioqAndAtoiqAboveTheHandOver(t *testing.T) {
	t.Parallel()

	const boundMuas = 0.01

	toMuas := func(rad float64) float64 { return rad * 180 / math.Pi * 3.6e9 }
	model := atmosphere.RefractionSOFA{}

	for _, env := range []atmosphere.Refraction{
		{Pressure: 1010, Temperature: 10, Humidity: 0.5, Wavelength: 0.55},
		{Pressure: 700, Temperature: -5, Humidity: 0.2, Wavelength: 0.40},
		{Pressure: 1050, Temperature: 30, Humidity: 0.9, Wavelength: 0.80},
		// Radio, iauRefco's other refractivity, where there is no hand-over.
		{Pressure: 1013.25, Temperature: 15, Humidity: 0.5, Wavelength: 20000},
	} {
		refa, refb := gofaext.Refco(env.Pressure, env.Temperature, env.Humidity, env.Wavelength)

		var astrom gofaext.ASTROM

		astrom.Phi, astrom.Sphi, astrom.Cphi = math.Pi/2, 1, 0
		astrom.Refa, astrom.Refb = refa, refb

		var worstTrue, worstApparent float64

		for h := 10.0; h <= 90.0; h += 0.01 {
			alt := h * math.Pi / 180

			// A true altitude of 10° or more is observed higher still, so
			// every sample here is above the hand-over.
			_, zob, _, _, _ := gofaext.Atioq(1.0, alt, &astrom)
			sofaTrue := (math.Pi/2 - zob) - alt
			gotTrue := model.RefractFromTrue(angle.Rad(alt), env).Radians()
			worstTrue = max(worstTrue, math.Abs(gotTrue-sofaTrue))

			_, di := gofaext.Atoiq("A", 0.3, math.Pi/2-alt, &astrom)
			sofaApparent := alt - di
			gotApparent := model.RefractFromApparent(angle.Rad(alt), env).Radians()
			worstApparent = max(worstApparent, math.Abs(gotApparent-sofaApparent))
		}

		if w := toMuas(worstTrue); !(w <= boundMuas) {
			t.Errorf("%.6g hPa, %g °C, %g µm: RefractFromTrue is %.3g µas from iauAtioq, bound %g",
				env.Pressure, env.Temperature, env.Wavelength, w, boundMuas)
		}

		if w := toMuas(worstApparent); !(w <= boundMuas) {
			t.Errorf("%.6g hPa, %g °C, %g µm: RefractFromApparent is %.3g µas from iauAtoiq, bound %g",
				env.Pressure, env.Temperature, env.Wavelength, w, boundMuas)
		}
	}
}
