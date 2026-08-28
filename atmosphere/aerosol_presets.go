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

	continentalCleanSSA      = 0.972
	continentalCleanAsymm    = 0.709
	continentalCleanAngstrom = 1.42

	continentalPollutedSSA      = 0.892
	continentalPollutedAsymm    = 0.698
	continentalPollutedAngstrom = 1.45

	maritimePollutedSSA      = 0.975
	maritimePollutedAsymm    = 0.756
	maritimePollutedAngstrom = 0.35

	maritimeTropicalSSA      = 0.998
	maritimeTropicalAsymm    = 0.774
	maritimeTropicalAngstrom = 0.04
)

// Aerosol scale heights, in metres, from OPAC's Table 5 "Height profiles of
// all aerosol types".
//
// # What Z is
//
// The paper's Eq. 5d distributes particles as
//
//	N(h) = N(0) * exp(-h/Z)
//
// with h the altitude above ground and Z "the scale height in kilometres,
// which describes the slope of the profile". That is the same profile
// [Builder.AerosolScaleHeight] carries: extinction is proportional to number
// density for a fixed size distribution, so the two share a scale height and
// no conversion is needed.
//
// # Why these differ so much between types
//
// They are describing different physics rather than different tunings. Of
// continental and urban aerosol the paper says Z = 8 km is "the value valid
// for air molecules", so those types are mixed through the atmosphere exactly
// as the air is. Sea salt is not: maritime aerosol is generated at the surface
// and falls out quickly, giving 1 km. Desert dust sits between, lofted but
// heavy, at 2 km.
//
// # What is deliberately not taken from Table 5
//
// Its H column, the thickness of the boundary-layer aerosol layer, and Hft,
// the free troposphere above it. OPAC stacks up to four discrete layers;
// [Atmosphere] carries one unbounded exponential, so only Z applies. For the
// continental and urban types the distinction barely arises, since the free
// troposphere carries Z = 8 km as well and the profile is continuous across
// the boundary.
const (
	// ContinentalScaleHeightM is OPAC's Z for the three continental types
	// and for Urban: 8 km, the molecular scale height.
	ContinentalScaleHeightM = 8000

	// DesertScaleHeightM is OPAC's Z for Desert: 2 km.
	DesertScaleHeightM = 2000

	// MaritimeScaleHeightM is OPAC's Z for the three maritime types: 1 km.
	MaritimeScaleHeightM = 1000
)

// Indicative aerosol optical depths at 550 nm, one per broad regime.
//
// # What these are, and what they are not
//
// They are starting points, not measurements, and they are not from OPAC.
// OPAC (Hess, Koepke & Schult 1998) supplies the aerosol *optical properties*
// the constructors below carry — single-scattering albedo, asymmetry
// parameter, Angstrom exponent — which are properties of an aerosol type and
// are legitimately constant for it. Optical depth is not: it is how much of
// that aerosol is overhead right now, it varies by an order of magnitude at
// one site across a year, and this package's own doc says so.
//
// So these exist for one purpose: to stop a caller inventing a number when
// they have none. A value typed from nowhere is indistinguishable from a
// measurement once it is in the code, and these at least carry their status
// in the name. Anyone who needs the real figure should fetch it — see
// [github.com/TuSKan/astrogo/atmosphere/dataset/cams.AOD550], which reads the
// Copernicus analysis for a site and an hour — or take it from an AERONET
// station.
//
// The ranges in each comment are the spread a regime plausibly covers; the
// constant is a representative value within it.
const (
	// CleanMountainAOD550 is a high, dry, remote site — the cleanest air
	// routinely observed. Roughly 0.02 to 0.05. Paranal and Mauna Kea sit
	// here, and so does a polar site.
	CleanMountainAOD550 = 0.03

	// ContinentalAOD550 is ordinary inland background air away from cities.
	// Roughly 0.05 to 0.15, and the value most temperate rural sites spend
	// most nights near.
	ContinentalAOD550 = 0.10

	// UrbanAOD550 is a city or its outskirts. Roughly 0.2 to 0.5, higher
	// under an inversion or downwind of industry, and the regime where the
	// aerosol term dominates the sky rather than merely contributing to it.
	UrbanAOD550 = 0.30
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
	return aerosolBuilder(heightM, aod550, continentalAverageSSA, continentalAverageAsymm, continentalAverageAngstrom, ContinentalScaleHeightM,
		"OPAC Continental average aerosol (Hess, Koepke & Schult 1998)")
}

// UrbanAerosol seeds a Builder with OPAC's "Urban" aerosol type (Hess,
// Koepke & Schult 1998) — continental aerosol modified by soot and
// other combustion/industrial products, the most absorbing (lowest
// single-scattering albedo) of the four presets in this file. See
// RuralAerosol's doc comment for parameter/caveat details shared by all
// four constructors.
func UrbanAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, urbanSSA, urbanAsymm, urbanAngstrom, ContinentalScaleHeightM,
		"OPAC Urban aerosol (Hess, Koepke & Schult 1998)")
}

