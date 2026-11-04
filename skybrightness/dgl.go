package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for diffuse galactic light.
var (
	// ErrDustIntensity is returned for a negative or non-finite 100 micron
	// intensity.
	ErrDustIntensity = errors.New("skybrightness: 100 micron intensity must be non-negative and finite")
)

// Diffuse galactic light — starlight scattered by interstellar dust.
//
//   - Model: the optical/100 micron correlation, Kawara et al. (2017) Eq. 7,
//     applied as Masana et al. (2021) Eq. 13-14 do.
//   - Primary reference: Kawara, K. et al. (2017), PASJ 69, 31.
//   - Dust map: Schlegel, D.J., Finkbeiner, D.P. & Davis, M. (1998), ApJ
//     500, 525 — the 100 micron diffuse emission map, from IRAS and
//     COBE/DIRBE.
//
// DGL contributes typically 20 to 30 per cent of the integrated light of the
// Milky Way (Leinert et al. 1998), which is why it cannot simply be folded
// into integrated starlight.
//
// The physical basis is that DGL is mostly forward-scattered starlight off
// interstellar grains, so it tracks the dust column, and the dust column is
// what the 100 micron thermal emission measures. The relation is empirical,
// not derived.
const (
	// DustEmissionOffsetMJy is the extragalactic background contribution
	// subtracted from the SFD map before the correlation is applied, in
	// MJy sr^-1 (Matsuoka et al. 2011, via Kawara Eq. 7).
	//
	// It matters that this is subtracted: the SFD map's zero point includes
	// an isotropic extragalactic term whose optical and 100 micron emission
	// are uncorrelated, so leaving it in would add a spurious all-sky DGL
	// floor.
	DustEmissionOffsetMJy = 0.8

	// MaxDustIntensityMJy is the upper bound on the 100 micron intensity
	// over which the correlation is usable, after Masana et al. (2021).
	// Beyond it the sightline is optically thick, the correlation flattens,
	// and the quadratic form starts predicting nonsense.
	MaxDustIntensityMJy = 50.0

	// megajanskyPerSrToSI converts I_nu in MJy sr^-1 to W m^-2 Hz^-1 sr^-1.
	megajanskyPerSrToSI = 1e-20

	// speedOfLightNMPerS is c in nanometers per second, for the
	// per-frequency to per-wavelength conversion I_lambda = I_nu * c/lambda^2.
	speedOfLightNMPerS = 2.99792458e17
)

// dglCoefficient is one row of Kawara et al. (2017) Table 2.
type dglCoefficient struct {
	Lambda unit.WavelengthNM

	// NuB is the tabulated correlation slope in nW m^-2 sr^-1 per
	// MJy sr^-1. The table calls it nu*b_i; the dimensionless b_i of Eq. 7
	// is recovered as b = NuB * lambda(um) / 3000.
	NuB float64

	// SigmaNuB is the table's uncertainty on NuB, in the same units. Kawara
	// et al. quote twice what simple propagation over their four samples
	// gives, because the samples are not independent.
	SigmaNuB float64

	// C is the quadratic coefficient in units of 1e-5 (MJy sr^-1)^-1.
	C float64

	// SigmaC is the table's uncertainty on C, in the same units.
	SigmaC float64
}

// dglCoefficients is Kawara et al. (2017) Table 2, the eight FOS bands.
//
// # On the quadratic coefficient's power of ten
//
// The published table heads this column "10^5 (MJy sr^-1)^-1". Taken
// literally the quadratic term would overwhelm the linear one immediately
// and DGL would be negative over the entire sky, so the intended scale is
// 10^-5 and the printed sign is a typo.
//
// That is not a guess. The quadratic turns over at I_100 = b/(2c), and
// Kawara fit samples bounded at 5, 10, 15, 20, 30 and 50 MJy sr^-1 — Masana
// et al. then restrict the relation to I_100 < 50. With 1e-5 the six
// well-measured bands turn over between 40 and 50 MJy sr^-1: the top of the
// fitted range, exactly where a quadratic fitted to saturating data should
// turn. TestDGLTurnoverMatchesTheFittedRange asserts this for every band, so
// the evidence for the reading is executable rather than a comment.
var dglCoefficients = [8]dglCoefficient{
	{225.0, 3.0, 1.4, 0.1, 0.4},
	{274.0, 3.9, 0.3, 0.3, 0.1},
	{319.0, 6.1, 0.4, 0.7, 0.1},
	{369.0, 8.5, 0.5, 1.1, 0.2},
	{418.0, 13.6, 0.5, 2.4, 0.3},
	{472.0, 17.5, 0.6, 3.3, 0.3},
	{550.0, 20.1, 0.6, 4.4, 0.4},
	{648.0, 21.0, 0.9, 4.5, 0.7},
}

