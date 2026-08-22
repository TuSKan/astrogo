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

// A sky map and a direction-by-direction evaluation agree exactly.
//
// SkyMap samples the incoming field once and shares it across every direction,
// where Estimate samples one per call. That is a pure optimisation — L_0 is the
// radiance above the atmosphere and does not depend on where the observer looks
// — so the two must produce the same numbers, not merely close ones.
//
// If they ever diverge, the shared field has picked up a dependence on the
// view direction, which would make every all-sky map subtly wrong in a way no
// single-direction test could see.
func TestSkyMapAgreesWithPerDirectionEstimates(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSFull, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	scene := presetGoldenScene(t, skybrightness.GAMBONSFull)
	q := skybrightness.Query{
		Scene: scene, Grid: in.Grid, Fidelity: skybrightness.Reference,
	}

	points, err := model.SkyMap(t.Context(), q, 4)
	if err != nil {
		t.Fatalf("SkyMap: %v", err)
	}

	if len(points) == 0 {
		t.Fatal("the map is empty")
	}

	var compared int

	for _, p := range points {
		// The same direction, evaluated on its own with no shared field.
		alone, err := model.Direction(t.Context(), q, p.Direction.Alt(), p.Direction.Az())
		if err != nil {
			t.Fatalf("Direction %v: %v", p.Direction, err)
		}

		mapped := p.Estimate.SpectralRadiance()
		single := alone.SpectralRadiance()

		if len(mapped) != len(single) {
			t.Fatalf("%v: %d slots against %d", p.Direction, len(mapped), len(single))
		}

		for i := range mapped {
			if single[i] == 0 {
				continue
			}

			if rel := math.Abs(mapped[i]-single[i]) / math.Abs(single[i]); rel > 1e-12 {
				t.Fatalf("%v slot %d: map %.17g, single %.17g, relative %.3g — the shared "+
					"incoming field has picked up a dependence on the view direction",
					p.Direction, i, mapped[i], single[i], rel)
			}

			compared++
		}

		// Per component too: a compensating pair would pass the total.
		for _, id := range alone.ComponentIDs() {
			a, okA := p.Estimate.Component(id)
			b, okB := alone.Component(id)

			if !okA || !okB {
				t.Fatalf("%v: %s present in one evaluation and not the other", p.Direction, id)
			}

			for i := range a {
				if b[i] == 0 {
					continue
				}

				if rel := math.Abs(a[i]-b[i]) / math.Abs(b[i]); rel > 1e-12 {
					t.Fatalf("%v, %s slot %d: map %.17g, single %.17g", p.Direction, id, i, a[i], b[i])
				}
			}
		}
	}

	t.Logf("%d directions, %d spectral values, all identical to 1e-12", len(points), compared)
}

// The scattering angle stays accurate at small separations.
//
// Forward scattering is where the Henyey-Greenstein phase function is most
// peaked, so it is where the angle has to be most accurate — and it is exactly
// where an acos of the dot product is worst. This compares against an
// independent great-circle formula over separations spanning nine orders of
// magnitude.
func TestSeparationIsAccurateNearZero(t *testing.T) {
	t.Parallel()

	// A reference through coord, which computes the same quantity by the same
	// stable route for equatorial coordinates. Altitude and azimuth are a
	// spherical frame like any other, so a separation in one is a separation
	// in the other.
	reference := func(a, b coord.AltAz) float64 {
		return coord.Separation(
			coord.NewICRS(a.Az(), a.Alt()),
			coord.NewICRS(b.Az(), b.Alt()),
		).Radians()
	}

	base := coord.NewAltAz(angle.Deg(35), angle.Deg(120))

	for _, deltaDeg := range []float64{
		1e-7, 1e-6, 1e-5, 1e-4, 1e-3, 1e-2, 0.1, 1, 10, 45, 90, 135, 179,
	} {
		other := coord.NewAltAz(angle.Deg(35+deltaDeg), angle.Deg(120))

		got := skybrightness.SeparationForTest(base, other)
		want := reference(base, other)

		if want == 0 {
			continue
		}

		if rel := math.Abs(got-want) / want; rel > 1e-12 {
			t.Errorf("at %g degrees: got %.17g, want %.17g, relative %.3g",
				deltaDeg, got, want, rel)
		}
	}
}