// DesertAerosol seeds a Builder with OPAC's "Desert" aerosol type (Hess,
// Koepke & Schult 1998) — coarse mineral dust, distinct from the
// generic continental models: markedly lower Angstrom exponent (flatter
// spectral extinction, characteristic of large particles) than
// Rural/Urban aerosol. See RuralAerosol's doc comment for parameter/
// caveat details shared by all four constructors.
func DesertAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, desertSSA, desertAsymm, desertAngstrom, DesertScaleHeightM,
		"OPAC Desert aerosol (Hess, Koepke & Schult 1998)")
}

// MaritimeAerosol seeds a Builder with OPAC's "Maritime clean" aerosol
// type (Hess, Koepke & Schult 1998) — sea-salt-dominated aerosol over
// open ocean away from continental/pollution influence, the least
// absorbing (highest single-scattering albedo) of the continental and
// maritime presets in this file. OPAC's "Maritime polluted" and "Maritime
// tropical" variants are [MaritimePollutedAerosol] and
// [MaritimeTropicalAerosol]; this constructor is the clean-air baseline.
// See RuralAerosol's doc comment for parameter/caveat details shared by
// all eight constructors.
func MaritimeAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, maritimeCleanSSA, maritimeCleanAsymm, maritimeCleanAngstrom, MaritimeScaleHeightM,
		"OPAC Maritime clean aerosol (Hess, Koepke & Schult 1998)")
}

// ContinentalCleanAerosol seeds a Builder with OPAC's "Continental clean"
// aerosol type (Hess, Koepke & Schult 1998) — remote continental air with
// very low anthropogenic influence, which OPAC composes with no soot at
// all and calls "a lower benchmark with respect to absorption in the solar
// spectral range". It is the least absorbing continental type and the
// natural companion to [RuralAerosol], which is OPAC's Continental average
// and does contain soot. See RuralAerosol's doc comment for parameter/
// caveat details shared by all eight constructors.
func ContinentalCleanAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, continentalCleanSSA, continentalCleanAsymm, continentalCleanAngstrom, ContinentalScaleHeightM,
		"OPAC Continental clean aerosol (Hess, Koepke & Schult 1998)")
}

// ContinentalPollutedAerosol seeds a Builder with OPAC's "Continental
// polluted" aerosol type (Hess, Koepke & Schult 1998) — areas highly
// polluted by man-made activity, carrying 2 ug m^-3 of soot and more than
// twice the water-soluble mass of Continental average. It sits between
// [RuralAerosol] and [UrbanAerosol] in absorption. See RuralAerosol's doc
// comment for parameter/caveat details shared by all eight constructors.
func ContinentalPollutedAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, continentalPollutedSSA, continentalPollutedAsymm, continentalPollutedAngstrom, ContinentalScaleHeightM,
		"OPAC Continental polluted aerosol (Hess, Koepke & Schult 1998)")
}

// MaritimePollutedAerosol seeds a Builder with OPAC's "Maritime polluted"
// aerosol type (Hess, Koepke & Schult 1998) — sea salt with continental
// pollution carried out over the water, so it absorbs measurably more than
// [MaritimeAerosol] and its Angstrom exponent is four times as steep, the
// pollution contributing particles far smaller than sea salt. See
// RuralAerosol's doc comment for parameter/caveat details shared by all
// eight constructors.
func MaritimePollutedAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, maritimePollutedSSA, maritimePollutedAsymm, maritimePollutedAngstrom, MaritimeScaleHeightM,
		"OPAC Maritime polluted aerosol (Hess, Koepke & Schult 1998)")
}

// MaritimeTropicalAerosol seeds a Builder with OPAC's "Maritime tropical"
// aerosol type (Hess, Koepke & Schult 1998) — the cleanest air OPAC
// tabulates outside the polar types, with a single-scattering albedo of
// 0.998 and an Angstrom exponent of 0.04, which is very nearly grey: sea
// salt is large enough that extinction barely varies across the visible.
// See RuralAerosol's doc comment for parameter/caveat details shared by
// all eight constructors.
func MaritimeTropicalAerosol(heightM, aod550 float64) *Builder {
	return aerosolBuilder(heightM, aod550, maritimeTropicalSSA, maritimeTropicalAsymm, maritimeTropicalAngstrom, MaritimeScaleHeightM,
		"OPAC Maritime tropical aerosol (Hess, Koepke & Schult 1998)")
}

// aerosolBuilder is the shared construction path for the eight named
// aerosol-type presets above: ISA surface conditions at heightM
// (mirroring StandardDefault), the given aerosol optical properties at
// aod550/550nm, and a Source provenance record naming the OPAC type.
// FidelityPrior matches StandardDefault's own choice — a cited
// reference/climatological value, not a live site measurement.
func aerosolBuilder(heightM, aod550, ssa, g, angstrom, scaleHeightM float64, sourceName string) *Builder {
	b := &Builder{s: Atmosphere{surface: AtAltitude(heightM)}}
	b.Aerosol(aod550, aerosolRefWavelengthNM, angstrom, ssa, g)
	b.AerosolScaleHeight(scaleHeightM)
	b.s.provenance.Source = SourceRef{Name: sourceName, Fidelity: FidelityPrior}

	return b
}
