package viirs_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset/raster"
	"github.com/TuSKan/astrogo/skybrightness/dataset/viirs"
	"github.com/TuSKan/astrogo/unit"
)

// topHatSpectrum is a flat source spectrum over a flat 500-900 nm response —
// a deliberately trivial fixture whose scaling can be computed by hand.
func topHatSpectrum() viirs.SourceSpectrum {
	return viirs.SourceSpectrum{
		WavelengthNM: []unit.WavelengthNM{500, 900},
		Shape:        []float64{1, 1},
		Response:     []float64{1, 1},
	}
}

// A flat spectrum through a flat 400 nm-wide response must spread the band
// radiance evenly across the band: 1 nW/cm^2/sr becomes 1e-5 W/m^2/sr, and
// divided over 400 nm that is 2.5e-8 W m^-2 sr^-1 nm^-1.
//
// This pins the unit conversion, which is the step where a factor of 1e-5 or
// a forgotten band width would produce a numerically plausible and
// scientifically useless answer.
func TestSourceSpectrumScaling(t *testing.T) {
	t.Parallel()

	site := paranal(t)
	grid := uniformGrid(1.0) // 1 nW/cm^2/sr everywhere

	emitters, err := viirs.FromGrid(grid, 2024).Emitters(viirs.Region{
		Site:     site,
		InnerM:   1_000,
		OuterM:   5_000,
		Sectors:  1,
		Spectrum: topHatSpectrum(),
		// One radial sample, so the summed radiance is one pixel's.
		RadialSamples: 1,
	})
	if err != nil {
		t.Fatalf("Emitters: %v", err)
	}

	if len(emitters) != 1 {
		t.Fatalf("got %d emitters, want 1", len(emitters))
	}

	const want = 1e-5 / 400 // W m^-2 sr^-1 nm^-1

	dst := make([]float64, 2)

	spectralGrid, err := unit.NewSpectralGrid(600, 100, 2)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	// Straight up from the source, where the emission weight is 1.
	if err := emitters[0].SourceRadiance(dst, spectralGrid, 0, angle.Deg(90)); err != nil {
		t.Fatalf("SourceRadiance: %v", err)
	}

	if rel := math.Abs(dst[0]-want) / want; rel > 1e-9 {
		t.Errorf("spectral radiance = %.6g, want %.6g W m^-2 sr^-1 nm^-1", dst[0], want)
	}
}

// The conversion is linear in the pixel radiance, and independent of the
// spectrum's absolute scale — a normalised curve and one ten times larger
// must give the same answer, because the shape cancels against the response
// integral.
func TestSourceSpectrumScaleIsShapeNormalised(t *testing.T) {
	t.Parallel()

	site := paranal(t)

	radianceFor := func(shape float64, pixel float64) float64 {
		spec := topHatSpectrum()
		spec.Shape = []float64{shape, shape}

		emitters, err := viirs.FromGrid(uniformGrid(pixel), 2024).Emitters(viirs.Region{
			Site: site, InnerM: 1_000, OuterM: 5_000, Sectors: 1, RadialSamples: 1, Spectrum: spec,
		})
		if err != nil {
			t.Fatalf("Emitters: %v", err)
		}

		g, err := unit.NewSpectralGrid(600, 100, 2)
		if err != nil {
			t.Fatalf("NewSpectralGrid: %v", err)
		}

		dst := make([]float64, 2)
		if err := emitters[0].SourceRadiance(dst, g, 0, angle.Deg(90)); err != nil {
			t.Fatalf("SourceRadiance: %v", err)
		}

		return dst[0]
	}

	base := radianceFor(1, 1)

	if got := radianceFor(10, 1); math.Abs(got-base)/base > 1e-12 {
		t.Errorf("a ten-times-larger shape gave %.6g, want the same %.6g", got, base)
	}

	if got := radianceFor(1, 3); math.Abs(got-3*base)/(3*base) > 1e-12 {
		t.Errorf("three times the pixel radiance gave %.6g, want %.6g", got, 3*base)
	}
}

