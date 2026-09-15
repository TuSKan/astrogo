package sgp4

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
)

// TestGravityModelsMatchValladosTable pins every number against
// getgravconst, which is the definition the reference states were produced
// with.
//
// Exact equality, not a tolerance. These are conventional constants: "nearly
// WGS-72" is not a gravity model, and a value that differs in the last digit
// describes a different model that happens to be close.
func TestGravityModelsMatchValladosTable(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		g            Gravity
		radiusKM, mu float64
		j2, j3, j4   float64
	}{
		{WGS72, 6378.135, 398600.8, 0.001082616, -0.00000253881, -0.00000165597},
		{WGS84, 6378.137, 398600.5, 0.00108262998905, -0.00000253215306, -0.00000161098761},
		{WGS72Old, 6378.135, 398600.79964, 0.001082616, -0.00000253881, -0.00000165597},
	} {
		m := constantsFor(tc.g)

		for _, f := range []struct {
			name      string
			got, want float64
		}{
			{"radiusKM", m.radiusKM, tc.radiusKM},
			{"mu", m.muKM3S2, tc.mu},
			{"j2", m.j2, tc.j2},
			{"j3", m.j3, tc.j3},
			{"j4", m.j4, tc.j4},
		} {
			if f.got != f.want {
				t.Errorf("%s %s = %.17g, want %.17g exactly", tc.g, f.name, f.got, f.want)
			}
		}

		// Derived members, checked as relationships rather than literals so
		// that a change to a base constant cannot leave them stale.
		if want := 1.0 / m.xke; m.tumin != want {
			t.Errorf("%s tumin = %.17g, want 1/xke = %.17g", tc.g, m.tumin, want)
		}

		if want := m.j3 / m.j2; m.j3oj2 != want {
			t.Errorf("%s j3oj2 = %.17g, want j3/j2 = %.17g", tc.g, m.j3oj2, want)
		}
	}
}

// TestWGS72OldKeepsItsHardcodedXKE is the distinction that makes WGS72Old a
// separate model rather than a rounding of WGS72.
//
// Vallado derives xke from the radius and mu for WGS72 and WGS84, and writes it
// out as a literal for WGS72Old. The literal does not equal what the derivation
// would give — which is the whole reason the row exists, and exactly the kind of
// inconsistency a later reader "tidies up".
func TestWGS72OldKeepsItsHardcodedXKE(t *testing.T) {
	t.Parallel()

	m := constantsFor(WGS72Old)

	const valladosLiteral = 0.0743669161

	if m.xke != valladosLiteral {
		t.Errorf("WGS72Old xke = %.17g, want the literal %.17g", m.xke, valladosLiteral)
	}

	derived := derivedXKE(m.radiusKM, m.muKM3S2)
	if m.xke == derived {
		t.Errorf("WGS72Old's xke now equals the derived value %.17g. Vallado hardcodes it "+
			"precisely because it does not, and deriving it makes this model indistinguishable "+
			"from a rounding of WGS72", derived)
	}

	// How far apart, so the next reader knows the size of what they would be
	// changing: about 2e-9 relative, which is small and is not nothing.
	if rel := math.Abs(m.xke-derived) / derived; rel > 1e-8 {
		t.Errorf("WGS72Old xke differs from the derived value by %.3g relative, which is "+
			"larger than this has ever been — check the constants", rel)
	}
}

// TestXKEIsWrittenTheWayValladoWritesIt guards an algebraic identity that is not
// a floating-point identity.
//
// 60/sqrt(r³/mu) and 60*sqrt(mu/r³) are the same number in exact arithmetic and
// not always the same float64. The reference states were produced by the first
// spelling, and reproducing them to the last bit — which is what this package
// is for — means keeping it.
func TestXKEIsWrittenTheWayValladoWritesIt(t *testing.T) {
	t.Parallel()

	const (
		r  = 6378.135
		mu = 398600.8
	)

	want := 60.0 / math.Sqrt(r*r*r/mu)

	if got := derivedXKE(r, mu); got != want {
		t.Errorf("derivedXKE = %.17g, want %.17g — the association matters", got, want)
	}
}