// dglQuadraticScale is the 1e-5 discussed above.
const dglQuadraticScale = 1e-5

// DGLCoefficientsAt returns the Kawara et al. (2017) correlation slope and
// quadratic coefficient at one wavelength, interpolated linearly between the
// eight tabulated bands.
//
// b is dimensionless and c is in (MJy sr^-1)^-1, the form Eq. 7 uses.
//
// Outside the tabulated 225 to 648 nm the nearest endpoint is held rather
// than extrapolated, and extrapolated reports true. Extrapolating matters
// most at the red end: the default optical grid runs to 1000 nm while the
// table stops at 648, and the slope is still rising there, so a linear
// extrapolation would keep climbing with nothing to check it.
func DGLCoefficientsAt(lambda unit.WavelengthNM) (b, c float64, extrapolated bool) {
	b, extrapolated = dglAt(lambda, dimensionlessSlope)
	c, _ = dglAt(lambda, func(row dglCoefficient) float64 { return row.C })

	return b, c * dglQuadraticScale, extrapolated
}

// dglCoefficientUncertaintiesAt returns the uncertainties of b and c at one
// wavelength, interpolated as [DGLCoefficientsAt] interpolates the
// coefficients and in the same units.
func dglCoefficientUncertaintiesAt(lambda unit.WavelengthNM) (sigmaB, sigmaC float64) {
	sigmaB, _ = dglAt(lambda, func(row dglCoefficient) float64 {
		return row.SigmaNuB * float64(row.Lambda) / 1000 / 3000
	})
	sigmaC, _ = dglAt(lambda, func(row dglCoefficient) float64 { return row.SigmaC })

	return sigmaB, sigmaC * dglQuadraticScale
}

// dglAt interpolates one quantity of the table's rows linearly at lambda,
// holding the nearest endpoint outside the tabulated 225 to 648 nm and
// reporting that it did.
func dglAt(lambda unit.WavelengthNM, of func(dglCoefficient) float64) (value float64, extrapolated bool) {
	first, last := dglCoefficients[0], dglCoefficients[len(dglCoefficients)-1]

	switch {
	case lambda <= first.Lambda:
		return of(first), lambda < first.Lambda
	case lambda >= last.Lambda:
		return of(last), lambda > last.Lambda
	}

	for i := 1; i < len(dglCoefficients); i++ {
		hi := dglCoefficients[i]
		if lambda > hi.Lambda {
			continue
		}

		lo := dglCoefficients[i-1]
		f := float64(lambda-lo.Lambda) / float64(hi.Lambda-lo.Lambda)

		return of(lo) + f*(of(hi)-of(lo)), false
	}

	return of(last), false
}

// dglOffsetUncertaintyMJy is the uncertainty of the non-ISM offset subtracted
// from the 100 micron intensity, [DustEmissionOffsetMJy]: 0.78 +/- 0.21
// MJy sr^-1 by Lagache et al. (2000), as Kawara et al. (2017) cite it.
const dglOffsetUncertaintyMJy = 0.21

// dglRelativeUncertainty is the relative uncertainty of the diffuse galactic
// light at one wavelength, along a sightline whose SFD 100 micron intensity is
// sfdIntensity, and whether one can be given at all.
//
// It propagates Eq. 7, D = b*I - c*I^2 with I = I_SFD - 0.8, to first order:
//
//	sigma_D^2 = I^2 sigma_b^2 + I^4 sigma_c^2 + (b - 2cI)^2 sigma_I^2
//
// The last term is Kawara et al.'s own Eq. 9, with sigma_I the offset's
// uncertainty; the first two are Table 2's, treated as independent because the
// paper gives no covariance. The SFD map's own error is not included: it is a
// property of the caller's dust map, not of this model.
//
// There is no figure outside the regime the correlation was fitted over: a
// sightline at or below the offset, above [MaxDustIntensityMJy], or past the
// quadratic's turnover.
func dglRelativeUncertainty(lambda unit.WavelengthNM, sfdIntensity float64) (float64, bool) {
	i100 := sfdIntensity - DustEmissionOffsetMJy
	if i100 <= 0 || i100 > MaxDustIntensityMJy {
		return 0, false
	}

	b, c, _ := DGLCoefficientsAt(lambda)
	if c > 0 && i100 >= b/(2*c) {
		return 0, false
	}

	sigmaB, sigmaC := dglCoefficientUncertaintiesAt(lambda)
	slope := b - 2*c*i100
	value := b*i100 - c*i100*i100

	variance := i100*i100*sigmaB*sigmaB +
		i100*i100*i100*i100*sigmaC*sigmaC +
		slope*slope*dglOffsetUncertaintyMJy*dglOffsetUncertaintyMJy

	return math.Sqrt(variance) / value, true
}

