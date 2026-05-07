package plan

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Observable represents anything that can appear on the sky at a given time.
// It provides a unified abstraction for celestial objects.
type Observable interface {
	// Name returns the display name
	Name() string
	// Position returns the ICRS coordinates of the target at the given time.
	// For fixed targets, time may be ignored. For moving targets, time is required.
	Position(t time.Time) (coord.ICRS, error)
	// GetDetails retrieves comprehensive properties about the observable at the given context.
	GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error)
}

// Target unifies all celestial objects (Deep Space, Planets, Satellites) into one struct.
type Target struct {
	Catalog  catalog.Target
	Provider eph.Provider
}

// NewTarget creates a new Observable Target.
// For fixed targets, prov can be nil.
// For moving targets (planets, satellites), prov is required.
//
// If the catalog entry carries a non-zero Coord but HasCoord is false,
// NewTarget sets HasCoord = true automatically so that callers who
// construct a catalog.Target inline (e.g. in tests) don't need to
// remember the flag.
func NewTarget(c catalog.Target, p eph.Provider) Target {
	if !c.HasCoord && (c.Coord.RA().Radians() != 0 || c.Coord.Dec().Radians() != 0) {
		c.HasCoord = true
	}
	return Target{Catalog: c, Provider: p}
}

// Name returns the target's name.
func (t Target) Name() string {
	return t.Catalog.Name
}

// Position returns the ICRS coordinates of the target.
func (t Target) Position(time time.Time) (coord.ICRS, error) {
	if t.Provider != nil {
		// Moving object
		idStr := t.Catalog.ID
		if idStr == "" {
			return coord.ICRS{}, errors.New("moving target requires a valid Catalog.ID")
		}
		idUint, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			return coord.ICRS{}, fmt.Errorf("invalid ID for moving target: %w", err)
		}
		ephID := eph.ID(idUint)

		pos, err := eph.Position(t.Provider, ephID, time)
		if err != nil {
			return coord.ICRS{}, fmt.Errorf("target: ephemeris error for %s: %w", t.Name(), err)
		}
		icrs, err := eph.ToICRS(pos)
		if err != nil {
			return coord.ICRS{}, fmt.Errorf("target: coordinate conversion error for %s: %w", t.Name(), err)
		}
		return icrs, nil
	}

	// Fixed object
	if !t.Catalog.HasCoord {
		return coord.NewICRS(angle.Rad(0), angle.Rad(0)), nil
	}

	hasPM := t.Catalog.PmRA.Radians() != 0 || t.Catalog.PmDec.Radians() != 0
	hasParallax := t.Catalog.Parallax.Radians() != 0
	if hasPM || hasParallax {
		return coord.NewICRSWithKinematics(
			t.Catalog.Coord.RA(), t.Catalog.Coord.Dec(),
			t.Catalog.PmRA, t.Catalog.PmDec,
			t.Catalog.Parallax,
			t.Catalog.RadialVelocity,
		), nil
	}

	return coord.NewICRS(t.Catalog.Coord.RA(), t.Catalog.Coord.Dec()), nil
}

// GeocentricVec returns the raw geocentric position vector (in AU).
// This is used by the event solver to route through the topocentric reduction
// pipeline (coord.Reducer), which applies proper parallax correction.
func (t Target) GeocentricVec(time time.Time) (vector.Vec3, error) {
	if t.Provider == nil {
		return vector.Vec3{}, errors.New("target: not a moving body")
	}
	id, ok := t.ephID()
	if !ok {
		return vector.Vec3{}, errors.New("moving target requires a valid numeric Catalog.ID")
	}
	return eph.Position(t.Provider, id, time)
}

// ephID parses the catalog ID into an ephemeris body identifier.
func (t Target) ephID() (eph.ID, bool) {
	n, err := strconv.ParseUint(t.Catalog.ID, 10, 32)
	if err != nil {
		return 0, false
	}
	return eph.ID(n), true
}

// GetDetails computes properties for the Target.
func (t Target) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(t, ctx, props...)
}

func NewSun(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "11", Name: "Sun", Kind: resolve.KindStar}, provider)
}

func NewMoon(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "10", Name: "Moon", Kind: resolve.KindMoon}, provider)
}

func NewMercury(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "1", Name: "Mercury", Kind: resolve.KindPlanet}, provider)
}

func NewVenus(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "2", Name: "Venus", Kind: resolve.KindPlanet}, provider)
}

func NewEarth(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "3", Name: "Earth", Kind: resolve.KindPlanet}, provider)
}

func NewMars(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "4", Name: "Mars", Kind: resolve.KindPlanet}, provider)
}

func NewJupiter(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "5", Name: "Jupiter", Kind: resolve.KindPlanet}, provider)
}

func NewSaturn(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "6", Name: "Saturn", Kind: resolve.KindPlanet}, provider)
}

func NewUranus(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "7", Name: "Uranus", Kind: resolve.KindPlanet}, provider)
}

func NewNeptune(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "8", Name: "Neptune", Kind: resolve.KindPlanet}, provider)
}

func NewPluto(provider eph.Provider) Target {
	return NewTarget(catalog.Target{ID: "9", Name: "Pluto", Kind: resolve.KindPlanet}, provider)
}
