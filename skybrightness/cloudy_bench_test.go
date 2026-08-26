package skybrightness_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// benchCloudy runs one component in one direction on the full optical grid.
//
// The grid matters more here than for any other component: this one's cost is
// a height integral per wavelength, so it scales with the grid where the
// analytic solution does not. The default optical grid is what a caller
// actually uses, so that is what is measured.
func benchCloudy(b *testing.B, c skybrightness.Component, scene *skybrightness.Scene) {
	b.Helper()

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)
	dir := coord.NewAltAz(angle.Deg(60), angle.Deg(0))

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		clear(dst)

		if _, err := c.AddRadiance(b.Context(), dst, grid, dir, scene); err != nil {
			b.Fatalf("AddRadiance: %v", err)
		}
	}
}

// The analytic clear-sky solution, for scale.
func BenchmarkArtificialSkyglowAnalytic(b *testing.B) {
	city := cloudyCity(b, 0, 5)

	c, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		b.Fatalf("NewArtificialSkyglow: %v", err)
	}

	benchCloudy(b, c, cloudyScene(b, 0, 0))
}

// The height-resolved solution over a clear sky, which integrates to the top
// of the atmosphere.
func BenchmarkCloudySkyglowClear(b *testing.B) {
	city := cloudyCity(b, 0, 5)

	c, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		b.Fatalf("NewCloudySkyglow: %v", err)
	}

	benchCloudy(b, c, cloudyScene(b, 0, 0))
}

// The same under an overcast deck, where the integral stops at the cloud base
// and one reflection term is added.
func BenchmarkCloudySkyglowOvercast(b *testing.B) {
	city := cloudyCity(b, 0, 5)

	c, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		b.Fatalf("NewCloudySkyglow: %v", err)
	}

	benchCloudy(b, c, cloudyScene(b, 1000, 0.7))
}

// Ten sources rather than one, since Eq. 28 sums over an inventory and a real
// one is not a single city.
func BenchmarkCloudySkyglowTenSources(b *testing.B) {
	emitters := make([]skybrightness.GroundEmitter, 0, 10)
	for i := range 10 {
		emitters = append(emitters, cloudyCity(b, float64(i)*36, 5+float64(i)))
	}

	c, err := skybrightness.NewCloudySkyglow(emitters)
	if err != nil {
		b.Fatalf("NewCloudySkyglow: %v", err)
	}

	benchCloudy(b, c, cloudyScene(b, 1000, 0.7))
}
