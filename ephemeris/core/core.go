// Package core provides shared types for the ephemeris package hierarchy.
//
// This package serves the same architectural role as catalog/resolve:
// it defines the interfaces and value types that both the root ephemeris
// package and its subpackages (jpl, satellite) depend on, breaking
// what would otherwise be a circular import.
//
// Users should import "github.com/TuSKan/astrogo/ephemeris" rather
// than this package directly — all exported symbols are re-exported
// there.
package core

import (
	"fmt"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// ─── Provider ────────────────────────────────────────────────────────────────

// State represents the kinematic state of a celestial body.
type State struct {
	Pos vector.Vec3 // Geocentric position in AU (ICRS-like)
	Vel vector.Vec3 // Geocentric velocity in AU/day (ICRS-like)
}

const kmPerAU = 149597870.7

// Distance returns the geocentric distance in AU.
func (s State) Distance() float64 { return s.Pos.Norm() }

// DistanceKm returns the geocentric distance in kilometres.
func (s State) DistanceKm() float64 { return s.Pos.Norm() * kmPerAU }

// Speed returns the velocity magnitude in AU/day.
func (s State) Speed() float64 { return s.Vel.Norm() }

// Provider is the interface for celestial ephemeris sources.
type Provider interface {
	// State returns the geocentric state (position and velocity) of the given
	// body at time t. The vectors are typically in an inertial frame like ICRS.
	State(id ID, t time.Time) (State, error)

	// Close releases any resources held by the provider (files, caches).
	// Providers with no resources may return nil.
	Close() error
}

// ─── Source ──────────────────────────────────────────────────────────────────

// Source defines the type of ephemeris data source.
type Source string

const (
	Planets    Source = "planets"    // JPL DE planetary ephemeris
	SmallBody  Source = "smallbody"  // JPL small-body SPK (generic query)
	Asteroids  Source = "asteroids"  // JPL asteroid SPK
	Comets     Source = "comets"     // JPL comet SPK
	Satellites Source = "satellites" // Artificial satellites (NORAD TLE/GP → SGP4)
	Stations   Source = "stations"   // Ground stations (reserved)
)

// ─── Body ID ─────────────────────────────────────────────────────────────────

// ID identifies a major Solar System body or a generic celestial object.
type ID uint32

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

// ─── Kind & Body ─────────────────────────────────────────────────────────────

// Kind identifies the category of a celestial ephemeris.
type Kind uint8

const (
	KindStar Kind = iota + 1
	KindPlanet
	KindMoon
	KindMinorBody
	KindComet
	KindBarycenter
	KindSatellite
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
