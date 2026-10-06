package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/magnitude"
)

// TestCometMagnitudesAgreeWithHorizons holds CometApparent and
// CometNuclearApparent to JPL Horizons' T-mag and N-mag, which are computed
// from the same SBDB parameters these take.
//
// The rows are Horizons observer ephemerides, CENTER='500@399',
// QUANTITIES='9,19,20,24' (T-mag, N-mag, r, delta, S-T-O), queried
// 2026-10-06 for 2026-10-01 to 03. Each comet's M1, k1, M2, k2 and PHCOF are
// as the ephemeris header printed them, and r, delta and the phase angle as
// its columns did. Horizons prints magnitudes to three decimals, so 1e-3 is
// its own rounding.
//
// 13P/Olbers is the case that matters: it carries a nuclear phase
// coefficient, and until #548 the nuclear magnitude had no phase term and came
// out 0.21 mag bright. 12P/Pons-Brooks carries none, which Horizons treats as
// zero. 2P/Encke has no nuclear parameters at all and checks the total
// magnitude alone.
func TestCometMagnitudesAgreeWithHorizons(t *testing.T) {
	t.Parallel()

	const tol = 1e-3

	for _, c := range []struct {
		name               string
		m1, k1, m2, k2, pc float64
		r, delta, betaDeg  float64
		tmag, nmag         float64 // NaN where Horizons prints n.a.
	}{
		{"13P 2026-10-01", 6.7, 18.5, 11.3, 5, 0.030, 8.046498471303, 7.99766505284555, 7.1473, 27.969, 20.557},
		{"13P 2026-10-02", 6.7, 18.5, 11.3, 5, 0.030, 8.053258512071, 8.02046892277455, 7.1369, 27.981, 20.565},
		{"13P 2026-10-03", 6.7, 18.5, 11.3, 5, 0.030, 8.060015282830, 8.04326414614158, 7.1247, 27.994, 20.573},
		{"12P 2026-10-01", 5, 15, 11, 10, 0, 8.850833107918, 9.30587418761089, 5.6361, 24.049, 25.314},
		{"12P 2026-10-02", 5, 15, 11, 10, 0, 8.857462725984, 9.32682021994447, 5.5787, 24.058, 25.322},
		{"2P 2026-10-01", 15.7, 4.5, 0, 0, 0, 2.151500241973, 1.17694878066333, 8.2940, 17.551, math.NaN()},
	} {
		if got := magnitude.CometApparent(c.m1, c.k1, c.r, c.delta); math.Abs(got-c.tmag) > tol {
			t.Errorf("%s: total magnitude %.4f, Horizons T-mag %.3f", c.name, got, c.tmag)
		}

		if math.IsNaN(c.nmag) {
			continue
		}

		got := magnitude.CometNuclearApparent(c.m2, c.k2, c.pc, c.r, c.delta, angle.Deg(c.betaDeg))
		if math.Abs(got-c.nmag) > tol {
			t.Errorf("%s: nuclear magnitude %.4f, Horizons N-mag %.3f", c.name, got, c.nmag)
		}
	}
}
