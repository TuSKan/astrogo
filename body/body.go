package body

import "fmt"

// ID identifies a major Solar System body or a generic celestial object.
type ID uint8

const (
	Mercury ID = iota + 1
	Venus
	Earth
	Mars
	Jupiter
	Saturn
	Uranus
	Neptune
	Pluto
	Moon
	Sun
	SolarSystemBarycenter
)

// String returns the conventional name of the body identifier.
func (id ID) String() string {
	switch id {
	case Mercury:
		return "Mercury"
	case Venus:
		return "Venus"
	case Earth:
		return "Earth"
	case Mars:
		return "Mars"
	case Jupiter:
		return "Jupiter"
	case Saturn:
		return "Saturn"
	case Uranus:
		return "Uranus"
	case Neptune:
		return "Neptune"
	case Pluto:
		return "Pluto"
	case Moon:
		return "Moon"
	case Sun:
		return "Sun"
	case SolarSystemBarycenter:
		return "SolarSystemBarycenter"
	default:
		return fmt.Sprintf("BodyID(%d)", id)
	}
}

// Kind identifies the category of a celestial body.
type Kind uint8

const (
	KindStar Kind = iota + 1
	KindPlanet
	KindMoon
	KindMinorBody
	KindComet
	KindBarycenter
)

// Body represents a named celestial body and its category.
type Body struct {
	ID   ID
	Name string
	Kind Kind
}

// Built-in major bodies.
var (
	SunBody     = Body{ID: Sun, Name: "Sun", Kind: KindStar}
	MoonBody    = Body{ID: Moon, Name: "Moon", Kind: KindMoon}
	MercuryBody = Body{ID: Mercury, Name: "Mercury", Kind: KindPlanet}
	VenusBody   = Body{ID: Venus, Name: "Venus", Kind: KindPlanet}
	EarthBody   = Body{ID: Earth, Name: "Earth", Kind: KindPlanet}
	MarsBody    = Body{ID: Mars, Name: "Mars", Kind: KindPlanet}
	JupiterBody = Body{ID: Jupiter, Name: "Jupiter", Kind: KindPlanet}
	SaturnBody  = Body{ID: Saturn, Name: "Saturn", Kind: KindPlanet}
	UranusBody  = Body{ID: Uranus, Name: "Uranus", Kind: KindPlanet}
	NeptuneBody = Body{ID: Neptune, Name: "Neptune", Kind: KindPlanet}
)

// Bodies is a utility list of all major Solar System bodies as concrete structs.
var Bodies = []Body{
	SunBody, MoonBody, MercuryBody, VenusBody, EarthBody,
	MarsBody, JupiterBody, SaturnBody, UranusBody, NeptuneBody,
}
