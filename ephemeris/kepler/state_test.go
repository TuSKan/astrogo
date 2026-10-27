package kepler_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// sunGM is the central body heliocentric elements default to, named so that a
// state can be converted under exactly the mass its source was built with.
var sunGM = kepler.CentralBody{ID: core.Sun, GM: constants.IAU.SunGravitationalParameter.Value}

// angleDiff is got − want wrapped into (−π, π], in radians.
func angleDiff(got, want angle.Angle) float64 {
	return math.Remainder(got.Radians()-want.Radians(), 2*math.Pi)
}

// TestFromStateStartsAtItsState: an ellipse is propagated from its state
// itself, so at its own epoch it is that state, bit for bit — no angles, no
// perihelion, nothing to round through.
func TestFromStateStartsAtItsState(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.LocationUTC)
	pos := vector.V3(-1.8133, 1.9071, 0.4415)
	vel := vector.V3(-0.007212, -0.006058, -0.000894)

	el, err := kepler.FromState(epoch, pos, vel, sunGM)
	if err != nil {
		t.Fatal(err)
	}

	gotPos, gotVel, err := el.StateAt(epoch)
	if err != nil {
		t.Fatal(err)
	}

	if gotPos != pos || gotVel != vel {
		t.Errorf("StateAt(epoch) = %v, %v; want the state it was given, %v, %v", gotPos, gotVel, pos, vel)
	}
}

// TestFromStateReportsTheOrbit: a state taken from a known orbit converts
// back to that orbit's elements, as published elements are reported — the
// J2000 ecliptic angles, the size, the mean anomaly at epoch — and propagates
// along the same orbit, for an ellipse and for a hyperbola.
func TestFromStateReportsTheOrbit(t *testing.T) {
	t.Parallel()

	const (
		angleTol = 1e-12 // radians
		relTol   = 1e-12
	)

	epoch := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.LocationUTC)

	ellipse, err := kepler.NewElements(epoch, unit.AU(2.7), 0.08,
		angle.Deg(10), angle.Deg(80), angle.Deg(73), angle.Deg(40))
	if err != nil {
		t.Fatal(err)
	}

	hyperbola, err := kepler.FromPerihelion(epoch, unit.AU(1.5), 1.2,
		angle.Deg(25), angle.Deg(130), angle.Deg(-40))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name  string
		orbit kepler.Elements
		at    float64 // days after the orbit's epoch the state is taken
	}{
		{"ellipse", ellipse, 0},
		{"hyperbola", hyperbola, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			at := epoch.Add(unit.Days(tc.at))

			pos, vel, err := tc.orbit.StateAt(at)
			if err != nil {
				t.Fatal(err)
			}

			el, err := kepler.FromState(at, pos, vel, sunGM)
			if err != nil {
				t.Fatal(err)
			}

			if !el.Epoch().Equal(at) {
				t.Errorf("Epoch = %v, want the state's, %v", el.Epoch(), at)
			}

			if d := math.Abs(el.Eccentricity() - tc.orbit.Eccentricity()); d > relTol {
				t.Errorf("Eccentricity = %v, want %v", el.Eccentricity(), tc.orbit.Eccentricity())
			}

			if got, want := el.PerihelionDistance().AU(), tc.orbit.PerihelionDistance().AU(); math.Abs(got-want) > relTol*want {
				t.Errorf("PerihelionDistance = %v AU, want %v", got, want)
			}

			for _, a := range []struct {
				name      string
				got, want angle.Angle
			}{
				{"Inclination", el.Inclination(), tc.orbit.Inclination()},
				{"AscendingNode", el.AscendingNode(), tc.orbit.AscendingNode()},
				{"ArgPeriapsis", el.ArgPeriapsis(), tc.orbit.ArgPeriapsis()},
			} {
				if d := angleDiff(a.got, a.want); math.Abs(d) > angleTol {
					t.Errorf("%s = %v, want %v", a.name, a.got, a.want)
				}
			}

			// The mean anomaly at the state's epoch: the published one for the
			// ellipse, n·(t − T) for the hyperbola.
			wantM := tc.orbit.MeanAnomaly().Radians()
			if tc.orbit.Eccentricity() > 1 {
				a := -tc.orbit.SemiMajorAxis().AU()
				wantM = math.Sqrt(muAUDay()/(a*a*a)) * tc.at
			}

			if d := angleDiff(el.MeanAnomaly(), angle.Rad(wantM)); math.Abs(d) > angleTol {
				t.Errorf("MeanAnomaly = %v rad, want %v", el.MeanAnomaly().Radians(), wantM)
			}

			// And it is the same orbit: two arithmetic paths, held to each
			// other's rounding rather than to each other's bits.
			for _, days := range []float64{-1000, -10, 10, 1000} {
				when := at.Add(unit.Days(days))

				got, _, err := el.StateAt(when)
				if err != nil {
					t.Fatal(err)
				}

				want, _, err := tc.orbit.StateAt(when)
				if err != nil {
					t.Fatal(err)
				}

				if d := got.Sub(want).Norm(); d > relTol*want.Norm() {
					t.Errorf("%+g days on: %v AU from the orbit it came from", days, d)
				}
			}
		})
	}
}

