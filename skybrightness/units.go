package skybrightness

import "math"

// SurfaceBrightnessV is sky surface brightness in V-band magnitudes per square
// arcsecond (mag/arcsec²). Because it is a magnitude, LARGER values are FAINTER
// (darker) skies. A pristine dark site is ~21.9 mag/arcsec²; bright urban skies
// are ~17–18.
type SurfaceBrightnessV float64

// Nanolambert is a linear sky-surface radiance in nanolamberts (nL), the unit
// used by Krisciunas & Schaefer (1991). Unlike [SurfaceBrightnessV], radiances
// are additive: the total sky brightness toward a pointing is the linear sum of
// its component radiances, and LARGER values are BRIGHTER skies.
//
// Combine sky-brightness components by summing Nanolambert values (use [Add] or
// the + operator), never by averaging or summing [SurfaceBrightnessV]
// magnitudes — magnitudes are logarithmic and summing them is a correctness
// bug.
type Nanolambert float64

// Coefficients for the nanolambert ↔ V mag/arcsec² conversion, from
// Krisciunas & Schaefer (1991), PASP 103, 1033 (after Garstang 1989):
//
//	B[nL] = 34.08 · exp(20.7233 − 0.92104 · V)
//
// Verified against the published model
// (https://adsabs.harvard.edu/abs/1991PASP..103.1033K).
const (
	// nlGarstangScale is the Garstang/KS zero-point scale (nL).
	nlGarstangScale = 34.08
	// nlGarstangExp is the KS 1991 exponent constant.
	nlGarstangExp = 20.7233
	// pogsonNat is the natural-log brightness change per magnitude, 0.4·ln(10).
	// KS 1991 writes it as the rounded literal 0.92104; using the exact value
	// keeps the doubling invariant (see Nanolambert) exact to machine precision.
	pogsonNat = 0.4 * math.Ln10
)

// Nanolamberts converts a V-band surface brightness to its linear radiance in
// nanolamberts via the Krisciunas & Schaefer (1991) / Garstang relation.
func (v SurfaceBrightnessV) Nanolamberts() Nanolambert {
	return Nanolambert(nlGarstangScale * math.Exp(nlGarstangExp-pogsonNat*float64(v)))
}

// SurfaceBrightnessV converts a linear radiance back to a V-band surface
// brightness (mag/arcsec²). A non-positive radiance represents zero flux and
// maps to an infinitely faint sky (+Inf mag).
func (b Nanolambert) SurfaceBrightnessV() SurfaceBrightnessV {
	if b <= 0 {
		return SurfaceBrightnessV(math.Inf(1))
	}

	return SurfaceBrightnessV((nlGarstangExp - math.Log(float64(b)/nlGarstangScale)) / pogsonNat)
}

// Add returns the linear sum of two radiances. This is the correct way to
// combine sky-brightness components; it is equivalent to the + operator and
// exists to document that intent.
func (b Nanolambert) Add(other Nanolambert) Nanolambert { return b + other }

// Constants for the millicandela-per-square-metre ↔ V mag/arcsec² conversion
// used by light-pollution atlases (Falchi et al. 2016; lightpollutionmap.info):
//
//	m[mag/arcsec²] = −2.5·log₁₀(L[mcd/m²] / 1.08e8)
//
// The natural zenith background 0.171168465 mcd/m² maps to 22.00 mag/arcsec².
// Source: lightpollutionmap.info/help.html. These are citable, not invented.
// mcdZeroPoint is the SQM photometric zero-point in mcd/m². The natural zenith
// background 0.171168465 mcd/m² maps through it to 22.0 mag/arcsec².
const mcdZeroPoint = 1.08e8

// SurfaceBrightnessFromMcdM2 converts a luminance in millicandelas per square
// metre (the unit used by light-pollution atlases) to a V-band surface
// brightness (mag/arcsec²) via m = −2.5·log₁₀(L/1.08e8). A non-positive
// luminance maps to an infinitely faint sky (+Inf mag).
func SurfaceBrightnessFromMcdM2(mcdM2 float64) SurfaceBrightnessV {
	if mcdM2 <= 0 {
		return SurfaceBrightnessV(math.Inf(1))
	}

	return SurfaceBrightnessV(-2.5 * math.Log10(mcdM2/mcdZeroPoint))
}

// McdM2 converts a V-band surface brightness back to a luminance in
// millicandelas per square metre.
func (v SurfaceBrightnessV) McdM2() float64 {
	return mcdZeroPoint * math.Pow(10, -0.4*float64(v))
}
