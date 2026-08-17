package starlight_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
	"github.com/TuSKan/astrogo/unit"
)

// tableFor renders a whole nside-1 map (12 pixels) as the plain text format
// Load consumes, with one radiance per pixel equal to its index.
func tableFor(header string) string {
	var b strings.Builder

	if header != "" {
		fmt.Fprintf(&b, "# bands: %s\n", header)
	}

	b.WriteString("# a comment that is not a band header\n\n")

	for pixel := range 12 {
		fmt.Fprintf(&b, "%d %g\n", pixel, float64(pixel))
	}

	return b.String()
}

func TestLoadNamedBands(t *testing.T) {
	t.Parallel()

	m, err := starlight.Load(strings.NewReader(tableFor("V")), starlight.Galactic)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := m.Grid().NumPixels(); got != 12 {
		t.Errorf("NumPixels = %d, want 12", got)
	}

	if m.Frame() != starlight.Galactic {
		t.Errorf("Frame = %q, want galactic", m.Frame())
	}

	if bands := m.Bands(); len(bands) != 1 || bands[0] != "V" {
		t.Errorf("Bands = %v, want [V]", bands)
	}

	for pixel := range int64(12) {
		got, err := m.Pixel("V", pixel)
		if err != nil {
			t.Fatalf("Pixel(%d): %v", pixel, err)
		}

		if got != float64(pixel) {
			t.Errorf("pixel %d holds %v, want %v", pixel, got, float64(pixel))
		}
	}
}

// Without a header the columns are numbered, so a table with no metadata is
// still usable rather than rejected.
func TestLoadUnnamedBands(t *testing.T) {
	t.Parallel()

	m, err := starlight.Load(strings.NewReader(tableFor("")), starlight.ICRS)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if bands := m.Bands(); len(bands) != 1 || bands[0] != "1" {
		t.Errorf("Bands = %v, want [1]", bands)
	}
}

// Multiple bands, comma-separated, which is how a published table is most
// likely to arrive.
func TestLoadMultipleBands(t *testing.T) {
	t.Parallel()

	var b strings.Builder

	b.WriteString("# bands: V, G, scotopic\n")

	for pixel := range 12 {
		fmt.Fprintf(&b, "%d, %g, %g, %g\n", pixel, float64(pixel), float64(pixel)*2, float64(pixel)*3)
	}

	m, err := starlight.Load(strings.NewReader(b.String()), starlight.Galactic)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got := len(m.Bands()); got != 3 {
		t.Fatalf("got %d bands, want 3", got)
	}

	for band, factor := range map[string]float64{"V": 1, "G": 2, "scotopic": 3} {
		got, err := m.Pixel(band, 5)
		if err != nil {
			t.Fatalf("Pixel(%q, 5): %v", band, err)
		}

		if want := 5 * factor; got != want {
			t.Errorf("band %q pixel 5 = %v, want %v", band, got, want)
		}
	}
}

// A hole in a sky map is not a dark patch of sky. A table missing a pixel
// must be rejected, not silently zero-filled — a zero radiance is a claim
// about the sky, and the absence of a row is not.
func TestLoadRejectsIncompleteMap(t *testing.T) {
	t.Parallel()

	var b strings.Builder

	b.WriteString("# bands: V\n")

	for pixel := range 11 { // one short of a complete nside-1 map
		fmt.Fprintf(&b, "%d %g\n", pixel, 1.0)
	}

	if _, err := starlight.Load(strings.NewReader(b.String()), starlight.Galactic); !errors.Is(err, starlight.ErrMapSize) {
		t.Errorf("11 of 12 pixels: err = %v, want ErrMapSize", err)
	}
}

