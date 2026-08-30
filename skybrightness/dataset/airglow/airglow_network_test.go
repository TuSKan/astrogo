//go:build network

package airglow_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
	"github.com/TuSKan/astrogo/time"
)

// The whole provider, against the real service.
//
// This is the only thing that checks the protocol, and the protocol was read
// out of ESO's own skycalc_cli rather than guessed: a POST that runs the model
// and returns a temporary directory name, a GET for the FITS inside it, and a
// GET that releases it. A substring assertion cannot tell a correct three-step
// exchange from a wrong one.
//
// It also checks the unit conversion, which is where the real risk is. SkyCalc
// reports photons per micrometre per square arcsecond; spectral radiance is
// per nanometre per steradian and carries the energy of a photon. Getting that
// wrong is a factor of a thousand or of 4.25e10, and both leave a spectrum that
// is positive, smooth and completely wrong. Only an absolute check catches it.
func TestFetchReturnsAPlausibleAirglowSpectrum(t *testing.T) {
	t.Parallel()

	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// A narrow band around V keeps the request small; the service samples at
	// 0.1 nm and a 300-2000 nm table is tens of megabytes.
	spec := airglow.Spec{
		Observatory:  airglow.Paranal,
		SolarFluxSFU: 100, // the value GAMBONS ships its reference spectrum at
		MinNM:        500,
		MaxNM:        600,
		StepNM:       0.1,
	}

	s, err := airglow.Fetch(ctx, spec)
	if err != nil {
		t.Skipf("SkyCalc did not answer: %v", err)
	}

	if len(s.LambdaNM) == 0 {
		t.Fatal("the spectrum is empty")
	}

	t.Logf("%d samples over %.1f-%.1f nm, source %q",
		len(s.LambdaNM), s.LambdaNM[0], s.LambdaNM[len(s.LambdaNM)-1], s.Source)

	for i := 1; i < len(s.LambdaNM); i++ {
		if s.LambdaNM[i] <= s.LambdaNM[i-1] {
			t.Fatalf("wavelengths are not ascending at %d: %v then %v",
				i, s.LambdaNM[i-1], s.LambdaNM[i])
		}
	}

	var sum, peak float64

	for i, v := range s.Radiance {
		if v < 0 || math.IsNaN(v) {
			t.Fatalf("radiance %v at %.2f nm", v, s.LambdaNM[i])
		}

		sum += v
		peak = math.Max(peak, v)
	}

	mean := sum / float64(len(s.Radiance))

	// Convert the band mean to a V surface brightness. Zenith airglow at a
	// dark site runs around 22 mag arcsec^-2; the window is wide because this
	// is a 100 nm mean rather than a passband integral and because airglow
	// genuinely varies. A factor-of-1000 unit slip lands 7.5 mag away and a
	// factor of 4.25e10 lands 26 mag away, so either is unmissable.
	const (
		vZeroFlux      = 3.63e-11
		arcsec2PerSter = 4.254517e10
	)

	mag := -2.5 * math.Log10(mean/(vZeroFlux*arcsec2PerSter))

	t.Logf("band-mean radiance %.4e W m^-2 sr^-1 nm^-1 = %.2f mag arcsec^-2, peak %.4e",
		mean, mag, peak)

	if mag < 19 || mag > 25 {
		t.Errorf("zenith airglow comes out at %.2f mag arcsec^-2; dark-site airglow is "+
			"near 22 and this window is already generous, so the unit conversion is wrong", mag)
	}

	// Emission lines must stand above the continuum. A spectrum that is flat
	// to within a factor of two over 500-600 nm is not airglow, and would mean
	// the airglow columns were not the ones read.
	if peak < 2*mean {
		t.Errorf("peak %.3e is not above the mean %.3e; this does not look like a line spectrum",
			peak, mean)
	}
}

// A rejected request must come back as an error rather than a wrong spectrum.
func TestFetchReportsServiceValidationErrors(t *testing.T) {
	t.Parallel()

	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Inside this package's own bounds so it reaches the service, but the
	// service caps airmass at 3 and this asks for a range it will not accept.
	_, err := airglow.Fetch(ctx, airglow.Spec{
		Observatory: airglow.Paranal,
		MinNM:       500,
		MaxNM:       500.05, // narrower than one sample at the default step
		StepNM:      0.1,
	})
	if err == nil {
		t.Error("a degenerate wavelength range produced no error")
	} else {
		t.Logf("service rejected it as expected: %v", err)
	}
}
