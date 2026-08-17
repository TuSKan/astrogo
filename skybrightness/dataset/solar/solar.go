// Package solar provides the solar spectral irradiance that fixes the
// absolute scale of every reflected-sunlight model in this library.
//
// Lunar irradiance and zodiacal light are both sunlight seen after one
// reflection, so both are only as accurate as the solar spectrum they are
// paired with. Which reference is used is therefore a decision the caller
// makes and the provenance records, not a default buried in a component —
// which is why [github.com/TuSKan/astrogo/skybrightness.NewScatteredMoonlight]
// requires a spectrum rather than supplying one.
//
// This package fetches the CALSPEC solar reference from STScI, the same one
// GAMBONS uses for its zodiacal light.
package solar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/apache/arrow-go/v18/arrow/array"

	"github.com/TuSKan/astrogo/fits"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the solar spectrum provider.
var (
	// ErrNoSpectrum is returned when a CALSPEC file carries no usable
	// wavelength/flux table.
	ErrNoSpectrum = errors.New("solar: file has no wavelength and flux columns")

	// ErrGrid is returned when a destination grid does not match the
	// requested resampling.
	ErrGrid = errors.New("solar: destination does not match the target wavelengths")
)

// CALSPECSolarReference is the file name of the CALSPEC solar spectrum. It
// carries its version, which is why the endpoint is not Mutable: a new
// reference arrives under a new name rather than replacing this one.
const CALSPECSolarReference = "sun_reference_stis_002.fits"

// CALSPEC flux and wavelength units, and their conversion to SI.
//
// CALSPEC publishes wavelength in angstrom and flux in
// erg s^-1 cm^-2 angstrom^-1. One of those is 1e-7 W over 1e-4 m^2 over
// 0.1 nm, so the flux conversion is a factor of 1e-2 and the wavelength one
// a factor of 0.1.
const (
	angstromToNM         = 0.1
	ergPerCM2PerAToSIPer = 1e-2
)

// Spectrum is a solar spectral irradiance at 1 AU.
type Spectrum struct {
	// WavelengthNM is ascending.
	WavelengthNM []unit.WavelengthNM

	// Irradiance is spectral irradiance in W m^-2 nm^-1, one per
	// wavelength.
	Irradiance []float64

	// Source names the file this came from, for provenance.
	Source string
}

// Open fetches and parses the CALSPEC solar reference.
//
// The download is consent-gated like every other bulk fetch: call
// [remote.EnableDownloads] with [remote.CALSPEC] first, or this fails with
// [remote.ErrDownloadDenied]. The file is a few megabytes.
func Open(ctx context.Context) (*Spectrum, error) {
	bucket, key, err := remote.GetFile(ctx, remote.CALSPEC, CALSPECSolarReference)
	if err != nil {
		return nil, fmt.Errorf("solar: fetch %s: %w", CALSPECSolarReference, err)
	}

	r, err := bucket.NewReader(ctx, key, nil)
	if err != nil {
		return nil, fmt.Errorf("solar: open %s: %w", key, err)
	}
	defer func() { _ = r.Close() }()

	s, err := Parse(r)
	if err != nil {
		return nil, err
	}

	s.Source = CALSPECSolarReference

	return s, nil
}

// Parse reads a CALSPEC-style FITS binary table, converting its angstrom
// wavelengths and erg s^-1 cm^-2 angstrom^-1 fluxes to nanometres and
// W m^-2 nm^-1.
//
// It is separate from [Open] so the conversion can be exercised without a
// network fetch.
func Parse(r io.Reader) (*Spectrum, error) {
	f, err := fits.Read(r)
	if err != nil {
		return nil, fmt.Errorf("solar: read FITS: %w", err)
	}

	wavelength, flux, err := columns(f)
	if err != nil {
		return nil, err
	}

	out := &Spectrum{
		WavelengthNM: make([]unit.WavelengthNM, 0, len(wavelength)),
		Irradiance:   make([]float64, 0, len(wavelength)),
	}

	for i := range wavelength {
		nm := wavelength[i] * angstromToNM
		if nm <= 0 || math.IsNaN(nm) || math.IsNaN(flux[i]) {
			continue
		}

		// A negative flux is a calibration artefact, not a measurement.
		v := flux[i] * ergPerCM2PerAToSIPer
		if v < 0 {
			v = 0
		}

		out.WavelengthNM = append(out.WavelengthNM, unit.WavelengthNM(nm))
		out.Irradiance = append(out.Irradiance, v)
	}

	if len(out.WavelengthNM) < 2 {
		return nil, fmt.Errorf("%w: %d usable rows", ErrNoSpectrum, len(out.WavelengthNM))
	}

	return out, nil
}

// At returns the irradiance at one wavelength by linear interpolation,
// and zero outside the tabulated range rather than an extrapolation.
func (s *Spectrum) At(lambda unit.WavelengthNM) float64 {
	n := len(s.WavelengthNM)
	if n == 0 || lambda < s.WavelengthNM[0] || lambda > s.WavelengthNM[n-1] {
		return 0
	}

	lo, hi := 0, n-1
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		if s.WavelengthNM[mid] <= lambda {
			lo = mid
		} else {
			hi = mid
		}
	}

	span := float64(s.WavelengthNM[hi] - s.WavelengthNM[lo])
	if span <= 0 {
		return s.Irradiance[lo]
	}

	f := float64(lambda-s.WavelengthNM[lo]) / span

	return s.Irradiance[lo]*(1-f) + s.Irradiance[hi]*f
}

// Resample writes the irradiance at each of the given wavelengths into dst.
//
// This is how a caller feeds
// [github.com/TuSKan/astrogo/skybrightness.NewScatteredMoonlight], whose
// spectrum must be sampled at [github.com/TuSKan/astrogo/magnitude.ROLOBands].
//
// The CALSPEC grid is far finer than the ROLO bands, so point interpolation
// misses the band-averaging a real filter would do. For a smooth continuum
// that is a small error; across the deep solar absorption lines it is not,
// and a caller needing band-integrated values should convolve with the
// actual band response instead.
func (s *Spectrum) Resample(dst []float64, at []unit.WavelengthNM) error {
	if len(dst) != len(at) {
		return fmt.Errorf("%w: %d destination slots, %d wavelengths", ErrGrid, len(dst), len(at))
	}

	for i, lambda := range at {
		dst[i] = s.At(lambda)
	}

	return nil
}

// columns pulls the wavelength and flux arrays out of the first binary
// table extension.
func columns(f *fits.File) ([]float64, []float64, error) {
	for _, hdu := range f.HDUs {
		table, ok := hdu.(*fits.BintableHDU)
		if !ok || table.Batch == nil {
			continue
		}

		wavelength := floatColumn(table, "WAVELENGTH")
		flux := floatColumn(table, "FLUX")

		if len(wavelength) == 0 || len(wavelength) != len(flux) {
			continue
		}

		return wavelength, flux, nil
	}

	return nil, nil, ErrNoSpectrum
}

// floatColumn reads one named column as float64, whatever width it was
// stored at.
func floatColumn(table *fits.BintableHDU, name string) []float64 {
	schema := table.Batch.Schema()

	idx := schema.FieldIndices(name)
	if len(idx) == 0 {
		return nil
	}

	switch col := table.Batch.Column(idx[0]).(type) {
	case *array.Float64:
		out := make([]float64, col.Len())
		for i := range out {
			out[i] = col.Value(i)
		}

		return out
	case *array.Float32:
		out := make([]float64, col.Len())
		for i := range out {
			out[i] = float64(col.Value(i))
		}

		return out
	default:
		return nil
	}
}
