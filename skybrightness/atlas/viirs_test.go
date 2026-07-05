package atlas

import (
	"bytes"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// radianceForTotalSB inverts SB = a·log₁₀(L) + b for the default coefficients,
// giving the radiance that the fit maps to a chosen TOTAL sky brightness.
func radianceForTotalSB(totalSB float64) float64 {
	return math.Pow(10, (totalSB-viirsZeroPoint)/viirsSlope)
}

// TestVIIRSArtificialOnly verifies the provider applies the cited log-linear fit
// and then subtracts the natural background, yielding an artificial-only floor.
//
// A radiance mapping to a TOTAL SB of 18.0 must give an artificial floor equal
// to SurfaceBrightnessFromMcdM2(totalMcd − naturalMcd), independently computed.
func TestVIIRSArtificialOnly(t *testing.T) {
	t.Parallel()

	const totalSB = 18.0

	rad := radianceForTotalSB(totalSB)

	s := synthTIFF{
		width: 2, height: 2,
		pixels:    []float32{float32(rad), float32(rad), float32(rad), float32(rad)},
		originLon: -46, originLat: -23, pxSize: 0.5,
	}

	p, err := NewVIIRSProvider(bytes.NewReader(s.build(t)))
	if err != nil {
		t.Fatalf("NewVIIRSProvider: %v", err)
	}

	lon, lat := s.centerLonLat(0, 0)

	got, err := p.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("ZenithBrightness: %v", err)
	}

	totalMcd := skybrightness.SurfaceBrightnessV(totalSB).McdM2()
	want := skybrightness.SurfaceBrightnessFromMcdM2(totalMcd - viirsNaturalMcdM2)

	testutil.AssertNear(t, "artificial SB", float64(got), float64(want), 1e-4)

	// The artificial floor must be fainter (larger mag) than the total SB, since
	// the natural term was removed.
	if !(float64(got) > totalSB) {
		t.Errorf("artificial SB %.3f should be fainter than total %.3f", float64(got), totalSB)
	}
}

// TestVIIRSMonotonic verifies brighter radiance ⇒ brighter (smaller) artificial SB.
func TestVIIRSMonotonic(t *testing.T) {
	t.Parallel()

	prev := math.Inf(1)

	for _, rad := range []float64{0.5, 2, 10, 50, 200} {
		got := float64(radianceToArtificialSB(rad, viirsSlope, viirsZeroPoint))
		if got >= prev {
			t.Errorf("radiance %g gave SB %.3f not brighter than previous %.3f", rad, got, prev)
		}

		prev = got
	}
}

// TestVIIRSNoLight verifies that non-positive radiance yields an infinitely
// faint (no) artificial floor, and that a radiance dimmer than the natural
// background also contributes nothing.
func TestVIIRSNoLight(t *testing.T) {
	t.Parallel()

	if sb := radianceToArtificialSB(0, viirsSlope, viirsZeroPoint); !math.IsInf(float64(sb), 1) {
		t.Errorf("zero radiance: got %v, want +Inf", float64(sb))
	}

	// A radiance whose total SB is fainter than the 22.0 natural floor ⇒ no
	// artificial excess ⇒ +Inf.
	faint := radianceForTotalSB(23.0)
	if sb := radianceToArtificialSB(faint, viirsSlope, viirsZeroPoint); !math.IsInf(float64(sb), 1) {
		t.Errorf("sub-natural radiance: got %v, want +Inf", float64(sb))
	}
}

// TestVIIRSCoefficientOverride verifies WithVIIRSCoefficients changes the fit.
func TestVIIRSCoefficientOverride(t *testing.T) {
	t.Parallel()

	s := synthTIFF{width: 2, height: 2, pixels: rampPixels(2, 2, 5), originLon: 0, originLat: 0, pxSize: 1}
	raw := s.build(t)
	lon, lat := s.centerLonLat(0, 0)

	def, _ := NewVIIRSProvider(bytes.NewReader(raw))
	alt, _ := NewVIIRSProvider(bytes.NewReader(raw), WithVIIRSCoefficients(-1.40, 20.71)) // DMSP pair

	a, err := def.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("default: %v", err)
	}

	b, err := alt.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("override: %v", err)
	}

	if a == b {
		t.Errorf("expected different SB under different coefficients: both %.4f", float64(a))
	}
}

// TestVIIRSDNBCalibration is a placeholder pinning the unresolved VIIRS-DNB
// recalibration (see the TODO(verify) in viirs.go). Sánchez de Miguel et al.
// 2020 publish DMSP and ISS coefficient pairs but no DNB pair; until a
// DNB-calibrated (a,b) with a ground-truth anchor is sourced, this test is
// skipped rather than asserting against the ISS stand-in.
func TestVIIRSDNBCalibration(t *testing.T) {
	t.Skip("TODO(verify): no published VIIRS-DNB radiance→SQM coefficients; ISS pair used as documented stand-in")
}
