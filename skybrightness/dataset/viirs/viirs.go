// Package viirs turns NASA VIIRS annual nighttime-lights composites into
// artificial-light sources for [github.com/TuSKan/astrogo/skybrightness].
//
// # VIIRS is a source input, never a sky brightness
//
// A VIIRS pixel measures upward radiance leaving the ground. It is not sky
// brightness, and converting one to the other with a fitted correlation —
// which is what most light-pollution maps do — throws away the geometry that
// determines what an observer actually sees. This package therefore produces
// [skybrightness.GroundEmitter] values that the artificial-skyglow component
// propagates, and never a brightness.
//
// # What the caller has to supply, and why
//
// A pixel radiance cannot determine a spectrum. Several different real
// installations — high-pressure sodium, 3000 K LED, metal halide — produce
// the same DNB reading while scattering completely differently, because
// Rayleigh scattering goes as roughly lambda^-4. Nor can it determine the
// upward emission function: the same total output shielded or unshielded
// gives the same pixel and a very different sky. Both are therefore explicit
// inputs, and every emitter this package produces is flagged
// [skybrightness.AssumedSourceSpectrum] and
// [skybrightness.AssumedEmissionFunction].
package viirs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/skybrightness/dataset/raster"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the VIIRS provider.
var (
	// ErrYearOutOfRange is returned for a year before the VIIRS record
	// begins.
	ErrYearOutOfRange = errors.New("viirs: year is before the VIIRS record")

	// ErrSpectrum is returned for a source spectrum that cannot scale a
	// band radiance — mismatched lengths, non-positive wavelengths, or a
	// response that integrates to zero.
	ErrSpectrum = errors.New("viirs: invalid source spectrum")

	// ErrSampling is returned for a nonsensical sampling region.
	ErrSampling = errors.New("viirs: invalid sampling region")

	// ErrArchive is returned when a downloaded archive does not contain the
	// expected GeoTIFF entry.
	ErrArchive = errors.New("viirs: archive is missing its GeoTIFF entry")
)

// The VIIRS annual composite record. Upstream publishes one year at a time,
// so LatestYear is a floor that [NewestYear] probes past rather than a
// hard limit.
const (
	EarliestYear = 2012
	LatestYear   = 2025

	// maxProbeAhead caps how far NewestYear probes past LatestYear. It is a
	// runaway guard, not a real limit on the record.
	maxProbeAhead = 5

	// nanoWattPerCM2ToWattPerM2 converts the DNB's published unit,
	// nW cm^-2 sr^-1, to W m^-2 sr^-1: 1e-9 W over 1e-4 m^2.
	nanoWattPerCM2ToWattPerM2 = 1e-5
)

// SourceSpectrum is the assumed spectral shape of the light a VIIRS pixel
// measured, together with the sensor response the measurement was integrated
// over.
//
// Both are needed because a DNB radiance is a single number produced by
// folding an unknown spectrum through a known response. Recovering a spectral
// radiance from it means assuming the shape and dividing out the response:
//
//	L_lambda(lambda) = L_DNB * S(lambda) / INT S(lambda) R(lambda) d lambda
//
// The shape's absolute scale is irrelevant — it cancels — so a normalised
// curve, a blackbody, or a manufacturer's SPD all work.
type SourceSpectrum struct {
	// WavelengthNM is the common wavelength grid, ascending.
	WavelengthNM []unit.WavelengthNM

	// Shape is the relative spectral power distribution of the sources, on
	// any scale.
	Shape []float64

	// Response is the sensor's relative spectral response on the same grid.
	// For the VIIRS Day/Night Band this spans roughly 500 to 900 nm; this
	// package ships no copy of it, because it has no citable one to ship.
	Response []float64
}

// scale returns the factor converting a DNB radiance in nW cm^-2 sr^-1 into
// spectral radiance in W m^-2 sr^-1 nm^-1 at each grid wavelength.
func (s SourceSpectrum) scale() ([]float64, error) {
	n := len(s.WavelengthNM)
	if n < 2 || len(s.Shape) != n || len(s.Response) != n {
		return nil, fmt.Errorf("%w: %d wavelengths, %d shape, %d response",
			ErrSpectrum, n, len(s.Shape), len(s.Response))
	}

	var overlap float64

	for i := 1; i < n; i++ {
		if s.WavelengthNM[i] <= s.WavelengthNM[i-1] {
			return nil, fmt.Errorf("%w: wavelengths are not ascending at index %d", ErrSpectrum, i)
		}

		// Trapezoid over the shape-response product.
		lo := s.Shape[i-1] * s.Response[i-1]
		hi := s.Shape[i] * s.Response[i]
		overlap += 0.5 * (lo + hi) * float64(s.WavelengthNM[i]-s.WavelengthNM[i-1])
	}

	if overlap <= 0 || math.IsNaN(overlap) {
		return nil, fmt.Errorf("%w: shape and response overlap integrates to %g", ErrSpectrum, overlap)
	}

	out := make([]float64, n)
	for i, v := range s.Shape {
		out[i] = v * nanoWattPerCM2ToWattPerM2 / overlap
	}

	return out, nil
}

