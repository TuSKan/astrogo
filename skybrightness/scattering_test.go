package skybrightness_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// scatterGrid is a short grid: the integral costs one field evaluation per
// source direction, and nothing under test here needs spectral detail.
func scatterGrid(t *testing.T) unit.SpectralGrid {
	t.Helper()

	g, err := unit.NewSpectralGrid(500, 10, 11)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	return g
}

// band returns a field of radiance value between two altitudes and nothing
// outside them.
func band(loDeg, hiDeg, value float64) skybrightness.SkyRadiance {
	return func(_ context.Context, dst skybrightness.SpectralRadiance, dir coord.AltAz) error {
		if d := dir.Alt().Degrees(); d < loDeg || d > hiDeg {
			return nil
		}

		for i := range dst {
			dst[i] += value
		}

		return nil
	}
}

// scatteredAt runs the integral and returns the radiance at the first
// wavelength.
func scatteredAt(
	t *testing.T, above skybrightness.SkyRadiance, view coord.AltAz, rings int,
) float64 {
	t.Helper()

	grid := scatterGrid(t)
	dst := skybrightness.NewSpectralRadiance(grid)

	if err := skybrightness.ScatteredIn(
		t.Context(), dst, above, auditScene(t), view, grid, rings,
	); err != nil {
		t.Fatalf("ScatteredIn: %v", err)
	}

	return dst[0]
}

// Doubling the incoming field doubles what is scattered out of it.
//
// Eq. 11 is a linear functional of L0, so this is the one property that holds
// whatever the kernel does, and it fails if the solid-angle weighting or the
// accumulation has picked up a term that does not belong to the source.
func TestScatteredInIsLinearInTheIncomingField(t *testing.T) {
	t.Parallel()

	one := scatteredAt(t, band(0, 90, 1e-9), zenith(), 8)
	two := scatteredAt(t, band(0, 90, 2e-9), zenith(), 8)

	if one <= 0 {
		t.Fatalf("a lit sky scattered %g into the line of sight", one)
	}

	if rel := math.Abs(two-2*one) / (2 * one); rel > 1e-12 {
		t.Errorf("doubling the field gave %g against twice %g; relative %.3g", two, one, rel)
	}
}

// A dark sky scatters nothing, and the destination is left as it was found.
func TestScatteredInIsZeroForADarkSky(t *testing.T) {
	t.Parallel()

	grid := scatterGrid(t)
	dst := skybrightness.NewSpectralRadiance(grid)

	const seeded = 1.25e-9
	for i := range dst {
		dst[i] = seeded
	}

	dark := func(context.Context, skybrightness.SpectralRadiance, coord.AltAz) error { return nil }

	if err := skybrightness.ScatteredIn(
		t.Context(), dst, dark, auditScene(t), zenith(), grid, 8,
	); err != nil {
		t.Fatalf("ScatteredIn: %v", err)
	}

	for i := range dst {
		if dst[i] != seeded {
			t.Fatalf("slot %d became %g; the integral must accumulate, not overwrite, "+
				"and an unlit sky must add nothing", i, dst[i])
		}
	}
}

// The quadrature converges: refining it changes the answer by little.
func TestScatteredInConvergesWithResolution(t *testing.T) {
	t.Parallel()

	coarse := scatteredAt(t, band(0, 90, 1e-9), zenith(), 8)
	fine := scatteredAt(t, band(0, 90, 1e-9), zenith(), 24)

	if coarse <= 0 || fine <= 0 {
		t.Fatalf("coarse %g, fine %g", coarse, fine)
	}

	// Under two per cent between eight rings and twenty-four. Not a tight
	// bound, but the point is that the integral is converging rather than
	// tracking the sample count, which is what a solid-angle weight that did
	// not sum to the hemisphere would do.
	if rel := math.Abs(fine-coarse) / fine; rel > 0.02 {
		t.Errorf("8 rings gave %g and 24 gave %g, %.1f per cent apart; the quadrature is "+
			"not converging", coarse, fine, 100*rel)
	}
}

// The scattered-in term is NOT a common factor: two fields carrying the same
// total flux over the hemisphere scatter differently into the same direction.
//
// This is the whole reason the integral exists. The effective-optical-depth
// transfer multiplies every extended component in a direction by one number,
// so it cannot change a ratio between two of them; Eq. 11 weights each by its
// own distribution over the sky and therefore can. If this test failed, the
// integral would be an expensive way to reproduce what kappa already does.
//
// The two bands are chosen to carry equal solid angle. The solid angle above
// altitude a is 2*pi*(1 - sin a), so altitude 41.81 degrees to the zenith and
// the horizon to 19.47 degrees each cover a third of the hemisphere:
// sin(41.81) = 2/3 and sin(19.47) = 1/3.
func TestScatteredInIsNotACommonFactor(t *testing.T) {
	t.Parallel()

	const (
		value     = 1e-9
		highLoDeg = 41.8103
		lowHiDeg  = 19.4712
	)

	high := scatteredAt(t, band(highLoDeg, 90, value), zenith(), 16)
	low := scatteredAt(t, band(0, lowHiDeg, value), zenith(), 16)

	if high <= 0 || low <= 0 {
		t.Fatalf("overhead third scattered %g, horizon third %g", high, low)
	}

	t.Logf("viewing the zenith: the overhead third of the sky scatters in %.4e, "+
		"the horizon third %.4e, ratio %.3f", high, low, high/low)

	// A common factor would give these two equal, since the fields carry the
	// same flux. Ten per cent is far below what is observed and far above
	// what the quadrature's own error could produce.
	if rel := math.Abs(high-low) / math.Max(high, low); rel < 0.10 {
		t.Errorf("equal-flux fields overhead and at the horizon scattered %g and %g, "+
			"within %.1f per cent — the integral is behaving as a common factor, which is "+
			"what it exists not to be", high, low, 100*rel)
	}
}

