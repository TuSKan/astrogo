//go:build validation

package starlight_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// The absolute scale of the map, bounded by numbers nobody in this repository
// chose.
//
// Integrated starlight away from the Galactic plane is about 23.5 mag arcsec^-2
// in V — the figure Masana et al. (2021) and Leinert et al. (1998) both land on,
// and roughly a hundredth of the natural sky's total.
//
// This map must come out **fainter** than that, and the reasons are now
// measured rather than assumed:
//
//   - It is integrated starlight alone. Quoted all-sky background figures
//     include diffuse galactic light, which Leinert et al. put at 20-30 per
//     cent of the integrated light of the Milky Way, plus the extragalactic
//     background. Both are separate Components here by design.
//   - It omits the stars Gaia saturates on. Measured against Hipparcos with
//     proper motion propagated to J2016.0, exactly **74** stars brighter than
//     V = 7 have no Gaia counterpart, 70 of them brighter than V = 3, and they
//     carry **6.4 per cent** of the whole-sky flux.
//   - It omits stars past G = 21, which Masana et al. put below 3 per cent
//     everywhere except a few points on the Galactic plane.
//
// An earlier revision asserted equality within a magnitude and passed while the
// colour transformation was inverted, because the resulting excess brightness
// happened to cancel what the map was missing. Bounding it on one side, against
// a total the map is a known subset of, is what makes the assertion mean
// something.
func TestGaiaMapMatchesThePublishedSurfaceBrightness(t *testing.T) {
	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	build := starlight.GaiaBuild{Order: 6,
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

	// The patch is not the exact sky the published figure averages over and the
	// Galaxy varies across it, so the window is wide — but it is one-sided about
	// the published value. Brighter than 23.5 means the map has more light than
	// every star in the sky put together, which it cannot; fainter than 25.0
	// means most of the light is missing. A zero-point, unit or sign error lands
	// outside one end or the other.
	if mag < 23.5 || mag > 25.0 {
		t.Errorf("integrated starlight is %.2f mag arcsec^-2; a Gaia-only map must sit between "+
			"23.5 (every star in the sky) and 25.0 (most of the light missing)", mag)
	}
}

// runOf finds a run of n consecutive pixels whose centres all satisfy want,
// so a targeted query can sample one part of the sky without aggregating the
// whole of it.
func runOf(t *testing.T, grid coord.HEALPix, n int, want func(b angle.Angle) bool) (first int64) {
	t.Helper()

	streak := int64(0)

	for pixel := range grid.NumPixels() {
		lon, lat, err := grid.Center(pixel)
		if err != nil {
			t.Fatalf("Center(%d): %v", pixel, err)
		}

		if want(coord.ICRSToGalactic(coord.NewICRS(lon, lat)).B()) {
			streak++
			if streak == int64(n) {
				return pixel - int64(n) + 1
			}

			continue
		}

		streak = 0
	}

	t.Fatal("no run of pixels satisfied the latitude condition")

	return 0
}

// The Milky Way has to be where the Milky Way is.
//
// Gaia indexes HEALPix in ICRS, and the map says so. Labelling it galactic
// instead would rotate the plane across the sky — and because the map still
// covers every direction and every value stays positive, nothing else here
// would notice. The plane-to-cap contrast is what does: a frame swap does not
// dim the plane, it moves it, so the two samples would come out alike.
//
// The full order-8 build puts the plane at 21.99 and the polar cap at 23.91
// mag arcsec^-2, over a monotonic profile in galactic latitude. This samples
// sixteen pixels of each rather than 786,432, so the bound is loose, but a
// washed-out contrast is unmissable.
func TestGaiaMapPutsTheMilkyWayInThePlane(t *testing.T) {
	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	build := starlight.GaiaBuild{Order: 6,
		Chunk: 16,
		Bands: []starlight.GaiaBand{starlight.GaiaJohnsonV()},
	}

	// NewHEALPix takes nside, which is 2^order.
	grid, err := coord.NewHEALPix(1 << 6)
	if err != nil {
		t.Fatalf("NewHEALPix: %v", err)
	}

	const run = 16

	plane := runOf(t, grid, run, func(b angle.Angle) bool { return math.Abs(b.Degrees()) < 10 })
	highLat := runOf(t, grid, run, func(b angle.Angle) bool { return math.Abs(b.Degrees()) > 60 })

	sample := func(first int64) float64 {
		m, _, err := starlight.RunChunk(ctx, build, first, first+run-1)
		if err != nil {
			t.Fatalf("RunChunk(%d): %v", first, err)
		}

		var total float64

		var n int

		for pixel := first; pixel <= first+run-1; pixel++ {
			v, err := m.Pixel("V", pixel)
			if err != nil {
				t.Fatalf("Pixel(%d): %v", pixel, err)
			}

			if v > 0 {
				total += v
				n++
			}
		}

		if n == 0 {
			t.Fatalf("pixels %d-%d carried no flux", first, first+run-1)
		}

		return total / float64(n)
	}

	const (
		vZeroFlux      = 3.63e-11
		arcsec2PerSter = 4.254517e10
	)

	toMag := func(v float64) float64 { return -2.5 * math.Log10(v/(vZeroFlux*arcsec2PerSter)) }

	planeMag, capMag := toMag(sample(plane)), toMag(sample(highLat))

	t.Logf("plane (pixels %d+) %.2f, cap (pixels %d+) %.2f mag arcsec^-2",
		plane, planeMag, highLat, capMag)

	if capMag-planeMag < 1.0 {
		t.Errorf("plane %.2f and cap %.2f differ by %.2f mag; the plane must be at least 1 mag brighter",
			planeMag, capMag, capMag-planeMag)
	}
}
