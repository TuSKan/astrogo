//go:build network

package crosssection_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/crosssection"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// The real file, checked against ozone's two textbook features.
//
// A cross-section table is a long column of numbers that all look alike, so
// the useful assertions are the ones a wrong file or a mangled unit could not
// satisfy: the Hartley maximum near 255 nm and the Chappuis maximum near 600
// nm, four orders of magnitude apart. Both are properties of the molecule, not
// of this repository.
func TestOzoneMatchesItsKnownBands(t *testing.T) {
	testutil.RequireReachable(t, "www.uv-vis-spectral-atlas-mainz.org:443")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	remote.EnableDownloads(16<<20, remote.MPIMainzCrossSections)
	defer remote.DisableDownloads(remote.MPIMainzCrossSections)

	xs, err := crosssection.Ozone(ctx)
	if err != nil {
		t.Fatalf("Ozone: %v", err)
	}

	// A standard 300 DU column, evaluated across the optical grid.
	grid, err := unit.NewSpectralGrid(330, 1, 671)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	tau := make([]float64, grid.Len())
	if err := xs.OzoneOpticalDepth(tau, grid, 300); err != nil {
		t.Fatalf("OzoneOpticalDepth: %v", err)
	}

	at := func(nm int) float64 { return tau[nm-330] }

	// The Chappuis band is what ozone does to the visible sky: a broad, weak
	// absorption peaking near 600 nm. For 300 DU it reaches about 0.04 in
	// optical depth — roughly 4 per cent of the light, which is why leaving
	// ozone out is a real error and not a catastrophic one.
	if peak := at(600); peak < 0.03 || peak > 0.05 {
		t.Errorf("600 nm gives tau = %.4f for 300 DU, want about 0.04", peak)
	}

	// It has to be a band, not a constant: the blue end is far more
	// transparent than the Chappuis peak.
	if blue := at(400); blue >= at(600) {
		t.Errorf("400 nm gives tau = %.4f, not below the 600 nm peak %.4f", blue, at(600))
	}

	// And it has to fall away again toward the red.
	if red := at(900); red >= at(600) {
		t.Errorf("900 nm gives tau = %.4f, not below the 600 nm peak %.4f", red, at(600))
	}

	// Nothing anywhere on the grid may be negative or NaN.
	for i, v := range tau {
		if v < 0 || math.IsNaN(v) {
			t.Fatalf("%v nm gives tau = %v", grid.At(i), v)
		}
	}
}
