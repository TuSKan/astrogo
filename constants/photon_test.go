package constants_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// A 550 nm photon carries about 2.25 electronvolts.
//
// # Why an eV cross-check rather than only hc/lambda
//
// Because restating hc/lambda in the test restates the implementation, and
// two copies of one expression agree however wrong the constants behind them
// are. The electronvolt figure comes from outside this package — green light
// sits near 2.25 eV in any physics text — so it checks the constants as well
// as the arithmetic.
func TestPhotonEnergyMatchesTheElectronvoltFigure(t *testing.T) {
	t.Parallel()

	const eVinJoules = 1.602176634e-19 // exact, SI 2019

	got := constants.PhotonEnergyJ(550) / eVinJoules

	if math.Abs(got-2.254) > 0.005 {
		t.Errorf("a 550 nm photon is %.4f eV, want about 2.254", got)
	}

	// And it falls as 1/lambda: twice the wavelength, half the energy.
	half := constants.PhotonEnergyJ(1100)
	if rel := math.Abs(half/constants.PhotonEnergyJ(550) - 0.5); rel > 1e-12 {
		t.Errorf("doubling the wavelength changed the energy by %.4f×, want exactly half",
			half/constants.PhotonEnergyJ(550))
	}
}

// A non-positive wavelength has no photon energy, and says so with zero
// rather than an infinity that would propagate silently.
func TestPhotonEnergyGuardsNonPositiveWavelength(t *testing.T) {
	t.Parallel()

	for _, nm := range []unit.WavelengthNM{0, -1, -550} {
		if got := constants.PhotonEnergyJ(nm); got != 0 {
			t.Errorf("PhotonEnergyJ(%g) = %g, want 0", float64(nm), got)
		}
	}

	// ToPhoton inherits the guard rather than dividing by it.
	if got := constants.ToPhoton(1, 0); got != 0 {
		t.Errorf("ToPhoton at zero wavelength = %g, want 0", float64(got))
	}
}

// ToPhoton and ToEnergy invert each other.
func TestPhotonEnergyConversionsRoundTrip(t *testing.T) {
	t.Parallel()

	for _, nm := range []unit.WavelengthNM{330, 550, 1000} {
		const radiance unit.SpectralRadiance = 3.7e-9

		back := constants.ToEnergy(constants.ToPhoton(radiance, nm), nm)

		if rel := math.Abs(float64(back-radiance) / float64(radiance)); rel > 1e-15 {
			t.Errorf("at %g nm the round trip gave %g, want %g (relative %.3g)",
				float64(nm), float64(back), float64(radiance), rel)
		}
	}
}

// One square arcsecond is 2.35e-11 steradians.
//
// The value is derived rather than written down, so this pins it against the
// definition it is derived from — an arcsecond is pi/(180*3600) radians, and
// the small-angle patch is that squared.
func TestArcsecondSquaredToSteradian(t *testing.T) {
	t.Parallel()

	perArcsec := math.Pi / (180 * 3600)
	want := perArcsec * perArcsec

	if rel := math.Abs(constants.ArcsecondSquaredToSteradian-want) / want; rel > 1e-15 {
		t.Errorf("got %g, want %g (relative %.3g)",
			constants.ArcsecondSquaredToSteradian, want, rel)
	}

	// Sanity: the whole sky is 4*pi sr, which is about 5.35e11 square
	// arcseconds. A conversion off by a factor would not survive this.
	sky := 4 * math.Pi / constants.ArcsecondSquaredToSteradian
	if sky < 5.3e11 || sky > 5.4e11 {
		t.Errorf("the sphere works out to %.3g square arcseconds, want about 5.35e11", sky)
	}
}
