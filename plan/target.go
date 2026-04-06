package plan

import (
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
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

// NewFixed creates a new Observable for a fixed catalog
func NewFixed(obj catalog.Target) Fixed {
	return Fixed{Object: obj}
}

// Fixed is an Observable wrapper around a catalog.Target.
type Fixed struct {
	Object catalog.Target
}

// Name returns the object name from the catalog.
func (f Fixed) Name() string {
	return f.Object.Name
}

// Position returns the fixed ICRS coordinates from the catalog object.
func (f Fixed) Position(_ time.Time) (*coord.ICRS, error) {
	if f.Object.Coord == nil {
		return coord.NewICRS(angle.Rad(0), angle.Rad(0)), nil
	}
	return f.Object.Coord, nil
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
func (c Custom) Position(_ time.Time) (*coord.ICRS, error) {
	if c.Coord == nil {
		return coord.NewICRS(angle.Rad(0), angle.Rad(0)), nil
	}
	return c.Coord, nil
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
