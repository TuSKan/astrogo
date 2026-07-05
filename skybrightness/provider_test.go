package skybrightness_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// TestScalarProviderConstant verifies the scalar provider reports the same
// brightness regardless of location.
func TestScalarProviderConstant(t *testing.T) {
	t.Parallel()

	const want = skybrightness.SurfaceBrightnessV(20.5)

	p := skybrightness.NewScalarProvider(want)

	for _, c := range [][2]float64{{0, 0}, {-23.55, -46.63}, {78.9, 11.9}} {
		got, err := p.ZenithBrightness(c[0], c[1])
		if err != nil {
			t.Fatalf("ZenithBrightness(%v): %v", c, err)
		}

		if got != want {
			t.Errorf("ZenithBrightness(%v) = %g, want %g", c, float64(got), float64(want))
		}
	}
}

// TestNewFloorFromProvider verifies the bridge from a geographic provider to an
// in-sky Floor: the resolved zenith brightness becomes a uniform floor radiance.
func TestNewFloorFromProvider(t *testing.T) {
	t.Parallel()

	const sqm = skybrightness.SurfaceBrightnessV(19.0)

	floor, err := skybrightness.NewFloorFromProvider(skybrightness.NewScalarProvider(sqm), -23.55, -46.63)
	if err != nil {
		t.Fatalf("NewFloorFromProvider: %v", err)
	}

	got, err := floor.Radiance(coord.NewAltAz(angle.Deg(45), angle.Deg(0)), nil)
	if err != nil {
		t.Fatalf("Radiance: %v", err)
	}

	want := sqm.Nanolamberts()
	testutil.AssertNear(t, "floor radiance", float64(got), float64(want), 1e-9)
}
