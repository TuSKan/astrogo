package finite_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/catalog/internal/finite"
)

// Parse refuses exactly what strconv.ParseFloat would wrongly let through,
// and what it would refuse anyway, and nothing else.
func TestParseRefusesWhatIsNotANumber(t *testing.T) {
	t.Parallel()

	for _, s := range []string{
		// ParseFloat accepts every one of these (#1080).
		"NaN", "nan", "+NaN", "Inf", "+Inf", "-Inf", "inf", "Infinity", "-infinity",
		// Overflow parses to an infinity with ErrRange.
		"1e400", "-1e400",
		// And what never parsed.
		"", " ", "abc", "1.2.3", " 1.5",
	} {
		if v, ok := finite.Parse(s); ok {
			t.Errorf("Parse(%q) = %v, true; want false", s, v)
		}
	}

	for s, want := range map[string]float64{
		"0": 0, "-0": 0, "1.5": 1.5, "-23.25": -23.25, "1e-300": 1e-300,
		"359.9999": 359.9999, "4.9e-324": 4.9e-324,
	} {
		if v, ok := finite.Parse(s); !ok || v != want {
			t.Errorf("Parse(%q) = %v, %t; want %v, true", s, v, ok, want)
		}
	}
}

func TestIs(t *testing.T) {
	t.Parallel()

	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if finite.Is(v) {
			t.Errorf("Is(%v) = true", v)
		}
	}

	for _, v := range []float64{0, -1, math.MaxFloat64, math.SmallestNonzeroFloat64} {
		if !finite.Is(v) {
			t.Errorf("Is(%v) = false", v)
		}
	}
}

// A position is finite, and in range, at the edges included.
func TestPosition(t *testing.T) {
	t.Parallel()

	nan, inf := math.NaN(), math.Inf(1)

	for _, c := range []struct {
		ra, dec float64
		want    bool
	}{
		{0, 0, true},
		{359.999, -89.999, true},
		{-180, 45, true},
		{360, 90, true},
		{-360, -90, true},
		{nan, 10, false},
		{10, nan, false},
		{inf, 10, false},
		{10, -inf, false},
		{10, 90.0001, false},
		{10, -91, false},
		{360.5, 0, false},
		{-361, 0, false},
		{4000, 0, false},
	} {
		if got := finite.Position(c.ra, c.dec); got != c.want {
			t.Errorf("Position(%v, %v) = %t, want %t", c.ra, c.dec, got, c.want)
		}
	}
}
