package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

func presetInputs(tb testing.TB) skybrightness.PresetInputs {
	tb.Helper()

	grid, err := unit.NewSpectralGrid(400, 1, 401)
	if err != nil {
		tb.Fatalf("NewSpectralGrid: %v", err)
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

	// A controlled comparison of one preset under two transfers, so exactly
	// one of the two runs can be the preset's own and [Model.Estimate] would
	// refuse the other. The direct term is not recoverable from the reference
	// estimate either: at Reference the scattered light is added into each
	// component's own buffer, precisely so a breakdown still attributes it.
	// So the measurement needs two evaluations and has to opt out to take
	// them.
	model = model.WithoutPreset()

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

// A preset refuses to be evaluated under somebody else's transfer.
//
// # What this guards
//
// [skybrightness.NewPreset] builds components; kappa, higher scattering orders
// and the fidelity that decides whether the Eq. 11 integral runs at all live on
// the caller's atmosphere and query. Three things to remember, and forgetting
// any of them used to return a plausible number: a sky smooth, positive,
// correctly shaped, and not the model whose name was on it.
//
// Each case below is a mistake a caller can make in one line, and each produced
// a usable-looking answer before the check existed.
func TestEstimateRefusesAMismatchedTransfer(t *testing.T) {
	t.Parallel()

	in := observatoryInputs(t)

	zenith := coord.NewAltAz(angle.Deg(90), angle.Deg(0))

	cases := []struct {
		name     string
		preset   skybrightness.Preset
		scene    skybrightness.Preset
		fidelity skybrightness.Fidelity
	}{{
		// 0.5 against 1 is a factor of two in the diffuse optical depth.
		name: "kappa too low", preset: skybrightness.GAMBONSFull,
		scene: skybrightness.GAMBONSWeb, fidelity: skybrightness.Reference,
	}, {
		name: "kappa too high", preset: skybrightness.GAMBONSWeb,
		scene: skybrightness.GAMBONSFull, fidelity: skybrightness.Standard,
	}, {
		// Same kappa, so only the scattering-order flag separates these two.
		name:   "higher orders on when the preset is first order",
		preset: skybrightness.GAMBONSFull, scene: skybrightness.Observatory,
		fidelity: skybrightness.Reference,
	}, {
		// The integral silently absent.
		name: "fidelity below the integral", preset: skybrightness.GAMBONSFull,
		scene: skybrightness.GAMBONSFull, fidelity: skybrightness.Standard,
	}, {
		// The integral added on top of a stand-in for it.
		name: "fidelity above a simplified transfer", preset: skybrightness.GAMBONSWeb,
		scene: skybrightness.GAMBONSWeb, fidelity: skybrightness.Reference,
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			model, err := skybrightness.NewPreset(c.preset, in)
			if err != nil {
				t.Fatalf("NewPreset: %v", err)
			}

			_, err = model.Estimate(t.Context(), skybrightness.Query{
				Scene:     presetGoldenScene(t, c.scene),
				Direction: zenith,
				Grid:      in.Grid,
				Fidelity:  c.fidelity,
			})
			if !errors.Is(err, skybrightness.ErrPresetMismatch) {
				t.Fatalf("got %v, want ErrPresetMismatch — this evaluates %q under %q's "+
					"transfer and returns a number for it", err, c.preset, c.scene)
			}
		})
	}
}

// The matching case is accepted, and so is a cheaper label for a preset that
// runs no integral.
//
// Without this the check could pass its own tests by rejecting everything.
// [skybrightness.Fast] is deliberately allowed where [skybrightness.Standard]
// is: the two run the same evaluation, and what the preset is defined by is
// whether the Eq. 11 integral runs, not which of the two cheaper labels a
// caller chose.
func TestEstimateAcceptsTheTransferThePresetNames(t *testing.T) {
	t.Parallel()

	in := observatoryInputs(t)
	zenith := coord.NewAltAz(angle.Deg(90), angle.Deg(0))

	for _, c := range []struct {
		name     string
		preset   skybrightness.Preset
		fidelity skybrightness.Fidelity
	}{
		{"web at standard", skybrightness.GAMBONSWeb, skybrightness.Standard},
		{"web at fast", skybrightness.GAMBONSWeb, skybrightness.Fast},
		{"natural at standard", skybrightness.NaturalSky, skybrightness.Standard},
		{"full at reference", skybrightness.GAMBONSFull, skybrightness.Reference},
		{"observatory at reference", skybrightness.Observatory, skybrightness.Reference},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			model, err := skybrightness.NewPreset(c.preset, in)
			if err != nil {
				t.Fatalf("NewPreset: %v", err)
			}

			if _, err := model.Estimate(t.Context(), skybrightness.Query{
				Scene:     presetGoldenScene(t, c.preset),
				Direction: zenith,
				Grid:      in.Grid,
				Fidelity:  c.fidelity,
			}); err != nil {
				t.Fatalf("Estimate: %v", err)
			}
		})
	}
}

