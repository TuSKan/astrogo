package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

func presetInputs(t *testing.T) skybrightness.PresetInputs {
	t.Helper()

	grid, err := unit.NewSpectralGrid(400, 1, 401)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	shape := skybrightness.NewSpectralRadiance(grid)
	glow := skybrightness.NewSpectralRadiance(grid)

	// Deliberately sloped, and deliberately unequal to the star map.
	//
	// A flat shape leaves the starlight number invariant under any permutation
	// of the grid, so a wavelength-indexing error would not show; and an
	// airglow equal to the starlight makes the two indistinguishable at the
	// zenith, where the van Rhijn factor is one, so a preset that registered
	// them the wrong way round would produce identical numbers. Neither
	// degeneracy is worth keeping in a fixture whose whole purpose is to be
	// discriminating.
	for i := range shape {
		shape[i] = 1 + 0.002*(float64(grid.At(i))-400)
		glow[i] = 2.5e-9
	}

	return skybrightness.PresetInputs{
		Stars:         uniformSky{value: 1e-9, galactic: true},
		StarShape:     shape,
		Dust:          uniformDust(2.0),
		AirglowZenith: glow,
		Grid:          grid,
		Band: magnitude.Passband{
			Name:            "test V",
			WavelengthNM:    []unit.WavelengthNM{499, 500, 600, 601},
			Response:        []float64{0, 1, 1, 0},
			Detector:        magnitude.EnergyIntegrating,
			VegaZeroPointJy: 3636,
		},
	}
}

// Each preset builds, and builds the five natural components Eq. 10 names.
//
// Not moonlight and not artificial skyglow: GAMBONS models a moonless night
// with no artificial light, and a preset that quietly included either would
// answer a different question from the one asked of it.
func TestPresetsBuildTheNaturalSky(t *testing.T) {
	t.Parallel()

	want := map[skybrightness.ComponentID]bool{
		skybrightness.Starlight:        true,
		skybrightness.DiffuseGalactic:  true,
		skybrightness.Extragalactic:    true,
		skybrightness.Zodiacal:         true,
		skybrightness.AirglowContinuum: true,
	}

	for _, p := range []skybrightness.Preset{
		skybrightness.GAMBONSWeb, skybrightness.NaturalSky, skybrightness.GAMBONSFull,
	} {
		model, err := skybrightness.NewPreset(p, presetInputs(t))
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}

		if model.Version() != string(p) {
			t.Errorf("%s: model version is %q; an estimate should name the preset that made it",
				p, model.Version())
		}

		got := map[skybrightness.ComponentID]bool{}
		for _, id := range model.Components() {
			got[id] = true
		}

		for id := range want {
			if !got[id] {
				t.Errorf("%s: no %s component", p, id)
			}
		}

		for _, unwanted := range []skybrightness.ComponentID{
			skybrightness.Moonlight, skybrightness.Artificial, skybrightness.Twilight,
		} {
			if got[unwanted] {
				t.Errorf("%s: registered %s, which is not part of the natural sky", p, unwanted)
			}
		}
	}
}

// The transfer factor each preset carries, and where it comes from.
func TestPresetDiffuseKappa(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		preset      skybrightness.Preset
		want        float64
		hongBounded bool
		why         string
	}{
		{skybrightness.GAMBONSWeb, 0.5, true, "the value the GAMBONS web service uses"},
		{skybrightness.NaturalSky, 0.75, true, "Duriscoe (2013), after Kwon (1989)"},
		{skybrightness.GAMBONSFull, 1, false,
			"not a scattering choice but the absence of one: the full model puts the " +
				"scattered light in Eq. 11, so the direct term carries the true extinction"},
	} {
		got, err := c.preset.DiffuseKappa()
		if err != nil {
			t.Fatalf("%s: %v", c.preset, err)
		}

		if got != c.want {
			t.Errorf("%s: kappa = %v, want %v — %s", c.preset, got, c.want, c.why)
		}

		// Hong et al. (1998) bound the effective-depth factor to 0.5 to 0.9,
		// and a preset that uses one must sit inside it. GAMBONSFull does not
		// use one at all, so the bound does not apply to it — checking it
		// there would be asserting that a model has an approximation it was
		// specifically defined without.
		if c.hongBounded && (got < 0.5 || got > 0.9) {
			t.Errorf("%s: kappa = %v is outside the 0.5 to 0.9 of Hong et al. (1998)", c.preset, got)
		}
	}

	if _, err := skybrightness.Preset("no-such-preset").DiffuseKappa(); !errors.Is(err, skybrightness.ErrPreset) {
		t.Error("an unknown preset returned a kappa rather than an error")
	}
}

