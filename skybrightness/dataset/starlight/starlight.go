// Package starlight provides the extra-atmospheric radiance of the natural
// sky: integrated starlight, diffuse galactic light and the extragalactic
// background, as a HEALPix map.
//
// # What this package is and is not
//
// It holds a *map* and the arithmetic to read it. It does not compute
// integrated starlight from a star catalogue — that is a bulk aggregation
// over Gaia DR3's 1.8 billion sources, an offline job producing a data
// product, not something a library does at query time. GAMBONS (Masana et
// al. 2021, MNRAS 501, 5443) is one such product; any map on the same grid
// works.
//
// # Band-integrated in, spectral out
//
// Published natural-sky maps give radiance **integrated over a passband** —
// Johnson V, Gaia G, scotopic, photopic. This module's engine is spectral,
// so a band value has to be spread across wavelengths, and doing that needs
// an assumed spectral shape. Integrated starlight is the summed light of
// stars of every type, so no single blackbody is right; the shape is
// therefore an explicit input, and a map read through it is flagged
// [skybrightness.AssumedSourceSpectrum].
//
// The alternative — treating a V-band radiance as though it were flat across
// the optical — would reproduce V correctly and every other band wrongly,
// which is the failure this module exists to avoid.
package starlight

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for the starlight map.
var (
	// ErrMapSize is returned when a map's pixel count is not 12*nside^2 for
	// a power-of-two nside.
	ErrMapSize = errors.New("starlight: pixel count is not a valid HEALPix map size")

	// ErrMapFormat is returned for a table row that cannot be parsed.
	ErrMapFormat = errors.New("starlight: malformed map row")

	// ErrPixelRange is returned for a pixel index outside the map.
	ErrPixelRange = errors.New("starlight: pixel index outside the map")

	// ErrBand is returned when a requested band is not in the map.
	ErrBand = errors.New("starlight: map has no such band")
)

// Frame names the sphere a map's pixels are defined on.
//
// This is not decoration. A map indexed in galactic coordinates read as
// though it were equatorial puts the Milky Way through the wrong part of the
// sky and still returns plausible numbers everywhere, so the frame travels
// with the data rather than being assumed.
type Frame string

// The frames a natural-sky map is published in.
const (
	// Galactic is the frame GAMBONS tabulates in.
	Galactic Frame = "galactic"

	// ICRS is the equatorial alternative.
	ICRS Frame = "icrs"
)

// Map is an all-sky map of extra-atmospheric radiance on a HEALPix grid.
//
// Values are radiance in W m^-2 sr^-1, integrated over the named band. They
// are *outside* the atmosphere: applying attenuation and scattering is the
// consuming component's job, not this package's.
type Map struct {
	grid  coord.HEALPix
	frame Frame

	// bands maps a band name to one radiance per pixel.
	bands map[string][]float64

	// Source records where the map came from, for provenance.
	Source string
}

// NewMap builds a map from per-pixel radiances, one slice per band. Every
// slice must have the same length, and that length must be a valid HEALPix
// pixel count.
func NewMap(frame Frame, bands map[string][]float64) (*Map, error) {
	if len(bands) == 0 {
		return nil, fmt.Errorf("%w: no bands", ErrBand)
	}

	npix := -1

	for name, values := range bands {
		if npix < 0 {
			npix = len(values)
		}

		if len(values) != npix {
			return nil, fmt.Errorf("%w: band %q has %d pixels, another has %d",
				ErrMapSize, name, len(values), npix)
		}
	}

	grid, err := gridFor(npix)
	if err != nil {
		return nil, err
	}

	copied := make(map[string][]float64, len(bands))
	for name, values := range bands {
		copied[name] = append([]float64(nil), values...)
	}

	return &Map{grid: grid, frame: frame, bands: copied}, nil
}

