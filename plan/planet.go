package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	mag "github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Planet represents a solar system body with an ephemeris provider
// (Sun, Moon, Mercury–Pluto).
type Planet struct {
	provider eph.Provider
	name     string
	id       eph.ID
}

// NewPlanet creates a planet target.
func NewPlanet(name string, id eph.ID, provider eph.Provider) *Planet {
	return &Planet{name: name, id: id, provider: provider}
}

// Convenience constructors for the major bodies.

// NewSun creates a Sun target.
func NewSun(provider eph.Provider) *Planet {
	return NewPlanet("Sun", eph.Sun, provider)
}

// NewMoon creates a Moon target.
func NewMoon(provider eph.Provider) *Planet {
	return NewPlanet("Moon", eph.Moon, provider)
}

// NewMercury creates a Mercury target.
func NewMercury(provider eph.Provider) *Planet {
	return NewPlanet("Mercury", eph.Mercury, provider)
}

// NewVenus creates a Venus target.
func NewVenus(provider eph.Provider) *Planet {
	return NewPlanet("Venus", eph.Venus, provider)
}

// NewEarth creates an Earth target.
func NewEarth(provider eph.Provider) *Planet {
	return NewPlanet("Earth", eph.Earth, provider)
}

// NewMars creates a Mars target.
func NewMars(provider eph.Provider) *Planet {
	return NewPlanet("Mars", eph.Mars, provider)
}

// NewJupiter creates a Jupiter target.
func NewJupiter(provider eph.Provider) *Planet {
	return NewPlanet("Jupiter", eph.Jupiter, provider)
}

// NewSaturn creates a Saturn target.
func NewSaturn(provider eph.Provider) *Planet {
	return NewPlanet("Saturn", eph.Saturn, provider)
}

// NewUranus creates an Uranus target.
func NewUranus(provider eph.Provider) *Planet {
	return NewPlanet("Uranus", eph.Uranus, provider)
}

// NewNeptune creates a Neptune target.
func NewNeptune(provider eph.Provider) *Planet {
	return NewPlanet("Neptune", eph.Neptune, provider)
}

// NewPluto creates a Pluto target.
func NewPluto(provider eph.Provider) *Planet {
	return NewPlanet("Pluto", eph.Pluto, provider)
}

// Name returns the planet's name.
func (p *Planet) Name() string {
	return p.name
}

// Provider returns the planet's ephemeris provider.
func (p *Planet) Provider() eph.Provider {
	return p.provider
}

// EphID returns the planet's ephemeris ID.
func (p *Planet) EphID() eph.ID {
	return p.id
}

// Position returns the planet's position.
func (p *Planet) Position(t time.Time) (coord.ICRS, error) {
	pos, err := eph.Position(p.provider, p.id, t)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("planet: ephemeris error for %s: %w", p.name, err)
	}

	icrs, err := eph.ToICRS(pos)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("planet: coordinate conversion error for %s: %w", p.name, err)
	}

	return icrs, nil
}

// GeocentricVec returns the planet's geocentric position.
func (p *Planet) GeocentricVec(t time.Time) (vector.Vec3, error) {
	v, err := eph.Position(p.provider, p.id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("planet: geocentric: %w", err)
	}

	return v, nil
}

// GetDetails returns the target details.
func (p *Planet) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(p, ctx, props...)
}

// ApparentMagnitude returns the Mallama & Hilton (2018) apparent magnitude.
func (p *Planet) ApparentMagnitude(t time.Time) (float64, error) {
	m, err := mag.PlanetApparent(p.provider, p.id, t)
	if err != nil {
		return 0, fmt.Errorf("planet: apparent magnitude: %w", err)
	}

	return m, nil
}

// ApparentMagnitudeCtx returns apparent magnitude (planets don't need atmospheric context).
func (p *Planet) ApparentMagnitudeCtx(t time.Time, _ *coord.Context) (float64, error) {
	return p.ApparentMagnitude(t)
}

// IsSun returns true if this planet is the Sun.
func (p *Planet) IsSun() bool { return p.id == eph.Sun }

// IsMoon returns true if this planet is the Moon.
func (p *Planet) IsMoon() bool { return p.id == eph.Moon }