// A preset resolves no data, so it must refuse rather than substitute when
// something it needs is absent.
func TestPresetRefusesMissingInputs(t *testing.T) {
	t.Parallel()

	full := presetInputs(t)

	for _, c := range []struct {
		name    string
		breakIt func(*skybrightness.PresetInputs)
	}{
		{"no star map", func(in *skybrightness.PresetInputs) { in.Stars = nil }},
		{"no dust map", func(in *skybrightness.PresetInputs) { in.Dust = nil }},
		{"no airglow spectrum", func(in *skybrightness.PresetInputs) { in.AirglowZenith = nil }},
		{"no starlight shape", func(in *skybrightness.PresetInputs) { in.StarShape = nil }},
		{"no grid", func(in *skybrightness.PresetInputs) { in.Grid = unit.SpectralGrid{} }},
	} {
		in := full
		c.breakIt(&in)

		if _, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, in); !errors.Is(err, skybrightness.ErrPreset) {
			t.Errorf("%s: err = %v, want ErrPreset", c.name, err)
		}
	}

	if _, err := skybrightness.NewPreset("no-such-preset", full); !errors.Is(err, skybrightness.ErrPreset) {
		t.Errorf("an unknown preset: err = %v, want ErrPreset", err)
	}
}

// The two presets differ in their transfer and in nothing else, which is what
// makes the difference between them attributable.
func TestPresetsDifferOnlyInTransfer(t *testing.T) {
	t.Parallel()

	web, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, presetInputs(t))
	if err != nil {
		t.Fatalf("GAMBONSWeb: %v", err)
	}

	natural, err := skybrightness.NewPreset(skybrightness.NaturalSky, presetInputs(t))
	if err != nil {
		t.Fatalf("NaturalSky: %v", err)
	}

	same := func(a, b []skybrightness.ComponentID) bool {
		if len(a) != len(b) {
			return false
		}

		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}

		return true
	}

	if !same(web.Components(), natural.Components()) {
		t.Errorf("the presets register different components: %v against %v",
			web.Components(), natural.Components())
	}

	webK, _ := skybrightness.GAMBONSWeb.DiffuseKappa()
	naturalK, _ := skybrightness.NaturalSky.DiffuseKappa()

	if math.Abs(webK-naturalK) < 1e-9 {
		t.Error("the presets carry the same kappa, so nothing distinguishes them")
	}
}

// Each preset reports the fidelity it has to be evaluated at.
//
// This is the one way to hold GAMBONSFull wrong that produces a plausible
// number rather than an error: asked at Standard fidelity it applies the true
// extinction and never adds the scattering term, so the sky comes out too
// faint with nothing to say it went wrong. The preset reports the level so a
// caller never has to remember it.
func TestPresetFidelity(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		preset skybrightness.Preset
		want   skybrightness.Fidelity
	}{
		{skybrightness.GAMBONSWeb, skybrightness.Standard},
		{skybrightness.NaturalSky, skybrightness.Standard},
		{skybrightness.GAMBONSFull, skybrightness.Reference},
	} {
		got, err := c.preset.Fidelity()
		if err != nil {
			t.Fatalf("%s: %v", c.preset, err)
		}

		if got != c.want {
			t.Errorf("%s: fidelity %v, want %v", c.preset, got, c.want)
		}
	}

	if _, err := skybrightness.Preset("no-such-preset").Fidelity(); !errors.Is(err, skybrightness.ErrPreset) {
		t.Error("an unknown preset returned a fidelity rather than an error")
	}
}

