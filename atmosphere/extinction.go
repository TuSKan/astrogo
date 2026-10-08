package atmosphere

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
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
	rayleigh, ozone, aerosol, err := s.opticalDepths(lambda)
	if err != nil {
		return 0, err
	}

	return magnitudesPerOpticalDepth * (rayleigh + ozone + aerosol), nil
}

// ExtinctionToward returns how many magnitudes this air dims light at
// wavelength lambda arriving from apparent altitude alt: the three terms of
// [Atmosphere.Extinction], each through the airmass of the layer it lies in,
//
//	A = 2.5 log10(e) * [tau_R * X_R + tau_O3 * X_O3 + tau_aer * X_aer]
//
// rather than one coefficient times one airmass.
//
//   - X_R is [Airmass], Pickering's (2002) airmass of the molecular
//     atmosphere.
//   - X_O3 is [OzoneAirmass], the airmass of a thin shell 20 km up, the
//     ozone layer. Near the horizon it is a third of X_R, 12.66 against
//     38.75 at 0°, so charging ozone X_R over-dimmed a target there: by
//     0.63 mag at 0°, 0.34 at 1° and 0.19 at 2° on VisibleTonight's
//     default air (#625).
//   - X_aer is X_R as well. That is the right airmass for aerosol with the
//     molecular scale height, 8 km, which OPAC gives its continental and
//     urban types. A thinner layer, maritime (1 km) or desert (2 km), has a
//     larger airmass near the horizon (Pickering's for 2 km is 88.8 at 0°),
//     which this does not model, so below about 5° it under-dims through
//     such air.
//
// High in the sky the three airmasses agree and this is Extinction times
// Airmass: at 30° they differ by 0.6%. It errors where Extinction does, and
// below the horizon, where Airmass does.
func (s *Atmosphere) ExtinctionToward(lambda unit.WavelengthNM, alt angle.Angle) (float64, error) {
	rayleigh, ozone, aerosol, err := s.opticalDepths(lambda)
	if err != nil {
		return 0, err
	}

	x, err := Airmass(alt)
	if err != nil {
		return 0, err
	}

	xO3, err := OzoneAirmass(alt)
	if err != nil {
		return 0, err
	}

	return magnitudesPerOpticalDepth * ((rayleigh+aerosol)*x + ozone*xO3), nil
}

// OzoneAirmass returns the airmass of a thin absorbing shell 20 km above an
// Earth of radius 6378 km, the ozone layer, seen from apparent altitude alt:
// Schaefer's (1998) formula as Pickering (DIO 12 ‡1, footnote 39) gives it,
//
//	X_O3 = (1 - (sin z / (1 + 20/6378))^2)^(-1/2)
//
// with z the zenith distance. Ozone lies above nearly all the air that
// scatters, so light crossing it travels a path set by the layer's height
// rather than by the molecular atmosphere's: 12.66 airmasses at the horizon
// against [Airmass]'s 38.75. High in the sky the two agree.
//
// Below the horizon the error wraps [ErrBelowHorizon], as Airmass's does.
func OzoneAirmass(alt angle.Angle) (float64, error) {
	if alt.Degrees() < 0 {
		return 0, ErrBelowHorizon
	}

	s := math.Cos(alt.Radians()) / (1 + 20.0/6378)

	return 1 / math.Sqrt(1-s*s), nil
}

// OzoneOpticalDepth returns the vertical optical depth of this air's ozone
// column, [Atmosphere.Ozone], at wavelength lambda: the column times the
// Serdyuchenko et al. (2014) cross section at 223 K averaged over 10 nm, the
// ozone term of [Atmosphere.Extinction].
//
// Ozone only absorbs, and it lies in a layer some 20 km up, so light that
// crosses it is dimmed by exp(-tau * X) with X from [OzoneAirmass].
//
// The wavelength must lie from 320 to 1000 nm, where the cross section is
// tabulated; outside it the error wraps [ErrExtinctionWavelength]. A zero
// column, which is an Atmosphere's default, is zero optical depth at any
// wavelength, since nothing absorbs whatever the cross section there.
func (s *Atmosphere) OzoneOpticalDepth(lambda unit.WavelengthNM) (unit.OpticalDepth, error) {
	if s.ozone == 0 {
		return 0, nil
	}

	nm := float64(lambda)
	if !(nm >= extinctionMinNM && nm <= extinctionMaxNM) {
		return 0, fmt.Errorf("%w: got %g nm", ErrExtinctionWavelength, nm)
	}

	return unit.OpticalDepth(ozoneCrossSectionAt(nm) * float64(s.ozone) * dobsonUnitMoleculesPerCM2), nil
}

// opticalDepths returns this air's Rayleigh, ozone and aerosol optical
// depths at lambda, which must lie in the modeled range.
func (s *Atmosphere) opticalDepths(lambda unit.WavelengthNM) (rayleigh, ozone, aerosol float64, err error) {
	nm := float64(lambda)
	if !(nm >= extinctionMinNM && nm <= extinctionMaxNM) {
		return 0, 0, 0, fmt.Errorf("%w: got %g nm", ErrExtinctionWavelength, nm)
	}

	tauR, err := RayleighOpticalDepth(lambda, s.surface.Pressure)
	if err != nil {
		return 0, 0, 0, err
	}

	tauO3, err := s.OzoneOpticalDepth(lambda)
	if err != nil {
		return 0, 0, 0, err
	}

	aerosol = float64(s.aerosol.TauAt(lambda))

	return float64(tauR), float64(tauO3), aerosol, nil
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
