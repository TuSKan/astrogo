package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// TestNanolambertRoundTrip verifies V → nL → V is the identity.
func TestNanolambertRoundTrip(t *testing.T) {
	t.Parallel()

	for _, v := range []float64{16.0, 18.5, 20.0, 21.0, 21.9, 22.5} {
		sb := skybrightness.SurfaceBrightnessV(v)
		got := float64(sb.Nanolamberts().SurfaceBrightnessV())
		testutil.AssertNear(t, "round-trip", got, v, 1e-12)
	}
}

// TestLinearSpaceInvariant is the mandatory linear-combination invariant: two
// equal-brightness sources combine to exactly 2.5·log₁₀(2) ≈ 0.7526 mag
// BRIGHTER (a factor-2 flux increase) — NOT the average of their magnitudes.
func TestLinearSpaceInvariant(t *testing.T) {
	t.Parallel()

	const v = 21.0

	sb := skybrightness.SurfaceBrightnessV(v)
	b := sb.Nanolamberts()

	// Sum the two equal radiances in LINEAR space, then convert back.
	combined := b.Add(b).SurfaceBrightnessV()

	want := v - 2.5*math.Log10(2) // ≈ 20.2474
	testutil.AssertNear(t, "linear doubling", float64(combined), want, 1e-12)

	// Sanity: a (wrong) magnitude average would yield exactly v again. Prove the
	// result is materially different — i.e. we did not average magnitudes.
	if math.Abs(float64(combined)-v) < 0.7 {
		t.Errorf("combined %.6f is too close to the magnitude average %.6f; "+
			"components were not summed in linear flux space", float64(combined), v)
	}
}

// TestMcdM2Anchor is the mandatory units anchor: the natural zenith background
// 0.171168465 mcd/m² maps to 22.00 V mag/arcsec² via m = −2.5·log₁₀(L/1.08e8)
// (lightpollutionmap.info atlas convention).
func TestMcdM2Anchor(t *testing.T) {
	t.Parallel()

	got := float64(skybrightness.SurfaceBrightnessFromMcdM2(0.171168465))
	testutil.AssertNear(t, "natural anchor", got, 22.0, 1e-5)
}

// TestMcdM2RoundTrip verifies mcd/m² → mag → mcd/m² is the identity.
func TestMcdM2RoundTrip(t *testing.T) {
	t.Parallel()

	for _, l := range []float64{0.171168465, 1.0, 6.64, 100.0} {
		got := skybrightness.SurfaceBrightnessFromMcdM2(l).McdM2()
		testutil.AssertRelNear(t, "mcd round-trip", got, l, 1e-12)
	}
}

// TestNanolambertMonotonic verifies fainter (larger) V maps to smaller radiance.
func TestNanolambertMonotonic(t *testing.T) {
	t.Parallel()

	brighter := skybrightness.SurfaceBrightnessV(18.0).Nanolamberts()
	fainter := skybrightness.SurfaceBrightnessV(21.0).Nanolamberts()

	if !(brighter > fainter) {
		t.Errorf("expected brighter sky (V=18) to have larger radiance than V=21: %g vs %g",
			brighter, fainter)
	}
}

// TestZeroRadianceInfinitelyFaint verifies non-positive radiance maps to +Inf mag.
func TestZeroRadianceInfinitelyFaint(t *testing.T) {
	t.Parallel()

	got := float64(skybrightness.Nanolambert(0).SurfaceBrightnessV())
	if !math.IsInf(got, 1) {
		t.Errorf("zero radiance: got %v, want +Inf", got)
	}
}

func BenchmarkNanolambertConversion(b *testing.B) {
	b.ReportAllocs()

	sb := skybrightness.SurfaceBrightnessV(21.0)

	var sink skybrightness.SurfaceBrightnessV
	for range b.N {
		sink = sb.Nanolamberts().SurfaceBrightnessV()
	}

	_ = sink
}
