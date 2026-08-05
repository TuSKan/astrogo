package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// radianceForTotalSB inverts SB = a·log₁₀(L) + b for the default
// coefficients, giving the radiance that the fit maps to a chosen TOTAL
// (not artificial-only) sky brightness — the same helper this test used to
// live alongside in skybrightness/atlas/viirs_test.go before A.1 moved
// RadianceToArtificialSB down into this core package.
func radianceForTotalSB(totalSB float64) float64 {
	return math.Pow(10, (totalSB-skybrightness.DefaultRadianceZeroPoint)/skybrightness.DefaultRadianceSlope)
}

// TestRadianceToArtificialSB verifies the log-linear fit is applied and the
// natural background is then subtracted, yielding an artificial-only floor:
// a radiance mapping to a TOTAL SB of 18.0 must give an artificial floor
// equal to SurfaceBrightnessFromMcdM2(totalMcd − naturalMcd), independently
// computed — golden values carried over from the pre-move atlas test.
func TestRadianceToArtificialSB(t *testing.T) {
	t.Parallel()

	const totalSB = 18.0

	rad := radianceForTotalSB(totalSB)

	got := skybrightness.RadianceToArtificialSB(rad, skybrightness.DefaultRadianceSlope, skybrightness.DefaultRadianceZeroPoint)

	totalMcd := skybrightness.SurfaceBrightnessV(totalSB).McdM2()
	want := skybrightness.SurfaceBrightnessFromMcdM2(totalMcd - 0.171168465) // natural zenith background ≡ 22.0 mag/arcsec²

	testutil.AssertNear(t, "artificial SB", float64(got), float64(want), 1e-4)

	// The artificial floor must be fainter (larger mag) than the total SB,
	// since the natural term was removed.
	if !(float64(got) > totalSB) {
		t.Errorf("artificial SB %.3f should be fainter than total %.3f", float64(got), totalSB)
	}
}

// TestRadianceToArtificialSBMonotonic verifies brighter radiance yields a
// brighter (smaller) artificial SB.
func TestRadianceToArtificialSBMonotonic(t *testing.T) {
	t.Parallel()

	prev := math.Inf(1)

	for _, rad := range []float64{0.5, 2, 10, 50, 200} {
		got := float64(skybrightness.RadianceToArtificialSB(rad, skybrightness.DefaultRadianceSlope, skybrightness.DefaultRadianceZeroPoint))
		if got >= prev {
			t.Errorf("radiance %g gave SB %.3f not brighter than previous %.3f", rad, got, prev)
		}

		prev = got
	}
}

// TestRadianceToArtificialSBNoLight verifies non-positive radiance yields an
// infinitely faint (no) artificial floor, and that a radiance dimmer than
// the natural background also contributes nothing.
func TestRadianceToArtificialSBNoLight(t *testing.T) {
	t.Parallel()

	if sb := skybrightness.RadianceToArtificialSB(0, skybrightness.DefaultRadianceSlope, skybrightness.DefaultRadianceZeroPoint); !math.IsInf(float64(sb), 1) {
		t.Errorf("zero radiance: got %v, want +Inf", float64(sb))
	}

	if sb := skybrightness.RadianceToArtificialSB(-5, skybrightness.DefaultRadianceSlope, skybrightness.DefaultRadianceZeroPoint); !math.IsInf(float64(sb), 1) {
		t.Errorf("negative radiance: got %v, want +Inf", float64(sb))
	}

	// A radiance whose total SB is fainter than the 22.0 natural floor ⇒ no
	// artificial excess ⇒ +Inf.
	faint := radianceForTotalSB(23.0)
	if sb := skybrightness.RadianceToArtificialSB(faint, skybrightness.DefaultRadianceSlope, skybrightness.DefaultRadianceZeroPoint); !math.IsInf(float64(sb), 1) {
		t.Errorf("sub-natural radiance: got %v, want +Inf", float64(sb))
	}
}

// TestRadianceToArtificialSBCoefficientOverride verifies a different
// (slope, zeroPoint) pair changes the result — e.g. the DMSP pair from
// Sánchez de Miguel et al. 2020 instead of the ISS-HDR default.
func TestRadianceToArtificialSBCoefficientOverride(t *testing.T) {
	t.Parallel()

	const rad = 5.0

	def := skybrightness.RadianceToArtificialSB(rad, skybrightness.DefaultRadianceSlope, skybrightness.DefaultRadianceZeroPoint)
	alt := skybrightness.RadianceToArtificialSB(rad, -1.40, 20.71) // DMSP pair

	if def == alt {
		t.Errorf("expected different SB under different coefficients: both %.4f", float64(def))
	}
}
