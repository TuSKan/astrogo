package satellite_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite"
)

// TestPropagatorExposesTheModelsOwnPredicates covers what replaced
// Satellite.Verified.
//
// Verified answered a question about astrogo's coverage — which regime had been
// checked against Vallado — and that question stopped having a false answer
// when the model was rewritten. It is gone rather than pinned to true, because
// a predicate that always agrees is worse than none: it reads as a check.
//
// The propagator answers questions about the orbit instead, and those do not
// expire: whether SGP4 takes its simplified drag branch, whether it hands over
// to SDP4, and where the perigee it branches on actually is.
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
