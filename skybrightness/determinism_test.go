package skybrightness_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// The tests in this file ask one question repeatedly and require the same
// answer to the bit.
//
// The defect they catch (#1066) was a sum taken in Go's map iteration order,
// which the runtime randomizes on every range statement. Whether a given
// reordering changes the last bit depends on the values, so each test repeats
// enough times that, at the rate measured on the unfixed code, it misses the
// defect with a probability below one in ten million. The rates were measured
// over 400 repeats of each question.

// The same question asked of the same model gives the same answer.
//
// At Reference fidelity the scattered-in light is added to the estimate's
// total component by component. That loop ranged over a map, and on the
// unfixed code 348 of 400 repeats of this question returned a spectrum
// differing from the first. At that rate eight repeats miss it with a
// probability of 0.13^8, about 1e-7; each costs a Reference evaluation, which
// is why there are not more.
func TestARepeatedEstimateIsBitIdentical(t *testing.T) {
	t.Parallel()

	const repeats = 8

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSFull, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	q := skybrightness.Query{
		Scene:    presetGoldenScene(t, skybrightness.GAMBONSFull),
		Grid:     in.Grid,
		Fidelity: skybrightness.Reference,
	}

	estimate := func() *skybrightness.Estimate {
		t.Helper()

		est, err := model.Direction(t.Context(), q, angle.Deg(30), angle.Deg(120))
		if err != nil {
			t.Fatalf("Direction: %v", err)
		}

		return est
	}

	want := estimate().SpectralRadiance()

	for n := range repeats {
		got := estimate().SpectralRadiance()

		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("evaluation %d differs from the first at slot %d: %.17g against %.17g",
					n+2, i, got[i], want[i])
			}
		}
	}
}

// Composition and ComponentIDs answer the same way every time they are asked
// of one estimate, and ComponentIDs answers in sorted order.
//
// Composition summed its total in ComponentIDs' order, which was map order.
// On the unfixed code 37 of 400 repeats differed from the first, so 200
// repeats miss it with a probability of about 0.91^200, or 6e-9.
func TestCompositionIsTheSameEveryTime(t *testing.T) {
	t.Parallel()

	const repeats = 200

	in := presetInputs(t)

	model, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, in)
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	est, err := model.Zenith(t.Context(), skybrightness.Query{
		Scene:    presetGoldenScene(t, skybrightness.GAMBONSWeb),
		Grid:     in.Grid,
		Fidelity: skybrightness.Standard,
	})
	if err != nil {
		t.Fatalf("Zenith: %v", err)
	}

	wantIDs := est.ComponentIDs()
	if len(wantIDs) < 3 || !slices.IsSorted(wantIDs) {
		t.Fatalf("ComponentIDs is %v; it must list every contributing term, in sorted order", wantIDs)
	}

	want, err := est.Composition(in.Band, magnitude.Vega)
	if err != nil {
		t.Fatalf("Composition: %v", err)
	}

	for n := range repeats {
		if ids := est.ComponentIDs(); !slices.Equal(ids, wantIDs) {
			t.Fatalf("call %d: ComponentIDs is %v, and the first call gave %v", n+2, ids, wantIDs)
		}

		got, err := est.Composition(in.Band, magnitude.Vega)
		if err != nil {
			t.Fatalf("Composition: %v", err)
		}

		if !slices.Equal(got, want) {
			t.Fatalf("call %d: the composition is\n%v\nand the first call gave\n%v", n+2, got, want)
		}
	}
}

// The budget's total and dominant term are the same every time.
//
// Both divide by the sum of the weights, which they took in map order. With
// these weights, which round differently in different orders as 0.1, 0.2 and
// 0.3 famously do, the share Dominant reports differed from the first in 242
// of 400 calls on the unfixed code, and 200 repeats cannot plausibly miss
// that. Total takes the same sum but was not seen to differ in 400 calls: its
// square root, or its sum of shares in the correlated case, absorbed the
// one-unit change in the last place every time. It is checked here because
// it uses the same total, not because this test is known to catch it.
func TestUncertaintyBudgetIsTheSameEveryTime(t *testing.T) {
	t.Parallel()

	const repeats = 200

	weights := map[skybrightness.ComponentID]unit.Radiance{
		skybrightness.AirglowContinuum: 0.1,
		skybrightness.Zodiacal:         0.2,
		skybrightness.Starlight:        0.3,
		skybrightness.DiffuseGalactic:  0.7,
		skybrightness.Extragalactic:    1.1,
		skybrightness.Artificial:       1.3,
		skybrightness.Moonlight:        1.7,
	}

	var budget skybrightness.UncertaintyBudget
	for _, id := range slices.Sorted(maps.Keys(weights)) {
		budget.Add(skybrightness.Uncertainty{Component: id, Relative: 0.3})
	}

	wantTotal := budget.Total(weights)
	wantDominant, wantShare := budget.Dominant(weights)

	for n := range repeats {
		if got := budget.Total(weights); got != wantTotal {
			t.Fatalf("call %d: Total is %.17g, and the first call gave %.17g", n+2, got, wantTotal)
		}

		if got, share := budget.Dominant(weights); got != wantDominant || share != wantShare {
			t.Fatalf("call %d: Dominant is %v at %.17g, and the first call gave %v at %.17g",
				n+2, got, share, wantDominant, wantShare)
		}
	}
}