func TestLoadRejectsMalformed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		table string
		want  error
	}{
		{"no radiance", "# bands: V\n0\n", starlight.ErrMapFormat},
		{"bad index", "# bands: V\nx 1.0\n", starlight.ErrMapFormat},
		{"bad radiance", "# bands: V\n0 abc\n", starlight.ErrMapFormat},
		{"negative radiance", "# bands: V\n0 -1.0\n", starlight.ErrMapFormat},
		{"ragged rows", "# bands: V\n0 1.0\n1 1.0 2.0\n", starlight.ErrMapFormat},
		{"duplicate pixel", "# bands: V\n0 1.0\n0 2.0\n", starlight.ErrMapFormat},
		{"header count mismatch", "# bands: V G\n" + rows(12, 1), starlight.ErrMapFormat},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := starlight.Load(strings.NewReader(tc.table), starlight.Galactic); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

// rows renders n pixels with v values each, all 1.0.
func rows(n, v int) string {
	var b strings.Builder

	for pixel := range n {
		fmt.Fprintf(&b, "%d", pixel)

		for range v {
			b.WriteString(" 1.0")
		}

		b.WriteString("\n")
	}

	return b.String()
}

// A lookup by direction must land in the pixel whose centre it is, which ties
// the map's indexing to coord.HEALPix's.
func TestRadianceAtFollowsTheGrid(t *testing.T) {
	t.Parallel()

	m, err := starlight.Load(strings.NewReader(tableFor("V")), starlight.Galactic)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	for pixel := range m.Grid().NumPixels() {
		lon, lat, err := m.Grid().Center(pixel)
		if err != nil {
			t.Fatalf("Center(%d): %v", pixel, err)
		}

		got, err := m.RadianceAt("V", lon, lat)
		if err != nil {
			t.Fatalf("RadianceAt: %v", err)
		}

		if got != float64(pixel) {
			t.Errorf("centre of pixel %d returned %v, want %v", pixel, got, float64(pixel))
		}
	}
}

func TestMapRejectsBadRequests(t *testing.T) {
	t.Parallel()

	m, err := starlight.Load(strings.NewReader(tableFor("V")), starlight.Galactic)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if _, err := m.RadianceAt("K", 0, 0); !errors.Is(err, starlight.ErrBand) {
		t.Errorf("unknown band: err = %v, want ErrBand", err)
	}

	for _, pixel := range []int64{-1, 12, 1000} {
		if _, err := m.Pixel("V", pixel); !errors.Is(err, starlight.ErrPixelRange) {
			t.Errorf("Pixel(%d): err = %v, want ErrPixelRange", pixel, err)
		}
	}
}

func TestNewMapValidates(t *testing.T) {
	t.Parallel()

	if _, err := starlight.NewMap(starlight.Galactic, nil); !errors.Is(err, starlight.ErrBand) {
		t.Errorf("no bands: err = %v, want ErrBand", err)
	}

	ragged := map[string][]float64{"V": make([]float64, 12), "G": make([]float64, 48)}
	if _, err := starlight.NewMap(starlight.Galactic, ragged); !errors.Is(err, starlight.ErrMapSize) {
		t.Errorf("ragged bands: err = %v, want ErrMapSize", err)
	}

	for _, npix := range []int{0, 7, 13, 100} {
		bad := map[string][]float64{"V": make([]float64, npix)}
		if _, err := starlight.NewMap(starlight.Galactic, bad); !errors.Is(err, starlight.ErrMapSize) {
			t.Errorf("%d pixels: err = %v, want ErrMapSize", npix, err)
		}
	}

	// 12*nside^2 for a non-power-of-two nside is the subtle one: 12*9 = 108
	// is divisible by 12 and a perfect square, but nside 3 is not a valid
	// HEALPix resolution.
	bad := map[string][]float64{"V": make([]float64, 108)}
	if _, err := starlight.NewMap(starlight.Galactic, bad); err == nil {
		t.Error("108 pixels (nside 3) was accepted, want an error")
	}
}

// A caller must not be able to mutate a map through the slice they built it
// from.
func TestNewMapCopiesInput(t *testing.T) {
	t.Parallel()

	values := make([]float64, 12)
	values[3] = 7

	m, err := starlight.NewMap(starlight.Galactic, map[string][]float64{"V": values})
	if err != nil {
		t.Fatalf("NewMap: %v", err)
	}

	values[3] = 999

	if got, _ := m.Pixel("V", 3); got != 7 {
		t.Errorf("pixel 3 = %v after mutating the caller's slice, want 7", got)
	}
}

// A flat shape through a flat 100 nm response spreads a band radiance evenly:
// 1 W m^-2 sr^-1 over 100 nm is 0.01 W m^-2 sr^-1 nm^-1.
func TestSpectralShapeScale(t *testing.T) {
	t.Parallel()

	s := starlight.SpectralShape{
		WavelengthNM: []unit.WavelengthNM{500, 600},
		Shape:        []float64{1, 1},
		Response:     []float64{1, 1},
	}

	scale, err := s.Scale()
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}

	for i, got := range scale {
		if want := 0.01; math.Abs(got-want) > 1e-12 {
			t.Errorf("scale[%d] = %v, want %v", i, got, want)
		}
	}

	// The shape's absolute scale must cancel against the overlap integral.
	scaled := starlight.SpectralShape{
		WavelengthNM: s.WavelengthNM,
		Shape:        []float64{25, 25},
		Response:     s.Response,
	}

	other, err := scaled.Scale()
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}

	for i := range other {
		if math.Abs(other[i]-scale[i]) > 1e-12 {
			t.Errorf("a 25x shape gave scale[%d] = %v, want the same %v", i, other[i], scale[i])
		}
	}
}

