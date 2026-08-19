package atmosphere

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for molecular absorption.
var (
	// ErrCrossSectionShape is returned when a cross-section table's
	// wavelength and value slices are empty, mismatched, or not strictly
	// increasing in wavelength.
	ErrCrossSectionShape = errors.New("atmosphere: cross section needs matching, non-empty, strictly increasing samples")

	// ErrCrossSectionValue is returned for a negative or non-finite cross
	// section. Absorption cannot create photons.
	ErrCrossSectionValue = errors.New("atmosphere: absorption cross section must be non-negative and finite")

	// ErrColumnAmount is returned for a negative column amount.
	ErrColumnAmount = errors.New("atmosphere: column amount must be non-negative")
)

// Molecular absorption over the optical range this module supports is
// dominated by three species, and the module represents exactly those:
//
//   - O3, the Chappuis band, a broad shallow feature peaking near 600 nm
//     that removes a few per cent across the visible and is the reason a
//     clear zenith sky is not purely Rayleigh-blue.
//   - O2, the narrow A band at 762 nm and the weaker B band at 688 nm.
//   - H2O, several vibrational-rotational bands through the red and
//     near-infrared, strongest beyond 900 nm and highly variable with
//     precipitable water.
//
// Species below 330 nm (the ozone Huggins and Hartley bands) are outside
// the module's spectral domain because ground-level night-sky radiance
// there is negligible; species beyond 1000 nm are outside it because the
// default grid stops there.
//
// This file provides the machinery to apply a cross section; it
// deliberately ships NO tabulated cross-section data. Those are datasets
// with their own provenance and versioning, and inventing numbers for them
// would be exactly the fabrication the design forbids. A provider layer
// supplies them; see docs/skybrightness.md §16.
//
// Ozone is what this machinery is for. Its Chappuis band is a broad
// continuum across the visible, which a tabulated cross section on a
// nanometre grid represents exactly. Serdyuchenko et al. (2014) is the
// chosen source and 223 K the chosen temperature — the nearest measured
// point to the effective temperature of stratospheric ozone.
//
// O2 and H2O are a different problem and are not merely unsupplied. They
// absorb in narrow lines — O2 at 688 and 762 nm, water vapour at 720, 820
// and 940 nm — and Beer-Lambert with a cross section band-averaged onto a
// nanometre grid is systematically wrong for them, always overestimating
// absorption, because exp(-tau) is convex and averaging the cross section
// first is not the same as averaging the transmission. They need a band
// model or correlated-k treatment, which is a capability this package does
// not have rather than a dataset it lacks.

// dobsonUnitMoleculesPerCM2 is the column density of one Dobson Unit, in
// molecules per square centimetre.
//
// A Dobson Unit is defined as a 0.01 mm thick layer of the pure gas at
// standard temperature and pressure, so this is derived from the SI-exact
// Boltzmann constant and the STP definition rather than hardcoded:
//
//	n0 = P0 / (k_B * T0)                 (number density at STP)
//	1 DU = 1e-5 m * n0                   (0.01 mm of that gas)
//
// which gives 2.687e20 molecules per square metre, or 2.687e16 per square
// centimetre.
var dobsonUnitMoleculesPerCM2 = func() float64 {
	const (
		stpPressurePa  = 101325.0 // IUPAC standard pressure
		stpTemperature = 273.15   // 0 degrees Celsius, in kelvin
		duThicknessM   = 1e-5     // 0.01 mm
		cm2PerM2       = 1e4
	)

	numberDensity := stpPressurePa / (constants.SI2019.BoltzmannConstant.Value * stpTemperature)

	return duThicknessM * numberDensity / cm2PerM2
}()

// DobsonUnitMoleculesPerCM2 reports the column density of one Dobson Unit,
// in molecules per square centimetre.
func DobsonUnitMoleculesPerCM2() float64 { return dobsonUnitMoleculesPerCM2 }