func TestSourceSpectrumRejectsBadInput(t *testing.T) {
	t.Parallel()

	site := paranal(t)

	cases := []struct {
		name string
		spec viirs.SourceSpectrum
	}{
		{"empty", viirs.SourceSpectrum{}},
		{"length mismatch", viirs.SourceSpectrum{
			WavelengthNM: []unit.WavelengthNM{500, 900},
			Shape:        []float64{1},
			Response:     []float64{1, 1},
		}},
		{"descending wavelengths", viirs.SourceSpectrum{
			WavelengthNM: []unit.WavelengthNM{900, 500},
			Shape:        []float64{1, 1},
			Response:     []float64{1, 1},
		}},
		{"zero response", viirs.SourceSpectrum{
			WavelengthNM: []unit.WavelengthNM{500, 900},
			Shape:        []float64{1, 1},
			Response:     []float64{0, 0},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := viirs.FromGrid(uniformGrid(1), 2024).Emitters(viirs.Region{
				Site: site, Sectors: 1, RadialSamples: 1, Spectrum: tc.spec,
			})
			if !errors.Is(err, viirs.ErrSpectrum) {
				t.Errorf("err = %v, want ErrSpectrum", err)
			}
		})
	}
}

// One emitter per azimuth sector, and no more — this is the bug fix.
//
// Kocifaj & Bará (2019) Eq. 9 sums over azimuthally separated sources. An
// earlier version of this package binned in distance as well, stacking
// several emitters at one azimuth, which made the total scale with the bin
// count instead of converging. Refining the radial sampling must now leave
// the emitter count alone.
func TestEmittersOnePerAzimuth(t *testing.T) {
	t.Parallel()

	site := paranal(t)

	const sectors = 8

	count := func(radial int) int {
		emitters, err := viirs.FromGrid(uniformGrid(5), 2024).Emitters(viirs.Region{
			Site: site, InnerM: 10_000, OuterM: 40_000,
			Sectors: sectors, RadialSamples: radial, Spectrum: topHatSpectrum(),
		})
		if err != nil {
			t.Fatalf("Emitters: %v", err)
		}

		return len(emitters)
	}

	for _, radial := range []int{1, 4, 20, 100} {
		if got := count(radial); got != sectors {
			t.Errorf("%d radial samples produced %d emitters, want %d", radial, got, sectors)
		}
	}
}

// Every emitter sits within the sampled annulus, at the radiance-weighted
// mean distance along its azimuth. Over a uniform raster that is the plain
// mean, which is the midpoint of the annulus.
func TestEmittersPlacement(t *testing.T) {
	t.Parallel()

	site := paranal(t)

	const (
		inner = 10_000.0
		outer = 40_000.0
	)

	emitters, err := viirs.FromGrid(uniformGrid(5), 2024).Emitters(viirs.Region{
		Site: site, InnerM: inner, OuterM: outer,
		Sectors: 4, RadialSamples: 60, Spectrum: topHatSpectrum(),
	})
	if err != nil {
		t.Fatalf("Emitters: %v", err)
	}

	for i, e := range emitters {
		d, err := coord.GroundDistance(site, e.Location())
		if err != nil {
			t.Fatalf("GroundDistance: %v", err)
		}

		if d < inner || d > outer {
			t.Errorf("emitter %d at %.0f m is outside [%v, %v]", i, d, inner, outer)
		}

		if want := (inner + outer) / 2; math.Abs(d-want) > 500 {
			t.Errorf("emitter %d at %.0f m, want the annulus midpoint %.0f over a uniform raster", i, d, want)
		}
	}
}

// Every emitter has to declare that its spectrum and emission function were
// assumed, because they were. A VIIRS pixel determines neither.
func TestEmittersFlagAssumptions(t *testing.T) {
	t.Parallel()

	emitters, err := viirs.FromGrid(uniformGrid(5), 2024).Emitters(viirs.Region{
		Site: paranal(t), Sectors: 2, RadialSamples: 4, Spectrum: topHatSpectrum(),
	})
	if err != nil {
		t.Fatalf("Emitters: %v", err)
	}

	for i, e := range emitters {
		q := e.Quality()

		if q&skybrightness.AssumedSourceSpectrum == 0 {
			t.Errorf("emitter %d does not flag its spectrum as assumed", i)
		}

		if q&skybrightness.AssumedEmissionFunction == 0 {
			t.Errorf("emitter %d does not flag its emission function as assumed", i)
		}
	}
}

