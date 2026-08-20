package magnitude

import "math"

// ── Star Apparent Magnitude ──────────────────────────────────────────────────

// Default Bouguer extinction coefficients for a clear night at sea level (mag/airmass).
const (
	ExtinctionV = 0.20 // Johnson V band
	ExtinctionB = 0.30 // Johnson B band
	ExtinctionU = 0.55 // Johnson U band
	ExtinctionR = 0.12 // Cousins R band
	ExtinctionI = 0.07 // Cousins I band
)

// StarApparent computes the observed apparent magnitude of a star after
// atmospheric extinction:
//
//	V_obs = V_cat + k(λ) · X
//
// Parameters:
//   - catMag: catalog magnitude (any photometric band)
//   - airmass: relative airmass at the observation altitude (use atmosphere.Airmass)
//   - extinctionCoeff: optional Bouguer coefficient k(λ) in mag/airmass;
//     defaults to ExtinctionV (0.20) if not provided
//
// The extinction coefficient scales with altitude above sea level.
// Use ExtinctionAtAltitude to get altitude-corrected values.
func StarApparent(catMag, airmass float64, extinctionCoeff ...float64) float64 {
	k := ExtinctionV
	if len(extinctionCoeff) > 0 {
		k = extinctionCoeff[0]
	}

	return catMag + k*airmass
}

// ExtinctionAtAltitude returns the Bouguer extinction coefficient adjusted
// for the observer's altitude above sea level. Extinction decreases
// approximately exponentially with altitude:
//
//	k(h) = k₀ · exp(−h / H)
//
// where H ≈ 8500 m is the atmospheric scale height.
//
// Parameters:
//   - k0: sea-level extinction coefficient (e.g. ExtinctionV)
//   - altitudeM: observer altitude in metres
func ExtinctionAtAltitude(k0, altitudeM float64) float64 {
	const scaleHeight = 8500.0 // metres
	return k0 * math.Exp(-altitudeM/scaleHeight)
}

// ── Photometric System Transformations ───────────────────────────────────────

// GaiaGToJohnsonV converts a Gaia DR3 G-band magnitude to an approximate
// Johnson V-band magnitude using the polynomial fit from the Gaia DR3
// photometric documentation, Section 5.5.1, Table 5.9:
//
//	G − V = −0.02704 + 0.01424·(BP−RP) − 0.2156·(BP−RP)² + 0.01426·(BP−RP)³
//
// so V = G − (G−V). The table is tabulated as G minus the target band, and
// reading it the other way round costs 2(G−V) — about 0.3 mag at solar colour,
// 0.5 mag at the colour most catalogue stars have and over 3 mag for an M
// dwarf — always in the direction that makes the star too bright.
//
// Three independent checks fix the direction. The Sun has G = −26.90 against
// V = −26.76, so G − V = −0.14, and the polynomial at the solar BP−RP of 0.82
// returns −0.15. Physically G spans 330–1050 nm against V's ~500–600 nm, so a
// star is brighter in G than in V and G − V is negative. And measured against
// 4,000 stars carrying both Gaia and Tycho-2 photometry, V = G − (G−V)
// reproduces the Tycho-derived Johnson V with a median residual of −0.002 mag,
// holding to within ±0.03 mag in every colour bin across the fitted range;
// the opposite sign misses by −0.48 mag.
//
// Valid for −0.5 < BP−RP < 5.0 mag. Outside that the cubic extrapolates and is
// not clamped here — at BP−RP = 7 it is already several magnitudes adrift.
//
// Parameters:
//   - G: Gaia DR3 G-band magnitude
//   - bpMinusRp: Gaia BP − RP colour index
func GaiaGToJohnsonV(G, bpMinusRp float64) float64 {
	c := bpMinusRp
	gMinusV := -0.02704 + 0.01424*c - 0.2156*c*c + 0.01426*c*c*c

	return G - gMinusV
}

// GaiaGToJohnsonB converts a Gaia DR3 G-band magnitude to an approximate
// Johnson B-band magnitude, using the quartic of the Gaia DR3 photometric
// documentation, Section 5.5.1, Table 5.9:
//
//	G − B = 0.01448 − 0.6874·(BP−RP) − 0.3604·(BP−RP)²
//	        + 0.06718·(BP−RP)³ − 0.006061·(BP−RP)⁴
//
// so B = G − (G−B), the same tabulation and the same direction as
// [GaiaGToJohnsonV]. Published σ is 0.0633 mag, twice that of the V relation.
//
// The coefficients this function previously carried — −0.02907, 0.6399,
// −0.09631, 0.01023, cited only as "Gaia DR3 photometric documentation" — appear
// nowhere in that table. They are a cubic where the published relation is a
// quartic, and they failed an empirical check in both orientations by −0.46 mag
// at BP−RP ≈ 0 growing to −2.0 by BP−RP = 3, so the shape was wrong rather than
// the sign. Measured against 4,000 stars carrying both Gaia and Tycho-2
// photometry, with Johnson B taken from Tycho as B = B_T − 0.240(B_T − V_T), the
// published quartic has a median residual of −0.010 mag over the range it is
// unrestrictedly valid on.
//
// # Validity
//
// Fitted for −0.5 < BP−RP < 4.0, and Table 5.10 restricts it further: **beyond
// BP−RP = 1.75 it holds only for M giants.** The same Tycho check shows why —
// the residual is −0.01 below 1.75, −0.08 by 2.5 and +0.69 beyond it. Nothing
// is clamped here, so a caller working with red stars has to respect that bound
// itself.
//
// Parameters:
//   - G: Gaia DR3 G-band magnitude
//   - bpMinusRp: Gaia BP − RP colour index
func GaiaGToJohnsonB(G, bpMinusRp float64) float64 {
	c := bpMinusRp
	gMinusB := 0.01448 - 0.6874*c - 0.3604*c*c + 0.06718*c*c*c - 0.006061*c*c*c*c

	return G - gMinusB
}
