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

// Each preset carries the vertical profile OPAC gives its own aerosol type.
//
// # Why this is worth a test
//
// Because it was missing, and its absence was invisible. The constructors set
// the optical properties and left the scale height at zero, so an atmosphere
// built from the recommended path looked complete, built without error, and
// was then refused by ArtificialSkyglow and CloudySkyglow when they finally
// read it — a failure three calls away from its cause.
//
// The values are Table 5's Z column, and the spread between them is the point:
// continental and urban aerosol is mixed through the atmosphere as the air is,
// which the paper states by saying Z = 8 km is "the value valid for air
// molecules"; sea salt is generated at the surface and falls out, at 1 km;
// desert dust is lofted but heavy, at 2 km. A single value across all four —
// 1538 m, say, which is 1/beta for the beta Kocifaj (2007) runs — would be one
// paper's continental fit applied to the ocean and the Sahara alike.
func TestAerosolPresetsCarryTheirOPACScaleHeight(t *testing.T) {
	t.Parallel()

	const (
		site = 2635.0
		aod  = 0.1
	)

	for _, c := range []struct {
		name  string
		build func(heightM, aod550 float64) *Builder
		wantM float64
	}{
		{"rural", RuralAerosol, ContinentalScaleHeightM},
		{"urban", UrbanAerosol, ContinentalScaleHeightM},
		{"desert", DesertAerosol, DesertScaleHeightM},
		{"maritime", MaritimeAerosol, MaritimeScaleHeightM},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			air, err := c.build(site, aod).Build()
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			got := float64(air.Aerosol().ScaleHeight)

			if got != c.wantM {
				t.Errorf("scale height is %g m, want %g", got, c.wantM)
			}

			// Zero is the specific failure this guards: it builds cleanly and
			// is rejected later.
			if got <= 0 {
				t.Error("a preset produced an atmosphere the skyglow components refuse")
			}
		})
	}

	// Table 5's own values, in kilometres, so a transcription slip shows here
	// rather than as a subtly wrong sky.
	for _, c := range []struct {
		name   string
		gotM   float64
		wantKM float64
	}{
		{"continental and urban", ContinentalScaleHeightM, 8},
		{"desert", DesertScaleHeightM, 2},
		{"maritime", MaritimeScaleHeightM, 1},
	} {
		if c.gotM != c.wantKM*1000 {
			t.Errorf("%s: %g m, and OPAC Table 5 gives Z = %g km", c.name, c.gotM, c.wantKM)
		}
	}
}

// The indicative optical depths are ordered and physically plausible.
//
// # What this guards, given they are three constants
//
// A transposition. These are the numbers a caller with no measurement types
// instead of inventing one, and their whole value is that clean < continental
// < urban — swap two and every downstream answer stays positive, stays
// smooth, and is wrong in a direction nobody notices, since an aerosol
// optical depth of 0.3 is as valid-looking as one of 0.03.
//
// The bounds are deliberately loose. They are not a claim about any site: an
// AOD below 0.01 is cleaner than the cleanest observed background and one
// above 1 is a dust storm, so anything landing outside says the constant was
// mistyped rather than that a judgement was revised.
func TestIndicativeAODsAreOrderedAndPlausible(t *testing.T) {
	t.Parallel()

	ordered := []struct {
		name string
		aod  float64
	}{
		{"CleanMountainAOD550", CleanMountainAOD550},
		{"ContinentalAOD550", ContinentalAOD550},
		{"UrbanAOD550", UrbanAOD550},
	}

	for i, c := range ordered {
		if c.aod < 0.01 || c.aod > 1 {
			t.Errorf("%s is %g, outside the 0.01 to 1 an aerosol optical depth at 550 nm "+
				"plausibly takes", c.name, c.aod)
		}

		if i > 0 && c.aod <= ordered[i-1].aod {
			t.Errorf("%s is %g and %s is %g; these are ordered from the cleanest air to "+
				"the dirtiest, and the ordering is the whole of what they assert",
				c.name, c.aod, ordered[i-1].name, ordered[i-1].aod)
		}
	}
}

