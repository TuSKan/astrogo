package refraction

import (
	"math"
	"testing"
)

// iauRefco's constants at 1013.25 hPa, 15 °C, 50% humidity and 0.55 µm, as
// gofa computes them, to seven figures; fixed here so this package's tests
// need no SOFA.
const (
	testRefa = 2.771954e-4
	testRefb = -3.171695e-7
	testP    = 1013.25
	testT    = 15.0
	testWL   = 0.55
)

// TestFromTrueIsAtioqAboveTheHandOver: wherever the observed altitude is at
// or above 10°, FromTrue is SOFAFromTrue to the last bit, so nothing above the
// hand-over moves.
func TestFromTrueIsAtioqAboveTheHandOver(t *testing.T) {
	for h := 9.95; h <= 90; h += 0.05 {
		alt := h * deg

		sofa := SOFAFromTrue(alt, testRefa, testRefb)
		if (alt+sofa)/deg < handoverHigh {
			continue
		}

		if got := FromTrue(alt, testRefa, testRefb, testP, testT, testWL); got != sofa {
			t.Fatalf("true %.2f°: %.17g, iauAtioq's %.17g", h, got, sofa)
		}
	}
}

// TestFromTrueInvertsFromApparent: below 5° of observed altitude the forward
// direction is the inverse of the reverse one, to a few microarcseconds.
// Above, it passes to iauAtioq's own forward series, and the round trip is
// as good as SOFA's, whose two directions are 0.018″ apart at 10°: measured,
// 0.031″ at worst, at a true altitude of 7.8°.
func TestFromTrueInvertsFromApparent(t *testing.T) {
	for h := -10.0; h < 12; h += 0.01 {
		alt := h * deg

		r := FromTrue(alt, testRefa, testRefb, testP, testT, testWL)
		back := FromApparent(alt+r, testRefa, testRefb, testP, testT, testWL)

		tolerance := 1e-5 // arcseconds
		if (alt+r)/deg > handoverLow {
			tolerance = 0.035
		}

		if d := (back - r) / deg * 3600; math.Abs(d) > tolerance {
			t.Fatalf("true %.2f°: %.6f″ forward, %.6f″ back, beyond %g″", h, r/deg*3600, back/deg*3600, tolerance)
		}
	}
}

// TestFromTrueIsContinuous: the forward direction has no step anywhere, in
// particular where it passes from the inverse to iauAtioq's series at 10° of
// observed altitude, where the two differ by 0.019″. Sampled every 0.0001°,
// the second difference of a smooth curve is its bend times the step
// squared, about 1e-5″ here; a step would show as itself.
func TestFromTrueIsContinuous(t *testing.T) {
	const s = 1e-4 // degrees

	r := func(h float64) float64 { return FromTrue(h*deg, testRefa, testRefb, testP, testT, testWL) / deg * 3600 }

	for h := -1.5; h <= 15; h += s {
		if d := math.Abs(r(h+s) - 2*r(h) + r(h-s)); d > 1e-3 {
			t.Fatalf("true %.4f°: the refraction's second difference is %.2e″, a step", h, d)
		}
	}
}

// TestRadioKeepsSOFAsSeries: Bennett-NA is an optical fit, so at a radio
// wavelength, where iauRefco switches to its radio refractivity, there is no
// hand-over.
func TestRadioKeepsSOFAsSeries(t *testing.T) {
	const radio = 21e4 // micrometers: the 21 cm line

	for _, h := range []float64{-1, 0, 2, 7} {
		alt := h * deg

		if got, want := FromApparent(alt, testRefa, testRefb, testP, testT, radio), SOFAFromApparent(alt, testRefa, testRefb); got != want {
			t.Errorf("apparent %g° at %g µm: %g, SOFA's series %g", h, radio, got, want)
		}

		if got, want := FromTrue(alt, testRefa, testRefb, testP, testT, radio), SOFAFromTrue(alt, testRefa, testRefb); got != want {
			t.Errorf("true %g° at %g µm: %g, SOFA's series %g", h, radio, got, want)
		}
	}
}

// TestHandoverWeight pins the weight's ends and its smoothness: 1 at and
// above 10°, 0 at and below 5°, and flat at both ends, so the slope of the
// blend is continuous.
func TestHandoverWeight(t *testing.T) {
	for _, tc := range []struct{ h, want float64 }{
		{-5, 0}, {5, 0}, {7.5, 0.5}, {10, 1}, {45, 1},
	} {
		if got := handoverWeight(tc.h); got != tc.want {
			t.Errorf("handoverWeight(%g) = %g, want %g", tc.h, got, tc.want)
		}
	}

	const e = 1e-6
	for _, end := range []float64{handoverLow, handoverHigh} {
		if slope := (handoverWeight(end+e) - handoverWeight(end-e)) / (2 * e); math.Abs(slope) > 1e-5 {
			t.Errorf("at %g°: the weight's slope is %g, want 0", end, slope)
		}
	}
}

// TestBennettTurnover is where the formula's argument is least: √7.32 − 4.32.
func TestBennettTurnover(t *testing.T) {
	arg := func(h float64) float64 { return h + bennettA/(h+bennettB) }

	if a, l, r := arg(bennettTurnover), arg(bennettTurnover-1e-4), arg(bennettTurnover+1e-4); a > l || a > r {
		t.Errorf("the argument at the turnover, %.9f, is not below its neighbors %.9f and %.9f", a, l, r)
	}

	if math.Abs(bennettTurnover-(-1.61445)) > 1e-5 {
		t.Errorf("turnover at %.5f°, want -1.61445°", bennettTurnover)
	}
}

// TestBennettEdges: no air refracts nothing; a line of sight below the foot
// of the taper is not refracted, either way; the zenith is not refracted; an
// unstated wavelength is the formula's own; and the forward direction inverts
// the reverse one exactly.
func TestBennettEdges(t *testing.T) {
	if got := Bennett(0, 0, testT, testWL); got != 0 {
		t.Errorf("Bennett with no pressure: %g, want 0", got)
	}

	if got := BennettFromTrue(0, 0, testT, testWL); got != 0 {
		t.Errorf("BennettFromTrue with no pressure: %g, want 0", got)
	}

	foot := bennettZero(testP, testT, testWL)

	for _, alt := range []float64{foot - 1e-6, -10 * deg, -90 * deg} {
		if got := Bennett(alt, testP, testT, testWL); got != 0 {
			t.Errorf("Bennett at %.4f°, below the taper's foot at %.4f°: %g, want 0", alt/deg, foot/deg, got)
		}

		if got := BennettFromTrue(alt, testP, testT, testWL); got != 0 {
			t.Errorf("BennettFromTrue at %.4f°: %g, want 0", alt/deg, got)
		}
	}

	if got := Bennett(90*deg, testP, testT, testWL); got != 0 {
		t.Errorf("Bennett at the zenith: %g, want 0", got)
	}

	if got := dispersion(0); got != 1 {
		t.Errorf("dispersion with no wavelength: %g, want 1", got)
	}

	for h := -4.0; h <= 89.0; h += 0.5 {
		alt := h * deg

		r := BennettFromTrue(alt, testP, testT, testWL)
		if back := Bennett(alt+r, testP, testT, testWL); math.Abs(back-r)/deg*3600 > 1e-6 {
			t.Errorf("true %g°: %.9f″ forward, %.9f″ back", h, r/deg*3600, back/deg*3600)
		}
	}
}
