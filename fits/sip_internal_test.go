package fits

import (
	"fmt"
	"maps"
	"math"
	"testing"
)

// TestSIPTermsAreTheKeywordsALookupFinds: reading the coefficients from the
// cards (#796) must find exactly what a lookup of prefix_p_q for every term
// the order allows found, no more and no less. The header holds decoys for
// every way a keyword can look like a term and not be one.
func TestSIPTermsAreTheKeywordsALookupFinds(t *testing.T) {
	t.Parallel()

	h := headerOf(
		"A_ORDER", "3",
		"A_0_2", "1.5E-6",
		"A_2_1", "-2E-9",
		"A_3_0", "0",
		"a_1_1", "4E-7", // lower case: Get upper-cases the keyword, so the lookup found it
		"A_02_0", "9", // a leading zero is a different keyword
		"A_+1_1", "9", // so is a sign
		"A_-1_2", "9",
		"A_1_2_3", "9", // three indices
		"A_3_1", "9", // p + q beyond the order
		"A_1", "9", // one index
		"AP_1_1", "9", // another prefix
		"B_1_1", "9",
		"A__1", "9",
	)

	got, err := parseSIPPoly(h, "A")
	if err != nil {
		t.Fatalf("parseSIPPoly: %v", err)
	}

	// The lookup the cards replaced: every term the order allows, by name.
	want := map[[2]int]float64{}

	for p := range 4 {
		for q := range 4 - p {
			v, _, err := optionalFloat(h, fmt.Sprintf("A_%d_%d", p, q), 0)
			if err != nil {
				t.Fatalf("reference lookup: %v", err)
			}

			if v != 0 {
				want[[2]int{p, q}] = v
			}
		}
	}

	if len(want) != 3 {
		t.Fatalf("precondition: the reference found %d terms, want 3", len(want))
	}

	if !maps.Equal(got, want) {
		t.Errorf("parseSIPPoly = %v, want %v", got, want)
	}
}

// TestIPow: x^n by repeated squaring, for every sign of n, including the most
// negative int, whose negation an int cannot hold.
func TestIPow(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		x    float64
		n    int
		want float64
	}{
		{2, 0, 1},
		{0, 0, 1},
		{2, 1, 2},
		{2, 10, 1024},
		{-3, 3, -27},
		{-3, 4, 81},
		{2, -3, 0.125},
		{0.5, 62, 0x1p-62},
		{1, math.MaxInt, 1},
		{1, math.MinInt, 1},
		{2, math.MinInt, 0},
		{0.5, math.MinInt, math.Inf(1)},
	} {
		if got := ipow(c.x, c.n); got != c.want {
			t.Errorf("ipow(%v, %d) = %v, want %v", c.x, c.n, got, c.want)
		}
	}
}

// TestSIPEvalCostsItsTerms: evaluating a polynomial allocates nothing, and an
// exponent of a hundred million costs what any term costs. The power tables
// this replaced were sized by the largest exponent and allocated on every
// evaluation, so that one term made each pixel cost 800 MB (#796).
//
// Not parallel: testing.AllocsPerRun measures the whole process, and refuses
// to run beside a parallel test.
func TestSIPEvalCostsItsTerms(t *testing.T) {
	poly := newSIPPoly(map[[2]int]float64{
		{2, 0}:         1e-5,
		{1, 1}:         -2e-6,
		{100000000, 0}: 3,
		{-1, 0}:        4, // SetSIP takes any exponent; this was an index out of range
	})

	var got float64

	if allocs := testing.AllocsPerRun(100, func() { got = poly.eval(1, 2) }); allocs != 0 {
		t.Errorf("eval allocates %v times, want 0", allocs)
	}

	// At u = 1 every power of u is 1: 1e-5 - 2e-6·2 + 3 + 4.
	if want := 1e-5 - 4e-6 + 3 + 4; math.Abs(got-want) > 1e-15 {
		t.Errorf("eval(1, 2) = %v, want %v", got, want)
	}
}

// TestSIPInverseEvaluatesBothAtTheUndistortedOffset: AP and BP are functions
// of the same undistorted offset. BP was evaluated at the offset AP had
// already moved (#796): here that gives v + 2u rather than v + u.
func TestSIPInverseEvaluatesBothAtTheUndistortedOffset(t *testing.T) {
	t.Parallel()

	w := NewWCS(2)
	w.SetSIPInverse(map[[2]int]float64{{1, 0}: 1}, map[[2]int]float64{{1, 0}: 1})

	if got, want := w.sipInverse([2]float64{3, 5}), [2]float64{6, 8}; got != want {
		t.Errorf("sipInverse(3, 5) = %v, want %v", got, want)
	}
}
