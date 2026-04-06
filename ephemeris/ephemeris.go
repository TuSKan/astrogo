package ephemeris

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// State represents the kinematic state of a celestial
type State struct {
	Pos vector.Vec3 // Geocentric position in AU (ICRS-like)
	Vel vector.Vec3 // Geocentric velocity in AU/day (ICRS-like)
}

// Provider is the interface for celestial ephemeris sources.
type Provider interface {
	// State returns the geocentric state (position and velocity) of the given
	// body at time t. The vectors are typically in an inertial frame like ICRS.
	State(id ID, t time.Time) (State, error)
}

// Default returns a SOFA-based ephemeris provider for the Sun and Moon.
func Default() Provider {
	return &sofaProvider{}
}

// Position is a convenience helper that returns the geocentric position
// of a body at time t.
func Position(p Provider, id ID, t time.Time) (vector.Vec3, error) {
	st, err := p.State(id, t)
	if err != nil {
		return vector.Vec3{}, err
	}
	return st.Pos, nil
}

// Velocity is a convenience helper that returns the geocentric velocity
// of a body at time t.
func Velocity(p Provider, id ID, t time.Time) (vector.Vec3, error) {
	st, err := p.State(id, t)
	if err != nil {
		return vector.Vec3{}, err
	}
	return st.Vel, nil
}

// ApparentState calculates the rigorously light-time delayed (retarded) geometric state
// of a target by repeatedly polling the ephemeris Provider at (t - tau).
// To satisfy classical planetary aberration, it strictly couples the true orbital curve
// via the provider rather than relying on linear vector shortcuts.
func ApparentState(p Provider, target ID, obsTime time.Time) (State, error) {
	st, err := p.State(target, obsTime)
	if err != nil {
		return State{}, err
	}

	tauDays := st.Pos.Norm() / 173.144632674
	for j := 0; j < 5; j++ {
		retardedTime := obsTime.AddDays(-tauDays)
		st, err = p.State(target, retardedTime)
		if err != nil {
			return State{}, err
		}
		tauDays = st.Pos.Norm() / 173.144632674
	}

	return st, nil
}

// ToICRS converts a geocentric Cartesian vector (in AU) to spherical ICRS coordinates.
// It assumes the input vector is already in an ICRS-compatible inertial frame.
func ToICRS(pos vector.Vec3) (*coord.ICRS, error) {
	r := math.Sqrt(pos.X*pos.X + pos.Y*pos.Y + pos.Z*pos.Z)
	if r < 1e-12 { // Avoid division by zero for very small or zero vectors
		return nil, errors.New("ephemeris: cannot convert near-zero vector to ICRS")
	}

	ra := math.Atan2(pos.Y, pos.X)
	dec := math.Asin(pos.Z / r)

	return coord.NewICRS(angle.Rad(ra).Wrap2Pi(), angle.Rad(dec)), nil
}

type sofaProvider struct{}

func (s *sofaProvider) State(id ID, t time.Time) (State, error) {
	tdb := t.TDB()
	d1, d2 := tdb.JDParts()
	switch id {
	case Sun:
		// Epv00 returns Earth heliocentric position/velocity.
		// Sun geocentric = -Earth heliocentric.
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return State{}, errors.New("ephemeris: sofa epv00 failed")
		}
		ph := pvh[0]
		vh := pvh[1]
		return State{
			Pos: vector.Vec3{X: -ph[0], Y: -ph[1], Z: -ph[2]},
			Vel: vector.Vec3{X: -vh[0], Y: -vh[1], Z: -vh[2]},
		}, nil

	case Moon:
		// Moon98 returns Moon geocentric position/velocity relative to GCRS (ICRS-like).
		pv := gofaext.Moon98(d1, d2)
		return State{
			Pos: vector.Vec3{X: pv[0][0], Y: pv[0][1], Z: pv[0][2]},
			Vel: vector.Vec3{X: pv[1][0], Y: pv[1][1], Z: pv[1][2]},
		}, nil

	default:
		return State{}, errors.New("ephemeris: unsupported body for sofa provider")
	}
}
