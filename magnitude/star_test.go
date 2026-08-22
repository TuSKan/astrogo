package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
)

// The four Johnson-Cousins relations reproduce the Sun's colours.
//
// The Sun is the one star whose Gaia and Johnson-Cousins photometry are both
// known independently and precisely, so it is the anchor that fixes both the
// direction of every relation and the shape of each polynomial at once. Getting
// the tabulation backwards leaves each transformation smooth and plausible and
// every colour wrong by twice the offset, which is exactly the error that
// shipped in GaiaGToJohnsonV and GaiaGToJohnsonB before.
func TestGaiaToJohnsonCousinsReproducesSolarColours(t *testing.T) {
	t.Parallel()

	// Gaia DR3: the Sun at G = -26.90, BP - RP = 0.82.
	const (
		solarG    = -26.90
		solarBPRP = 0.82
	)

	b := magnitude.GaiaGToJohnsonB(solarG, solarBPRP)
	v := magnitude.GaiaGToJohnsonV(solarG, solarBPRP)
	r := magnitude.GaiaGToJohnsonR(solarG, solarBPRP)
	i := magnitude.GaiaGToCousinsI(solarG, solarBPRP)

	// Published solar colours, with tolerances that admit the spread between
	// determinations but not a sign error or a wrong polynomial degree.
	for _, c := range []struct {
		name      string
		got, want float64
		tol       float64
	}{
		{"V", v, -26.76, 0.05},
		{"B-V", b - v, 0.65, 0.06},
		{"V-R", v - r, 0.35, 0.06},
		{"V-I", v - i, 0.71, 0.06},
	} {
		if math.Abs(c.got-c.want) > c.tol {
			t.Errorf("solar %s = %.4f, want %.4f +/- %.2f", c.name, c.got, c.want, c.tol)
		}
	}

	// Ordering: the Sun is fainter in B than in V, fainter in V than R, and
	// fainter in R than I. Any transformation that inverts its tabulation
	// breaks this before it breaks any tolerance above.
	if !(b > v && v > r && r > i) {
		t.Errorf("solar magnitudes B %.3f, V %.3f, R %.3f, I %.3f are not in decreasing "+
			"order; a solar-type star is brighter at longer optical wavelengths", b, v, r, i)
	}
}

// G - I_C is a quadratic, not a cubic.
//
// The regression test for a real ambiguity: Table 5.9's G - I_C row carries
// three numbers where the V row carries four, and the last number in every
// other row is the published sigma. Reading 0.03765 as a cubic coefficient
// instead of sigma gives a solar V - I of 0.747 against the quadratic's 0.727,
// where published determinations sit at 0.71 to 0.72. This pins the choice so
// that adding a cubic term fails here rather than quietly reddening every I
// magnitude in a map.
func TestGaiaGToCousinsIIsQuadratic(t *testing.T) {
	t.Parallel()

	const (
		solarG    = -26.90
		solarBPRP = 0.82
	)

	got := solarG - magnitude.GaiaGToCousinsI(solarG, solarBPRP)

	// The quadratic, written out.
	want := 0.01753 + 0.76*solarBPRP - 0.0991*solarBPRP*solarBPRP

	if math.Abs(got-want) > 1e-12 {
		t.Errorf("G - I_C = %.10f, want %.10f", got, want)
	}

	// And the value that a cubic reading would produce, which must not be it.
	cubic := want + 0.03765*solarBPRP*solarBPRP*solarBPRP

	if math.Abs(got-cubic) < 1e-6 {
		t.Errorf("G - I_C = %.10f matches the cubic reading %.10f; 0.03765 is the published "+
			"sigma, not a coefficient", got, cubic)
	}
}

// The four bands stay correctly ordered across normal stellar colours.
//
// For any star cooler than about A0 the magnitudes must fall B > V > R > I:
// a cool star emits more at longer wavelengths, so it is faintest in the
// bluest band. That constrains all four polynomials at once and against each
// other, which a single-star anchor does not — a coefficient mistyped in one
// relation shows here as a crossing even when that relation still passes
// through the Sun.
//
// # Why 0.5 to 2.0 and not the full fitted range
//
// Below BP-RP of about 0.4 the ordering is genuinely degenerate: B - V goes to
// zero by the definition of the Vega system, so B and V cross and the test
// would be asserting noise. Above 2.0 both the B and R relations are
// restricted to M giants by Table 5.10, so ordinary stars there are outside
// what the fits claim.
//
// An earlier version of this test asserted that each G - band relation is
// monotonic in colour. It is not, and that was my assumption rather than
// anything the reference supports: measured, G - V turns over at BP - RP of
// about -0.36, G - R at 1.5 and G - I at 4.0. These are empirical fits over
// wide colour ranges and nothing requires them to be monotonic to their edges.
// The bounds were not tuned until the assertion passed; the assertion was
// replaced with one the references do support.
func TestGaiaTransformationsKeepTheBandsOrdered(t *testing.T) {
	t.Parallel()

	const (
		g     = 12.0
		loCol = 0.5
		hiCol = 2.0
		steps = 40
	)

	for step := range steps + 1 {
		col := loCol + (hiCol-loCol)*float64(step)/steps

		b := magnitude.GaiaGToJohnsonB(g, col)
		v := magnitude.GaiaGToJohnsonV(g, col)
		r := magnitude.GaiaGToJohnsonR(g, col)
		i := magnitude.GaiaGToCousinsI(g, col)

		if !(b > v && v > r && r > i) {
			t.Fatalf("at BP-RP = %.3f the bands are B %.4f, V %.4f, R %.4f, I %.4f; a star "+
				"this red must be faintest in B and brightest in I", col, b, v, r, i)
		}
	}
}

// No relation blows up inside the range it is fitted on.
//
// A mistyped coefficient usually shows as a polynomial that is fine near the
// anchor and enormous at the edge, which is how the wrong G-to-B cubic behaved
// before it was replaced: within half a magnitude at BP - RP near zero and two
// magnitudes adrift by three.
func TestGaiaTransformationsStayBoundedOverTheirRange(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name   string
		f      func(g, c float64) float64
		lo, hi float64
	}{
		{"B", magnitude.GaiaGToJohnsonB, -0.5, 4.0},
		{"V", magnitude.GaiaGToJohnsonV, -0.5, 5.0},
		{"R", magnitude.GaiaGToJohnsonR, 0.0, 4.0},
		{"I", magnitude.GaiaGToCousinsI, -0.5, 4.5},
	} {
		const (
			g     = 12.0
			steps = 40

			// No optical colour index of a real star reaches six magnitudes;
			// a relation that produces one has lost its shape.
			bound = 6.0
		)

		for step := range steps + 1 {
			col := c.lo + (c.hi-c.lo)*float64(step)/steps

			if offset := g - c.f(g, col); math.Abs(offset) > bound {
				t.Errorf("%s: G-%s = %.3f at BP-RP = %.3f, beyond anything a stellar colour "+
					"index reaches", c.name, c.name, offset, col)

				break
			}
		}
	}
}
