package optics

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the throughput and background-rate layer.
var (
	// ErrThroughputShape indicates a throughput curve's wavelength and
	// efficiency slices are empty, mismatched, or not strictly increasing
	// in wavelength.
	ErrThroughputShape = errors.New("optics: throughput needs matching, non-empty, strictly increasing samples")

	// ErrThroughputEfficiency indicates an efficiency value outside [0,1]
	// or not finite. An efficiency above 1 would create photons.
	ErrThroughputEfficiency = errors.New("optics: throughput efficiency must be in [0,1] and finite")
)

// Throughput is a wavelength-dependent efficiency curve: the fraction of
// incident light surviving one element of an optical system, or the whole
// system once the elements are multiplied together.
//
// It covers every element the spec's instrument layer names — mirror
// reflectivity, windows, filters, lenses, detector quantum efficiency —
// because they are all the same physical thing, a dimensionless fraction
// of wavelength. Keeping them one type means a system response is built
// by multiplication rather than by a bespoke struct per element kind.
type Throughput struct {
	// WavelengthNM is strictly increasing.
	WavelengthNM []unit.WavelengthNM
	// Efficiency holds values in [0,1], one per wavelength.
	Efficiency []float64
	// Name identifies the element, e.g. "primary mirror", "SDSS g filter".
	Name string
	// Reference cites the curve's origin, per the repository's provenance
	// convention.
	Reference string
}

// Validate reports whether the curve is usable.
func (t Throughput) Validate() error {
	if len(t.WavelengthNM) == 0 || len(t.WavelengthNM) != len(t.Efficiency) {
		return fmt.Errorf("%w: %q has %d wavelengths and %d efficiencies",
			ErrThroughputShape, t.Name, len(t.WavelengthNM), len(t.Efficiency))
	}

	for i, e := range t.Efficiency {
		if e < 0 || e > 1 || math.IsNaN(e) || math.IsInf(e, 0) {
			return fmt.Errorf("%w: %q sample %d = %g", ErrThroughputEfficiency, t.Name, i, e)
		}

		if i > 0 && t.WavelengthNM[i] <= t.WavelengthNM[i-1] {
			return fmt.Errorf("%w: %q not increasing at sample %d", ErrThroughputShape, t.Name, i)
		}
	}

	return nil
}

// Sample resamples the curve onto grid, writing grid.Len() values into dst.
// Wavelengths outside the curve's tabulated range get zero efficiency: an
// element is opaque where its measurement says nothing, which is the
// conservative reading and prevents an unmeasured tail from inventing
// signal.
func (t Throughput) Sample(dst []float64, grid unit.SpectralGrid) error {
	if err := t.Validate(); err != nil {
		return err
	}

	if err := grid.Resample(dst, t.WavelengthNM, t.Efficiency, 0); err != nil {
		return fmt.Errorf("optics: throughput %q: %w", t.Name, err)
	}

	return nil
}

