//go:build network

package crosssection_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/atmosphere"
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
		testutil.SkipOnUpstreamFailure(t, err)
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

// atmosphere.Extinction carries ozone as 10 nm means of this same file, so
// that a planning call needs no download. Every one of those means is
// recomputed here from the file, with the rule the table was built by: the
// node at 600 nm is the mean of every sample from 595 up to, not including,
// 605 nm.
//
// The table is read back through Extinction, as the difference a column of
// ozone makes to it, since Extinction is its only door. Its values are printed
// to five significant figures, so rounding is at most 5e-5 of a value; 1e-4
// leaves room for nothing else. Measured: 2.7e-5, at 380 nm.
func TestExtinctionOzoneIsTheDatasetBinned(t *testing.T) {
	testutil.RequireReachable(t, "www.uv-vis-spectral-atlas-mainz.org:443")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	remote.EnableDownloads(16<<20, remote.MPIMainzCrossSections)
	defer remote.DisableDownloads(remote.MPIMainzCrossSections)

	xs, err := crosssection.Ozone(ctx)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("Ozone: %v", err)
	}

	const (
		column    = 1000.0 // DU
		magPerTau = 2.5 * math.Log10E
		bound     = 1e-4 // relative
	)

	without, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	with, err := atmosphere.NewBuilder().Ozone(column).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	perSigma := magPerTau * column * atmosphere.DobsonUnitMoleculesPerCM2()

	var worst, worstAt float64

	for node := 320.0; node <= 1000; node += 10 {
		var sum float64

		var n int

		for i, nm := range xs.WavelengthNM {
			if float64(nm) >= node-5 && float64(nm) < node+5 {
				sum += xs.SigmaCM2[i]
				n++
			}
		}

		if n == 0 {
			t.Fatalf("the file has no samples within 5 nm of %g nm", node)
		}

		want := sum / float64(n)

		a, err := with.Extinction(unit.WavelengthNM(node))
		if err != nil {
			t.Fatalf("Extinction at %g nm: %v", node, err)
		}

		b, err := without.Extinction(unit.WavelengthNM(node))
		if err != nil {
			t.Fatalf("Extinction at %g nm: %v", node, err)
		}

		got := (a - b) / perSigma

		rel := got/want - 1
		if math.Abs(rel) > math.Abs(worst) {
			worst, worstAt = rel, node
		}

		if math.Abs(rel) > bound {
			t.Errorf("%g nm: Extinction carries %.5g cm^2, the file's mean over %d samples is %.5g "+
				"(%+.2e relative)", node, got, n, want, rel)
		}
	}

	t.Logf("worst %+.2e relative, at %g nm", worst, worstAt)
}
