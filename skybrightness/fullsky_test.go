package skybrightness_test

import (
	"context"
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// uniformDust is a stand-in 100 micron map: a constant intensity everywhere,
// so the diffuse-galactic term is exercised without needing the SFD product.
type uniformDust float64

func (u uniformDust) IntensityAt(_, _ angle.Angle) (float64, error) { return float64(u), nil }

// assembleSky builds a Model holding every component the module has, over a
// scene at Paranal. It is the thing the engine exists to do, and until this
// existed the components had never been run together.
func assembleSky(t *testing.T, when gotime.Time) (*skybrightness.Model, *skybrightness.Scene, unit.SpectralGrid) {
	t.Helper()

	grid := skybrightness.DefaultOpticalGrid()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.02, 550, 1.3, 0.95, 0.65).
		AerosolScaleHeight(1500).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	scene := &skybrightness.Scene{
		Observer:   loc,
		Time:       when,
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}

	moon, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	// Capped against the same star map the starlight component uses, which is
	// the pairing Masana et al. intend: dust cannot scatter more light than
	// the stars along that sightline provide.
	dgl, err := skybrightness.NewDiffuseGalacticLight(
		uniformDust(3.0), uniformSky{value: 8.05e-10, galactic: true}, testBand())
	if err != nil {
		t.Fatalf("NewDiffuseGalacticLight: %v", err)
	}

	// A flat airglow reference, standing in for an ESO SkyCalc spectrum.
	zenith := skybrightness.NewSpectralRadiance(grid)
	for i := range zenith {
		zenith[i] = 1.5e-9
	}

	glow, err := skybrightness.NewAirglow(zenith, grid, 0, false)
	if err != nil {
		t.Fatalf("NewAirglow: %v", err)
	}

	city, err := coord.NewGeodetic(angle.Deg(-70.1), angle.Deg(-24.4), 2000)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	artificial, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{
		&skybrightness.UniformEmitter{
			At:           city,
			Name:         "Antofagasta",
			WavelengthNM: []unit.WavelengthNM{300, 600, 1100},
			Radiance:     []float64{1e-5, 1e-5, 1e-5},
			Emission:     skybrightness.UpwardEmission{Cosine: 1, HorizontalFraction: 0.3},
			Flags:        skybrightness.AssumedSourceSpectrum | skybrightness.AssumedEmissionFunction,
		},
	})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	// A uniform star map at the radiance the order-6 Gaia patch measured off
	// the Galactic plane, 8.05e-10 W m^-2 sr^-1 nm^-1 in V.
	//
	// That measurement predates the magnitude cut and therefore includes
	// resolved bright stars, which carry about a fifth of the light at G = 6.
	// A cut map is the quantity a background model should use, so this figure
	// is an overestimate of the diffuse background and will move once a cut
	// map has been built and re-measured. It is a plausible number for a
	// composition test, not a validated one. A real
	// map varies by two orders of magnitude between the plane and the caps;
	// this is the quiet end, so the share integrated starlight takes here is a
	// floor rather than a typical value.
	//
	// The value was measured in Johnson V and is normalised here against the
	// synthetic band the rest of this file uses. The two overlap closely enough
	// for a composition test on a smooth solar-like shape, but it is the
	// mismatch StarMap's own documentation warns about, and a production caller
	// passes the band its map was actually built for.
	stars, err := skybrightness.NewIntegratedStarlight(
		uniformSky{value: 8.05e-10, galactic: true}, solarShape(grid), grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	model, err := skybrightness.NewModel("full-sky-test",
		moon, skybrightness.NewZodiacalLight(), dgl, glow, artificial, stars)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	return model, scene, grid
}

// vMagPerArcsec projects a spectrum's 554 nm radiance to mag/arcsec^2 against
// the Johnson V zero point.
func vMagPerArcsec(t *testing.T, grid unit.SpectralGrid, s skybrightness.SpectralRadiance) float64 {
	t.Helper()

	return toMagPerArcsec(s[gridIndex(t, grid, 554)])
}