// TestGravityAgreesWithTheConstantsPackage asserts the relationship between
// this frozen table and astrogo's living one, in both directions.
//
// # Why the table is frozen at all
//
// constants publishes the best current value of a constant and tracks new
// realizations as they are adopted, which is the right contract for a package
// describing the Earth and the wrong one for a table that defines a model. If mu
// here moved, every TLE ever fitted would be propagated by a model that no
// longer matches the one that produced it, and nothing would say so.
//
// # Why that is not an excuse to skip the check
//
// A frozen copy is how ephemeris/satellite came to hold WGS-84's values while
// its propagator was configured for WGS-72. The copy is kept and the
// relationship is asserted, so the agreement is checked where the two describe
// the same thing and the disagreement is deliberate and documented where they
// do not.
func TestGravityAgreesWithTheConstantsPackage(t *testing.T) {
	t.Parallel()

	m72 := constantsFor(WGS72)

	if got, want := m72.radiusKM*1e3, constants.WGS72.SemiMajorAxis.Value; got != want {
		t.Errorf("WGS-72 radius here is %.17g m, constants.WGS72 says %.17g", got, want)
	}

	if got, want := m72.muKM3S2*1e9, constants.WGS72.GeocentricGravitationalConstant.Value; got != want {
		t.Errorf("WGS-72 mu here is %.17g m³/s², constants.WGS72 says %.17g", got, want)
	}

	// The WGS-84 row deliberately does NOT agree, and by how much is recorded
	// so that a reader who notices the mismatch finds an answer rather than
	// filing a bug. constants.WGS84 carries the standard's 398600.4418 km³/s²;
	// Vallado's table is the 1984-vintage 398600.5.
	m84 := constantsFor(WGS84)

	gap := m84.muKM3S2 - constants.WGS84.GeocentricGravitationalConstant.Value/1e9
	if math.Abs(gap-0.0582) > 1e-9 {
		t.Errorf("SGP4's WGS-84 mu is %.17g km³/s² against constants.WGS84's %.17g — a gap of "+
			"%.6g, where 0.0582 is expected.\n"+
			"  If constants moved, that is correct and this table must NOT follow it: the "+
			"model means the vintage it was fitted with. Update the expected gap and say why.",
			m84.muKM3S2, constants.WGS84.GeocentricGravitationalConstant.Value/1e9, gap)
	}

	if m84.radiusKM*1e3 != constants.WGS84.SemiMajorAxis.Value {
		t.Errorf("the WGS-84 radius has always agreed with the standard (6378137 m) and now "+
			"does not: %.17g vs %.17g", m84.radiusKM*1e3, constants.WGS84.SemiMajorAxis.Value)
	}
}

// TestGravityStringAndValidity covers the enum's own surface, including the
// zero value, which is WGS72 and is the default for a reason.
func TestGravityStringAndValidity(t *testing.T) {
	t.Parallel()

	var zero Gravity
	if zero != WGS72 {
		t.Errorf("the zero Gravity is %v, want WGS72 — a caller who does not choose must get "+
			"the model TLEs are fitted with", zero)
	}

	for _, g := range []Gravity{WGS72, WGS84, WGS72Old} {
		if !g.valid() {
			t.Errorf("%v reports itself invalid", g)
		}

		if g.String() == "" {
			t.Errorf("Gravity(%d) has an empty String()", int(g))
		}
	}

	bogus := Gravity(42)
	if bogus.valid() {
		t.Error("Gravity(42) reports itself valid")
	}

	if got := bogus.String(); got != "Gravity(42)" {
		t.Errorf("Gravity(42).String() = %q, want %q", got, "Gravity(42)")
	}

	// An unknown model must not silently become something else in the table.
	// It falls through to WGS-72's numbers, which is the safe default, but the
	// propagator will refuse it at construction — that is New's job, not this
	// table's, and this records which of the two holds the check.
	if constantsFor(bogus) != constantsFor(WGS72) {
		t.Error("constantsFor(unknown) returned something that is not the WGS-72 row")
	}
}
