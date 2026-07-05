package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// TestVisualLimitingMagKnownPoints checks the Schaefer/Unihedron NELM relation
//
//	NELM = 7.93 − 5·log₁₀(10^(4.316 − m_sky/5) + 1)
//
// at airmass 1 (no extinction penalty). The expected values are pinned to full
// precision as a regression baseline; each was independently hand-checked to
// ~3 significant figures (a wrong coefficient would shift the result by tens of
// percent), e.g. for m_sky = 22.0:
//
//	10^(4.316 − 4.4) = 10^−0.084 ≈ 0.8241
//	5·log₁₀(1.8241) ≈ 1.305
//	NELM = 7.93 − 1.305 ≈ 6.625  (matches 6.6247 below)
func TestVisualLimitingMagKnownPoints(t *testing.T) {
	t.Parallel()

	m := skybrightness.NewVisualLimitingMag()

	cases := []struct {
		sky  float64
		want float64
	}{
		{22.0, 6.62471141030896}, // pristine dark site (hand-check ≈ 6.625)
		{21.0, 6.11554257232086}, // rural              (hand-check ≈ 6.116)
		{18.0, 3.96805557419435}, // bright urban       (hand-check ≈ 3.968)
	}

	for _, c := range cases {
		got, err := m.LimitingMagnitude(skybrightness.SurfaceBrightnessV(c.sky), 1)
		if err != nil {
			t.Fatalf("LimitingMagnitude(%.1f): %v", c.sky, err)
		}

		testutil.AssertNear(t, "NELM", got, c.want, 1e-9)
	}
}

// TestVisualLimitingMagInfiniteDark verifies an infinitely faint sky yields the
// bright limit (7.93) with no special-casing.
func TestVisualLimitingMagInfiniteDark(t *testing.T) {
	t.Parallel()

	m := skybrightness.NewVisualLimitingMag()

	got, err := m.LimitingMagnitude(skybrightness.SurfaceBrightnessV(math.Inf(1)), 1)
	if err != nil {
		t.Fatalf("LimitingMagnitude(+Inf): %v", err)
	}

	testutil.AssertNear(t, "bright-limit NELM", got, 7.93, 1e-9)
}

// TestVisualLimitingMagAirmassPenalty verifies the k·(X−1) extinction penalty:
// airmass 2 with the default k=0.172 dims the limit by exactly 0.172 mag, and
// airmass < 1 is clamped to 1 (no brightening below the zenith).
func TestVisualLimitingMagAirmassPenalty(t *testing.T) {
	t.Parallel()

	m := skybrightness.NewVisualLimitingMag()
	sky := skybrightness.SurfaceBrightnessV(21.0)

	zenith, _ := m.LimitingMagnitude(sky, 1)
	airmass2, _ := m.LimitingMagnitude(sky, 2)
	below, _ := m.LimitingMagnitude(sky, 0.5)

	testutil.AssertNear(t, "airmass-2 penalty", zenith-airmass2, 0.172, 1e-9)
	testutil.AssertNear(t, "airmass<1 clamped", below, zenith, 1e-9)
}

// TestVisualLimitingMagCustomExtinction verifies WithLimMagExtinction overrides
// the penalty coefficient.
func TestVisualLimitingMagCustomExtinction(t *testing.T) {
	t.Parallel()

	m := skybrightness.NewVisualLimitingMag(skybrightness.WithLimMagExtinction(0.30))
	sky := skybrightness.SurfaceBrightnessV(21.0)

	zenith, _ := m.LimitingMagnitude(sky, 1)
	airmass2, _ := m.LimitingMagnitude(sky, 2)

	testutil.AssertNear(t, "custom extinction penalty", zenith-airmass2, 0.30, 1e-9)
}

// TestVisualLimitingMagMonotonic verifies brighter sky ⇒ shallower limiting
// magnitude across the urban→pristine range.
func TestVisualLimitingMagMonotonic(t *testing.T) {
	t.Parallel()

	m := skybrightness.NewVisualLimitingMag()

	prev := math.Inf(-1)

	for sky := 16.0; sky <= 22.0; sky += 0.5 {
		got, _ := m.LimitingMagnitude(skybrightness.SurfaceBrightnessV(sky), 1)
		if got < prev {
			t.Errorf("NELM not monotonic: sky=%.1f gave %.4f < previous %.4f", sky, got, prev)
		}

		prev = got
	}
}
