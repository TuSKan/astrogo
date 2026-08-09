package atmosphere

import "testing"

// TestAerosolPresets_BuildClean confirms every named preset builds
// without error for a physically reasonable AOD and reproduces the
// caller-supplied AOD and the published SSA/asymmetry/Angstrom values
// verbatim (no silent rounding/relabeling between the constructor and
// Build()).
func TestAerosolPresets_BuildClean(t *testing.T) {
	cases := []struct {
		name       string
		build      func(heightM, aod550 float64) *Builder
		wantSSA    float64
		wantAsymm  float64
		wantAngstr float64
	}{
		{"Rural", RuralAerosol, continentalAverageSSA, continentalAverageAsymm, continentalAverageAngstrom},
		{"Urban", UrbanAerosol, urbanSSA, urbanAsymm, urbanAngstrom},
		{"Desert", DesertAerosol, desertSSA, desertAsymm, desertAngstrom},
		{"Maritime", MaritimeAerosol, maritimeCleanSSA, maritimeCleanAsymm, maritimeCleanAngstrom},
	}

	const heightM = 500.0

	const aod550 = 0.15

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			atm, err := c.build(heightM, aod550).Build()
			if err != nil {
				t.Fatalf("%s: Build() error = %v", c.name, err)
			}

			a := atm.Aerosol()

			if got := float64(a.OpticalDepth); got != aod550 {
				t.Errorf("%s: OpticalDepth = %v, want %v", c.name, got, aod550)
			}

			if got := float64(a.ReferenceWavelength); got != aerosolRefWavelengthNM {
				t.Errorf("%s: ReferenceWavelength = %v, want %v", c.name, got, aerosolRefWavelengthNM)
			}

			if got := float64(a.SingleScatteringAlbedo); got != c.wantSSA {
				t.Errorf("%s: SingleScatteringAlbedo = %v, want %v", c.name, got, c.wantSSA)
			}

			if got := float64(a.Asymmetry); got != c.wantAsymm {
				t.Errorf("%s: Asymmetry = %v, want %v", c.name, got, c.wantAsymm)
			}

			if got := float64(a.AngstromExp); got != c.wantAngstr {
				t.Errorf("%s: AngstromExp = %v, want %v", c.name, got, c.wantAngstr)
			}

			wantSurface := AtAltitude(heightM)
			if got := atm.Refraction(); got != wantSurface {
				t.Errorf("%s: Refraction() = %+v, want %+v (AtAltitude(%v))", c.name, got, wantSurface, heightM)
			}
		})
	}
}

// TestAerosolPresets_SSAOrdering pins the physical ordering the real
// OPAC table shows: Urban (soot-heavy) is the most absorbing, Maritime
// clean (sea salt) the least, with Desert dust (mineral, mildly
// absorbing) and Rural/continental-average background (least polluted
// of the continental types) in between — Desert is slightly *more*
// absorbing than clean continental background in OPAC's own numbers,
// not less, which is why this is asserted against the transcribed table
// rather than an assumed ordering. This is the cross-check that gives
// confidence the transcribed values are attached to the right aerosol
// type, not just that they parse.
func TestAerosolPresets_SSAOrdering(t *testing.T) {
	if !(urbanSSA < desertSSA) {
		t.Errorf("Urban SSA (%v) should be less than Desert SSA (%v) — urban soot is more absorbing", urbanSSA, desertSSA)
	}

	if !(desertSSA < continentalAverageSSA) {
		t.Errorf("Desert SSA (%v) should be less than Rural/Continental-average SSA (%v)", desertSSA, continentalAverageSSA)
	}

	if !(continentalAverageSSA < maritimeCleanSSA) {
		t.Errorf("Rural/Continental-average SSA (%v) should be less than Maritime SSA (%v) — sea salt is nearly non-absorbing", continentalAverageSSA, maritimeCleanSSA)
	}
}

// TestAerosolPresets_AngstromOrdering pins the other real physical
// signature in the OPAC table: coarse-particle aerosols (Desert,
// Maritime) have a markedly flatter spectral extinction slope (lower
// Angstrom exponent) than fine-particle aerosols (Rural, Urban).
func TestAerosolPresets_AngstromOrdering(t *testing.T) {
	const coarseCeiling = 0.5 // Desert/Maritime must stay well below Rural/Urban's ~1.4

	if desertAngstrom >= coarseCeiling {
		t.Errorf("Desert Angstrom exponent (%v) should be well below %v (coarse mineral dust)", desertAngstrom, coarseCeiling)
	}

	if maritimeCleanAngstrom >= coarseCeiling {
		t.Errorf("Maritime Angstrom exponent (%v) should be well below %v (coarse sea salt)", maritimeCleanAngstrom, coarseCeiling)
	}

	if continentalAverageAngstrom <= coarseCeiling {
		t.Errorf("Rural/Continental-average Angstrom exponent (%v) should be well above %v (fine particles)", continentalAverageAngstrom, coarseCeiling)
	}

	if urbanAngstrom <= coarseCeiling {
		t.Errorf("Urban Angstrom exponent (%v) should be well above %v (fine particles)", urbanAngstrom, coarseCeiling)
	}
}

// TestAerosolPresets_NegativeAODErrors confirms the presets reuse
// Builder.Aerosol's own validation rather than bypassing it.
func TestAerosolPresets_NegativeAODErrors(t *testing.T) {
	if _, err := RuralAerosol(0, -0.1).Build(); err == nil {
		t.Error("RuralAerosol(0, -0.1).Build() should error on negative AOD")
	}
}

// TestAerosolPresets_Chainable confirms the returned *Builder composes
// with further Builder calls before Build() — the whole point of
// returning *Builder instead of a terminal *Atmosphere.
func TestAerosolPresets_Chainable(t *testing.T) {
	atm, err := DesertAerosol(1000, 0.2).
		PrecipitableWater(3).
		SurfaceAlbedo(UniformAlbedo(0.3)).
		Build()
	if err != nil {
		t.Fatalf("chained Build() error = %v", err)
	}

	if got := float64(atm.PrecipitableWater()); got != 3 {
		t.Errorf("PrecipitableWater() = %v, want 3 (chained call should not be overridden by the preset)", got)
	}
}
