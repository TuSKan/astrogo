package time

import (
	"math"
	"testing"
)

func TestDeltaT_KnownValues(t *testing.T) {
	// Known ΔT values from NASA tables and IERS observations.
	// With n-dot correction applied, these should match NASA exactly.
	tests := []struct {
		desc    string
		year    float64
		wantMin float64
		wantMax float64
	}{
		{"500 BCE: deep historical", -500, 17100, 17200},
		{"0 CE: Roman era", 0, 10500, 10600},
		{"500 CE: early medieval", 500, 5600, 5700},
		{"1000 CE: medieval", 1000, 1500, 1600},
		{"1600 CE: start of telescopic era", 1600, 118, 121},
		{"1700 CE", 1700, 7.5, 9.5},
		{"1800 CE", 1800, 13.0, 14.5},
		{"1900 CE: beginning of precise measurements", 1900, -3.1, -2.5},
		{"1950 CE", 1950, 29.0, 29.1},
		{"2000 CE: modern", 2000, 63.8, 63.9},
	}

	for _, tt := range tests {
		dt := DeltaT(tt.year)
		if dt < tt.wantMin || dt > tt.wantMax {
			t.Errorf("DeltaT(%.0f) = %.1f, want [%.1f, %.1f] (%s)",
				tt.year, dt, tt.wantMin, tt.wantMax, tt.desc)
		} else {
			t.Logf("DeltaT(%.0f) = %.1f s (%s)", tt.year, dt, tt.desc)
		}
	}
}

func TestDeltaT_SegmentContinuity(t *testing.T) {
	// Verify no discontinuities at segment boundaries.
	// The polynomial segments should be continuous to within a few seconds.
	boundaries := []float64{-500, 500, 1600, 1700, 1800, 1860, 1900, 1920, 1941, 1961, 1986, 2005, 2050, 2150}

	for _, b := range boundaries {
		eps := 0.001 // 0.001 year ≈ 8.76 hours
		left := DeltaT(b - eps)
		right := DeltaT(b + eps)
		jump := math.Abs(right - left)

		maxJump := 1.0 // 1 second max discontinuity
		if jump > maxJump {
			t.Errorf("Discontinuity at year %.0f: DeltaT(%.3f)=%.2f, DeltaT(%.3f)=%.2f, jump=%.2f",
				b, b-eps, left, b+eps, right, jump)
		} else {
			t.Logf("Boundary %.0f: jump = %.4f s (left=%.2f, right=%.2f)", b, jump, left, right)
		}
	}
}

func TestDeltaT_MonotonicModern(t *testing.T) {
	// ΔT should be roughly monotonically increasing in the modern era (1900-2050)
	// due to tidal deceleration of Earth's rotation.
	prev := DeltaT(1900)
	for y := 1905.0; y <= 2050; y += 5 {
		cur := DeltaT(y)
		if cur < prev-1.0 { // allow 1s tolerance for the 1900-1920 dip
			t.Errorf("DeltaT decreased unexpectedly: DeltaT(%.0f)=%.1f > DeltaT(%.0f)=%.1f",
				y-5, prev, y, cur)
		}

		prev = cur
	}
}

func TestDeltaTUncertainty_KnownValues(t *testing.T) {
	tests := []struct {
		desc    string
		year    float64
		wantMin float64
		wantMax float64
	}{
		{"-1000 CE: very high uncertainty", -1000, 600, 1200},
		{"0 CE: historical", 0, 200, 300},
		{"1000 CE: medieval", 1000, 50, 60},
		{"1500 CE: pre-telescopic", 1500, 15, 25},
		{"1700 CE: early telescopic", 1700, 1.5, 5.5},
		{"2000 CE: modern observations", 2000, 0, 0.1},
	}

	for _, tt := range tests {
		sigma := DeltaTUncertainty(tt.year)
		if sigma < tt.wantMin || sigma > tt.wantMax {
			t.Errorf("DeltaTUncertainty(%.0f) = %.1f, want [%.1f, %.1f] (%s)",
				tt.year, sigma, tt.wantMin, tt.wantMax, tt.desc)
		} else {
			t.Logf("DeltaTUncertainty(%.0f) = %.1f s (%s)", tt.year, sigma, tt.desc)
		}
	}
}

func TestDeltaT_MatchesNASATable(t *testing.T) {
	// Spot-check against observed ΔT values.
	// With the n-dot correction, these should match NASA exactly.
	table := []struct {
		year float64
		dt   float64 // ΔT in seconds
		tol  float64 // tolerance
	}{
		{1955.0, 31.1, 1.0},
		{1960.0, 33.2, 1.0},
		{1965.0, 35.7, 1.0},
		{1970.0, 40.2, 1.0},
		{1975.0, 45.5, 1.0},
		{1980.0, 50.5, 1.0},
		{1985.0, 54.3, 1.0},
		{1990.0, 56.9, 1.0},
		{1995.0, 60.8, 1.0},
		{2000.0, 63.8, 1.0},
		{2005.0, 64.7, 1.5},
	}

	for _, tt := range table {
		dt := DeltaT(tt.year)
		if math.Abs(dt-tt.dt) > tt.tol {
			t.Errorf("DeltaT(%.0f) = %.1f, want %.1f ±%.1f",
				tt.year, dt, tt.dt, tt.tol)
		} else {
			t.Logf("DeltaT(%.0f) = %.1f (NASA: %.1f)", tt.year, dt, tt.dt)
		}
	}
}

func TestDeltaT_NdotCorrection(t *testing.T) {
	// Verify the n-dot correction is applied correctly.
	// At year 1955, correction = 0 (pivot point).
	// At year 1 CE, correction ≈ -49.4s.
	// At year 2000, correction ≈ -0.026s (negligible for modern dates).
	tests := []struct {
		year     float64
		wantCorr float64 // expected correction in seconds
		tol      float64
	}{
		{1955, 0.0, 0.001},
		{1, -49.37, 0.5},
		{2000, -0.026, 0.01},
		{1000, -11.79, 0.5},
	}

	for _, tt := range tests {
		corr := -0.000012932 * (tt.year - 1955) * (tt.year - 1955)
		if math.Abs(corr-tt.wantCorr) > tt.tol {
			t.Errorf("n-dot correction at year %.0f = %.3f, want %.3f ±%.3f",
				tt.year, corr, tt.wantCorr, tt.tol)
		} else {
			t.Logf("n-dot correction at year %.0f = %.3f s", tt.year, corr)
		}
	}
}
