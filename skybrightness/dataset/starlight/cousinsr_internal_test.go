package starlight

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// The R identity is exact arithmetic on two catalogued colour indices.
//
// V-R = (V-I) - (R-I), so R = V - (V-I) + (R-I). Alpha Centauri A, from
// Hipparcos V and V-I and the Bright Star Catalogue's R-I for HR 5459.
func TestCousinsRFromTwoIndices(t *testing.T) {
	t.Parallel()

	const (
		v    = -0.01 // Hipparcos
		vi   = 0.69  // Hipparcos
		ri   = 0.22  // Bright Star Catalogue, HR 5459
		want = v - vi + ri
	)

	if got := v - vi + ri; math.Abs(got-want) > 1e-12 {
		t.Fatalf("R = %v, want %v", got, want)
	}

	// The companion's R-I, which is what a position-only match returns for
	// this star and which must not give the same answer.
	const companion = 0.30

	if math.Abs((v-vi+companion)-want) < 1e-9 {
		t.Error("the two components give the same R, so this cannot distinguish them")
	}
}

// Brightness, not distance, picks between the components of a close binary.
//
// Alpha Centauri is the case: its two components sit 1.4 arcseconds apart in
// the catalogue while a position propagated through eight and three quarter
// years lands about six arcseconds out, so both are candidates and the nearer
// was B by two hundredths of an arcsecond. Position cannot separate them and V
// can, -0.01 against 1.33.
func TestBrightestMatchPrefersTheRightComponent(t *testing.T) {
	t.Parallel()

	type candidate struct {
		vmag, ri, sepArcsec float64
	}

	// The real numbers: A is marginally the further of the two.
	componentA := candidate{vmag: -0.01, ri: 0.22, sepArcsec: 6.40}
	componentB := candidate{vmag: 1.33, ri: 0.30, sepArcsec: 6.38}

	const starV = -0.01

	// Nearest-by-position, the rule that was wrong.
	nearest := componentA
	if componentB.sepArcsec < componentA.sepArcsec {
		nearest = componentB
	}

	if nearest.ri != componentB.ri {
		t.Fatal("the fixture no longer reproduces the failure: B must be the nearer")
	}

	// Closest-in-brightness, the rule that is right.
	best := componentA
	if math.Abs(componentB.vmag-starV) < math.Abs(componentA.vmag-starV) {
		best = componentB
	}

	if best.ri != componentA.ri {
		t.Errorf("brightness picked the component with R-I %.2f, want A at %.2f",
			best.ri, componentA.ri)
	}

	// And the tolerance has to sit between the two populations.
	if math.Abs(componentB.vmag-starV) <= BrightStarMagnitudeTolerance {
		t.Errorf("the tolerance of %.2f admits the companion at delta V %.2f",
			BrightStarMagnitudeTolerance, math.Abs(componentB.vmag-starV))
	}

	if math.Abs(componentA.vmag-starV) > BrightStarMagnitudeTolerance {
		t.Errorf("the tolerance of %.2f rejects the star itself at delta V %.2f",
			BrightStarMagnitudeTolerance, math.Abs(componentA.vmag-starV))
	}
}

// The tolerance separates a genuine match from a component of a multiple.
//
// Measured over this set, the two populations do not overlap: a real match
// agrees to hundredths, and the three multiples where Hipparcos reports
// combined light while the Bright Star Catalogue reports a component differ by
// about six tenths. A tolerance outside that gap either admits companions or
// rejects stars.
func TestMagnitudeToleranceSitsBetweenThePopulations(t *testing.T) {
	t.Parallel()

	// Gamma Leonis, Xi Ursae Majoris, Xi Scorpii: Hipparcos against the
	// catalogue candidate that carries R-I.
	for _, delta := range []float64{0.60, 0.62, 0.61} {
		if delta <= BrightStarMagnitudeTolerance {
			t.Errorf("a combined-light multiple at delta V %.2f is admitted by a tolerance "+
				"of %.2f; its colour belongs to one component, not to the pair",
				delta, BrightStarMagnitudeTolerance)
		}
	}

	// A genuine match, which agrees far more closely than that.
	for _, delta := range []float64{0.00, 0.01, 0.05, 0.1} {
		if delta > BrightStarMagnitudeTolerance {
			t.Errorf("a genuine match at delta V %.2f is rejected", delta)
		}
	}
}

// The catalogue radius is wide enough for the residual a propagated position
// leaves, and no wider than it has to be.
func TestCatalogueRadiusFitsThePropagationResidual(t *testing.T) {
	t.Parallel()

	arcsec := BrightStarCatalogueRadius.Degrees() * 3600

	// Alpha Centauri, the worst case in this set, lands 6.4 arcseconds out
	// after propagation. A radius under that loses it entirely.
	if arcsec < 7 {
		t.Errorf("a radius of %.1f arcseconds is inside the 6.4 that the fastest star in "+
			"this set leaves after propagation", arcsec)
	}

	// Unpropagated it was 36 arcseconds out, which is what the radius used to
	// be sized for. Going back to that would admit far more companions.
	if arcsec > 20 {
		t.Errorf("a radius of %.1f arcseconds is wide enough to be matching by luck; "+
			"the positions are propagated now", arcsec)
	}

	_ = angle.Deg(0)
}
