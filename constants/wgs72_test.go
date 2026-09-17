package constants_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/unit/dim"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestWGS72_SemiMajorAxis(t *testing.T) {
	testutil.AssertExact(t, "WGS72 SemiMajorAxis", constants.WGS72.SemiMajorAxis.Value, 6_378_135.0)
}

func TestWGS72_InverseFlattening(t *testing.T) {
	testutil.AssertExact(t, "WGS72 InverseFlattening", constants.WGS72.InverseFlattening.Value, 298.26)
}

func TestWGS72_GM(t *testing.T) {
	testutil.AssertExact(t, "WGS72 GM", constants.WGS72.GeocentricGravitationalConstant.Value, 3.986_008e14)
}

func TestWGS72_J2(t *testing.T) {
	testutil.AssertExact(t, "WGS72 J2", constants.WGS72.DynamicalFormFactor.Value, 1.082_616e-3)
}

// TestWGS72_J2IsTheValueSGP4Branches asserts the member ephemeris/satellite
// reads, in the spelling Vallado's getgravconst publishes it in.
//
// It is a duplicate of the assertion above by value and not by intent: that one
// says the standard is transcribed correctly, this one says the consumer will
// get the number its own arithmetic was validated against. If WGS 72 were ever
// restated in a different normalization, the first would follow the standard
// and the second would have to fail.
func TestWGS72_J2IsTheValueSGP4Branches(t *testing.T) {
	if got := constants.WGS72.DynamicalFormFactor.Value; got != 0.001082616 {
		t.Errorf("J2 = %.10g, want 0.001082616 — the literal in Vallado's "+
			"getgravconst(wgs72), which ephemeris/satellite reproduces a branch from",
			got)
	}
}

func TestWGS72_Units(t *testing.T) {
	if constants.WGS72.SemiMajorAxis.Unit != unit.Meter {
		t.Errorf("SemiMajorAxis.Unit = %v, want unit.Meter", constants.WGS72.SemiMajorAxis.Unit)
	}

	if constants.WGS72.InverseFlattening.Unit.Dimension != dim.Dimensionless {
		t.Errorf("InverseFlattening.Unit.Dimension = %+v, want Dimensionless", constants.WGS72.InverseFlattening.Unit.Dimension)
	}
}

func TestWGS72_AllExact(t *testing.T) {
	for _, c := range constants.WGS72.All() {
		if !c.Exact {
			t.Errorf("%s: Exact = false, want true — WGS 72's parameters are adopted by the standard, not measured", c.Symbol)
		}

		if c.Uncertainty != 0 {
			t.Errorf("%s: Uncertainty = %v, want 0", c.Symbol, c.Uncertainty)
		}
	}
}

func TestWGS72_Name(t *testing.T) {
	if got := constants.WGS72.Name(); got == "" {
		t.Errorf("WGS72.Name() is empty")
	}
}

// TestWGS72_PolarRadius is the shape check that catches a transposed digit in
// either defining parameter: an ellipsoid whose poles are further out than its
// equator, or one the size of the Moon, passes every exactness test above.
func TestWGS72_PolarRadius(t *testing.T) {
	a := constants.WGS72.SemiMajorAxis.Value
	b := a * (1 - 1/constants.WGS72.InverseFlattening.Value)

	if b >= a {
		t.Errorf("polar radius %v should be less than semi-major axis %v", b, a)
	}

	if b < 6.356e6 || b > 6.357e6 {
		t.Errorf("polar radius %v outside plausible band [6.356e6, 6.357e6]", b)
	}
}

// TestWGS72AndWGS84AreDifferentAndKnowIt asserts the gaps between the two
// realizations, so that neither can be quietly edited into the other.
//
// The two ellipsoids are 2 m apart in the equatorial radius — small enough that
// a ground track computed with the wrong one looks entirely reasonable. Their
// gravitational constants differ in the fourth significant figure, which is not
// small at all once it is inside a propagator: astrogo's satellite positions
// were 93 times further from Vallado's reference suite for exactly that reason
// until the mix-up was found.
//
// The numbers below are therefore recorded as assertions rather than as a
// sentence in a comment.
func TestWGS72AndWGS84AreDifferentAndKnowIt(t *testing.T) {
	da := constants.WGS84.SemiMajorAxis.Value - constants.WGS72.SemiMajorAxis.Value
	if math.Abs(da-2.0) > 1e-9 {
		t.Errorf("WGS84 a - WGS72 a = %v m, want exactly 2", da)
	}

	dgm := constants.WGS72.GeocentricGravitationalConstant.Value -
		constants.WGS84.GeocentricGravitationalConstant.Value

	// 3.986008e14 - 3.986004418e14 = 3.582e8 m³/s², i.e. 0.3582 km³/s².
	if math.Abs(dgm-3.582e8) > 1e3 {
		t.Errorf("WGS72 GM - WGS84 GM = %v m³/s², want 3.582e8", dgm)
	}

	// And DE440's Earth GM is a third value again — a measurement, where the
	// other two are conventions. If these three ever coincide, something has
	// been copied that should not have been.
	de440 := constants.DE440.EarthGravitationalParameter.Value
	if de440 == constants.WGS84.GeocentricGravitationalConstant.Value ||
		de440 == constants.WGS72.GeocentricGravitationalConstant.Value {
		t.Errorf("DE440's Earth GM (%v) now equals a WGS value; a measured constant has been "+
			"replaced by a conventional one", de440)
	}
}
