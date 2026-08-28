package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// Roach & Meinel (1955), quoted by Leinert et al.: for an emitting layer at
// 100 km the van Rhijn function reaches 5.7 at the horizon. That is a
// published number from outside this implementation, and it pins the Earth
// radius, the ratio and the square root together.
func TestVanRhijnAgainstRoachAndMeinel(t *testing.T) {
	t.Parallel()

	got, err := atmosphere.VanRhijn(angle.Deg(90), 100_000)
	if err != nil {
		t.Fatalf("VanRhijn: %v", err)
	}

	if math.Abs(got-5.7) > 0.05 {
		t.Errorf("h = 100 km at the horizon gives %.3f, want the published 5.7", got)
	}
}

// At the zenith the layer is seen face on and the function is exactly 1 —
// the normalisation the whole relation is defined against.
func TestVanRhijnZenith(t *testing.T) {
	t.Parallel()

	for _, h := range []float64{80_000, 87_000, 100_000, 300_000} {
		got, err := atmosphere.VanRhijn(0, h)
		if err != nil {
			t.Fatalf("VanRhijn(0, %v): %v", h, err)
		}

		if math.Abs(got-1) > 1e-15 {
			t.Errorf("h = %v m at the zenith gives %v, want exactly 1", h, got)
		}
	}
}

// The enhancement grows monotonically toward the horizon — the geometric
// claim the function exists to make — and a lower layer brightens more,
// because it is seen at a shallower angle.
func TestVanRhijnGrowsTowardTheHorizon(t *testing.T) {
	t.Parallel()

	prev := 0.0

	for _, z := range []float64{0, 20, 40, 60, 75, 85, 89} {
		got, err := atmosphere.VanRhijn(angle.Deg(z), atmosphere.AirglowLayerHeightM)
		if err != nil {
			t.Fatalf("VanRhijn(%v): %v", z, err)
		}

		if got <= prev {
			t.Errorf("z = %v gives %v, not above the %v nearer the zenith", z, got, prev)
		}

		prev = got
	}

	low, err := atmosphere.VanRhijn(angle.Deg(88), 87_000)
	if err != nil {
		t.Fatalf("VanRhijn: %v", err)
	}

	high, err := atmosphere.VanRhijn(angle.Deg(88), 300_000)
	if err != nil {
		t.Fatalf("VanRhijn: %v", err)
	}

	if low <= high {
		t.Errorf("an 87 km layer gives %v at 88 deg, want more than a 300 km layer's %v", low, high)
	}
}

func TestVanRhijnRejectsBadHeight(t *testing.T) {
	t.Parallel()

	for _, h := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if _, err := atmosphere.VanRhijn(angle.Deg(45), h); !errors.Is(err, atmosphere.ErrScaleHeightRange) {
			t.Errorf("h = %v: err = %v, want ErrScaleHeightRange", h, err)
		}
	}
}

// airglowFixture is a flat zenith spectrum on a small grid.
func airglowFixture(t *testing.T, value float64) (unit.SpectralGrid, skybrightness.SpectralRadiance) {
	t.Helper()

	grid, err := unit.NewSpectralGrid(500, 10, 5)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	zenith := skybrightness.NewSpectralRadiance(grid)
	for i := range zenith {
		zenith[i] = value
	}

	return grid, zenith
}

// The component is the zenith spectrum times the van Rhijn factor, band by
// band — the spectrum's shape is preserved and only its scale changes with
// direction.
func TestAirglowScalesTheZenithSpectrum(t *testing.T) {
	t.Parallel()

	grid, zenith := airglowFixture(t, 3e-9)
	dst := skybrightness.NewSpectralRadiance(grid)

	const z = 60.0

	if _, err := skybrightness.AirglowRadiance(dst, grid, zenith, angle.Deg(z), 0); err != nil {
		t.Fatalf("AirglowRadiance: %v", err)
	}

	want, err := atmosphere.VanRhijn(angle.Deg(z), atmosphere.AirglowLayerHeightM)
	if err != nil {
		t.Fatalf("VanRhijn: %v", err)
	}

	for i := range dst {
		if rel := math.Abs(dst[i]-zenith[i]*want) / (zenith[i] * want); rel > 1e-12 {
			t.Errorf("band %d = %v, want %v", i, dst[i], zenith[i]*want)
		}
	}
}

// Airglow brightens toward the horizon, which is what distinguishes it from a
// constant floor and is the reason the module refuses to model it as one.
func TestAirglowBrightensTowardTheHorizon(t *testing.T) {
	t.Parallel()

	grid, zenith := airglowFixture(t, 1e-9)

	at := func(z float64) float64 {
		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := skybrightness.AirglowRadiance(dst, grid, zenith, angle.Deg(z), 0); err != nil {
			t.Fatalf("AirglowRadiance(%v): %v", z, err)
		}

		return dst[0]
	}

	prev := 0.0

	for _, z := range []float64{0, 30, 60, 80, 89} {
		if v := at(z); v <= prev {
			t.Errorf("z = %v gives %v, not above the %v nearer the zenith", z, v, prev)
		} else {
			prev = v
		}
	}

	// Between the zenith and 89 degrees the geometric enhancement is a
	// factor of several, not a few per cent.
	if ratio := at(89) / at(0); ratio < 3 || ratio > 8 {
		t.Errorf("horizon/zenith = %.2f, want the van Rhijn factor of about 6", ratio)
	}
}

