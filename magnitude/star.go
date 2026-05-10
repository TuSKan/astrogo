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
// photometric documentation (Riello et al. 2021, Table 5.7):
//
//	V − G = −0.02704 + 0.01424·(BP−RP) − 0.2156·(BP−RP)² + 0.01426·(BP−RP)³
//
// Valid for −0.5 < BP−RP < 5.0 mag. Outside this range the polynomial
// extrapolates linearly.
//
// Parameters:
//   - G: Gaia DR3 G-band magnitude
//   - bpMinusRp: Gaia BP − RP colour index
func GaiaGToJohnsonV(G, bpMinusRp float64) float64 {
	c := bpMinusRp
	dV := -0.02704 + 0.01424*c - 0.2156*c*c + 0.01426*c*c*c

	return G + dV
}

// GaiaGToJohnsonB converts a Gaia DR3 G-band magnitude to an approximate
// Johnson B-band magnitude:
//
//	B − G = −0.02907 + 0.6399·(BP−RP) − 0.09631·(BP−RP)² + 0.01023·(BP−RP)³
//
// From Gaia DR3 photometric documentation.
func GaiaGToJohnsonB(G, bpMinusRp float64) float64 {
	c := bpMinusRp
	dB := -0.02907 + 0.6399*c - 0.09631*c*c + 0.01023*c*c*c

	return G + dB
}