// The engine, assembled and run. Every component contributes, the total is the
// linear sum, and the answer lands where a real dark site does.
//
// A moonless night at Paranal reads about 21.5 to 22.0 mag/arcsec^2 in V. The
// components here are a reference airglow, a uniform stand-in dust map and one
// modest city, so the total is not a prediction of that particular night — but
// it has to land in the range a dark sky occupies, and each term has to carry
// the share the literature gives it.
func TestFullSkyAssembles(t *testing.T) {
	t.Parallel()

	// A new Moon epoch, so the sky is genuinely dark and the natural terms
	// dominate.
	when, phase := nearFullMoonUp(t)
	dark := when.Add(14 * 24 * gotime.Hour)

	model, scene, grid := assembleSky(t, dark)

	est, err := model.Estimate(context.Background(), skybrightness.Query{
		Scene:     scene,
		Direction: coord.NewAltAz(angle.Deg(70), angle.Deg(120)),
		Grid:      grid,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	total := vMagPerArcsec(t, grid, est.SpectralRadiance())
	t.Logf("full-moon reference epoch was phase %.1f deg; dark epoch total = %.2f mag/arcsec^2",
		phase.Degrees(), total)

	if est.Quality.Flags&skybrightness.NoComponents != 0 {
		t.Fatal("the model reported NoComponents with five registered")
	}

	// Every component must have been evaluated and recorded.
	for _, id := range []skybrightness.ComponentID{
		skybrightness.Moonlight, skybrightness.Zodiacal,
		skybrightness.DiffuseGalactic, skybrightness.AirglowContinuum,
		skybrightness.Artificial,
	} {
		if _, ok := est.Component(id); !ok {
			t.Errorf("component %q produced no spectrum", id)
		}
	}

	// The total is the linear sum of the parts, band by band. Summing
	// magnitudes instead would be a correctness bug, and this is what
	// catches it.
	sum := skybrightness.NewSpectralRadiance(grid)

	for _, id := range est.ComponentIDs() {
		part, _ := est.Component(id)
		if err := sum.Add(part); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	got := est.SpectralRadiance()
	for i := range got {
		// Relative, not absolute: the engine and this test add the parts in
		// different orders, so the last bit differs. What must hold is that
		// the total is the linear sum and not, say, a magnitude sum — which
		// would be wrong by tens of per cent, not by an ulp.
		if rel := math.Abs(got[i]-sum[i]) / sum[i]; rel > 1e-12 {
			t.Fatalf("band %d: total %v is not the sum of parts %v (relative %.2e)",
				i, got[i], sum[i], rel)
		}
	}

	if total < 18 || total > 24 {
		t.Errorf("total = %.2f mag/arcsec^2, want a plausible dark-sky value", total)
	}
}

// Each component's share, on a dark night, against what the literature says
// it should be. This is the test that would catch a term entering with the
// wrong scale — the kind of error that leaves the total plausible while the
// composition is wrong.
func TestFullSkyComponentShares(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	model, scene, grid := assembleSky(t, when.Add(14*24*gotime.Hour))

	est, err := model.Estimate(context.Background(), skybrightness.Query{
		Scene:     scene,
		Direction: coord.NewAltAz(angle.Deg(80), angle.Deg(200)),
		Grid:      grid,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	idx := gridIndex(t, grid, 554)
	total := est.SpectralRadiance()[idx]

	if total <= 0 {
		t.Fatal("the assembled sky has no radiance at 554 nm")
	}

	for _, id := range est.ComponentIDs() {
		part, _ := est.Component(id)

		if part[idx] <= 0 {
			t.Logf("%-18s absent in this geometry", id)

			continue
		}

		t.Logf("%-18s %8.4g W m^-2 sr^-1 nm^-1  %5.1f%%  %.2f mag/arcsec^2",
			id, part[idx], 100*part[idx]/total, toMagPerArcsec(part[idx]))
	}

	// Airglow and zodiacal light are the two largest natural terms at a dark
	// site away from the Milky Way; Leinert et al. and Masana et al. both
	// put airglow first.
	airglow, _ := est.Component(skybrightness.AirglowContinuum)
	zodiacal, _ := est.Component(skybrightness.Zodiacal)

	if airglow[idx] <= 0 || zodiacal[idx] <= 0 {
		t.Fatalf("a natural term is absent: airglow %v, zodiacal %v", airglow[idx], zodiacal[idx])
	}

	if share := zodiacal[idx] / total; share < 0.1 || share > 0.7 {
		t.Errorf("zodiacal is %.1f%% of the sky, want a substantial but not dominant share", share*100)
	}

	// At a dark site with one modest city 30 km away, the natural sky must
	// outweigh the artificial one. If it does not, either the fixture is not
	// a dark site or a term has entered with the wrong scale — and the total
	// would still look plausible while the composition was wrong.
	artificial, _ := est.Component(skybrightness.Artificial)
	if natural := airglow[idx] + zodiacal[idx]; artificial[idx] >= natural {
		t.Errorf("artificial %.4g outweighs the natural %.4g at a dark site", artificial[idx], natural)
	}
}

// The engine must answer for every direction above the horizon without
// failing, including the awkward ones: the zenith, the horizon itself, and
// the azimuth wrap.
func TestFullSkyAllDirections(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	model, scene, grid := assembleSky(t, when)

	idx := gridIndex(t, grid, 554)

	for _, alt := range []float64{90, 75, 60, 45, 30, 15, 5, 1} {
		for _, az := range []float64{0, 45, 90, 180, 270, 359.9} {
			est, err := model.Estimate(context.Background(), skybrightness.Query{
				Scene:     scene,
				Direction: coord.NewAltAz(angle.Deg(alt), angle.Deg(az)),
				Grid:      grid,
			})
			if err != nil {
				t.Fatalf("alt %v az %v: %v", alt, az, err)
			}

			v := est.SpectralRadiance()[idx]
			if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("alt %v az %v produced %v", alt, az, v)
			}
		}
	}
}

// The sky brightens toward the horizon. Airglow's van Rhijn enhancement, the
// artificial term's own geometry and the longer scattering path all push the
// same way, so this is the aggregate behaviour of the assembled engine rather
// than any one component's.
func TestFullSkyBrightensTowardTheHorizon(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	model, scene, grid := assembleSky(t, when.Add(14*24*gotime.Hour))

	idx := gridIndex(t, grid, 554)

	at := func(alt float64) float64 {
		est, err := model.Estimate(context.Background(), skybrightness.Query{
			Scene:     scene,
			Direction: coord.NewAltAz(angle.Deg(alt), angle.Deg(90)),
			Grid:      grid,
		})
		if err != nil {
			t.Fatalf("alt %v: %v", alt, err)
		}

		return est.SpectralRadiance()[idx]
	}

	if zenith, low := at(88), at(10); low <= zenith {
		t.Errorf("10 deg altitude gives %.4g, not more than the zenith's %.4g", low, zenith)
	}
}

// Quality flags from every component reach the estimate, so a caller can see
// what the number rests on. An assembled sky with assumed source spectra and
// a climatological airglow must say both.
func TestFullSkyPropagatesQuality(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	model, scene, grid := assembleSky(t, when)

	est, err := model.Estimate(context.Background(), skybrightness.Query{
		Scene:     scene,
		Direction: coord.NewAltAz(angle.Deg(40), angle.Deg(10)),
		Grid:      grid,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	for _, want := range []struct {
		flag skybrightness.Flag
		name string
	}{
		{skybrightness.ClimatologicalAirglow, "ClimatologicalAirglow"},
		{skybrightness.AssumedSourceSpectrum, "AssumedSourceSpectrum"},
		{skybrightness.AssumedEmissionFunction, "AssumedEmissionFunction"},
		{skybrightness.ApproximateMultipleScattering, "ApproximateMultipleScattering"},
	} {
		if est.Quality.Flags&want.flag == 0 {
			t.Errorf("the estimate does not carry %s", want.name)
		}
	}

	t.Logf("quality: %s", est.Quality.Flags)
}

// Provenance for every component travels with the result, so a number can be
// traced to the papers behind it.
func TestFullSkyProvenance(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	model, scene, grid := assembleSky(t, when)

	est, err := model.Estimate(context.Background(), skybrightness.Query{
		Scene:     scene,
		Direction: coord.NewAltAz(angle.Deg(60), angle.Deg(0)),
		Grid:      grid,
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	if n := len(est.Reproducibility.Components); n != 6 {
		t.Fatalf("got %d provenance records, want 6", n)
	}

	for _, p := range est.Reproducibility.Components {
		if p.PrimaryReference == "" {
			t.Errorf("%q has no primary reference", p.Model)
		}

		if len(p.KnownApproximations) == 0 {
			t.Errorf("%q lists no approximations, which is unlikely to be true", p.Model)
		}
	}
}

func BenchmarkFullSkyEstimate(b *testing.B) {
	t := &testing.T{}

	when := gotime.Date(2026, 3, 3, 5, 0, 0, 0, gotime.UTC)
	model, scene, grid := assembleSky(t, when)

	if t.Failed() {
		b.Skip("fixture setup failed")
	}

	dir := coord.NewAltAz(angle.Deg(60), angle.Deg(45))
	q := skybrightness.Query{Scene: scene, Direction: dir, Grid: grid}

	// Warm the per-scene caches, which a sky map amortises.
	if _, err := model.Estimate(context.Background(), q); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := model.Estimate(context.Background(), q); err != nil {
			b.Fatal(err)
		}
	}
}
