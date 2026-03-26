package ephemeris

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Provider is the interface for celestial ephemeris sources.
type Provider interface {
	// Position returns the geocentric position of the given body at time t
	// as a Cartesian vector in astronomical units (AU) relative to the frame
	// of the provider (typically ICRS/J2000).
	Position(id body.ID, t time.Time) (vector.Vec3, error)

	// Velocity returns the geocentric velocity of the given body at time t
	// in AU/day.
	Velocity(id body.ID, t time.Time) (vector.Vec3, error)
}

// Default returns a SOFA-based ephemeris provider for the Sun and Moon.
func Default() Provider {
	return &sofaProvider{}
}

// Position is a high-level helper that returns the apparent ICRS coordinates
// of a body. In v1, it assumes geocentric positions and ignores topocentric
// parallax unless explicitly extended.
func Position(p Provider, b body.Body, t time.Time, site observatory.Site) (coord.ICRS, error) {
	pos, err := p.Position(b.ID, t)
	if err != nil {
		return coord.ICRS{}, err
	}

	// Convert Cartesian AU to spherical ICRS
	// Rectangular to spherical:
	// x = r * cos(dec) * cos(ra)
	// y = r * cos(dec) * sin(ra)
	// z = r * sin(dec)
	r := math.Sqrt(pos.X*pos.X + pos.Y*pos.Y + pos.Z*pos.Z)
	ra := math.Atan2(pos.Y, pos.X)
	dec := math.Asin(pos.Z / r)

	return coord.ICRS{
		RA:  angle.Rad(ra).Wrap2Pi(),
		Dec: angle.Rad(dec),
	}, nil
}

type sofaProvider struct{}

func (s *sofaProvider) Position(id body.ID, t time.Time) (vector.Vec3, error) {
	d1, d2 := t.JDParts()
	switch id {
	case body.Sun:
		// Epv00 returns Earth heliocentric position.
		// Sun geocentric position = -Earth heliocentric position.
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return vector.Vec3{}, errors.New("ephemeris: sofa epv00 failed")
		}
		ph := pvh[0]
		return vector.Vec3{X: -ph[0], Y: -ph[1], Z: -ph[2]}, nil

	case body.Moon:
		pv := gofaext.Moon98(d1, d2)
		return vector.Vec3{X: pv[0][0], Y: pv[0][1], Z: pv[0][2]}, nil

	default:
		return vector.Vec3{}, errors.New("ephemeris: unsupported body for sofa provider")
	}
}

func (s *sofaProvider) Velocity(id body.ID, t time.Time) (vector.Vec3, error) {
	d1, d2 := t.JDParts()
	switch id {
	case body.Sun:
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return vector.Vec3{}, errors.New("ephemeris: sofa epv00 failed")
		}
		vh := pvh[1]
		return vector.Vec3{X: -vh[0], Y: -vh[1], Z: -vh[2]}, nil

	case body.Moon:
		pv := gofaext.Moon98(d1, d2)
		return vector.Vec3{X: pv[1][0], Y: pv[1][1], Z: pv[1][2]}, nil

	default:
		return vector.Vec3{}, errors.New("ephemeris: unsupported body for sofa provider")
	}
}

// StateVector returns position and velocity in a single call.
func (s *sofaProvider) StateVector(id body.ID, t time.Time) (pos, vel vector.Vec3, err error) {
	pos, err = s.Position(id, t)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}
	vel, err = s.Velocity(id, t)
	return pos, vel, err
}
