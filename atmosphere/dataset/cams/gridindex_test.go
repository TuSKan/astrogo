package cams

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/vector"
)

// A non-finite site has no grid cell. gridIndex used to convert its NaN
// anyway: on amd64 that reached Var.At as longitude -8 on the 0.4° grid, and on
// arm64 it silently read the cell at the North Pole and Greenwich (#979).
func TestGridIndexRefusesANonFiniteSite(t *testing.T) {
	t.Parallel()

	// The 0.4° global grid.
	dims := map[string]int{"latitude": 451, "longitude": 900}

	// NewGeodetic refuses a non-finite position, but every coordinate type's
	// FromUnitVector is exported and unchecked, so a NaN site can still exist.
	var site coord.Geodetic
	site.FromUnitVector(vector.V3(math.NaN(), math.NaN(), math.NaN()))

	if !math.IsNaN(site.Lon().Radians()) {
		t.Fatalf("the fixture's longitude is %v, want NaN", site.Lon())
	}

	if lat, lon, err := gridIndex(dims, &site); !errors.Is(err, ErrAOD) {
		t.Errorf("gridIndex of a NaN site = (%d, %d), %v; want ErrAOD", lat, lon, err)
	}

	// A finite site still gets a cell on the grid, so the guard is not
	// refusing everything. Which cell is TestAODOrientationIsSane's business:
	// Paranal's latitude row sits on a rounding boundary, 286.5 on paper and
	// 286.49999999999997 in floating point, so it is not pinned here.
	paranal, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	lat, lon, err := gridIndex(dims, paranal)
	if err != nil {
		t.Fatalf("gridIndex of Paranal: %v", err)
	}

	if lat < 0 || lat >= dims["latitude"] || lon < 0 || lon >= dims["longitude"] {
		t.Errorf("gridIndex of Paranal = (%d, %d), off the %d by %d grid", lat, lon, dims["latitude"], dims["longitude"])
	}
}