// gridFor recovers the HEALPix resolution from a pixel count.
func gridFor(npix int) (coord.HEALPix, error) {
	if npix <= 0 || npix%12 != 0 {
		return coord.HEALPix{}, fmt.Errorf("%w: %d pixels", ErrMapSize, npix)
	}

	nside2 := npix / 12

	nside := int64(math.Round(math.Sqrt(float64(nside2))))
	if nside*nside != int64(nside2) {
		return coord.HEALPix{}, fmt.Errorf("%w: %d pixels is not 12*nside^2", ErrMapSize, npix)
	}

	grid, err := coord.NewHEALPix(nside)
	if err != nil {
		return coord.HEALPix{}, fmt.Errorf("starlight: %w", err)
	}

	return grid, nil
}

// Grid returns the map's pixelation.
func (m *Map) Grid() coord.HEALPix { return m.grid }

// Frame returns the sphere the pixels are defined on.
func (m *Map) Frame() Frame { return m.frame }

// Bands returns the band names present, in no particular order.
func (m *Map) Bands() []string {
	out := make([]string, 0, len(m.bands))
	for name := range m.bands {
		out = append(out, name)
	}

	return out
}

// RadianceAt returns the band-integrated radiance in the direction (lon,
// lat), which must be expressed in the map's own frame.
//
// The value is the containing pixel's, not an interpolation. HEALPix pixels
// are cells of a piecewise-constant map, and smoothing across them would
// blur structure the map is there to represent — most visibly the edge of
// the Milky Way.
func (m *Map) RadianceAt(band string, lon, lat angle.Angle) (float64, error) {
	values, ok := m.bands[band]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrBand, band)
	}

	pixel := m.grid.PixelOf(lon, lat)
	if pixel < 0 || pixel >= int64(len(values)) {
		return 0, fmt.Errorf("%w: %d", ErrPixelRange, pixel)
	}

	return values[pixel], nil
}

// Pixel returns the radiance of one pixel directly, for callers walking the
// whole map.
func (m *Map) Pixel(band string, pixel int64) (float64, error) {
	values, ok := m.bands[band]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrBand, band)
	}

	if pixel < 0 || pixel >= int64(len(values)) {
		return 0, fmt.Errorf("%w: %d not in [0, %d)", ErrPixelRange, pixel, len(values))
	}

	return values[pixel], nil
}

// Load reads a whitespace- or comma-separated table of per-pixel radiances.
//
// The format is deliberately plain, because the maps this consumes are
// published as plain tables and a bespoke binary format would be one more
// thing to get wrong:
//
//   - Lines beginning with # are comments. A comment of the form
//     "# bands: V G scotopic" names the columns; without one the columns are
//     named "1", "2", ... in order.
//   - Every other line is one pixel: an integer NESTED pixel index followed
//     by one radiance per band, in W m^-2 sr^-1.
//   - Rows may appear in any order, and every pixel of the map must appear
//     exactly once. A missing pixel is an error rather than a zero, because
//     a hole in a sky map is not a dark patch of sky.
func Load(r io.Reader, frame Frame) (*Map, error) {
	var (
		names   []string
		rows    = map[int64][]float64{}
		nbands  = -1
		scanner = bufio.NewScanner(r)
	)

	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())

		if text == "" {
			continue
		}

		if strings.HasPrefix(text, "#") {
			if declared, ok := bandHeader(text); ok {
				names = declared
			}

			continue
		}

		pixel, values, err := parseRow(text)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %w", ErrMapFormat, line, err)
		}

		if nbands < 0 {
			nbands = len(values)
		}

		if len(values) != nbands {
			return nil, fmt.Errorf("%w: line %d has %d values, an earlier row had %d",
				ErrMapFormat, line, len(values), nbands)
		}

		if _, dup := rows[pixel]; dup {
			return nil, fmt.Errorf("%w: pixel %d appears twice", ErrMapFormat, pixel)
		}

		rows[pixel] = values
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("starlight: read map: %w", err)
	}

	return assemble(rows, names, nbands, frame)
}

// bandHeader recognises a "# bands: ..." comment.
func bandHeader(comment string) ([]string, bool) {
	trimmed := strings.TrimSpace(strings.TrimPrefix(comment, "#"))

	rest, ok := strings.CutPrefix(strings.ToLower(trimmed), "bands:")
	if !ok {
		return nil, false
	}

	// Recover the original casing for the names themselves.
	original := strings.TrimSpace(trimmed[len(trimmed)-len(rest):])

	fields := strings.FieldsFunc(original, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})

	if len(fields) == 0 {
		return nil, false
	}

	return fields, true
}

