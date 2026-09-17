package unit_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

// TestLengthRoundTripsThroughEveryUnit is the property the type exists for: a
// value put in through one spelling and read back through the same one is the
// value that went in.
//
// Exactness is asserted where it is achievable and a relative tolerance where
// it is not. Metres round-trip exactly because they are the storage; the rest
// are a multiply and a divide by the same scale factor, which is correct to a
// unit in the last place and not further.
func TestLengthRoundTripsThroughEveryUnit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		make func(float64) unit.Length
		read func(unit.Length) float64
	}{
		{"metres", unit.Metres, unit.Length.Metres},
		{"millimetres", unit.Millimetres, unit.Length.Millimetres},
		{"kilometres", unit.Km, unit.Length.Km},
		{"astronomical units", unit.AU, unit.Length.AU},
		{"parsecs", unit.Pc, unit.Length.Pc},
		{"light-years", unit.LightYears, unit.Length.LightYears},
	} {
		for _, v := range []float64{0, 1, -1, 1e-6, 2635, 8178, 1.5e8, -4.2e3} {
			got := tc.read(tc.make(v))

			if v == 0 {
				if got != 0 {
					t.Errorf("%s: zero round-tripped to %g", tc.name, got)
				}

				continue
			}

			if rel := math.Abs(got-v) / math.Abs(v); rel > 1e-15 {
				t.Errorf("%s: %g round-tripped to %g (relative %.3g)", tc.name, v, got, rel)
			}
		}
	}
}

// TestLengthConversionsAgreeWithTheKnownValues checks the type against numbers
// that exist outside this package, rather than only against itself.
//
// A round trip is satisfied by any self-consistent scale factor, including a
// wrong one. These are the definitions.
func TestLengthConversionsAgreeWithTheKnownValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		got  float64
		want float64
		rel  float64
	}{
		// IAU 2012 Resolution B2: the au is exactly 149 597 870 700 m.
		{"1 AU in km", unit.AU(1).Km(), 149597870.7, 1e-15},
		{"1 AU in metres", unit.AU(1).Metres(), 1.495978707e11, 1e-15},

		// The parsec is the distance at which 1 au subtends 1 arcsec, so it is
		// exactly 648000/pi au.
		{"1 pc in AU", unit.Pc(1).AU(), 648000 / math.Pi, 1e-15},

		// A light-year is c times a Julian year, both exact by definition.
		{"1 ly in metres", unit.LightYears(1).Metres(), 9.4607304725808e15, 1e-15},

		// And the mixed direction, which is what a caller actually does.
		{"1 km in metres", unit.Km(1).Metres(), 1000, 1e-15},
		{"1 mm in metres", unit.Millimetres(1).Metres(), 1e-3, 1e-15},
	} {
		if rel := math.Abs(tc.got-tc.want) / math.Abs(tc.want); rel > tc.rel {
			t.Errorf("%s = %.17g, want %.17g (relative %.3g)", tc.name, tc.got, tc.want, rel)
		}
	}
}

// TestLengthAndQuantityCannotDisagree is the contract that justifies reading
// the scale factors from the Unit table rather than from constants of this
// type's own.
//
// Two representations of the same physical quantity that each carry their own
// copy of "how long an au is" will eventually disagree about the definition,
// and nothing will report it. These read the same table, so they cannot.
//
// They still differ in the last ulp, because the two routes are arithmetically
// different operations — see the comment at the assertion. That is rounding;
// what this rules out is divergence.
func TestLengthAndQuantityCannotDisagree(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		l    unit.Length
		u    unit.Unit
		read func(unit.Length) float64
	}{
		{"metres", unit.Metres(2635), unit.Meter, unit.Length.Metres},
		{"kilometres", unit.Km(384400), unit.Kilometer, unit.Length.Km},
		{"astronomical units", unit.AU(1.5), unit.AstronomicalUnit, unit.Length.AU},
		{"parsecs", unit.Pc(8178), unit.Parsec, unit.Length.Pc},
		{"light-years", unit.LightYears(4.2), unit.LightYear, unit.Length.LightYears},
	} {
		// The Quantity route: convert through the dimensioned machinery.
		q, err := tc.l.Quantity().In(tc.u)
		if err != nil {
			t.Errorf("%s: converting to %v: %v", tc.name, tc.u, err)
			continue
		}

		// The named-type route: read the accessor.
		direct := tc.read(tc.l)

		// A few ulp rather than bit-for-bit: the accessor divides by the scale
		// factor and Quantity multiplies by a reciprocal, and those are not the
		// same operation in floating point. What cannot drift is the
		// definition, since both read the same ScaleFactor.
		if math.Abs(q.Value-direct) > 1e-12*math.Abs(direct) {
			t.Errorf("%s: Quantity says %.17g and the accessor says %.17g — further "+
				"apart than the last few ulp, so the two representations have drifted",
				tc.name, q.Value, direct)
		}
	}
}

// TestLengthFromRejectsWhatIsNotALength covers the one thing a named scalar
// cannot check for itself, which is why this direction returns an error while
// [unit.Length.Quantity] does not.
func TestLengthFromRejectsWhatIsNotALength(t *testing.T) {
	t.Parallel()

	// A length, which must convert.
	l, err := unit.LengthFrom(unit.New(1.5, unit.AstronomicalUnit))
	if err != nil {
		t.Fatalf("a length was rejected: %v", err)
	}

	if rel := math.Abs(l.AU()-1.5) / 1.5; rel > 1e-15 {
		t.Errorf("1.5 AU came back as %v", l)
	}

	// A time, which must not. Nothing sensible can be done with it, and
	// silently treating the number as metres is the failure this prevents.
	if _, err := unit.LengthFrom(unit.New(60, unit.Second)); err == nil {
		t.Error("a duration was accepted as a length")
	} else if !errors.As(err, &unit.IncompatibleUnitError{}) {
		t.Logf("rejected with %v", err)
	}
}

// TestLengthStringPicksTheUnitAReaderExpects pins the thresholds, because a
// String that silently changed unit would make two log lines incomparable.
func TestLengthStringPicksTheUnitAReaderExpects(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		l    unit.Length
		want string
	}{
		{"a telescope aperture", unit.Metres(8.2), "8.2 m"},
		{"a site height", unit.Metres(2635), "2.635 km"},
		{"a satellite range", unit.Km(408), "408 km"},
		{"the Moon", unit.Km(384400), "384400 km"},
		{"a solar system distance", unit.AU(30.1), "30.1 AU"},
		{"a nearby star", unit.Pc(1.3), "1.3 pc"},
		{"the Galactic centre", unit.Pc(8178), "8178 pc"},
		{"zero", unit.Metres(0), "0 m"},
		{"negative, which a Cartesian component is", unit.Pc(-8178), "-8178 pc"},
	} {
		if got := tc.l.String(); got != tc.want {
			t.Errorf("%s: String() = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestLengthHelpers covers IsZero and Abs, and the distinction IsZero draws.
func TestLengthHelpers(t *testing.T) {
	t.Parallel()

	if !unit.Metres(0).IsZero() {
		t.Error("zero is not reported as zero")
	}

	if unit.Metres(1e-300).IsZero() {
		t.Error("a very small length is reported as zero; IsZero is exact by design")
	}

	if got := unit.Pc(-8178).Abs(); got != unit.Pc(8178) {
		t.Errorf("Abs gave %v", got)
	}

	// Abs leaves a positive length alone rather than round-tripping it through
	// a conversion that might not be exact.
	if got := unit.AU(1.5).Abs(); got != unit.AU(1.5) {
		t.Errorf("Abs changed a positive length to %v", got)
	}
}
