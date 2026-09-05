package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

var errMovingBodyFails = errors.New("events_altitudes_test: geocentric vector always fails")

// errMovingBody fails on GeocentricVec, covering the branch a fixed target
// cannot reach.
type errMovingBody struct{}

func (errMovingBody) Name() string { return "ErrMovingBody" }

func (errMovingBody) Position(time.Time) (coord.ICRS, error) {
	return coord.ICRS{}, nil
}

func (errMovingBody) GetDetails(*coord.Context, ...string) (*TargetDetails, error) {
	return nil, errMovingBodyFails
}

func (errMovingBody) GeocentricVec(time.Time) (vector.Vec3, error) {
	return vector.Vec3{}, errMovingBodyFails
}

func (errMovingBody) Provider() eph.Provider { return nil }
func (errMovingBody) EphID() eph.ID          { return 0 }

// Compile-time proof this fixture reaches the MovingBody branch. Without it the
// type falls through to the Position path and the test passes while covering
// the wrong one, which is exactly what happened first time.
var _ MovingBody = errMovingBody{}

// TestAltitudesAtReportsFailureRatherThanAGuess covers the path an event takes
// when a target cannot be evaluated at the refined time.
//
// altitudesAt returns ok=false and both callers skip the event. That matters
// more than it looks: the alternative to skipping is appending an Event whose
// altitude fields are the zero value, which would read as a target sitting
// exactly on the horizon rather than as missing data.
//
// Both target shapes are covered because they reach the failure by different
// routes — a MovingBody through GeocentricVec, everything else through
// Position — and only one of them is reachable from a given fixture.
func TestAltitudesAtReportsFailureRatherThanAGuess(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("altitudes", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	when := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)
	geomAtm := atmosphere.Refraction{Pressure: 0}

	for _, tc := range []struct {
		name   string
		target Observable
	}{
		{"a fixed target whose Position fails", errObservable{}},
		{"a MovingBody whose GeocentricVec fails", errMovingBody{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := EventSpec{Target: tc.target, Observer: site}

			geom, refr, ok := altitudesAt(spec, when, geomAtm)
			if ok {
				t.Fatalf("ok was true for a target that cannot be evaluated; "+
					"got geom %v and refr %v", geom.Alt(), refr.Alt())
			}

			// Both altitudes must be the zero value, so a caller that ignores
			// ok cannot mistake a stale reading for a fresh one.
			if geom != (coord.AltAz{}) || refr != (coord.AltAz{}) {
				t.Errorf("failure returned non-zero altitudes geom=%v refr=%v", geom, refr)
			}
		})
	}
}

// TestAltitudesAtSeparatesTheTwoAtmospheres pins the reason the helper exists:
// the two altitudes it returns must actually be different quantities.
//
// A refactor that passed the same atmosphere twice would leave every Event
// with two identical fields again, which is the defect this replaced.
func TestAltitudesAtSeparatesTheTwoAtmospheres(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("altitudes", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	// A star near the horizon, where refraction is largest and the two
	// atmospheres disagree most.
	star := NewStar("low", angle.Deg(0), angle.Deg(44))
	when := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)

	spec := EventSpec{Target: star, Observer: site}

	geom, refr, ok := altitudesAt(spec, when, atmosphere.Refraction{Pressure: 0})
	if !ok {
		t.Fatal("altitudesAt failed on an ordinary star")
	}

	if refr.Alt() <= geom.Alt() {
		t.Errorf("refracted altitude %v is not above geometric %v; refraction "+
			"raises an object, so the two atmospheres are not being applied",
			refr.Alt(), geom.Alt())
	}

	// Azimuth is unaffected by refraction, so it must match exactly.
	if geom.Az() != refr.Az() {
		t.Errorf("azimuth differs between the two atmospheres: %v vs %v",
			geom.Az(), refr.Az())
	}
}
