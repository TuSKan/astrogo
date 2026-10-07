package atmosphere

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// ErrExtinctionWavelength is returned by [Atmosphere.Extinction] for a
// wavelength outside the range the model covers and was validated over.
var ErrExtinctionWavelength = errors.New("atmosphere: extinction is modeled from 320 to 1000 nm")

// Extinction model bounds, in nanometers: the range Buton et al. (2013)
// tabulate their decomposition over, and the span of [ozoneCrossSection].
const (
	extinctionMinNM = 320
	extinctionMaxNM = 1000
)

// magnitudesPerOpticalDepth converts an optical depth into the magnitudes
// it dims a source by: 2.5 log10(e).
const magnitudesPerOpticalDepth = 2.5 * math.Log10E

// Extinction returns the extinction coefficient of this air at wavelength
// lambda, in magnitudes per airmass: what a star loses to it for each airmass
// of path, so that m = m0 + k * X.
//
// It is the sum of the three continua that matter in the optical, each from
// this Atmosphere's own state:
//
//	k(lambda) = 2.5 log10(e) * [tau_R(lambda, P) + tau_O3(lambda) + tau_aer(lambda)]
//
// The three optical depths are:
//
//   - Rayleigh scattering by the air above the site, from the surface
//     pressure, by [RayleighOpticalDepth];
//   - ozone absorption by the column [Atmosphere.Ozone], through the
//     Serdyuchenko et al. (2014) cross section at 223 K, the one
//     skybrightness uses, averaged over 10 nm;
//   - aerosol extinction, [Aerosol.TauAt].
//
// That is the decomposition Patat et al. (2011, A&A 527, A91) and Buton et
// al. (2013, A&A 549, A8) fit to Paranal and Mauna Kea. Given each paper's
// own pressure, ozone column and aerosol, it reproduces Patat's measured
// curve within 0.0097 mag per airmass from 347.5 to 794 nm, and Buton's
// components within 0.0035 from 400 nm (see extinction_reference_test.go).
//
// # What it leaves out
//
// The narrow O2 and H2O lines, which this package cannot represent honestly
// (see [CrossSection]); Patat et al. put O2 at 0.01 and 0.03 mag per airmass
// of broad-band R and I, and below 0.002 in U, B and V. Cloud layers, which
// dim a star by covering it rather than by a coefficient. And temperature,
// which none of the three terms here depends on.
//
// # Accuracy by wavelength
//
// [RayleighOpticalDepth] is a single power law that reads 4% low at 330 nm
// and within about 1% from 400 nm redward (#592), so the blue end of U
// carries up to 0.03 mag per airmass at sea level. The ozone average smooths
// the narrow structure of the Huggins band below 350 nm.
//
// A broadband coefficient is this averaged over the passband and the source's
// spectrum, which a single wavelength stands in for only where k is close to
// linear across the band.
//
// The wavelength must lie from 320 to 1000 nm; outside it the error wraps
// [ErrExtinctionWavelength]. An Atmosphere with no surface pressure has no
// Rayleigh term to compute, and the error wraps [ErrPressure].
func (s *Atmosphere) Extinction(lambda unit.WavelengthNM) (float64, error) {
	nm := float64(lambda)
	if !(nm >= extinctionMinNM && nm <= extinctionMaxNM) {
		return 0, fmt.Errorf("%w: got %g nm", ErrExtinctionWavelength, nm)
	}

	rayleigh, err := RayleighOpticalDepth(lambda, s.surface.Pressure)
	if err != nil {
		return 0, err
	}

	ozone := ozoneCrossSectionAt(nm) * float64(s.ozone) * dobsonUnitMoleculesPerCM2
	aerosol := float64(s.aerosol.TauAt(lambda))

	return magnitudesPerOpticalDepth * (float64(rayleigh) + ozone + aerosol), nil
}

// ozoneCrossSectionAt interpolates [ozoneCrossSection] linearly at nm, which
// the caller has checked lies within the table.
func ozoneCrossSectionAt(nm float64) float64 {
	pos := (nm - extinctionMinNM) / ozoneStepNM
	i := int(pos)

	if i >= len(ozoneCrossSection)-1 {
		return ozoneCrossSection[len(ozoneCrossSection)-1]
	}

	f := pos - float64(i)

	return ozoneCrossSection[i] + f*(ozoneCrossSection[i+1]-ozoneCrossSection[i])
}

// ozoneStepNM is the spacing of [ozoneCrossSection].
const ozoneStepNM = 10

// ozoneCrossSection is the ozone absorption cross section at 223 K, in cm^2
// per molecule, at 320, 330, ..., 1000 nm.
//
// Each value is the mean of every sample of Serdyuchenko et al. (2014), AMT
// 7, 609 and 625, with node - 5 <= lambda < node + 5 nm: the file
// remote.OzoneSerdyuchenko223K names, which
// skybrightness/dataset/crosssection.Ozone fetches. Sky brightness reads that
// file at full resolution; extinction reads it here so that a planning call
// needs no download, and so that the two never disagree about ozone by more
// than the averaging. TestExtinctionOzoneIsTheDatasetBinned, in that package,
// recomputes every value from the file.
//
// Ten nanometers resolves what a star's extinction needs. The Chappuis band,
// which is ozone's whole effect on the visible, is a continuum some 200 nm
// wide; only the Huggins band below 350 nm has finer structure, and a
// broad-band coefficient averages that away regardless.
var ozoneCrossSection = [...]float64{
	2.3128e-20, 5.1458e-21, 9.6312e-22, 1.5734e-22, 2.4279e-23, 4.2966e-24, // 320-370
	1.6322e-24, 3.5468e-24, 9.2081e-24, 2.0183e-23, 3.8659e-23, 6.8147e-23, // 380-430
	1.2735e-22, 1.6857e-22, 3.1800e-22, 3.6921e-22, 6.8231e-22, 7.7482e-22, // 440-490
	1.1791e-21, 1.5356e-21, 1.7939e-21, 2.5448e-21, 2.8818e-21, 3.2853e-21, // 500-550
	3.8742e-21, 4.5761e-21, 4.5622e-21, 4.4355e-21, 5.0348e-21, 4.7114e-21, // 560-610
	4.0023e-21, 3.4988e-21, 2.9319e-21, 2.4440e-21, 2.0461e-21, 1.6540e-21, // 620-670
	1.3257e-21, 1.0775e-21, 8.2382e-22, 6.9214e-22, 5.8974e-22, 4.4790e-22, // 680-730
	4.0823e-22, 3.9723e-22, 2.6584e-22, 2.4465e-22, 2.9609e-22, 1.9475e-22, // 740-790
	1.4157e-22, 1.7366e-22, 1.9273e-22, 8.8742e-23, 6.7832e-23, 1.1563e-22, // 800-850
	1.0722e-22, 4.3911e-23, 3.4966e-23, 4.6808e-23, 5.4839e-23, 2.4849e-23, // 860-910
	1.2969e-23, 1.3120e-23, 2.7690e-23, 2.3696e-23, 8.7151e-24, 3.6654e-24, // 920-970
	1.7385e-24, 1.6756e-23, 7.7570e-24, // 980-1000
}