// parseRow reads one pixel index and its radiances.
func parseRow(text string) (int64, []float64, error) {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == ';'
	})

	if len(fields) < 2 {
		return 0, nil, fmt.Errorf("%w: need a pixel index and at least one radiance", ErrMapFormat)
	}

	pixel, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: pixel index %q", ErrMapFormat, fields[0])
	}

	values := make([]float64, len(fields)-1)

	for i, f := range fields[1:] {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return 0, nil, fmt.Errorf("%w: radiance %q", ErrMapFormat, f)
		}

		if v < 0 || math.IsNaN(v) {
			return 0, nil, fmt.Errorf("%w: radiance %q is not a usable value", ErrMapFormat, f)
		}

		values[i] = v
	}

	return pixel, values, nil
}

// assemble turns parsed rows into a dense map, insisting every pixel is
// present.
func assemble(rows map[int64][]float64, names []string, nbands int, frame Frame) (*Map, error) {
	grid, err := gridFor(len(rows))
	if err != nil {
		return nil, err
	}

	if len(names) == 0 {
		names = make([]string, nbands)
		for i := range names {
			names[i] = strconv.Itoa(i + 1)
		}
	}

	if len(names) != nbands {
		return nil, fmt.Errorf("%w: header names %d bands, rows carry %d",
			ErrMapFormat, len(names), nbands)
	}

	bands := make(map[string][]float64, nbands)
	for _, name := range names {
		bands[name] = make([]float64, len(rows))
	}

	for pixel, values := range rows {
		if pixel < 0 || pixel >= grid.NumPixels() {
			return nil, fmt.Errorf("%w: %d not in [0, %d)", ErrPixelRange, pixel, grid.NumPixels())
		}

		for i, name := range names {
			bands[name][pixel] = values[i]
		}
	}

	return NewMap(frame, bands)
}

// SpectralShape spreads a band-integrated radiance across wavelengths.
//
// A published map gives one number per band. Turning that into the spectral
// radiance this module's engine works in needs the spectral shape of the
// light and the response of the band it was integrated over:
//
//	L_lambda(lambda) = L_band * S(lambda) / INT S(lambda) R(lambda) d lambda
//
// The shape's absolute scale cancels, so a normalised curve, a stellar
// spectrum or a synthetic composite all work. What it cannot be is
// "flat" by default: integrated starlight is the summed light of stars of
// every spectral type, and assuming a shape is a real assumption that this
// package makes the caller state.
type SpectralShape struct {
	// WavelengthNM is the common grid, ascending.
	WavelengthNM []unit.WavelengthNM

	// Shape is the relative spectral radiance, on any scale.
	Shape []float64

	// Response is the band's relative spectral response on the same grid.
	Response []float64
}

// Scale returns the factor converting a band-integrated radiance in
// W m^-2 sr^-1 into spectral radiance in W m^-2 sr^-1 nm^-1 at each grid
// wavelength.
func (s SpectralShape) Scale() ([]float64, error) {
	n := len(s.WavelengthNM)
	if n < 2 || len(s.Shape) != n || len(s.Response) != n {
		return nil, fmt.Errorf("%w: %d wavelengths, %d shape, %d response",
			ErrMapFormat, n, len(s.Shape), len(s.Response))
	}

	var overlap float64

	for i := 1; i < n; i++ {
		if s.WavelengthNM[i] <= s.WavelengthNM[i-1] {
			return nil, fmt.Errorf("%w: wavelengths are not ascending at index %d", ErrMapFormat, i)
		}

		lo := s.Shape[i-1] * s.Response[i-1]
		hi := s.Shape[i] * s.Response[i]
		overlap += 0.5 * (lo + hi) * float64(s.WavelengthNM[i]-s.WavelengthNM[i-1])
	}

	if overlap <= 0 || math.IsNaN(overlap) {
		return nil, fmt.Errorf("%w: shape and response overlap integrates to %g", ErrMapFormat, overlap)
	}

	out := make([]float64, n)
	for i, v := range s.Shape {
		out[i] = v / overlap
	}

	return out, nil
}
