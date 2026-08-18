//go:build validation

package starlight_test

import (
	"context"
	"math"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// The absolute scale of the map, checked against a number nobody in this
// repository chose.
//
// Integrated starlight away from the Galactic plane is about 23.5 mag arcsec^-2
// in V — the figure Masana et al. (2021) and Leinert et al. (1998) both land on,
// and roughly a hundredth of the natural sky's total. Every other test here
// checks that the query is well formed or that the arithmetic is
// self-consistent; none of them would notice if the three zero points behind
// GaiaJohnsonV were combined wrongly, because a factor-of-ten error in the
// absolute scale is invisible to an internal consistency check.
//
// This builds a small patch of real sky and asserts that number.
func TestGaiaMapMatchesThePublishedSurfaceBrightness(t *testing.T) {
	//nolint:noctx // a reachability pre-check, not a request that should honour a deadline
	if c, err := net.DialTimeout("tcp", "gea.esac.esa.int:443", 5*time.Second); err != nil {
		t.Skipf("Gaia archive unreachable: %v", err)
	} else {
		_ = c.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	build := starlight.GaiaBuild{
		Order: 6,
		Chunk: 64,
		Bands: []starlight.GaiaBand{starlight.GaiaJohnsonV()},
	}

	// Gaia's source_id indexes HEALPix in ICRS, not galactic, so pixels 0-63
	// at order 6 are base pixel 0 of the north equatorial cap rather than a
	// chosen Galactic latitude. Most of that patch is off the plane, which is
	// what makes the published figure the right comparison — and if the patch
	// were dominated by the plane the measured value would come out brighter
	// than the bound below, not fainter, so the test would say so.
	m, counts, err := starlight.RunChunk(ctx, build, 0, 63)
	if err != nil {
		t.Fatalf("RunChunk: %v", err)
	}

	// The V zero point in surface brightness: mag arcsec^-2 from a spectral
	// radiance in W m^-2 sr^-1 nm^-1, via Johnson V's Vega flux density and
	// the steradians in a square arcsecond.
	const (
		vZeroFlux      = 3.63e-11
		arcsec2PerSter = 4.254517e10
	)

	var (
		total   float64
		sampled int
		sources int64
	)

	for pixel := range int64(64) {
		v, err := m.Pixel("V", pixel)
		if err != nil {
			t.Fatalf("Pixel(%d): %v", pixel, err)
		}

		if v <= 0 {
			continue
		}

		total += v
		sampled++
		sources += counts[pixel]
	}

	if sampled == 0 {
		t.Fatal("no pixel in the patch carried any flux")
	}

	mean := total / float64(sampled)
	mag := -2.5 * math.Log10(mean/(vZeroFlux*arcsec2PerSter))

	t.Logf("%d pixels, %d sources, mean %.4e W m^-2 sr^-1 nm^-1, %.2f mag arcsec^-2",
		sampled, sources, mean, mag)

	// One magnitude either side: the patch is not the exact sky the published
	// figure averages over, and the shape of the Galaxy varies across it. A
	// zero-point or unit error would miss by far more than that.
	if mag < 22.5 || mag > 24.5 {
		t.Errorf("integrated starlight is %.2f mag arcsec^-2, want 23.5 within a magnitude", mag)
	}
}