// A narrower response concentrates the same band radiance into fewer
// nanometres, so the spectral radiance must rise.
func TestSpectralShapeNarrowerResponseIsBrighter(t *testing.T) {
	t.Parallel()

	wide := starlight.SpectralShape{
		WavelengthNM: []unit.WavelengthNM{500, 600},
		Shape:        []float64{1, 1},
		Response:     []float64{1, 1},
	}

	narrow := starlight.SpectralShape{
		WavelengthNM: []unit.WavelengthNM{500, 600},
		Shape:        []float64{1, 1},
		Response:     []float64{0.5, 0.5},
	}

	w, err := wide.Scale()
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}

	n, err := narrow.Scale()
	if err != nil {
		t.Fatalf("Scale: %v", err)
	}

	if n[0] <= w[0] {
		t.Errorf("halving the response gave %v, want more than %v", n[0], w[0])
	}
}

func TestSpectralShapeRejectsBadInput(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		shape starlight.SpectralShape
	}{
		{"empty", starlight.SpectralShape{}},
		{"length mismatch", starlight.SpectralShape{
			WavelengthNM: []unit.WavelengthNM{500, 600},
			Shape:        []float64{1},
			Response:     []float64{1, 1},
		}},
		{"descending", starlight.SpectralShape{
			WavelengthNM: []unit.WavelengthNM{600, 500},
			Shape:        []float64{1, 1},
			Response:     []float64{1, 1},
		}},
		{"zero response", starlight.SpectralShape{
			WavelengthNM: []unit.WavelengthNM{500, 600},
			Shape:        []float64{1, 1},
			Response:     []float64{0, 0},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := tc.shape.Scale(); !errors.Is(err, starlight.ErrMapFormat) {
				t.Errorf("err = %v, want ErrMapFormat", err)
			}
		})
	}
}

// A realistic-size map: GAMBONS publishes at HEALPix order 8, nside 256.
func BenchmarkRadianceAt(b *testing.B) {
	const npix = 12 * 256 * 256

	values := make([]float64, npix)
	for i := range values {
		values[i] = float64(i%1000) * 1e-9
	}

	m, err := starlight.NewMap(starlight.Galactic, map[string][]float64{"V": values})
	if err != nil {
		b.Fatal(err)
	}

	lon, lat := angle.Deg(123.4), angle.Deg(-42.1)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := m.RadianceAt("V", lon, lat); err != nil {
			b.Fatal(err)
		}
	}
}