// TestFromStateRefusesWhatNoConicDescribes: a position at the central body,
// radial motion, a state or a mass parameter that is not a finite number.
func TestFromStateRefusesWhatNoConicDescribes(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.LocationUTC)
	pos, vel := vector.V3(1, 0, 0), vector.V3(0, 0.017, 0)

	for _, tc := range []struct {
		name     string
		pos, vel vector.Vec3
		central  kepler.CentralBody
		want     error
	}{
		{"at the central body", vector.Zero(), vel, sunGM, kepler.ErrInvalidElements},
		{"radial motion", pos, vector.V3(0.01, 0, 0), sunGM, kepler.ErrUnsupportedOrbit},
		{"not a number", vector.V3(math.NaN(), 0, 0), vel, sunGM, kepler.ErrInvalidElements},
		{"an infinite velocity", pos, vector.V3(0, math.Inf(1), 0), sunGM, kepler.ErrInvalidElements},
		{"no mass", pos, vel, kepler.CentralBody{ID: core.Sun}, kepler.ErrInvalidElements},
		{"a negative mass", pos, vel, kepler.CentralBody{ID: core.Sun, GM: -1}, kepler.ErrInvalidElements},
	} {
		if _, err := kepler.FromState(epoch, tc.pos, tc.vel, tc.central); !errors.Is(err, tc.want) {
			t.Errorf("%s: FromState = %v, want %v", tc.name, err, tc.want)
		}
	}
}

// TestFromStateTakesNoSatelliteModifiers: a state is osculating in the ICRF,
// so a Laplace plane, a secular precession or a period has nothing to apply
// to, and propagating one so modified fails rather than quietly ignoring it.
// A new central body re-derives the orbit through the same state.
func TestFromStateTakesNoSatelliteModifiers(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.LocationUTC)
	pos, vel := vector.V3(1.2, 0.3, -0.1), vector.V3(-0.003, 0.015, 0.002)

	el, err := kepler.FromState(epoch, pos, vel, sunGM)
	if err != nil {
		t.Fatal(err)
	}

	for name, modified := range map[string]kepler.Elements{
		"Laplace plane":      el.WithLaplacePlane(kepler.LaplacePlane{RA: angle.Deg(268), Dec: angle.Deg(64.5)}),
		"secular precession": el.WithSecularPrecession(kepler.SecularPrecession{ApsisPeriod: 100}),
		"period":             el.WithPeriod(400),
	} {
		if _, _, err := modified.StateAt(epoch); !errors.Is(err, kepler.ErrInvalidElements) {
			t.Errorf("%s: StateAt = %v, want ErrInvalidElements", name, err)
		}
	}

	// Twice the mass: the same state on a larger orbit, by vis-viva.
	heavier := sunGM
	heavier.GM *= 2

	moved := el.WithCentralBody(heavier)

	mu := 2 * muAUDay()
	wantA := 1 / (2/pos.Norm() - vel.Dot(vel)/mu)

	if got := moved.SemiMajorAxis().AU(); math.Abs(got-wantA) > 1e-14*wantA {
		t.Errorf("SemiMajorAxis under twice the mass = %v AU, want %v", got, wantA)
	}

	if gotPos, gotVel, err := moved.StateAt(epoch); err != nil || gotPos != pos || gotVel != vel {
		t.Errorf("StateAt(epoch) under twice the mass = %v, %v, %v; want the same state", gotPos, gotVel, err)
	}
}

// TestFromStateRegisters: a provider locates an orbit built from a state about
// the central body it was given.
func TestFromStateRegisters(t *testing.T) {
	t.Parallel()

	const id core.ID = 1_000_001

	epoch := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.LocationUTC)
	pos, vel := vector.V3(1.2, 0.3, -0.1), vector.V3(-0.003, 0.015, 0.002)

	el, err := kepler.FromState(epoch, pos, vel, sunGM)
	if err != nil {
		t.Fatal(err)
	}

	p := kepler.New()
	if err := p.Register(id, el); err != nil {
		t.Fatal(err)
	}

	st, err := p.Locate(id, epoch)
	if err != nil {
		t.Fatal(err)
	}

	if st.Center != core.Sun || st.Pos != pos || st.Vel != vel {
		t.Errorf("Locate(epoch) = %v about %v; want %v, %v about the Sun", st, st.Center, pos, vel)
	}
}
