package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// The budget weights each component's uncertainty by its share of the sky.
//
// # Why weighting is the whole point
//
// A component contributing one per cent of the light can be 50 per cent wrong
// and barely move the answer; one contributing most of it cannot. An
// unweighted average of the relative uncertainties would say the opposite,
// and would say it in a plausible-looking number.
//
// The arithmetic here is worked by hand rather than restated from the
// implementation: with airglow at three quarters of the light and 20 per cent
// uncertain, and starlight at one quarter and 40 per cent uncertain, the
// contributions are 0.15 and 0.10, which combine in quadrature to 0.1803.
func TestUncertaintyBudgetWeightsByShare(t *testing.T) {
	t.Parallel()

	var b skybrightness.UncertaintyBudget

	b.Add(skybrightness.Uncertainty{
		Component: skybrightness.AirglowContinuum, Relative: 0.20, Source: "airglow variability",
	})
	b.Add(skybrightness.Uncertainty{
		Component: skybrightness.Starlight, Relative: 0.40, Source: "map zero point",
	})

	weights := map[skybrightness.ComponentID]unit.Radiance{
		skybrightness.AirglowContinuum: 3,
		skybrightness.Starlight:        1,
	}

	got := b.Total(weights)
	want := math.Hypot(0.75*0.20, 0.25*0.40)

	if math.Abs(got-want) > 1e-12 {
		t.Errorf("total is %.6f, want %.6f", got, want)
	}

	// The larger contribution wins even though its relative uncertainty is
	// the smaller of the two — which is exactly the judgement the budget
	// exists to make.
	dom, share := b.Dominant(weights)

	if dom.Component != skybrightness.AirglowContinuum {
		t.Errorf("dominant is %s, want airglow: 0.75*0.20 beats 0.25*0.40", dom.Component)
	}

	if math.Abs(share-0.15) > 1e-12 {
		t.Errorf("dominant share is %.6f, want 0.15", share)
	}

	if dom.Source != "airglow variability" {
		t.Errorf("dominant source is %q; the budget must carry what to go and measure",
			dom.Source)
	}
}

// Correlated terms add linearly, which is the conservative answer.
//
// Two components sharing an aerosol assumption are correlated through it, and
// quadrature would report an optimistic total. Linear is larger for any two
// positive terms, so this also checks the flag is read at all.
func TestUncertaintyBudgetCorrelatedAddsLinearly(t *testing.T) {
	t.Parallel()

	build := func(correlated bool) *skybrightness.UncertaintyBudget {
		b := &skybrightness.UncertaintyBudget{Correlated: correlated}
		b.Add(skybrightness.Uncertainty{Component: skybrightness.Zodiacal, Relative: 0.3})
		b.Add(skybrightness.Uncertainty{Component: skybrightness.Starlight, Relative: 0.3})

		return b
	}

	weights := map[skybrightness.ComponentID]unit.Radiance{
		skybrightness.Zodiacal: 1, skybrightness.Starlight: 1,
	}

	independent := build(false).Total(weights)
	correlated := build(true).Total(weights)

	if want := 0.3; math.Abs(correlated-want) > 1e-12 {
		t.Errorf("correlated total is %.6f, want %.6f (0.15 + 0.15)", correlated, want)
	}

	if want := math.Hypot(0.15, 0.15); math.Abs(independent-want) > 1e-12 {
		t.Errorf("independent total is %.6f, want %.6f", independent, want)
	}

	if correlated <= independent {
		t.Errorf("correlated %.6f is no larger than independent %.6f; adding in phase is the "+
			"conservative choice and must not report the smaller number",
			correlated, independent)
	}
}

// A budget with no light in it returns zero rather than dividing by it.
//
// Weights sum to zero whenever every component is dark — below the horizon,
// or a spectrum that has not been filled — and a share of nothing is not a
// number.
func TestUncertaintyBudgetHandlesNoLight(t *testing.T) {
	t.Parallel()

	var b skybrightness.UncertaintyBudget
	b.Add(skybrightness.Uncertainty{Component: skybrightness.Starlight, Relative: 0.4})

	for _, weights := range []map[skybrightness.ComponentID]unit.Radiance{
		{},
		{skybrightness.Starlight: 0},
	} {
		if got := b.Total(weights); got != 0 {
			t.Errorf("total with no light is %g, want 0", got)
		}

		if dom, share := b.Dominant(weights); share != 0 || dom.Component != "" {
			t.Errorf("dominant with no light is %v at %g, want the zero value", dom, share)
		}
	}
}

// Zero, Scale and Clone do what a hot loop needs them to.
//
// Zero and Scale exist to keep an all-sky loop allocation-free, so they must
// act in place; Clone exists to hand a caller a spectrum they can keep, so it
// must not. Getting either backwards produces a sky that is subtly wrong in a
// way no single-direction test would show: the previous direction's radiance
// leaking into the next one.
func TestSpectralRadianceInPlaceOperations(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 100, 5)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	s := skybrightness.NewSpectralRadiance(grid)
	for i := range s {
		s[i] = float64(i + 1)
	}

	clone := s.Clone()

	s.Scale(2)

	for i := range s {
		if want := float64(i+1) * 2; s[i] != want {
			t.Errorf("after Scale sample %d is %g, want %g", i, s[i], want)
		}

		if clone[i] != float64(i+1) {
			t.Errorf("Scale reached into the clone: sample %d is %g, want %g",
				i, clone[i], float64(i+1))
		}
	}

	s.Zero()

	for i, v := range s {
		if v != 0 {
			t.Errorf("after Zero sample %d is %g, want 0", i, v)
		}
	}

	// Zero keeps the allocation, which is the reason it exists at all.
	if len(s) != len(clone) {
		t.Errorf("Zero changed the length to %d, want %d — it must reset in place",
			len(s), len(clone))
	}
}