// The full model is brighter than the same sky with no scattering treatment,
// and brighter than the simplified one at the zenith.
//
// Both directions are stated by Masana et al.: the scattered term adds light
// that the direct term alone does not carry, and the simplified model
// "underestimates the brightness at zenith". A preset that came out fainter
// than either would mean the integral is subtracting rather than adding.
func TestGAMBONSFullIsBrighterThanItsDirectTerm(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSFull, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	scene := presetGoldenScene(t, skybrightness.GAMBONSFull)

	at := func(f skybrightness.Fidelity) float64 {
		t.Helper()

		est, err := model.Estimate(t.Context(), skybrightness.Query{
			Scene:     scene,
			Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
			Grid:      in.Grid,
			Fidelity:  f,
		})
		if err != nil {
			t.Fatalf("estimate at %v: %v", f, err)
		}

		r, err := est.Radiance()
		if err != nil {
			t.Fatalf("Radiance: %v", err)
		}

		return float64(r)
	}

	direct := at(skybrightness.Standard)
	full := at(skybrightness.Reference)

	if full <= direct {
		t.Errorf("the full model gives %.6e and its direct term alone %.6e; the scattering "+
			"integral must add light, not remove it", full, direct)
	}

	t.Logf("zenith: direct %.6e, with the Eq. 11 term %.6e, scattered share %.1f per cent",
		direct, full, 100*(full-direct)/full)
}

// The two GAMBONS models differ in the direction and by the amount the paper
// says they do.
//
// Masana et al. (2024) Section 5, describing Fig. 2: "the simplified model
// overestimates the brightness of the natural sky near the horizon in all the
// bands, while it underestimates the brightness at zenith", and "it is
// expected that the simplified model differ less than 0.1 magnitude per arcsec
// for the most of the cases".
//
// Three claims, none of which anything here was fitted to. The kappa of 0.5
// comes from the web service, the scattering kernel from Kocifaj and Kranicz,
// and the crossover between them is a consequence rather than a parameter. If
// the integral had the wrong sign, the wrong solid-angle weighting or the
// wrong phase function, the sign of this difference is where it would show.
func TestTheTwoGAMBONSModelsDifferAsThePaperDescribes(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	brightness := func(p skybrightness.Preset, altDeg float64) float64 {
		t.Helper()

		model, err := skybrightness.NewPreset(p, in)
		if err != nil {
			t.Fatalf("%s: NewPreset: %v", p, err)
		}

		fidelity, err := p.Fidelity()
		if err != nil {
			t.Fatalf("%s: Fidelity: %v", p, err)
		}

		est, err := model.Estimate(t.Context(), skybrightness.Query{
			Scene:     presetGoldenScene(t, p),
			Direction: coord.NewAltAz(angle.Deg(altDeg), angle.Deg(45)),
			Grid:      in.Grid,
			Fidelity:  fidelity,
		})
		if err != nil {
			t.Fatalf("%s at %g: %v", p, altDeg, err)
		}

		sb, err := est.SurfaceBrightness(in.Band, magnitude.Vega)
		if err != nil {
			t.Fatalf("%s: SurfaceBrightness: %v", p, err)
		}

		return sb
	}

	t.Logf("  %5s %12s %12s %11s", "alt", "web", "full", "full - web")

	for _, altDeg := range []float64{90, 60, 30, 10} {
		web := brightness(skybrightness.GAMBONSWeb, altDeg)
		full := brightness(skybrightness.GAMBONSFull, altDeg)

		t.Logf("  %5.0f %12.6f %12.6f %+11.6f", altDeg, web, full, full-web)

		// Magnitudes run backwards, so a brighter sky is the smaller number.
		switch {
		case altDeg >= 30:
			if full >= web {
				t.Errorf("at %g degrees the full model gives %.4f and the simplified one "+
					"%.4f mag/arcsec2; the paper has the simplified model underestimating "+
					"the brightness away from the horizon, so the full one must be brighter",
					altDeg, full, web)
			}

		case altDeg <= 10:
			if full < web {
				t.Errorf("at %g degrees the full model gives %.4f and the simplified one "+
					"%.4f mag/arcsec2; the paper has the simplified model overestimating "+
					"the brightness near the horizon, so the full one must not be brighter",
					altDeg, full, web)
			}
		}

		if d := math.Abs(full - web); d > 0.1 {
			t.Errorf("at %g degrees the two models differ by %.4f mag, above the tenth of a "+
				"magnitude the paper expects between them", altDeg, d)
		}
	}
}
