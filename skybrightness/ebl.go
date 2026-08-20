package skybrightness

import (
	"context"
	"fmt"
	"sort"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// eblPoint is one published band of the integrated galaxy light.
type eblPoint struct {
	// PivotNM is the filter's pivot wavelength, Koushan et al. Table 1.
	PivotNM float64

	// NuInu is the band's integrated surface brightness in nW m^-2 sr^-1,
	// Koushan et al. Table 3, column "EBL".
	NuInu float64

	// TotalErrorPercent is that table's final column, the four listed error
	// terms added in quadrature as a percentage of the IGL.
	TotalErrorPercent float64

	// Band names the filter, carried so a reader can find the row.
	Band string
}

// koushanIGL is Table 3 of Koushan et al. (2021), MNRAS 503, 2033, with pivot
// wavelengths from its Table 1.
//
// These nine numbers were read from the paper's own source rather than from a
// summary of a publisher's page, because an earlier revision of
// docs/skybrightness.md recorded them from the latter and said in terms that
// they must be confirmed before entering the code. They were, and they matched;
// the point is that the check happened, not that it found anything.
//
// The quantity is nu*I_nu, an integrated surface brightness over the band, not
// a spectral radiance. Dividing by the pivot wavelength is what turns one into
// the other, and forgetting to is an error of a factor of several hundred.
var koushanIGL = []eblPoint{
	{PivotNM: 357.7, NuInu: 4.13, TotalErrorPercent: 6.87, Band: "u"},
	{PivotNM: 474.4, NuInu: 5.76, TotalErrorPercent: 4.02, Band: "g"},
	{PivotNM: 631.2, NuInu: 8.11, TotalErrorPercent: 4.08, Band: "r"},
	{PivotNM: 758.4, NuInu: 9.94, TotalErrorPercent: 4.44, Band: "i"},
	{PivotNM: 883.3, NuInu: 10.71, TotalErrorPercent: 5.20, Band: "Z"},
	{PivotNM: 1022.4, NuInu: 11.58, TotalErrorPercent: 4.52, Band: "Y"},
	{PivotNM: 1254.6, NuInu: 11.22, TotalErrorPercent: 4.92, Band: "J"},
	{PivotNM: 1647.7, NuInu: 11.17, TotalErrorPercent: 4.73, Band: "H"},
	{PivotNM: 2154.9, NuInu: 9.42, TotalErrorPercent: 5.21, Band: "Ks"},
}

// ExtragalacticBackground is the integrated light of galaxies too faint to
// resolve, arriving isotropically from outside the Galaxy.
//
// # What it is, and why it is a lower limit
//
// The value comes from counting galaxies and adding up their light, so it
// measures what has been detected. Anything genuinely diffuse, or any galaxy
// below the survey limit, is missing from it by construction. That makes the
// number a floor rather than a central estimate, and it is reported as one:
// [Provenance.KnownApproximations] says so, and callers combining components
// should read it that way.
//
// Koushan et al. bound the headroom from the other side. Their comparison
// against very-high-energy gamma-ray attenuation finds the two "fully
// consistent with our IGL measurements in the u-Ks range without the need to
// include any significant additional source of diffuse light", so the room
// above the floor is small rather than unknown.
//
// # Isotropy
//
// The extragalactic background has no direction dependence worth modelling at
// this level: its anisotropy is a cosmological signal orders of magnitude below
// the mean, and nothing else in this package would be sensitive to it. Only the
// airmass varies with direction, so the component is a constant spectrum with
// direct attenuation, the same treatment [IntegratedStarlight] gets.
//
// # Size
//
// It is small. At 630 nm the tabulated 8.11 nW m^-2 sr^-1 is about 1 per cent
// of the integrated starlight this package's own map carries, roughly 27.7
// mag arcsec^-2 against starlight's 22.8. Implementing it matters for
// completeness and for the error budget, not because it will move a total.
type ExtragalacticBackground struct {
	// spectral holds I_lambda in W m^-2 sr^-1 nm^-1 at each tabulated pivot
	// wavelength, precomputed so evaluation is interpolation only.
	spectral []float64
}

// NewExtragalacticBackground returns the component over the published table.
//
// It takes no arguments: unlike airglow or starlight there is nothing for a
// caller to supply, because the measurement is a single published spectrum with
// no site, epoch or geometry dependence.
func NewExtragalacticBackground() *ExtragalacticBackground {
	spectral := make([]float64, len(koushanIGL))

	for i, p := range koushanIGL {
		// nu*I_nu [nW m^-2 sr^-1] -> I_lambda [W m^-2 sr^-1 nm^-1].
		spectral[i] = p.NuInu * 1e-9 / p.PivotNM
	}

	return &ExtragalacticBackground{spectral: spectral}
}

// ID implements [Component].
func (e *ExtragalacticBackground) ID() ComponentID { return Extragalactic }

// AddRadiance implements [Component].
//
// Below the horizon there is no sky, so nothing is added.
func (e *ExtragalacticBackground) AddRadiance(
	_ context.Context,
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	dir coord.AltAz,
	scene *Scene,
) (Flag, error) {
	if dir.Alt() <= 0 {
		return 0, nil
	}

	airmass, err := atmosphere.Airmass(dir.Alt())
	if err != nil {
		return 0, fmt.Errorf("skybrightness: extragalactic: airmass: %w", err)
	}

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()

	flags := Flag(0)

	for i := range dst {
		lambda := grid.At(i)

		value, extrapolated := e.at(float64(lambda))
		if extrapolated {
			flags |= ExtrapolatedModel
		}

		rayleigh, err := atmosphere.RayleighOpticalDepth(lambda, float64(pressure))
		if err != nil {
			return 0, fmt.Errorf("skybrightness: extragalactic: %w", err)
		}

		slant := (rayleigh + unit.OpticalDepth(aerosol.TauAt(lambda))) * unit.OpticalDepth(airmass)

		dst[i] += value * float64(atmosphere.Transmission(slant))
	}

	return flags, nil
}

// Provenance implements [Component].
func (e *ExtragalacticBackground) Provenance() Provenance {
	return Provenance{
		Model:            "integrated galaxy light, isotropic, with direct attenuation",
		Version:          "koushan2021-table3",
		PrimaryReference: "Koushan, S. et al. (2021), MNRAS 503, 2033 (GAMA/DEVILS)",
		SecondaryReferences: []string{
			"Driver, S. P. et al. (2016b), ApJ 827, 108 — superseded over 0.3-2.2 um",
		},
		Equations: "Table 3 integrated EBL per band, pivot wavelengths from Table 1",
		ValidityDomain: "357.7-2154.9 nm, the pivot wavelengths of u to Ks; outside " +
			"that range the endpoint value is held and ExtrapolatedModel is set",
		KnownApproximations: []string{
			"The integrated galaxy light is a lower limit, not a central estimate: " +
				"it counts detected galaxies, so undetected sources and any truly " +
				"diffuse component are absent by construction. Koushan et al. find " +
				"very-high-energy gamma-ray attenuation consistent with these values " +
				"across u-Ks without needing additional diffuse light, which bounds " +
				"the headroom above the floor.",
			"Nine published bands are interpolated linearly in wavelength. The " +
				"spectrum is sampled, not resolved, and that coarseness is part of " +
				"the error budget rather than separate from it.",
			"Isotropic. The measured anisotropy of the extragalactic background is " +
				"far below the precision of anything else in this package.",
			"Only the directly attenuated term is applied; light scattered out of " +
				"the beam is not returned to it, matching IntegratedStarlight.",
		},
	}
}

// at interpolates the tabulated spectral radiance, reporting whether the
// wavelength fell outside the measured range.
//
// Outside the table the endpoint value is held rather than extrapolated. Nine
// points spanning 357.7 to 2154.9 nm do not constrain a slope beyond their
// ends, and a linear extrapolation of the blue end reaches zero at 240 nm and
// negative below it — a component that emits negative light is worse than one
// that is slightly wrong. The caller learns which happened from
// [ExtrapolatedModel].
//
// Interpolation is linear in wavelength. The IGL rises smoothly from u to Y and
// turns over by Ks, so a straight line between neighbouring pivots is within a
// few per cent of any smoother curve through the same points — comfortably
// inside the 4 to 7 per cent the measurements themselves carry.
func (e *ExtragalacticBackground) at(lambdaNM float64) (value float64, extrapolated bool) {
	n := len(koushanIGL)

	if lambdaNM <= koushanIGL[0].PivotNM {
		return e.spectral[0], lambdaNM < koushanIGL[0].PivotNM
	}

	if lambdaNM >= koushanIGL[n-1].PivotNM {
		return e.spectral[n-1], lambdaNM > koushanIGL[n-1].PivotNM
	}

	i := sort.Search(n, func(k int) bool { return koushanIGL[k].PivotNM >= lambdaNM })

	lo, hi := koushanIGL[i-1].PivotNM, koushanIGL[i].PivotNM
	t := (lambdaNM - lo) / (hi - lo)

	return e.spectral[i-1] + t*(e.spectral[i]-e.spectral[i-1]), false
}