// Missing data is not measured darkness. A bin that resolves to no-data must
// be dropped, not turned into a zero-radiance source, or an unmapped region
// would read as pristine sky.
func TestEmittersSkipNoData(t *testing.T) {
	t.Parallel()

	nan := math.NaN()
	grid := &raster.Grid{
		Width: 2, Height: 1, Data: []float64{nan, nan},
		GT: raster.GeoTransform{A: -180, B: 180, C: 0, D: 90, E: 0, F: -180},
	}

	emitters, err := viirs.FromGrid(grid, 2024).Emitters(viirs.Region{
		Site: paranal(t), Sectors: 4, RadialSamples: 4, Spectrum: topHatSpectrum(),
	})
	if err != nil {
		t.Fatalf("Emitters: %v", err)
	}

	if len(emitters) != 0 {
		t.Errorf("got %d emitters over a no-data raster, want none", len(emitters))
	}
}

// The noise floor: VIIRS reads a small positive radiance over genuinely dark
// ground, and turning that into light sources would invent a city.
func TestEmittersMinRadiance(t *testing.T) {
	t.Parallel()

	region := viirs.Region{
		Site: paranal(t), Sectors: 4, RadialSamples: 4, Spectrum: topHatSpectrum(),
	}

	quiet := viirs.FromGrid(uniformGrid(0.3), 2024)

	all, err := quiet.Emitters(region)
	if err != nil {
		t.Fatalf("Emitters: %v", err)
	}

	if len(all) == 0 {
		t.Fatal("no emitters at all with no threshold set")
	}

	region.MinRadiance = 0.5

	filtered, err := quiet.Emitters(region)
	if err != nil {
		t.Fatalf("Emitters: %v", err)
	}

	if len(filtered) != 0 {
		t.Errorf("got %d emitters below the 0.5 threshold, want none", len(filtered))
	}
}

func TestRegionRejectsBadInput(t *testing.T) {
	t.Parallel()

	r := viirs.FromGrid(uniformGrid(1), 2024)

	if _, err := r.Emitters(viirs.Region{Spectrum: topHatSpectrum()}); !errors.Is(err, viirs.ErrSampling) {
		t.Errorf("no site: err = %v, want ErrSampling", err)
	}

	_, err := r.Emitters(viirs.Region{
		Site: paranal(t), InnerM: 50_000, OuterM: 10_000, Spectrum: topHatSpectrum(),
	})
	if !errors.Is(err, viirs.ErrSampling) {
		t.Errorf("inverted annulus: err = %v, want ErrSampling", err)
	}
}

func TestOpenRejectsEarlyYear(t *testing.T) {
	t.Parallel()

	if _, _, err := viirs.Open(t.Context(), 2000); !errors.Is(err, viirs.ErrYearOutOfRange) {
		t.Errorf("year 2000: err = %v, want ErrYearOutOfRange", err)
	}
}

func TestRadianceAt(t *testing.T) {
	t.Parallel()

	r := viirs.FromGrid(uniformGrid(7), 2024)

	if r.Year() != 2024 {
		t.Errorf("Year() = %d, want 2024", r.Year())
	}

	got, err := r.RadianceAt(-70.4, -24.6)
	if err != nil {
		t.Fatalf("RadianceAt: %v", err)
	}

	if got != 7 {
		t.Errorf("RadianceAt = %v, want 7", got)
	}
}

// uniformGrid covers the whole globe with one radiance value, so a test can
// place a site anywhere without worrying about coverage.
func uniformGrid(radiance float64) *raster.Grid {
	const w, h = 4, 2

	data := make([]float64, w*h)
	for i := range data {
		data[i] = radiance
	}

	return &raster.Grid{
		Width: w, Height: h, Data: data,
		GT: raster.GeoTransform{A: -180, B: 360.0 / w, C: 0, D: 90, E: 0, F: -180.0 / h},
	}
}

func paranal(t *testing.T) *coord.Geodetic {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return loc
}
