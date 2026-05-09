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

func NewSun(provider eph.Provider) *Planet     { return NewPlanet("Sun", eph.Sun, provider) }
func NewMoon(provider eph.Provider) *Planet    { return NewPlanet("Moon", eph.Moon, provider) }
func NewMercury(provider eph.Provider) *Planet { return NewPlanet("Mercury", eph.Mercury, provider) }
func NewVenus(provider eph.Provider) *Planet   { return NewPlanet("Venus", eph.Venus, provider) }
func NewEarth(provider eph.Provider) *Planet   { return NewPlanet("Earth", eph.Earth, provider) }
func NewMars(provider eph.Provider) *Planet    { return NewPlanet("Mars", eph.Mars, provider) }
func NewJupiter(provider eph.Provider) *Planet { return NewPlanet("Jupiter", eph.Jupiter, provider) }
func NewSaturn(provider eph.Provider) *Planet  { return NewPlanet("Saturn", eph.Saturn, provider) }
func NewUranus(provider eph.Provider) *Planet  { return NewPlanet("Uranus", eph.Uranus, provider) }
func NewNeptune(provider eph.Provider) *Planet { return NewPlanet("Neptune", eph.Neptune, provider) }
func NewPluto(provider eph.Provider) *Planet   { return NewPlanet("Pluto", eph.Pluto, provider) }

func (p *Planet) Name() string           { return p.name }
func (p *Planet) Provider() eph.Provider { return p.provider }
func (p *Planet) EphID() eph.ID          { return p.id }

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

func (p *Planet) GeocentricVec(t time.Time) (vector.Vec3, error) {
	return eph.Position(p.provider, p.id, t)
}

func (p *Planet) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(p, ctx, props...)
}

// ApparentMagnitude returns the Mallama & Hilton (2018) apparent magnitude.
func (p *Planet) ApparentMagnitude(t time.Time) (float64, error) {
	return mag.PlanetApparent(p.provider, p.id, t)
}

// ApparentMagnitudeCtx returns apparent magnitude (planets don't need atmospheric context).
func (p *Planet) ApparentMagnitudeCtx(t time.Time, _ *coord.Context) (float64, error) {
	return p.ApparentMagnitude(t)
}

// IsSun returns true if this planet is the Sun.
func (p *Planet) IsSun() bool { return p.id == eph.Sun }

// IsMoon returns true if this planet is the Moon.
func (p *Planet) IsMoon() bool { return p.id == eph.Moon }