// CrossSection is a tabulated absorption cross section for one molecular
// species, in square centimetres per molecule — the unit spectroscopic
// databases publish.
//
// It is a concrete type rather than an interface: there is exactly one way
// to turn a cross section and a column amount into an optical depth, and
// an interface here would add indirection without a second implementation.
// The data-provider boundary is the struct itself, which a provider fills
// from a dataset and hands over.
type CrossSection struct {
	// Species names the absorber, e.g. "O3", "O2", "H2O".
	Species string

	// WavelengthNM is strictly increasing.
	WavelengthNM []unit.WavelengthNM

	// SigmaCM2 holds the cross section per molecule, in cm^2.
	SigmaCM2 []float64

	// TemperatureK is the temperature the table was measured at. Ozone
	// cross sections in particular vary by several per cent across
	// stratospheric temperatures, so a table without this is ambiguous.
	TemperatureK float64

	// Reference cites the measurement, per the repository's provenance
	// convention.
	Reference string
}

// Validate reports whether the table is usable.
func (c CrossSection) Validate() error {
	if len(c.WavelengthNM) == 0 || len(c.WavelengthNM) != len(c.SigmaCM2) {
		return fmt.Errorf("%w: %q has %d wavelengths and %d values",
			ErrCrossSectionShape, c.Species, len(c.WavelengthNM), len(c.SigmaCM2))
	}

	for i, s := range c.SigmaCM2 {
		if s < 0 || math.IsNaN(s) || math.IsInf(s, 0) {
			return fmt.Errorf("%w: %q sample %d = %g", ErrCrossSectionValue, c.Species, i, s)
		}

		if i > 0 && c.WavelengthNM[i] <= c.WavelengthNM[i-1] {
			return fmt.Errorf("%w: %q not increasing at sample %d", ErrCrossSectionShape, c.Species, i)
		}
	}

	return nil
}

// OpticalDepth writes the vertical absorption optical depth onto grid,
// given a column amount in molecules per square centimetre:
//
//	tau(lambda) = sigma(lambda) * N
//
// Beer-Lambert with no line-by-line structure resolved: the tabulated
// cross section is assumed already band-averaged to the grid's resolution.
// That assumption is safe for the ozone Chappuis continuum and wrong for
// the narrow O2 A band, where a 1 nm grid cannot resolve individual lines
// and a band-averaged transmittance is not the average of the
// monochromatic transmittances. A provider supplying O2 must therefore
// supply an already band-averaged effective cross section for the target
// grid, and say so in its Reference.
//
// Wavelengths outside the tabulated range get zero optical depth: no
// measurement means no claim of absorption, which is the conservative
// reading and keeps an unmeasured tail from inventing extinction.
func (c CrossSection) OpticalDepth(dst []float64, grid unit.SpectralGrid, columnMoleculesPerCM2 float64) error {
	if err := c.Validate(); err != nil {
		return err
	}

	if columnMoleculesPerCM2 < 0 || math.IsNaN(columnMoleculesPerCM2) {
		return fmt.Errorf("%w: %q column %g", ErrColumnAmount, c.Species, columnMoleculesPerCM2)
	}

	if len(dst) != grid.Len() {
		return fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	if err := grid.Resample(dst, c.WavelengthNM, c.SigmaCM2, 0); err != nil {
		return fmt.Errorf("atmosphere: cross section %q: %w", c.Species, err)
	}

	for i := range dst {
		dst[i] *= columnMoleculesPerCM2
	}

	return nil
}

// OzoneOpticalDepth is OpticalDepth with the column given in Dobson Units,
// which is how ozone is universally reported and how
// [Atmosphere.Ozone] carries it.
func (c CrossSection) OzoneOpticalDepth(dst []float64, grid unit.SpectralGrid, column unit.OzoneColumnDU) error {
	if column < 0 {
		return fmt.Errorf("%w: %g DU", ErrColumnAmount, float64(column))
	}

	return c.OpticalDepth(dst, grid, float64(column)*dobsonUnitMoleculesPerCM2)
}
