package magnitude_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/unit"
)

// An infinite distance is refused. It gave zero irradiance in every band,
// which reads as a Moon that is there and dark, with no error (#1038).
func TestROLOIrradianceRefusesAnInfiniteDistance(t *testing.T) {
	t.Parallel()

	n := len(magnitude.ROLOBands())
	refl, solar, dst := make([]float64, n), make([]float64, n), make([]float64, n)

	for i := range refl {
		refl[i], solar[i] = 0.1, 1.8
	}

	if err := magnitude.ROLOIrradiance(dst, refl, solar, unit.AU(1), unit.Km(384400)); err != nil || !(dst[0] > 0) {
		t.Fatalf("finite distances: band 0 %v, err %v", dst[0], err)
	}

	for _, d := range [][2]unit.Length{
		{unit.AU(math.Inf(1)), unit.Km(384400)},
		{unit.AU(1), unit.Meters(math.Inf(1))},
	} {
		if err := magnitude.ROLOIrradiance(dst, refl, solar, d[0], d[1]); !errors.Is(err, magnitude.ErrROLODistance) {
			t.Errorf("distances %v: err %v, want ErrROLODistance", d, err)
		}
	}
}
