package plan

import (
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/openngc"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Observable represents anything that can appear on the sky at a given time.
// It provides a unified abstraction for fixed celestial objects, moving solar system
// bodies, and custom user-defined coordinates.
type Observable interface {
	// Name returns the display name of the
	Name() string
	// Position returns the ICRS coordinates of the target at the given time.
	// For fixed targets, time may be ignored. For moving targets, time is required.
	Position(t time.Time) (*coord.ICRS, error)
}

// NewFixed is a legacy wrapper for NewDeepSpace.
func NewFixed(obj catalog.Target) DeepSpace {
	return NewDeepSpace(obj)
}

// NewDeepSpace creates a new Observable for a deep space target
// (e.g. Star, Galaxy, Nebula), which automatically propagates proper motion.
func NewDeepSpace(obj catalog.Target) DeepSpace {
	return DeepSpace{Object: obj}
}

// NewDefaultDeepSpace creates a new Observable for a deep space target using
// the default OpenNGC provider.
func NewDefaultDeepSpace(name string) (DeepSpace, error) {
	provider := openngc.New()
	obj, ok := provider.Resolve(name)
	if !ok {
		return DeepSpace{}, fmt.Errorf("target: %s not found in default catalog (OpenNGC)", name)
	}
	return NewDeepSpace(obj), nil
}

// DeepSpace is an Observable wrapper around a catalog.Target that propagates kinematics.
type DeepSpace struct {
	Object catalog.Target
}

// Name returns the object name from the catalog.
func (f DeepSpace) Name() string {
	return f.Object.Name
}

// Position returns the ICRS coordinates from the catalog object, including
// any stellar kinematics (proper motion, parallax, radial velocity).
// The coordinates are returned at the J2000 catalog epoch; SOFA handles
// rigorous space-motion propagation to the observation epoch internally.
func (f DeepSpace) Position(t time.Time) (*coord.ICRS, error) {
	if f.Object.Coord == nil {
		return coord.NewICRS(angle.Rad(0), angle.Rad(0)), nil
	}

	// If the target has kinematics, attach them to the ICRS so SOFA
	// can apply rigorous space-motion propagation via Atcoq/Atciq.
	hasPM := f.Object.PmRA.Radians() != 0 || f.Object.PmDec.Radians() != 0
	hasParallax := f.Object.Parallax.Radians() != 0
	if hasPM || hasParallax {
		return coord.NewICRSWithKinematics(
			f.Object.Coord.RA(), f.Object.Coord.Dec(),
			f.Object.PmRA, f.Object.PmDec,
			f.Object.Parallax,
			f.Object.RadialVelocity,
		), nil
	}

	// No kinematics — return a defensive copy so callers cannot mutate
	// the underlying catalog entry.
	return coord.NewICRS(f.Object.Coord.RA(), f.Object.Coord.Dec()), nil
}

// Custom is an Observable that represents an arbitrary fixed coordinate.
type Custom struct {
	Label string
	Coord *coord.ICRS
}

// Name returns the label, or "Custom" if empty.
func (c Custom) Name() string {
	if c.Label == "" {
		return "Custom"
	}
	return c.Label
}

// Position returns the stored fixed coordinate.
// Returns a defensive copy so callers cannot mutate the original.
func (c Custom) Position(_ time.Time) (*coord.ICRS, error) {
	if c.Coord == nil {
		return coord.NewICRS(angle.Rad(0), angle.Rad(0)), nil
	}
	return coord.NewICRS(c.Coord.RA(), c.Coord.Dec()), nil
}

// NewBody creates a new moving target using the provided ephemeris provider.
func NewBody(id ephemeris.ID, p ephemeris.Provider) Body {
	return Body{ID: id, Provider: p}
}

// NewDefaultBody creates a new moving target using the default ephemeris provider.
func NewDefaultBody(id ephemeris.ID) Body {
	return Body{ID: id, Provider: ephemeris.Default()}
}

// Body is an Observable that represents a moving solar-system ephemeris.
// It uses an ephemeris.Provider to compute coordinates at a given time.
type Body struct {
	ID       ephemeris.ID
	Provider ephemeris.Provider
}

// Name returns the conventional name of the solar-system ephemeris.
func (b Body) Name() string {
	return b.ID.String()
}

// Position returns the geocentric ICRS coordinates of the body at time t.
func (b Body) Position(t time.Time) (*coord.ICRS, error) {
	if b.Provider == nil {
		return nil, errors.New("target: nil ephemeris provider")
	}

	// Obtain the geocentric position vector (in AU).
	pos, err := ephemeris.Position(b.Provider, b.ID, t)
	if err != nil {
		return nil, fmt.Errorf("target: ephemeris error for %s: %w", b.Name(), err)
	}

	// Convert the position vector into coord.ICRS.
	icrs, err := ephemeris.ToICRS(pos)
	if err != nil {
		return nil, fmt.Errorf("target: coordinate conversion error for %s: %w", b.Name(), err)
	}

	return icrs, nil
}

// GeocentricVec returns the raw geocentric position vector (in AU) for the body.
// This is used by the event solver to route through the topocentric reduction
// pipeline (coord.Reducer), which applies proper parallax correction — critical
// for the Moon (~1° topocentric parallax).
func (b Body) GeocentricVec(t time.Time) (vector.Vec3, error) {
	if b.Provider == nil {
		return vector.Vec3{}, errors.New("target: nil ephemeris provider")
	}
	return ephemeris.Position(b.Provider, b.ID, t)
}