// The four types added alongside the original set carry OPAC Table 3's own
// numbers, and sit where the physics says they should.
//
// # Why an ordering test rather than only the literals
//
// Because the literals are the thing most likely to be wrong, and comparing
// them against themselves proves nothing. OPAC's Table 3 is printed as a
// column-major block that pdftotext renders with the single-scattering
// albedo column offset from its row labels — read naively it assigns
// Maritime clean an albedo of 0.888 and Desert 0.817, which would make
// desert dust more absorbing than sea salt is reflective and urban soot less
// absorbing than dust. The orderings below are what distinguishes a correct
// transcription from that one, and they are physical facts about the
// aerosols rather than restatements of the table.
func TestAerosolPresets_AddedTypesMatchOPAC(t *testing.T) {
	t.Parallel()

	// Soot is what absorbs, and OPAC's continental series adds it in steps:
	// clean has none at all, average has some, polluted has 2 ug m^-3, urban
	// 7.8. Single-scattering albedo must fall monotonically along that series.
	continental := []struct {
		name string
		ssa  float64
	}{
		{"Continental clean", continentalCleanSSA},
		{"Continental average", continentalAverageSSA},
		{"Continental polluted", continentalPollutedSSA},
		{"Urban", urbanSSA},
	}

	for i := 1; i < len(continental); i++ {
		if continental[i].ssa >= continental[i-1].ssa {
			t.Errorf("%s SSA %v is not below %s's %v; OPAC's continental series adds soot "+
				"at every step and soot is what absorbs",
				continental[i].name, continental[i].ssa,
				continental[i-1].name, continental[i-1].ssa)
		}
	}

	// The maritime series runs the other way from the continental one: sea
	// salt is nearly non-absorbing, and pollution carried out over the water
	// is what lowers the albedo.
	if !(maritimePollutedSSA < maritimeCleanSSA) {
		t.Errorf("Maritime polluted SSA %v is not below Maritime clean's %v",
			maritimePollutedSSA, maritimeCleanSSA)
	}

	if !(maritimeCleanSSA < maritimeTropicalSSA) {
		t.Errorf("Maritime clean SSA %v is not below Maritime tropical's %v; tropical is the "+
			"cleanest air OPAC tabulates outside the polar types",
			maritimeCleanSSA, maritimeTropicalSSA)
	}

	// Every albedo is a fraction, and none reaches one: a real tropospheric
	// aerosol always absorbs something.
	for _, c := range []struct {
		name string
		ssa  float64
	}{
		{"Continental clean", continentalCleanSSA},
		{"Continental polluted", continentalPollutedSSA},
		{"Maritime polluted", maritimePollutedSSA},
		{"Maritime tropical", maritimeTropicalSSA},
	} {
		if c.ssa <= 0 || c.ssa >= 1 {
			t.Errorf("%s SSA %v is not in (0,1)", c.name, c.ssa)
		}
	}

	// Particle size again: the continental types are fine-particle and steep,
	// the maritime ones coarse and nearly grey. Maritime tropical at 0.04 is
	// the flattest in the table.
	if continentalCleanAngstrom <= 1 || continentalPollutedAngstrom <= 1 {
		t.Errorf("continental Angstrom exponents %v and %v should be above 1 (fine particles)",
			continentalCleanAngstrom, continentalPollutedAngstrom)
	}

	if maritimeTropicalAngstrom >= maritimePollutedAngstrom {
		t.Errorf("Maritime tropical Angstrom %v is not below Maritime polluted's %v; the "+
			"pollution contributes particles far smaller than sea salt",
			maritimeTropicalAngstrom, maritimePollutedAngstrom)
	}
}

// The added types build, and carry the scale height OPAC's Table 5 gives for
// their family.
//
// Continental clean and polluted share the 8 km molecular scale height with
// the rest of the continental series; both maritime variants share sea
// salt's 1 km. Getting this wrong is invisible in the optical properties and
// changes every height-resolved answer.
func TestAerosolPresets_AddedTypesCarryTheirScaleHeight(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		build func(heightM, aod550 float64) *Builder
		want  float64
	}{
		{"ContinentalCleanAerosol", ContinentalCleanAerosol, ContinentalScaleHeightM},
		{"ContinentalPollutedAerosol", ContinentalPollutedAerosol, ContinentalScaleHeightM},
		{"MaritimePollutedAerosol", MaritimePollutedAerosol, MaritimeScaleHeightM},
		{"MaritimeTropicalAerosol", MaritimeTropicalAerosol, MaritimeScaleHeightM},
	} {
		air, err := c.build(0, 0.1).Build()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if got := float64(air.Aerosol().ScaleHeight); got != c.want {
			t.Errorf("%s scale height is %g m, want %g", c.name, got, c.want)
		}

		if src := air.Provenance().Source.Name; src == "" {
			t.Errorf("%s records no source; every preset names its OPAC type", c.name)
		}
	}
}
