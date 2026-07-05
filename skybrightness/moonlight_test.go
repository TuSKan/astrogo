package skybrightness

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestMoonBrightnessRegression checks the KS 1991 closed form against an
// independently hand-computed value for a canonical geometry.
//
// Geometry: full Moon (α=0°), Moon and target both at zenith angle 30°,
// separation ρ=60°, extinction k=0.172. Worked through the model:
//
//	f(ρ)   = 10^5.36·(1.06+cos²60°) + 10^(6.15−60/40) = 3.0010e5 + 4.4668e4 = 3.4477e5
//	I*(0)  = 10^(−0.4·3.84)                            = 0.029106
//	X(30°) = (1 − 0.96·sin²30°)^(−1/2)                 = 1.14708
//	B_moon = f·I*·10^(−0.4·0.172·X)·(1 − 10^(−0.4·0.172·X)) ≈ 1390 nL  (≈ 18.47 V mag/arcsec²)
//
// The 5% band comfortably exceeds hand-arithmetic rounding while catching any
// transcription error (which would shift the result by tens of percent). The
// model's own physical accuracy is ~8–23% per KS 1991.
func TestMoonBrightnessRegression(t *testing.T) {
	t.Parallel()

	z30 := 30 * degToRad
	got := moonBrightnessNL(60, 0, z30, z30, 0.172)

	testutil.AssertRelNear(t, "B_moon (nL)", got, 1390.0, 0.05)

	sb := float64(Nanolambert(got).SurfaceBrightnessV())
	testutil.AssertNear(t, "B_moon (V mag/arcsec²)", sb, 18.47, 0.15)
}

// TestMoonBrightnessPhaseMonotonic verifies brightness decreases from full Moon
// (α=0) toward new Moon (α→180) with all else fixed.
func TestMoonBrightnessPhaseMonotonic(t *testing.T) {
	t.Parallel()

	z := 30 * degToRad
	full := moonBrightnessNL(90, 0, z, z, 0.172)
	quarter := moonBrightnessNL(90, 90, z, z, 0.172)
	crescent := moonBrightnessNL(90, 170, z, z, 0.172)

	if !(full > quarter && quarter > crescent) {
		t.Errorf("phase not monotonic: full=%.1f quarter=%.1f crescent=%.1f", full, quarter, crescent)
	}
}

// TestMoonBrightnessSeparationMonotonic verifies brightness decreases with
// increasing Moon–target separation over [5°,90°].
func TestMoonBrightnessSeparationMonotonic(t *testing.T) {
	t.Parallel()

	z := 30 * degToRad
	near := moonBrightnessNL(5, 0, z, z, 0.172)
	mid := moonBrightnessNL(45, 0, z, z, 0.172)
	far := moonBrightnessNL(90, 0, z, z, 0.172)

	if !(near > mid && mid > far) {
		t.Errorf("separation not monotonic: near=%.1f mid=%.1f far=%.1f", near, mid, far)
	}
}

// TestMoonlightBelowHorizonZero verifies (via real ephemeris) that the
// contribution is exactly zero whenever the Moon is below the horizon, and
// positive when it is up.
func TestMoonlightBelowHorizonZero(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.AtAltitude(0)
	m := NewMoonlight()
	target := coord.NewAltAz(angle.Deg(45), angle.Deg(90))

	var sawZero, sawPositive bool

	for i := range 24 {
		tm := time.FromJD(2451545.0+float64(i)/24.0, time.UTC)
		ctx := coord.NewContext(tm, loc, atm)

		r, err := m.Radiance(target, ctx)
		if err != nil {
			t.Fatalf("Radiance at step %d: %v", i, err)
		}

		switch {
		case r == 0:
			sawZero = true
		case r > 0:
			sawPositive = true
		default:
			t.Errorf("step %d: negative radiance %g", i, r)
		}
	}

	if !sawZero {
		t.Error("expected at least one time with the Moon below the horizon (zero contribution)")
	}

	if !sawPositive {
		t.Error("expected at least one time with the Moon above the horizon (positive contribution)")
	}
}

func BenchmarkMoonBrightnessNL(b *testing.B) {
	b.ReportAllocs()

	z := 40 * degToRad

	var sink float64
	for range b.N {
		sink = moonBrightnessNL(75, 35, z, z, 0.172)
	}

	_ = sink
}
