package satellite_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite"
)

// TestVerifiedClearsEverything is the inverse of the test it replaces.
//
// Satellite.Verified used to flag a low-perigee element set, because astrogo's
// propagation could not reproduce Vallado's reference vectors in that band — by
// as much as 3438 km. That band is now among the best-agreeing in the suite:
// satellite 28350, the case the predicate was really about, went from 3438.51 km
// to 6.3e-09 km when the model was rewritten (#309, #319).
//
// So the assertion flips. A predicate that still returned false there would be
// warning a caller away from a result that is correct to nanometres, which is
// worse than saying nothing.
//
// The low-perigee element set below is Vallado's own 28350, kept because it is
// the exact input that used to be flagged. If Verified ever returns false for
// it again, that is a claim about astrogo's coverage and it needs a measurement
// behind it.
func TestVerifiedClearsEverything(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		line1, line2 string
	}{
		{
			name:  "an ordinary low Earth orbit",
			line1: "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927",
			line2: "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537",
		},
		{
			// Vallado: "Near Earth, perigee = 127.20 (< 156) s4 mod".
			name:  "the low-perigee case this predicate was built for",
			line1: "1 28350U 04020A   06167.21788666  .16154492  76267-5  18678-3 0  8894",
			line2: "2 28350  64.9977 345.6130 0024870 260.7578  99.9590 16.47856722116490",
		},
	} {
		sat, err := satellite.NewFromTLE(tc.name, tc.line1, tc.line2)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		ok, reason := sat.Verified()
		if !ok {
			t.Errorf("%s: Verified reported false (%q). There is no regime left to flag: "+
				"every case in Vallado's suite now agrees to 4.1e-06 km or better, the "+
				"low-perigee band included.", tc.name, reason)
		}

		if reason != "" {
			t.Errorf("%s: Verified reported true with a reason attached: %q", tc.name, reason)
		}
	}
}

// TestPropagatorExposesTheModelsOwnPredicates covers what a caller should reach
// for instead of Verified.
//
// Verified answered a question about astrogo's coverage, which changed. The
// propagator answers questions about the orbit, which do not: whether SGP4
// takes its simplified drag branch, whether it hands over to SDP4, and where
// the perigee it branches on actually is.
func TestPropagatorExposesTheModelsOwnPredicates(t *testing.T) {
	t.Parallel()

	const (
		issLine1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		issLine2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatal(err)
	}

	p := sat.Propagator()
	if p == nil {
		t.Fatal("Propagator() returned nil")
	}

	if p.DeepSpace() {
		t.Error("the ISS is not a deep-space object")
	}

	if p.SimplifiedDrag() {
		t.Error("a 400 km orbit does not take the simplified drag branch")
	}

	// Around 400 km, and the two the right way round.
	perigee, apogee := p.PerigeeAltitude(), p.ApogeeAltitude()
	if perigee > apogee || perigee < 300 || apogee > 500 {
		t.Errorf("perigee %g km, apogee %g km — not a 400 km orbit", perigee, apogee)
	}

	// The elements the propagator holds must be the ones the wrapper reports,
	// or the two can disagree about which orbit they describe.
	if el := p.Elements(); el.MeanMotion != sat.MeanMotion {
		t.Errorf("Satellite.MeanMotion is %g and the propagator's elements say %g",
			sat.MeanMotion, el.MeanMotion)
	}
}
