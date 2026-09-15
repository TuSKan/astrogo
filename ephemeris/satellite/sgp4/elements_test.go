package sgp4_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/time"
)

// good returns an element set that passes, so each case below can change
// exactly one thing.
func good() sgp4.Elements {
	return sgp4.Elements{
		NORAD:        25544,
		Epoch:        time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		Inclination:  angle.Deg(51.6416),
		RAAN:         angle.Deg(247.4627),
		ArgPerigee:   angle.Deg(130.5360),
		MeanAnomaly:  angle.Deg(325.0288),
		Eccentricity: 0.0006703,
		MeanMotion:   15.72125391,
		BStar:        -1.1606e-5,
	}
}

// TestValidateRefusesTheImpossible walks each branch, and each "accepted" case
// is the more interesting half: a validator that rejects a legitimate extreme
// is a validator that makes part of the model untestable.
func TestValidateRefusesTheImpossible(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		mutate  func(*sgp4.Elements)
		wantErr bool
	}{
		{"the baseline", func(*sgp4.Elements) {}, false},

		{"zero mean motion", func(e *sgp4.Elements) { e.MeanMotion = 0 }, true},
		{"negative mean motion", func(e *sgp4.Elements) { e.MeanMotion = -15 }, true},
		{
			// Vallado's satellite 33334: a period of 274 years, built to drive
			// SGP4 into its mean-motion error return. Absurd, legitimate, and
			// a validator that refused it would make that error path
			// unreachable from a real element set.
			name:    "a mean motion of 0.00001 rev/day",
			mutate:  func(e *sgp4.Elements) { e.MeanMotion = 0.00001 },
			wantErr: false,
		},

		{"eccentricity of 1", func(e *sgp4.Elements) { e.Eccentricity = 1 }, true},
		{"eccentricity above 1", func(e *sgp4.Elements) { e.Eccentricity = 1.5 }, true},
		{"negative eccentricity", func(e *sgp4.Elements) { e.Eccentricity = -1e-9 }, true},
		{
			// SGP4 clamps a circular orbit to 1e-6 internally to avoid a
			// division; that is the model's business, and an element set is
			// entitled to say zero.
			name:    "a perfectly circular orbit",
			mutate:  func(e *sgp4.Elements) { e.Eccentricity = 0 },
			wantErr: false,
		},
		{
			// Vallado's satellite 33333, at e = 0.995.
			name:    "a near-parabolic orbit",
			mutate:  func(e *sgp4.Elements) { e.Eccentricity = 0.995 },
			wantErr: false,
		},

		{"inclination below zero", func(e *sgp4.Elements) { e.Inclination = angle.Deg(-1) }, true},
		{"inclination past 180", func(e *sgp4.Elements) { e.Inclination = angle.Deg(180.1) }, true},
		{"equatorial", func(e *sgp4.Elements) { e.Inclination = 0 }, false},
		{"exactly retrograde", func(e *sgp4.Elements) { e.Inclination = angle.Rad(math.Pi) }, false},

		{"NaN eccentricity", func(e *sgp4.Elements) { e.Eccentricity = math.NaN() }, true},
		{"NaN inclination", func(e *sgp4.Elements) { e.Inclination = angle.Rad(math.NaN()) }, true},
		{"NaN B*", func(e *sgp4.Elements) { e.BStar = math.NaN() }, true},
		{"infinite B*", func(e *sgp4.Elements) { e.BStar = math.Inf(1) }, true},
		{
			// B* is a fitted coefficient, not a physical drag area, and a
			// negative one is ordinary in a catalogue.
			name:    "negative B*",
			mutate:  func(e *sgp4.Elements) { e.BStar = -0.5 },
			wantErr: false,
		},

		{"zero epoch", func(e *sgp4.Elements) { e.Epoch = time.Time{} }, true},
	} {
		el := good()
		tc.mutate(&el)

		err := el.Validate()

		switch {
		case tc.wantErr && err == nil:
			t.Errorf("%s: Validate accepted it", tc.name)
		case !tc.wantErr && err != nil:
			t.Errorf("%s: Validate refused it: %v", tc.name, err)
		case tc.wantErr && !errors.Is(err, sgp4.ErrElements):
			t.Errorf("%s: error is %v, want one wrapping ErrElements", tc.name, err)
		}
	}
}

// TestParsedElementsAreAlreadyValid pins the contract ParseTLE's doc comment
// makes, so nothing downstream has to re-check.
func TestParsedElementsAreAlreadyValid(t *testing.T) {
	t.Parallel()

	for _, s := range loadValladoTLEs(t) {
		el, err := sgp4.ParseTLE(s.line1, s.line2)
		if err != nil {
			continue
		}

		if err := el.Validate(); err != nil {
			t.Errorf("satellite %s: ParseTLE returned an element set that fails Validate: %v",
				s.satnum, err)
		}
	}
}