// A hand-assembled model carries no preset and is not checked.
//
// Calling [skybrightness.NewModel] is a statement that the caller owns the
// transfer as well as the components, so there is nothing to contradict. A
// check that fired here would make the guard a tax on every model rather than
// a property of presets.
func TestModelsWithoutAPresetAreNotChecked(t *testing.T) {
	t.Parallel()

	model, err := skybrightness.NewModel("hand-assembled")
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	if _, ok := model.Preset(); ok {
		t.Error("a model from NewModel reports a preset")
	}

	// Any transfer at all, including one no preset uses.
	if _, err := model.Estimate(t.Context(), skybrightness.Query{
		Scene:     presetGoldenScene(t, skybrightness.Observatory),
		Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
		Grid:      skybrightness.DefaultOpticalGrid(),
		Fidelity:  skybrightness.Standard,
	}); err != nil {
		t.Fatalf("Estimate: %v", err)
	}
}

// WithoutPreset drops the check and keeps the components.
//
// The escape hatch has to actually work, because the alternative for anyone
// measuring what a transfer choice is worth is rebuilding the component set by
// hand and hoping it stays in step with [skybrightness.NewPreset].
func TestWithoutPresetDropsTheCheck(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSFull, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	free := model.WithoutPreset()

	if p, ok := model.Preset(); !ok || p != skybrightness.GAMBONSFull {
		t.Errorf("the original reports %q, %t — WithoutPreset must not mutate it", p, ok)
	}

	if _, ok := free.Preset(); ok {
		t.Error("the copy still reports a preset")
	}

	if a, b := model.Components(), free.Components(); len(a) != len(b) {
		t.Fatalf("the copy holds %d components against %d", len(b), len(a))
	}

	// The transfer the original would refuse.
	if _, err := free.Estimate(t.Context(), skybrightness.Query{
		Scene:     presetGoldenScene(t, skybrightness.GAMBONSFull),
		Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
		Grid:      in.Grid,
		Fidelity:  skybrightness.Standard,
	}); err != nil {
		t.Fatalf("Estimate: %v", err)
	}
}

// SkyMap refuses a mismatch too, and before it spends anything.
//
// A whole sky at reference fidelity samples the hemisphere once and then
// evaluates every direction, so a check that lived only in Estimate would
// report the mistake a few hundred component evaluations late and once per
// direction.
func TestSkyMapRefusesAMismatchedTransfer(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSFull, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	_, err = model.SkyMap(t.Context(), skybrightness.Query{
		Scene:    presetGoldenScene(t, skybrightness.GAMBONSWeb),
		Grid:     in.Grid,
		Fidelity: skybrightness.Reference,
	}, 3)
	if !errors.Is(err, skybrightness.ErrPresetMismatch) {
		t.Fatalf("got %v, want ErrPresetMismatch", err)
	}
}

// observatoryInputs adds what the Observatory preset needs beyond the natural
// sky: a solar spectrum for the lunar reflectance and a ground-emitter
// inventory for artificial skyglow.
func observatoryInputs(tb testing.TB) skybrightness.PresetInputs {
	tb.Helper()

	in := presetInputs(tb)
	in.SolarSpectrum = solarSpectrumFixture(tb)
	in.Emitters = []skybrightness.GroundEmitter{cityAt(tb, 0, 30, 1e-3)}

	return in
}

// Observatory registers the two components the reproductions must not.
//
// This is the difference between a preset that answers "what would GAMBONS
// say" and one that answers "how bright is this sky". Moonlight and artificial
// skyglow are absent from GAMBONS by design — it models the natural sky on a
// moonless night — so a preset carrying them cannot be validated against their
// export, and one lacking them cannot answer for a real night.
func TestObservatoryCarriesMoonlightAndSkyglow(t *testing.T) {
	t.Parallel()

	model, err := skybrightness.NewPreset(skybrightness.Observatory, observatoryInputs(t))
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	got := map[skybrightness.ComponentID]bool{}
	for _, id := range model.Components() {
		got[id] = true
	}

	for _, id := range []skybrightness.ComponentID{
		skybrightness.Starlight, skybrightness.DiffuseGalactic, skybrightness.Extragalactic,
		skybrightness.Zodiacal, skybrightness.AirglowContinuum,
		skybrightness.Moonlight, skybrightness.Artificial,
	} {
		if !got[id] {
			t.Errorf("no %s component", id)
		}
	}

	// And the reproductions still must not.
	for _, p := range []skybrightness.Preset{
		skybrightness.GAMBONSWeb, skybrightness.NaturalSky, skybrightness.GAMBONSFull,
	} {
		reproduction, err := skybrightness.NewPreset(p, observatoryInputs(t))
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}

		for _, id := range reproduction.Components() {
			if id == skybrightness.Moonlight || id == skybrightness.Artificial {
				t.Errorf("%s registered %s; supplying the inputs must not change what a "+
					"preset reproducing a moonless-night model contains", p, id)
			}
		}
	}
}

