package skybrightness_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

func zenith() coord.AltAz { return coord.NewAltAz(angle.Deg(90), angle.Zero()) }

// TestAirglowDefault verifies the default airglow floor round-trips to ~21.9.
func TestAirglowDefault(t *testing.T) {
	t.Parallel()

	r, err := skybrightness.NewAirglow().Radiance(zenith(), nil)
	if err != nil {
		t.Fatalf("Radiance: %v", err)
	}

	testutil.AssertNear(t, "default airglow", float64(r.SurfaceBrightnessV()), 21.9, 1e-9)
}

// TestAirglowCustom verifies a caller-specified floor is used.
func TestAirglowCustom(t *testing.T) {
	t.Parallel()

	r, _ := skybrightness.NewAirglowSB(20.0).Radiance(zenith(), nil)
	testutil.AssertNear(t, "custom airglow", float64(r.SurfaceBrightnessV()), 20.0, 1e-9)
}

// TestAirglowZeroValue verifies the zero-value Airglow falls back to the default.
func TestAirglowZeroValue(t *testing.T) {
	t.Parallel()

	var a skybrightness.Airglow

	r, _ := a.Radiance(zenith(), nil)
	testutil.AssertNear(t, "zero-value airglow", float64(r.SurfaceBrightnessV()), 21.9, 1e-9)
}
