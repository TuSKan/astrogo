package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/magnitude"
)

// The asteroid phase laws held to sbpy (NASA-Planetary-Science/sbpy,
// sbpy/photometry/tests/test_iau.py), the implementation they were ported
// from (#834): each law's published evaluate() sweep at np.linspace(0, π, 10)
// with r = Δ = 1, and the G₁ and G₂ its H,G₁₂ forms map to. sbpy prints eight
// decimals, so the sweeps are held to 10⁻⁸ mag, twice that rounding; a wrong
// knot value, end derivative or mapping coefficient moves a magnitude by
// orders more.
const sbpyPrinted = 1e-8

// TestAsteroidPhaseLawsMatchSbpy: all four laws at sbpy's ten phase angles,
// which put each spline between its knots, the linear part below 7.5° at 0°,
// and the extrapolation past 150° at 160° and 180°.
func TestAsteroidPhaseLawsMatchSbpy(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		mag  func(alpha angle.Angle) float64
		want [10]float64
	}{
		{
			"H,G (3.34, 0.12)",
			func(a angle.Angle) float64 { return magnitude.AsteroidHG(3.34, 0.12, 1, 1, a) },
			[10]float64{3.34, 4.37957695, 4.97935011, 5.55797517, 6.20811269, 7.03560828, 8.2295693, 10.19445176, 14.29255427, math.Inf(1)},
		},
		{
			"H,G1,G2 (7.063, 0.62, 0.14)",
			func(a angle.Angle) float64 { return magnitude.AsteroidHG1G2(7.063, 0.62, 0.14, 1, 1, a) },
			[10]float64{7.063, 8.07436233, 8.68048572, 9.29834638, 9.96574599, 10.72080704, 11.52317465, 12.15094612, 18.65369516, 18.65389398},
		},
		{
			"H,G12 (7.121, 0.68)",
			func(a angle.Angle) float64 { return magnitude.AsteroidHG12(7.121, 0.68, 1, 1, a) },
			[10]float64{7.121, 8.07252953, 8.67890827, 9.2993879, 9.96817595, 10.72086969, 11.51208664, 12.12722017, 18.70628001, 18.70647883},
		},
		{
			"H,G12* (7.121, 0.68)",
			func(a angle.Angle) float64 { return magnitude.AsteroidHG12Star(7.121, 0.68, 1, 1, a) },
			[10]float64{7.121, 8.12425116, 8.71866169, 9.32576929, 9.98752147, 10.7522335, 11.60154855, 12.28573613, 18.49298496, 18.49318378},
		},
	} {
		for i, want := range c.want {
			alpha := angle.Rad(math.Pi * float64(i) / 9)

			// sbpy's H,G answers +∞ at 180°, where both basis functions are
			// zero; astrogo's floor there is #835.
			if math.IsInf(want, 1) {
				continue
			}

			if got := c.mag(alpha); !(math.Abs(got-want) <= sbpyPrinted) {
				t.Errorf("%s at α = %d°: %.10f, sbpy %.8f", c.name, 20*i, got, want)
			}
		}
	}
}

// TestAsteroidG12MappingsMatchSbpy: H,G₁₂ on both sides of its break at
// G₁₂ = 0.2, and H,G₁₂*, give the magnitudes of H,G₁,G₂ at the G₁ and G₂
// sbpy maps them to. sbpy's values are the mappings' exact decimal results,
// so the magnitudes agree to rounding.
func TestAsteroidG12MappingsMatchSbpy(t *testing.T) {
	t.Parallel()

	const h, r, delta = 7.121, 2.5, 1.5

	for _, c := range []struct {
		name   string
		mag    func(alpha angle.Angle) float64
		g1, g2 float64
	}{
		{"H,G12 = 0.1", func(a angle.Angle) float64 { return magnitude.AsteroidHG12(h, 0.1, r, delta, a) }, 0.13691, 0.53088},
		{"H,G12 = 0.68", func(a angle.Angle) float64 { return magnitude.AsteroidHG12(h, 0.68, r, delta, a) }, 0.669592, 0.1407},
		{"H,G12* = 0.68", func(a angle.Angle) float64 { return magnitude.AsteroidHG12Star(h, 0.68, r, delta, a) }, 0.5731968132, 0.17124272},
	} {
		for deg := 0.0; deg <= 150; deg += 15 {
			got := c.mag(angle.Deg(deg))
			want := magnitude.AsteroidHG1G2(h, c.g1, c.g2, r, delta, angle.Deg(deg))

			if !(math.Abs(got-want) <= 1e-12) {
				t.Errorf("%s at α = %v°: %.15f, H,G1,G2 at sbpy's (%v, %v) %.15f", c.name, deg, got, c.g1, c.g2, want)
			}
		}
	}
}

// TestAsteroidPhaseLawsAreHAtZeroPhase: at opposition every basis function is
// 1, so with r = Δ = 1 each law gives H. H,G's are exactly 1; Φ₁ and Φ₂ of
// H,G₁,G₂ are 1 − 6α/π and 1 − 9α/(5π) below 7.5°, carried from the 7.5°
// knot by slopes Muinonen et al. print to eight digits, so 1 to about 10⁻⁹.
func TestAsteroidPhaseLawsAreHAtZeroPhase(t *testing.T) {
	t.Parallel()

	zero := angle.Deg(0)

	if got := magnitude.AsteroidHG(3.34, 0.12, 1, 1, zero); got != 3.34 {
		t.Errorf("H,G at α = 0: %v, want H = 3.34 exactly", got)
	}

	for _, c := range []struct {
		name string
		got  float64
	}{
		{"H,G1,G2", magnitude.AsteroidHG1G2(7.063, 0.62, 0.14, 1, 1, zero)},
		{"H,G12", magnitude.AsteroidHG12(7.063, 0.1, 1, 1, zero)},
		{"H,G12*", magnitude.AsteroidHG12Star(7.063, 0.68, 1, 1, zero)},
	} {
		if !(math.Abs(c.got-7.063) <= sbpyPrinted) {
			t.Errorf("%s at α = 0: %.10f, want H = 7.063", c.name, c.got)
		}
	}
}