// How the hemispheric quadrature converges, and what limits it.
//
// The integral is over solid angle, dOmega = sin(z) dz dphi. The azimuth half
// is periodic and a uniform rule is spectrally accurate there; the zenith half
// is midpoint, which converges as the square of the step for a smooth
// integrand.
//
// # The finding
//
// It is not the rule that limits the accuracy, it is the field. Against a
// smooth field the midpoint rule converges cleanly and quadratically. Against
// a field with an edge in it the error stops falling and oscillates at a few
// tenths of a per cent however fine the grid gets, which is what any
// quadrature does on a discontinuity — the error becomes a question of where
// the samples happen to fall relative to the edge.
//
// That matters because the real incoming field HAS edges: it is built from a
// HEALPix star map and a dust map, both piecewise constant. So replacing
// midpoint with a higher-order rule would buy nothing on the term that
// actually dominates, and the honest way to buy accuracy is more samples.
// Which is affordable now, and is why the default is what it is.
func TestScatteredInQuadratureConvergence(t *testing.T) {
	t.Parallel()

	view := coord.NewAltAz(angle.Deg(55), angle.Deg(200))

	// A smooth field: radiance varying gently with altitude, no edge anywhere.
	smooth := func(_ context.Context, dst skybrightness.SpectralRadiance, dir coord.AltAz) error {
		v := 1e-9 * (1 + 0.5*math.Cos(2*dir.Alt().Radians()))
		for i := range dst {
			dst[i] += v
		}

		return nil
	}

	// A field with an edge: a band twenty degrees wide across the hemisphere,
	// standing in for the Milky Way and for the pixel edges of a real map.
	edged := func(_ context.Context, dst skybrightness.SpectralRadiance, dir coord.AltAz) error {
		const inclination = 60 * math.Pi / 180

		alt, az := dir.Alt().Radians(), dir.Az().Radians()
		z := math.Sin(alt)*math.Cos(inclination) - math.Cos(alt)*math.Sin(az)*math.Sin(inclination)

		if math.Abs(math.Asin(math.Max(-1, math.Min(1, z)))) > 10*math.Pi/180 {
			return nil
		}

		for i := range dst {
			dst[i] += 1e-9
		}

		return nil
	}

	counts := []int{4, 8, 12, 16, 24, 32, 48}

	for _, c := range []struct {
		name  string
		field skybrightness.SkyRadiance
	}{
		{"smooth", smooth},
		{"with an edge", edged},
	} {
		// A fine run stands in for the true value.
		truth := scatteredAt(t, c.field, view, 200)
		if truth <= 0 {
			t.Fatalf("%s: the reference integral is %g", c.name, truth)
		}

		t.Logf("")
		t.Logf("  field %s:", c.name)
		t.Logf("    %6s %14s %10s", "rings", "value", "error")

		errs := map[int]float64{}

		for _, rings := range counts {
			got := scatteredAt(t, c.field, view, rings)
			errs[rings] = math.Abs(got-truth) / truth

			t.Logf("    %6d %14.6e %9.4f%%", rings, got, 100*errs[rings])
		}

		if c.name == "smooth" {
			// Quadratic convergence: quadrupling the rings should cut the
			// error by about sixteen. Allowing a factor of four of slack still
			// distinguishes a working rule from a broken one.
			if ratio := errs[8] / errs[32]; ratio < 4 {
				t.Errorf("smooth field: 8 rings is %.3g and 32 is %.3g, a factor of %.1f; "+
					"midpoint should be roughly quadratic and this is not converging",
					errs[8], errs[32], ratio)
			}

			// The default has to be well under the physical uncertainty, and
			// it is: half a per cent is six thousandths of a magnitude,
			// against the 0.046 mag by which this module and GAMBONS disagree
			// about the same sky. One per cent is the bound because that is
			// still an order of magnitude below what the model itself is
			// worth; the measured value is 0.55, and a change that pushed it
			// past one would mean the rule had degraded rather than merely
			// moved.
			if def := errs[skybrightness.DefaultScatteringRings]; def > 0.01 {
				t.Errorf("smooth field: the default of %d rings is %.3f per cent off, "+
					"which is no longer an order of magnitude below the physical uncertainty",
					skybrightness.DefaultScatteringRings, 100*def)
			}
		}

		if c.name == "with an edge" {
			// No bound here of the kind the smooth case gets, because there is
			// nothing to converge to: the error against a discontinuity is set
			// by where the samples land, not by the step, and it does not fall
			// with resolution. What is asserted is only that it stays within
			// the band it wanders in, so a change that made the rule much
			// worse would still show.
			//
			// This is not the accuracy of a real evaluation. A real field is
			// edged, but its edges are HEALPix pixels far smaller than a
			// quadrature cell, and integrated over the hemisphere they average
			// out: measured against the published star map, twelve rings and
			// twenty-four differ by four hundredths of a per cent. This
			// integrand is one large edge and is the adversarial case.
			for rings, e := range errs {
				if e > 0.10 {
					t.Errorf("with an edge: %d rings is %.1f per cent off, beyond the band "+
						"this rule wanders in", rings, 100*e)
				}
			}
		}
	}
}
