package constellation_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
)

// TestLookupRefusesANonFinitePosition is #1055. A NaN crosses no boundary, and
// Ursa Minor's boundary encircles the pole, so its inverted parity called
// every such position Ursa Minor, with no error.
func TestLookupRefusesANonFinitePosition(t *testing.T) {
	t.Parallel()

	nan, inf := math.NaN(), math.Inf(1)

	for _, c := range [][2]float64{
		{nan, 10}, {10, nan}, {nan, nan},
		{inf, 10}, {-inf, 10}, {10, inf}, {10, -inf}, {-inf, -inf},
	} {
		name, abbr, err := constellation.Lookup(coord.NewICRS(angle.Deg(c[0]), angle.Deg(c[1])))
		if !errors.Is(err, coord.ErrNotFinite) {
			t.Errorf("RA %g Dec %g: Lookup returned %q (%s) and %v, want coord.ErrNotFinite",
				c[0], c[1], name, abbr, err)
		}

		if name != "" || abbr != "" {
			t.Errorf("RA %g Dec %g: Lookup returned %q (%s) alongside its error", c[0], c[1], name, abbr)
		}
	}
}
