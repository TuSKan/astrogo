package airglow

import (
	"errors"
	"math"
	"testing"
)

// The service rejects a partial body with a 500 rather than filling the gaps
// from its own defaults, so the request has to carry every field. That was
// found by sending fifteen fields and getting "Internal Server Error", then
// sending all thirty-five and getting a job.
func TestRequestCarriesEveryField(t *testing.T) {
	t.Parallel()

	req, err := Spec{Observatory: Paranal, MinNM: 500, MaxNM: 600}.request()
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	// Fields the caller never sets must still arrive with ESO's defaults.
	if req.PWVMode != "pwv" || req.PWV != 3.5 {
		t.Errorf("precipitable water defaults lost: %q %v", req.PWVMode, req.PWV)
	}

	if req.VacAir != "vac" || req.WRes != 20000 {
		t.Errorf("vacuum/resolution defaults lost: %q %v", req.VacAir, req.WRes)
	}

	if req.MoonSunSep != 90 || req.EclLat != 90 {
		t.Errorf("geometry defaults lost: %v %v", req.MoonSunSep, req.EclLat)
	}

	if req.LSFGaussFWHM != 5 || req.LSFBoxcarFWHM != 5 {
		t.Errorf("line-spread defaults lost: %v %v", req.LSFGaussFWHM, req.LSFBoxcarFWHM)
	}
}

// Everything but airglow must be switched off, and the airmass pinned at one.
//
// The component this feeds applies van Rhijn itself. Asking SkyCalc to tilt the
// spectrum as well would apply the path length twice, and the result would be
// too faint away from the zenith while looking entirely reasonable at it.
func TestRequestIsolatesAirglowAtTheZenith(t *testing.T) {
	t.Parallel()

	req, err := Spec{}.request()
	if err != nil {
		t.Fatalf("request: %v", err)
	}

	if req.Airmass != 1.0 {
		t.Errorf("airmass = %v, want 1: the component applies van Rhijn itself", req.Airmass)
	}

	for name, got := range map[string]string{
		"incl_moon":      req.InclMoon,
		"incl_starlight": req.InclStarlight,
		"incl_zodiacal":  req.InclZodiacal,
		"incl_therm":     req.InclThermal,
	} {
		if got != "N" {
			t.Errorf("%s = %q, want N: only airglow belongs in this spectrum", name, got)
		}
	}

	if req.InclAirglow != "Y" {
		t.Errorf("incl_airglow = %q, want Y", req.InclAirglow)
	}

	// Unset fields take the documented defaults rather than zero, since a
	// zero wavelength range would be rejected and a zero solar flux would be
	// a different sky.
	if req.SolarFlux != 130 || req.WMin != 300 || req.WMax != 2000 || req.WDelta != 0.1 {
		t.Errorf("defaults not applied: flux %v, %v-%v nm, step %v",
			req.SolarFlux, req.WMin, req.WMax, req.WDelta)
	}
}

// A request the service would refuse must fail here instead, so a typo does not
// become a round trip to somebody else's server.
func TestSpecValidates(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		spec Spec
	}{
		{"unknown observatory", Spec{Observatory: "lapalma"}},
		{"negative solar flux", Spec{SolarFluxSFU: -1}},
		{"NaN solar flux", Spec{SolarFluxSFU: math.NaN()}},
		{"below SkyCalc's range", Spec{MinNM: 100, MaxNM: 500}},
		{"above SkyCalc's range", Spec{MinNM: 500, MaxNM: 40000}},
		{"inverted range", Spec{MinNM: 900, MaxNM: 400}},
		{"negative step", Spec{MinNM: 500, MaxNM: 600, StepNM: -0.1}},
	} {
		if _, err := c.spec.request(); !errors.Is(err, ErrSpec) {
			t.Errorf("%s: err = %v, want ErrSpec", c.name, err)
		}
	}
}

// Interpolation, and the deliberate choice to return zero outside the range.
//
// Holding the endpoint flat, which is what the extragalactic background does,
// would be wrong here: an airglow spectrum is a line forest, so whichever band
// happened to sit at the edge would be continued across everything beyond it.
func TestSpectrumAt(t *testing.T) {
	t.Parallel()

	s := &Spectrum{
		LambdaNM: []float64{500, 501, 502},
		Radiance: []float64{1e-9, 3e-9, 2e-9},
	}

	for _, c := range []struct{ lambda, want float64 }{
		{500, 1e-9},
		{501, 3e-9},
		{502, 2e-9},
		{500.5, 2e-9},   // midway up
		{501.5, 2.5e-9}, // midway down
		{499.9, 0},      // below
		{502.1, 0},      // above
	} {
		if got := s.At(c.lambda); math.Abs(got-c.want) > 1e-18 {
			t.Errorf("At(%v) = %v, want %v", c.lambda, got, c.want)
		}
	}

	// An empty spectrum answers zero rather than panicking.
	if got := (&Spectrum{}).At(550); got != 0 {
		t.Errorf("empty spectrum returned %v", got)
	}
}

// The photon-to-energy conversion, checked against a hand-worked value.
//
// SkyCalc reports ph/s/m2/micron/arcsec2. A radiance is per nanometre, per
// steradian, and in watts. Getting any one of those wrong leaves a spectrum
// that is positive and smooth and out by a factor of 1000, 4.25e10 or 3.6e-19.
func TestPhotonConversionMatchesAHandWorkedValue(t *testing.T) {
	t.Parallel()

	// 160 ph/s/m2/micron/arcsec2 at 550 nm.
	const (
		perMicronPerArcsec2 = 160.0
		lambdaNM            = 550.0
	)

	// per micron -> per nm, per arcsec^2 -> per sr, then photon -> joule.
	const (
		arcsec2PerSter = 4.254517e10
		planck         = 6.62607015e-34
		lightSpeed     = 2.99792458e8
	)

	photonPerNMPerSr := perMicronPerArcsec2 / 1000 * arcsec2PerSter
	energyPerPhoton := planck * lightSpeed / (lambdaNM * 1e-9)
	want := photonPerNMPerSr * energyPerPhoton

	// Roughly 22 mag arcsec^-2 in V, which is what dark-site airglow is.
	const (
		vZeroFlux = 3.63e-11
	)

	mag := -2.5 * math.Log10(want/(vZeroFlux*arcsec2PerSter))
	if mag < 21 || mag > 23 {
		t.Errorf("the worked example lands at %.2f mag arcsec^-2; it was chosen to be near 22, "+
			"so this test's own arithmetic is wrong", mag)
	}

	t.Logf("160 ph/s/m2/um/arcsec2 at 550 nm = %.4e W m^-2 sr^-1 nm^-1 = %.2f mag arcsec^-2",
		want, mag)
}
