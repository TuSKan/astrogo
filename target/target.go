package target

import (
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// Observable represents anything that can appear on the sky at a given time.
// It provides a unified abstraction for fixed celestial objects, moving solar system
// bodies, and custom user-defined coordinates.
type Observable interface {
	// Name returns the display name of the target.
	Name() string
	// Position returns the ICRS coordinates of the target at the given time.
	// For fixed targets, time may be ignored. For moving targets, time is required.
	Position(t time.Time) (coord.ICRS, error)
}

// NewFixed creates a new Observable for a fixed catalog target.
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
func (f Fixed) Position(_ time.Time) (coord.ICRS, error) {
	return f.Object.Coord, nil
}

// Custom is an Observable that represents an arbitrary fixed coordinate.
type Custom struct {
	Label string
	Coord coord.ICRS
}

// Name returns the label, or "Custom" if empty.
func (c Custom) Name() string {
	if c.Label == "" {
		return "Custom"
	}
	return c.Label
}

// Position returns the stored fixed coordinate.
func (c Custom) Position(_ time.Time) (coord.ICRS, error) {
	return c.Coord, nil
}

// NewBody creates a new moving target using the provided ephemeris provider.
func NewBody(id body.ID, p ephemeris.Provider) Body {
	return Body{ID: id, Provider: p}
}

// NewDefaultBody creates a new moving target using the default ephemeris provider.
func NewDefaultBody(id body.ID) Body {
	return Body{ID: id, Provider: ephemeris.Default()}
}

// Body is an Observable that represents a moving solar-system body.
// It uses an ephemeris.Provider to compute coordinates at a given time.
type Body struct {
	ID       body.ID
	Provider ephemeris.Provider
}

// Name returns the conventional name of the solar-system body.
func (b Body) Name() string {
	return b.ID.String()
}

// Position returns the geocentric ICRS coordinates of the body at time t.
func (b Body) Position(t time.Time) (coord.ICRS, error) {
	if b.Provider == nil {
		return coord.ICRS{}, errors.New("target: nil ephemeris provider")
	}

	// Obtain the geocentric position vector (in AU).
	pos, err := ephemeris.Position(b.Provider, b.ID, t)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("target: ephemeris error for %s: %w", b.Name(), err)
	}

	// Convert the position vector into coord.ICRS.
	icrs, err := ephemeris.ToICRS(pos)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("target: coordinate conversion error for %s: %w", b.Name(), err)
	}

	return icrs, nil
}
