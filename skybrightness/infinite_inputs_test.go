package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// A heliocentric distance or an Earth longitude that is not finite is
// refused. R^-2.3 of an infinite distance is zero, so the zodiacal term
// vanished, and a NaN longitude reached the seasonal factor above 60 degrees
// of latitude and came out as a NaN radiance, both with no error (#1038).
func TestZodiacalRadianceRefusesANonFiniteObserver(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 10, 31)
	if err != nil {
		t.Fatal(err)
	}

	good := skybrightness.ZodiacalGeometry{
		DifferentialLongitude: angle.Deg(90),
		EclipticLatitude:      angle.Deg(70),
		SunDistance:           unit.AU(1),
		EarthLongitude:        angle.Deg(30),
	}

	dst := skybrightness.NewSpectralRadiance(grid)
	if _, err := skybrightness.ZodiacalRadiance(dst, grid, good); err != nil || !(dst[15] > 0) {
		t.Fatalf("finite geometry: 550 nm %v, err %v", dst[15], err)
	}

	for _, c := range []struct {
		name   string
		mutate func(*skybrightness.ZodiacalGeometry)
	}{
		{"distance +Inf", func(g *skybrightness.ZodiacalGeometry) { g.SunDistance = unit.AU(math.Inf(1)) }},
		{"Earth longitude NaN", func(g *skybrightness.ZodiacalGeometry) { g.EarthLongitude = angle.Deg(math.NaN()) }},
		{"Earth longitude +Inf", func(g *skybrightness.ZodiacalGeometry) { g.EarthLongitude = angle.Deg(math.Inf(1)) }},
	} {
		geom := good
		c.mutate(&geom)

		dst.Zero()

		if _, err := skybrightness.ZodiacalRadiance(dst, grid, geom); !errors.Is(err, skybrightness.ErrZodiacalGeometry) {
			t.Errorf("%s: 550 nm %v, err %v, want ErrZodiacalGeometry", c.name, dst[15], err)
		}
	}
}

// A solar spectrum with an infinite band is refused, as one with a negative or
// NaN band is. The check was v < 0 || NaN (#1038).
func TestNewScatteredMoonlightRefusesAnInfiniteBand(t *testing.T) {
	t.Parallel()

	solar := make([]float64, len(magnitude.ROLOBands()))
	for i := range solar {
		solar[i] = 1.8
	}

	if _, err := skybrightness.NewScatteredMoonlight(solar); err != nil {
		t.Fatalf("a finite spectrum: %v", err)
	}

	solar[3] = math.Inf(1)

	if m, err := skybrightness.NewScatteredMoonlight(solar); !errors.Is(err, skybrightness.ErrNoSolarSpectrum) {
		t.Errorf("band 3 = +Inf: %v and err %v, want ErrNoSolarSpectrum", m != nil, err)
	}
}
