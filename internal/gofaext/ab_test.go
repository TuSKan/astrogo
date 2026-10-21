package gofaext_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestAbIsSOFAs holds the wrapper to SOFA's own test vector for iauAb (t_sofa_c's
// t_ab, as gofa's test/astrometry_test.go ports it), to SOFA's tolerance. The
// wrapper passes its arguments in iauAb's order; swapping s and bm1, both near
// one, would move the answer by parts in 10⁻⁴, far past the tolerance.
func TestAbIsSOFAs(t *testing.T) {
	t.Parallel()

	pnat := [3]float64{-0.76321968546737951, -0.60869453983060384, -0.21676408580639883}
	v := [3]float64{2.1044018893653786e-5, -8.9108923304429319e-5, -3.8633714797716569e-5}

	got := gofaext.Ab(pnat, v, 0.99980921395708788, 0.99999999506209258)
	want := [3]float64{-0.7631631094219556269, -0.6087553082505590832, -0.2167926269368471279}

	for i := range 3 {
		if math.Abs(got[i]-want[i]) > 1e-12 {
			t.Errorf("Ab[%d] = %.19f, SOFA's t_ab gives %.19f", i, got[i], want[i])
		}
	}
}