// dimensionlessSlope converts the tabulated nu*b_i into Eq. 7's b_i.
//
// Kawara's Table 2 states nu*b_i = [3000/lambda(um)] * b_i, where the 3000
// is exactly the factor turning I_nu in MJy sr^-1 into nu*I_nu in
// nW m^-2 sr^-1. Inverting it leaves b dimensionless, which is what Eq. 7
// needs since both sides of that equation are in MJy sr^-1.
func dimensionlessSlope(row dglCoefficient) float64 {
	return row.NuB * float64(row.Lambda) / 1000 / 3000
}

// MaxDGLToStarlightRatio caps the diffuse galactic light against the
// integrated starlight along the same sightline.
//
// Dust scatters starlight, so the DGL cannot exceed a bounded fraction of the
// starlight available to be scattered. Masana et al. (2021) apply 0.35 after
// Toller (1981). The correlation this package evaluates is fitted at high
// galactic latitude and has no such knowledge of its own: extended to a dusty
// low-latitude sightline it will happily predict more scattered light than
// there is starlight to scatter.
const MaxDGLToStarlightRatio = 0.35

// DiffuseGalacticRadiance accumulates the spectral radiance of starlight
// scattered by interstellar dust into dst, from the 100 micron intensity of
// the Schlegel, Finkbeiner & Davis (1998) map along that line of sight.
//
// Kawara et al. (2017) Eq. 7, with Eq. 14's offset:
//
//	I_nu(DGL) = b*I_100 - c*I_100^2       I_100 = I_SFD - 0.8 MJy sr^-1
//
// sfdIntensity is in MJy sr^-1, the unit the SFD map publishes. Both sides
// of Eq. 7 are per-frequency intensities, so the result is converted to the
// per-wavelength spectral radiance this module works in by
// I_lambda = I_nu * c / lambda^2.
//
// Three things are clamped, each because the empirical fit does not bound
// itself:
//
//   - A sightline whose SFD intensity is below the 0.8 MJy sr^-1
//     extragalactic offset contributes nothing rather than a negative
//     radiance.
//   - Past the quadratic's turnover the fit predicts falling and eventually
//     negative DGL. Zero is returned there and ExtrapolatedModel is
//     reported, since the model has left the regime it was fitted in.
//   - Above [MaxDustIntensityMJy] the sightline is optically thick and the
//     correlation flattens; the value is still returned, flagged.
//
// One constraint is deliberately not applied here. Masana et al. cap the
// DGL-to-integrated-starlight ratio at 0.35, after Toller (1981). That needs
// the starlight in the same direction, which this function does not have, so
// it belongs to whatever assembles the natural sky from its parts —
// [DiffuseGalacticLight] applies it when given a star map.
func DiffuseGalacticRadiance(dst SpectralRadiance, grid unit.SpectralGrid, sfdIntensity float64) (Flag, error) {
	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	if sfdIntensity < 0 || math.IsNaN(sfdIntensity) || math.IsInf(sfdIntensity, 0) {
		return 0, fmt.Errorf("%w: got %g MJy/sr", ErrDustIntensity, sfdIntensity)
	}

	dust := sfdIntensity - DustEmissionOffsetMJy
	if dust <= 0 {
		return 0, nil
	}

	var flags Flag

	if sfdIntensity > MaxDustIntensityMJy {
		flags |= ExtrapolatedModel
	}

	for i := range dst {
		lambda := grid.At(i)

		b, c, extrapolated := DGLCoefficientsAt(lambda)
		if extrapolated {
			flags |= ExtrapolatedModel
		}

		// Eq. 7, in MJy sr^-1 on both sides.
		intensity := b*dust - c*dust*dust
		if intensity <= 0 {
			flags |= ExtrapolatedModel

			continue
		}

		// I_nu -> I_lambda, and MJy sr^-1 -> SI.
		nm := float64(lambda)
		dst[i] += intensity * megajanskyPerSrToSI * speedOfLightNMPerS / (nm * nm)
	}

	return flags, nil
}