// Past 40 degrees the geometry alone overstates the brightness, because
// extinction and scattering along the longer path work against it. Leinert et
// al. say so explicitly, so the result is flagged there.
func TestAirglowFlagsLargeZenithAngles(t *testing.T) {
	t.Parallel()

	grid, zenith := airglowFixture(t, 1e-9)

	flagsAt := func(z float64) skybrightness.Flag {
		dst := skybrightness.NewSpectralRadiance(grid)

		flags, err := skybrightness.AirglowRadiance(dst, grid, zenith, angle.Deg(z), 0)
		if err != nil {
			t.Fatalf("AirglowRadiance(%v): %v", z, err)
		}

		return flags
	}

	for _, z := range []float64{0, 20, 40} {
		if flagsAt(z)&skybrightness.ExtrapolatedModel != 0 {
			t.Errorf("z = %v was flagged as extrapolated but is inside the stated range", z)
		}
	}

	for _, z := range []float64{50, 70, 89} {
		if flagsAt(z)&skybrightness.ExtrapolatedModel == 0 {
			t.Errorf("z = %v is past 40 degrees but was not flagged", z)
		}
	}

	// A supplied reference spectrum is climatology until it is tied to a
	// measurement of the night in question, and must say so at every angle.
	for _, z := range []float64{0, 45, 95} {
		if flagsAt(z)&skybrightness.ClimatologicalAirglow == 0 {
			t.Errorf("z = %v did not report the spectrum as climatological", z)
		}
	}
}

// Below the horizon there is no emitting layer in the line of sight.
func TestAirglowBelowTheHorizon(t *testing.T) {
	t.Parallel()

	grid, zenith := airglowFixture(t, 1e-9)
	dst := skybrightness.NewSpectralRadiance(grid)

	if _, err := skybrightness.AirglowRadiance(dst, grid, zenith, angle.Deg(95), 0); err != nil {
		t.Fatalf("AirglowRadiance: %v", err)
	}

	for i, v := range dst {
		if v != 0 {
			t.Errorf("band %d is %v below the horizon, want 0", i, v)
		}
	}
}

func TestAirglowAccumulates(t *testing.T) {
	t.Parallel()

	grid, zenith := airglowFixture(t, 2e-9)

	once := skybrightness.NewSpectralRadiance(grid)
	if _, err := skybrightness.AirglowRadiance(once, grid, zenith, angle.Deg(30), 0); err != nil {
		t.Fatalf("AirglowRadiance: %v", err)
	}

	twice := skybrightness.NewSpectralRadiance(grid)
	for range 2 {
		if _, err := skybrightness.AirglowRadiance(twice, grid, zenith, angle.Deg(30), 0); err != nil {
			t.Fatalf("AirglowRadiance: %v", err)
		}
	}

	if rel := math.Abs(twice[0]-2*once[0]) / (2 * once[0]); rel > 1e-12 {
		t.Errorf("two calls gave %v, want twice the %v of one", twice[0], once[0])
	}
}

func TestAirglowRejectsBadInput(t *testing.T) {
	t.Parallel()

	grid, zenith := airglowFixture(t, 1e-9)
	dst := skybrightness.NewSpectralRadiance(grid)

	if _, err := skybrightness.AirglowRadiance(make(skybrightness.SpectralRadiance, 2), grid, zenith, 0, 0); !errors.Is(err, unit.ErrGridMismatch) {
		t.Errorf("short destination: err = %v, want ErrGridMismatch", err)
	}

	if _, err := skybrightness.AirglowRadiance(dst, grid, make(skybrightness.SpectralRadiance, 2), 0, 0); !errors.Is(err, skybrightness.ErrAirglowSpectrum) {
		t.Errorf("short spectrum: err = %v, want ErrAirglowSpectrum", err)
	}

	negative := skybrightness.NewSpectralRadiance(grid)
	negative[2] = -1

	if _, err := skybrightness.AirglowRadiance(dst, grid, negative, 0, 0); !errors.Is(err, skybrightness.ErrAirglowSpectrum) {
		t.Errorf("negative spectrum: err = %v, want ErrAirglowSpectrum", err)
	}
}

func BenchmarkAirglow(b *testing.B) {
	grid := skybrightness.DefaultOpticalGrid()

	zenith := skybrightness.NewSpectralRadiance(grid)
	for i := range zenith {
		zenith[i] = 1e-9
	}

	dst := skybrightness.NewSpectralRadiance(grid)

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := skybrightness.AirglowRadiance(dst, grid, zenith, angle.Deg(45), 0); err != nil {
			b.Fatal(err)
		}
	}
}