// Observatory refuses without the data only it needs.
//
// Both are inputs rather than defaults because neither can be invented: ROLO's
// absolute scale depends on the solar spectrum chosen, and satellite radiance
// alone cannot determine a source spectrum or an upward emission function. A
// preset that supplied either would be reporting somebody else's sky.
func TestObservatoryRefusesWithoutItsOwnInputs(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name    string
		breakIt func(*skybrightness.PresetInputs)
	}{
		{"no solar spectrum", func(in *skybrightness.PresetInputs) { in.SolarSpectrum = nil }},
		{"no emitters", func(in *skybrightness.PresetInputs) { in.Emitters = nil }},
	} {
		in := observatoryInputs(t)
		c.breakIt(&in)

		if _, err := skybrightness.NewPreset(skybrightness.Observatory, in); !errors.Is(
			err, skybrightness.ErrPreset,
		) {
			t.Errorf("%s: err = %v, want ErrPreset", c.name, err)
		}
	}
}

// Higher scattering orders add light, and only where they belong.
//
// Winkler (2022) puts the shortfall of a single-scattering treatment
// proportional to the molecular optical depth, so turning it on must brighten
// the sky. It applies to the scattered term alone: the direct term is
// extinction and has no scattering order, and under the simplified transfer
// the scattered light is already stood in for by kappa, so a factor there
// would count it twice.
func TestMultipleScatteringBrightensOnlyTheScatteredTerm(t *testing.T) {
	t.Parallel()

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSFull, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	at := func(multiple bool, fidelity skybrightness.Fidelity) float64 {
		t.Helper()

		atm, err := atmosphere.NewBuilder().
			Surface(743, 284).
			Aerosol(0.02, 550, 1.3, 0.95, 0.65).
			AerosolScaleHeight(1500).
			DiffuseScattering(1).
			MultipleScattering(multiple).
			Build()
		if err != nil {
			t.Fatalf("atmosphere Build: %v", err)
		}

		loc, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
		if err != nil {
			t.Fatalf("NewGeodetic: %v", err)
		}

		est, err := model.WithoutPreset().Estimate(t.Context(), skybrightness.Query{
			Scene: &skybrightness.Scene{
				Observer:   loc,
				Time:       gotime.Date(2026, 3, 20, 5, 0, 0, 0, gotime.UTC),
				Atmosphere: atm,
				Ephemeris:  eph.Default(),
			},
			Direction: coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
			Grid:      in.Grid,
			Fidelity:  fidelity,
		})
		if err != nil {
			t.Fatalf("Estimate: %v", err)
		}

		r, err := est.Radiance()
		if err != nil {
			t.Fatalf("Radiance: %v", err)
		}

		return float64(r)
	}

	off, on := at(false, skybrightness.Reference), at(true, skybrightness.Reference)

	if on <= off {
		t.Errorf("with higher orders %.6e, without %.6e; they must add light", on, off)
	}

	t.Logf("higher scattering orders add %.2f per cent at the zenith", 100*(on/off-1))

	// At Standard fidelity there is no scattered term for it to apply to, so
	// the setting must change nothing at all.
	if a, b := at(false, skybrightness.Standard), at(true, skybrightness.Standard); a != b {
		t.Errorf("without the scattering integral the setting changed the answer, "+
			"%.10e against %.10e; there is no scattered term for it to multiply", a, b)
	}
}

// Each preset reports whether it wants higher scattering orders.
//
// The third transport choice a scene has to carry, after kappa and fidelity,
// and the quietest of the three to get wrong: a scene that forgets it produces
// a sky a few per cent fainter with nothing to say why. Only the module's own
// model wants it — Eq. 11 is explicitly first order, so a scene claiming to be
// GAMBONS and carrying higher orders is no longer reproducing GAMBONS.
func TestPresetMultipleScattering(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		preset skybrightness.Preset
		want   bool
	}{
		{skybrightness.GAMBONSWeb, false},
		{skybrightness.NaturalSky, false},
		{skybrightness.GAMBONSFull, false},
		{skybrightness.Observatory, true},
	} {
		got, err := c.preset.MultipleScattering()
		if err != nil {
			t.Fatalf("%s: %v", c.preset, err)
		}

		if got != c.want {
			t.Errorf("%s: multiple scattering = %v, want %v", c.preset, got, c.want)
		}
	}

	if _, err := skybrightness.Preset("no-such-preset").MultipleScattering(); !errors.Is(
		err, skybrightness.ErrPreset,
	) {
		t.Error("an unknown preset answered rather than erroring")
	}
}