// sampler is the minimum a Raster needs of its backing store, so the
// emitter-building logic is testable against an in-memory grid and does not
// require a multi-gigabyte download.
type sampler interface {
	SampleBilinear(lonDeg, latDeg float64) (float64, error)
}

// Raster is a VIIRS annual composite, sampled by longitude and latitude.
//
// Samples are upward radiance in nW cm^-2 sr^-1, the unit the product
// publishes.
type Raster struct {
	src  sampler
	year int
}

// FromGrid wraps an already-decoded raster. Its samples must be DNB radiance
// in nW cm^-2 sr^-1.
func FromGrid(g *raster.Grid, year int) *Raster { return &Raster{src: g, year: year} }

// Year reports which annual composite this is.
func (r *Raster) Year() int { return r.year }

// RadianceAt returns the upward DNB radiance at a location, in
// nW cm^-2 sr^-1.
func (r *Raster) RadianceAt(lonDeg, latDeg float64) (float64, error) {
	v, err := r.src.SampleBilinear(lonDeg, latDeg)
	if err != nil {
		return 0, fmt.Errorf("viirs: sample at %.4f, %.4f: %w", lonDeg, latDeg, err)
	}

	return v, nil
}

// Open downloads (if permitted) and decodes the annual composite for a year,
// returning a Raster and the closer for its backing store.
//
// The download is gated like every other bulk fetch in this library: call
// [remote.EnableDownloads] with [remote.VIIRSAnnual] first, or this fails
// with [remote.ErrDownloadDenied]. The archives run from roughly 700 MB to
// over a gigabyte.
func Open(ctx context.Context, year int) (*Raster, io.Closer, error) {
	if year < EarliestYear {
		return nil, nil, fmt.Errorf("%w: %d (the record starts at %d)",
			ErrYearOutOfRange, year, EarliestYear)
	}

	// No upper bound on purpose: bounding by LatestYear would make
	// NewestYear useless the moment upstream published past the compiled-in
	// constant. A year upstream does not carry surfaces as a fetch error,
	// which is accurate and self-updating.
	bucket, key, err := remote.GetFile(ctx, remote.VIIRSAnnual, archiveName(year))
	if err != nil {
		return nil, nil, fmt.Errorf("viirs: fetch %d composite: %w", year, err)
	}

	tiffKey, err := extractTIFF(ctx, bucket, key, year)
	if err != nil {
		return nil, nil, err
	}

	at, err := file.NewReaderAt(ctx, bucket, tiffKey)
	if err != nil {
		return nil, nil, fmt.Errorf("viirs: open %s: %w", tiffKey, err)
	}

	reader, err := raster.Open(at, nil)
	if err != nil {
		closeQuietly(at)

		return nil, nil, fmt.Errorf("viirs: decode %s: %w", tiffKey, err)
	}

	return &Raster{src: reader, year: year}, at, nil
}

// NewestYear reports the newest annual composite upstream actually
// publishes, probing forward from [LatestYear].
//
// It is deliberately forgiving: a probe failure returns the best year
// confirmed so far — never worse than [LatestYear] — alongside the error, so
// a caller that ignores the error still gets a usable year rather than zero.
func NewestYear(ctx context.Context) (int, error) {
	newest := LatestYear

	for year := LatestYear + 1; year <= LatestYear+maxProbeAhead; year++ {
		ok, err := remote.Exists(ctx, remote.VIIRSAnnual, archiveName(year))
		if err != nil {
			return newest, fmt.Errorf("viirs: probe %d: %w", year, err)
		}

		if !ok {
			break
		}

		newest = year
	}

	return newest, nil
}

func archiveName(year int) string { return fmt.Sprintf("viirs_%d_raw.zip", year) }

func entryName(year int) string { return fmt.Sprintf("viirs_%d_raw.tif", year) }