// Looking through more air scatters more light in.
func TestScatteredInGrowsTowardTheHorizon(t *testing.T) {
	t.Parallel()

	sky := band(0, 90, 1e-9)

	up := scatteredAt(t, sky, zenith(), 12)
	down := scatteredAt(t, sky, coord.NewAltAz(angle.Deg(20), angle.Deg(0)), 12)

	if down <= up {
		t.Errorf("the zenith gathers %g and 20 degrees altitude %g; a longer path through "+
			"the same lit sky must scatter more in, not less", up, down)
	}
}

// The field above the atmosphere is recovered exactly.
//
// A uniform star map with a flat spectral shape has a known extra-atmospheric
// radiance — the map's own value, at every wavelength, because the shape is
// normalised to average one across the band. Running it through the component,
// which attenuates it, and then through AboveAtmosphere, which undoes that,
// must return the number it started as.
func TestAboveAtmosphereRecoversTheExtraAtmosphericField(t *testing.T) {
	t.Parallel()

	const value = 3.5e-9

	grid := scatterGrid(t)

	shape := skybrightness.NewSpectralRadiance(grid)
	for i := range shape {
		shape[i] = 1
	}

	stars, err := skybrightness.NewIntegratedStarlight(
		uniformSky{value: value, galactic: true}, shape, grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	model, err := skybrightness.NewModel("recover", stars)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	scene := auditScene(t)

	above, err := model.AboveAtmosphere(skybrightness.Query{Scene: scene, Grid: grid})
	if err != nil {
		t.Fatalf("AboveAtmosphere: %v", err)
	}

	for _, altDeg := range []float64{90, 60, 30, 10} {
		dst := skybrightness.NewSpectralRadiance(grid)

		if err := above(t.Context(), dst,
			coord.NewAltAz(angle.Deg(altDeg), angle.Deg(30))); err != nil {
			t.Fatalf("alt %g: %v", altDeg, err)
		}

		for i := range dst {
			if rel := math.Abs(dst[i]-value) / value; rel > 1e-9 {
				t.Errorf("alt %g, slot %d: recovered %.10e, want %.10e — relative %.3g; "+
					"the de-extinction does not invert the transfer the component applied",
					altDeg, i, dst[i], value, rel)
			}
		}
	}
}

// Below the horizon there is no sky, which is what makes Eq. 11 a hemisphere.
func TestAboveAtmosphereIsDarkBelowTheHorizon(t *testing.T) {
	t.Parallel()

	grid := scatterGrid(t)

	shape := skybrightness.NewSpectralRadiance(grid)
	for i := range shape {
		shape[i] = 1
	}

	stars, err := skybrightness.NewIntegratedStarlight(
		uniformSky{value: 1e-9, galactic: true}, shape, grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	model, err := skybrightness.NewModel("below", stars)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	above, err := model.AboveAtmosphere(
		skybrightness.Query{Scene: auditScene(t), Grid: grid})
	if err != nil {
		t.Fatalf("AboveAtmosphere: %v", err)
	}

	dst := skybrightness.NewSpectralRadiance(grid)

	if err := above(t.Context(), dst,
		coord.NewAltAz(angle.Deg(-10), angle.Deg(0))); err != nil {
		t.Fatalf("below the horizon: %v", err)
	}

	for i := range dst {
		if dst[i] != 0 {
			t.Fatalf("slot %d is %g below the horizon", i, dst[i])
		}
	}
}

// A model carrying a component that is itself a scattering integral cannot
// have an extended-source transfer undone, and says so rather than returning
// a number.
func TestAboveAtmosphereRefusesScatteringComponents(t *testing.T) {
	t.Parallel()

	grid := scatterGrid(t)

	moon, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	model, err := skybrightness.NewModel("with-moon", moon)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	if _, err := model.AboveAtmosphere(
		skybrightness.Query{Scene: auditScene(t), Grid: grid},
	); !errors.Is(err, skybrightness.ErrScattering) {
		t.Errorf("err = %v, want ErrScattering: moonlight is already a scattering integral "+
			"over a source outside the sky field, so feeding it back into one counts it twice",
			err)
	}
}

// Input that cannot produce an integral is refused.
func TestScatteredInRejectsBadInput(t *testing.T) {
	t.Parallel()

	grid := scatterGrid(t)
	scene := auditScene(t)
	sky := band(0, 90, 1e-9)

	for _, c := range []struct {
		name  string
		dst   skybrightness.SpectralRadiance
		above skybrightness.SkyRadiance
		scene *skybrightness.Scene
	}{
		{"no incoming field", skybrightness.NewSpectralRadiance(grid), nil, scene},
		{"no scene", skybrightness.NewSpectralRadiance(grid), sky, nil},
		{"short destination", make(skybrightness.SpectralRadiance, 2), sky, scene},
	} {
		if err := skybrightness.ScatteredIn(
			t.Context(), c.dst, c.above, c.scene, zenith(), grid, 4,
		); err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
}
