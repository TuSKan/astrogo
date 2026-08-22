package skybrightness_test

import (
	"errors"
	"math"
	"testing"

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

	for _, p := range []skybrightness.Preset{skybrightness.GAMBONSWeb, skybrightness.NaturalSky} {
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
		preset skybrightness.Preset
		want   float64
		why    string
	}{
		{skybrightness.GAMBONSWeb, 0.5, "the value the GAMBONS web service uses"},
		{skybrightness.NaturalSky, 0.75, "Duriscoe (2013), after Kwon (1989)"},
	} {
		got, err := c.preset.DiffuseKappa()
		if err != nil {
			t.Fatalf("%s: %v", c.preset, err)
		}

		if got != c.want {
			t.Errorf("%s: kappa = %v, want %v — %s", c.preset, got, c.want, c.why)
		}

		// Hong et al. (1998) bound it; a value outside that range would not be
		// one either paper supports.
		if got < 0.5 || got > 0.9 {
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
