package atmosphere

// Aerosol-type reference optical properties from Hess, M., P. Koepke, and
// I. Schult (1998), "Optical Properties of Aerosols and Clouds: The
// Software Package OPAC," Bull. Amer. Meteor. Soc., 79, 831-844, Table 3
// ("Selected optical properties of all tropospheric aerosol types at a
// relative humidity of 80%"), at OPAC's 0.55um reference wavelength.
// Values transcribed 2026-08-09 directly from the paper's real digital
// PDF (BAMS/AMS, not a scan) via pdftotext, cross-checked for internal
// physical consistency before use (see the doc comment on
// aerosolBuilder below).
//
// Only the aerosol's spectral/scattering *shape* is fixed here — single-
// scattering albedo, asymmetry parameter, and the Angstrom exponent used
// to extrapolate optical depth away from 550nm. Aerosol optical depth
// itself is a real, time-varying quantity with no defensible constant
// value; every constructor below takes it as a caller-supplied
// parameter, never a baked-in number.
//
// 80% RH is OPAC's own published illustrative reference case for this
// table, not a "dry" or "0% RH" assumption — the water-soluble
// components in Continental/Urban/Maritime aerosol are hygroscopic, so
// these values sit toward the moist end of that type's real range. A
// caller modeling a specific humidity regime should treat these as a
// documented starting point, not a universal constant.
//
// OPAC tabulates two Angstrom exponents per type (0.35-0.5um and
// 0.5-0.8um spectral bands); each preset below uses the 0.5-0.8um value
// — astrogo's spectral grids run visible-to-near-IR (e.g.
// skybrightness.DefaultOpticalGrid spans 330-1000nm) and 550nm sits at
// that band's blue edge. A single power-law Angstrom exponent is itself
// an approximation of a real spectrum's curvature regardless of which
// band is chosen.
const (
	aerosolRefWavelengthNM = 550.0

	continentalAverageSSA      = 0.925
	continentalAverageAsymm    = 0.703
	continentalAverageAngstrom = 1.42

	urbanSSA      = 0.817
	urbanAsymm    = 0.689
	urbanAngstrom = 1.43

	desertSSA      = 0.888
	desertAsymm    = 0.729
	desertAngstrom = 0.17

	maritimeCleanSSA      = 0.997
	maritimeCleanAsymm    = 0.772
	maritimeCleanAngstrom = 0.08
)

// RuralAerosol seeds a Builder with OPAC's "Continental average" aerosol
// type (Hess, Koepke & Schult 1998) — background continental aerosol
// away from major urban or desert sources, the OPAC analogue of the
// classical Shettle & Fenn (1979) "Rural" model. aod550 is the caller-
// supplied aerosol optical depth at 550nm; single-scattering albedo,
// asymmetry parameter, and Angstrom exponent come from the published
// model (see the package-level doc comment above for the 80% RH
// caveat). Surface pressure/temperature come from the ICAO ISA profile
// at heightM, matching StandardDefault. Chain further Builder calls
// (PrecipitableWater, SurfaceAlbedo, Source, ...) before Build().
func RuralAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, continentalAverageSSA, continentalAverageAsymm, continentalAverageAngstrom,
		"OPAC Continental average aerosol (Hess, Koepke & Schult 1998)")
}

// UrbanAerosol seeds a Builder with OPAC's "Urban" aerosol type (Hess,
// Koepke & Schult 1998) — continental aerosol modified by soot and
// other combustion/industrial products, the most absorbing (lowest
// single-scattering albedo) of the four presets in this file. See
// RuralAerosol's doc comment for parameter/caveat details shared by all
// four constructors.
func UrbanAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, urbanSSA, urbanAsymm, urbanAngstrom,
		"OPAC Urban aerosol (Hess, Koepke & Schult 1998)")
}

// DesertAerosol seeds a Builder with OPAC's "Desert" aerosol type (Hess,
// Koepke & Schult 1998) — coarse mineral dust, distinct from the
// generic continental models: markedly lower Angstrom exponent (flatter
// spectral extinction, characteristic of large particles) than
// Rural/Urban aerosol. See RuralAerosol's doc comment for parameter/
// caveat details shared by all four constructors.
func DesertAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, desertSSA, desertAsymm, desertAngstrom,
		"OPAC Desert aerosol (Hess, Koepke & Schult 1998)")
}

// MaritimeAerosol seeds a Builder with OPAC's "Maritime clean" aerosol
// type (Hess, Koepke & Schult 1998) — sea-salt-dominated aerosol over
// open ocean away from continental/pollution influence, the least
// absorbing (highest single-scattering albedo) of the four presets in
// this file. OPAC also publishes "Maritime polluted" and "Maritime
// tropical" variants (not exposed here); this constructor is the clean-
// air baseline. See RuralAerosol's doc comment for parameter/caveat
// details shared by all four constructors.
func MaritimeAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, maritimeCleanSSA, maritimeCleanAsymm, maritimeCleanAngstrom,
		"OPAC Maritime clean aerosol (Hess, Koepke & Schult 1998)")
}

// aerosolBuilder is the shared construction path for the four named
// aerosol-type presets above: ISA surface conditions at heightM
// (mirroring StandardDefault), the given aerosol optical properties at
// aod550/550nm, and a Source provenance record naming the OPAC type.
// FidelityPrior matches StandardDefault's own choice — a cited
// reference/climatological value, not a live site measurement.
func aerosolBuilder(heightM, aod550, ssa, g, angstrom float64, sourceName string) *Builder {
	b := &Builder{s: Atmosphere{surface: AtAltitude(heightM)}}
	b.Aerosol(aod550, aerosolRefWavelengthNM, angstrom, ssa, g)
	b.s.provenance.Source = SourceRef{Name: sourceName, Fidelity: FidelityPrior}

	return b
}