// System multiplies throughput curves onto a single sampled response on
// grid — the total efficiency of an optical train. An empty list yields
// unit efficiency at every wavelength, i.e. a perfectly transmitting
// system, which is the correct identity for a product.
func System(dst []float64, grid unit.SpectralGrid, elements ...Throughput) error {
	if len(dst) != grid.Len() {
		return fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	for i := range dst {
		dst[i] = 1
	}

	scratch := make([]float64, grid.Len())

	for _, e := range elements {
		if err := e.Sample(scratch, grid); err != nil {
			return err
		}

		for i := range dst {
			dst[i] *= scratch[i]
		}
	}

	return nil
}

// Instrument is an observing system's light-collecting and detecting
// configuration, enough to turn a sky spectral radiance into a detector
// background rate.
//
// It is deliberately separate from Telescope: a Telescope describes
// aperture and focal length for visual/geometric optics, while an
// Instrument describes the radiometric chain. A caller with a Telescope
// and a Sensor can build the matching Instrument with NewInstrument.
type Instrument struct {
	// CollectingAreaM2 is the effective light-collecting area in square
	// metres, already net of central obstruction.
	CollectingAreaM2 float64

	// PixelSolidAngleSR is the solid angle one pixel subtends on the sky,
	// in steradians.
	PixelSolidAngleSR float64

	// Throughput is the full system response — every mirror, window,
	// filter, lens and the detector QE multiplied together.
	Throughput []Throughput

	// ReadNoiseElectrons is the detector's read noise, in electrons RMS
	// per pixel per readout. Zero describes a noiseless read, which no
	// real detector has — it is the honest default only because this
	// field arrived after the type did, and [Instrument.SNR] says what a
	// zero here means for its answer.
	ReadNoiseElectrons float64

	// DarkCurrentEPerSec is thermal dark current, in electrons per pixel
	// per second. Zero is a fair approximation for a cooled sensor over a
	// short exposure and a poor one for a warm sensor over a long one,
	// which is the same trade every term in the noise budget makes.
	DarkCurrentEPerSec float64

	// Name identifies the instrument.
	Name string
}

// NewInstrument derives an Instrument from a Telescope and the angular
// size of one pixel, which is what a Sensor plus focal length gives.
//
// obstructionFraction is the fraction of the aperture AREA blocked by a
// secondary mirror and its supports, in [0,1); pass 0 for an unobstructed
// refractor.
func NewInstrument(
	name string,
	t Telescope,
	pixelScale angle.Angle,
	obstructionFraction float64,
	throughput ...Throughput,
) (Instrument, error) {
	if obstructionFraction < 0 || obstructionFraction >= 1 || math.IsNaN(obstructionFraction) {
		return Instrument{}, fmt.Errorf("%w: obstruction fraction %g", ErrNonPositiveDimension, obstructionFraction)
	}

	radiusM := t.ApertureMM() / 2 * 1e-3
	area := math.Pi * radiusM * radiusM * (1 - obstructionFraction)

	// A pixel's solid angle is its angular width squared in the small-angle
	// limit, which holds to far better than any other term here for the
	// arcsecond-scale pixels this is used with.
	w := pixelScale.Radians()
	if !positiveFinite(w) {
		return Instrument{}, fmt.Errorf("%w: pixel scale %g rad", ErrNonPositiveDimension, w)
	}

	return Instrument{
		Name:              name,
		CollectingAreaM2:  area,
		PixelSolidAngleSR: w * w,
		Throughput:        throughput,
	}, nil
}

// Validate reports whether the instrument is usable.
func (i Instrument) Validate() error {
	if !positiveFinite(i.CollectingAreaM2) {
		return fmt.Errorf("%w: collecting area %g m^2", ErrNonPositiveDimension, i.CollectingAreaM2)
	}

	if !positiveFinite(i.PixelSolidAngleSR) {
		return fmt.Errorf("%w: pixel solid angle %g sr", ErrNonPositiveDimension, i.PixelSolidAngleSR)
	}

	for _, t := range i.Throughput {
		if err := t.Validate(); err != nil {
			return err
		}
	}

	return nil
}

// PhotonRate returns the photon rate an instrument records from a sky of
// the given spectral radiance, in photons per second per square metre per
// steradian — i.e. before the collecting area and pixel solid angle are
// applied.
//
// spectrum is W m^-2 sr^-1 nm^-1 sampled on grid. The integrand converts
// each sample to a photon rate with E = hc/lambda before applying the
// system response, because a detector counts photons, not joules: doing
// the conversion after integration would weight the band by energy and
// give a different, wrong answer for any non-flat spectrum.
func (i Instrument) PhotonRate(spectrum []float64, grid unit.SpectralGrid) (unit.PhotonRadiance, error) {
	if err := i.Validate(); err != nil {
		return 0, err
	}

	if len(spectrum) != grid.Len() {
		return 0, fmt.Errorf("%w: %d spectrum samples, grid has %d",
			unit.ErrGridMismatch, len(spectrum), grid.Len())
	}

	response := make([]float64, grid.Len())
	if err := System(response, grid, i.Throughput...); err != nil {
		return 0, err
	}

	integrand := make([]float64, grid.Len())

	for k := range integrand {
		lambda := grid.At(k)
		photons := constants.ToPhoton(unit.SpectralRadiance(spectrum[k]), lambda)
		integrand[k] = float64(photons) * response[k]
	}

	total, err := grid.Integrate(integrand)
	if err != nil {
		return 0, fmt.Errorf("optics: instrument %q photon rate: %w", i.Name, err)
	}

	return unit.PhotonRadiance(total), nil
}

// BackgroundRate returns the sky background rate in electrons per pixel
// per second, the quantity exposure-time calculators actually need.
//
// It assumes the detector quantum efficiency is already one of the
// Throughput elements, so each surviving photon yields one electron. A
// detector with gain or an avalanche stage applies that separately —
// folding it in here would hide it inside a number callers compare against
// read noise.
//
// # Relationship to the general form
//
// Roellinghoff, Spencer & Funk (2025), A&A (arXiv:2505.12895) Eq. 5 gives
// the rigorous pixel rate as a double integral over the pixel area and
// solid angle,
//
//	F(eta) = int_{A_j} dx dy int_Omega d omega P(x,y | omega, eta) Phi(omega, x)
//
// with P the telescope point-source function, A_j the pixel area and eta
// the pointing direction. This function implements its small-pixel limit:
// the point-source function is treated as a delta function and the sky
// radiance as uniform across one pixel, so the double integral collapses
// to radiance times collecting area times pixel solid angle.
//
// That limit is exact for a diffuse background whose structure is large
// compared with the PSF, which is the case for sky brightness — the sky
// varies on degree scales and a PSF on arcsecond scales. It is NOT valid
// for a point source, where the PSF shape is the whole problem. A caller
// wanting per-pixel structure from a resolved gradient, or wanting to
// place a star on the same detector, needs the full integral.
func (i Instrument) BackgroundRate(spectrum []float64, grid unit.SpectralGrid) (unit.ElectronsPerPixelPerSecond, error) {
	rate, err := i.PhotonRate(spectrum, grid)
	if err != nil {
		return 0, err
	}

	return unit.ElectronsPerPixelPerSecond(float64(rate) * i.CollectingAreaM2 * i.PixelSolidAngleSR), nil
}
