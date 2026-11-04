package raster

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// A NaN location has no pixel. bilinear's coverage check is false for NaN, so
// it used to reach the corner index: "no data" on amd64, where NaN converts to
// MinInt64 and every corner is out of bounds, and NaN with no error on arm64,
// where it converts to 0 (#982). It is now outside coverage, as ±Inf already
// was. Both SampleBilinears share bilinear, so the Grid stands for the Reader.
func TestSampleBilinearRefusesANaNLocation(t *testing.T) {
	t.Parallel()

	// A 4 by 4 grid from 0° to 4° east and 0° to 4° north, one degree a pixel.
	g := &Grid{
		Width: 4, Height: 4,
		Data: []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		GT:   GeoTransform{A: 0, B: 1, C: 0, D: 4, E: 0, F: -1},
	}

	for _, c := range []struct{ lon, lat float64 }{
		{math.NaN(), 1.5},
		{1.5, math.NaN()},
		{math.NaN(), math.NaN()},
		{math.Inf(1), 1.5},
	} {
		v, err := g.SampleBilinear(angle.Deg(c.lon), angle.Deg(c.lat))
		if !errors.Is(err, ErrOutOfCoverage) {
			t.Errorf("SampleBilinear(%v, %v) = %v, %v; want ErrOutOfCoverage", c.lon, c.lat, v, err)
		}
	}

	// A finite location inside the grid still samples.
	if v, err := g.SampleBilinear(angle.Deg(1.5), angle.Deg(1.5)); err != nil || v != 10 {
		t.Errorf("SampleBilinear(1.5, 1.5) = %v, %v; want 10 and no error", v, err)
	}
}
