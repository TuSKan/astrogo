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
	// Planets is the source type for planetary ephemeris.
	Planets Source = "planets" // JPL DE planetary ephemeris
	// SmallBody is the source type for small-body ephemeris.
	SmallBody Source = "smallbody" // JPL small-body SPK (generic query)
	// Asteroids is the source type for asteroid ephemeris.
	Asteroids Source = "asteroids" // JPL asteroid SPK
	// Comets is the source type for comet ephemeris.
	Comets Source = "comets" // JPL comet SPK
	// Satellites is the source type for satellite ephemeris.
	Satellites Source = "satellites" // Artificial satellites (NORAD TLE/GP → SGP4)
	// Stations is the source type for ground station ephemeris.
	Stations Source = "stations" // Ground stations (reserved)
)

// ─── Body ID ─────────────────────────────────────────────────────────────────

// ID identifies a major Solar System body or a generic celestial object.
type ID uint32

const (
	// Mercury is the identifier for Mercury.
	Mercury ID = iota + 1
	// Venus is the identifier for Venus.
	Venus
	// Earth is the identifier for Earth.
	Earth
	// Mars is the identifier for Mars.
	Mars
	// Jupiter is the identifier for Jupiter.
	Jupiter
	// Saturn is the identifier for Saturn.
	Saturn
	// Uranus is the identifier for Uranus.
	Uranus
	// Neptune is the identifier for Neptune.
	Neptune
	// Pluto is the identifier for Pluto.
	Pluto
	// Moon is the identifier for the Moon.
	Moon
	// Sun is the identifier for the Sun.
	Sun
	// SolarSystemBarycenter is the identifier for the solar system barycenter.
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
	// KindStar is the kind of a star.
	KindStar Kind = iota + 1
	// KindPlanet is the kind of a planet.
	KindPlanet
	// KindMoon is the kind of a moon.
	KindMoon
	// KindMinorBody is the kind of a minor body.
	KindMinorBody
	// KindComet is the kind of a comet.
	KindComet
	// KindBarycenter is the kind of a barycenter.
	KindBarycenter
	// KindSatellite is the kind of a satellite.
	KindSatellite
)

// Body represents a named celestial body and its category.
type Body struct {
	// Name is the name of the body.
	Name string
	// ID is the identifier of the body.
	ID ID
	// Kind is the kind of the body.
	Kind Kind
}

// Built-in major bodies.
//
//nolint:gochecknoglobals // IAU body registry — immutable catalog data
var (
	// SunBody is the Sun body.
	SunBody = Body{ID: Sun, Name: "Sun", Kind: KindStar}
	// MoonBody is the Moon body.
	MoonBody = Body{ID: Moon, Name: "Moon", Kind: KindMoon}
	// MercuryBody is the Mercury body.
	MercuryBody = Body{ID: Mercury, Name: "Mercury", Kind: KindPlanet}
	// VenusBody is the Venus body.
	VenusBody = Body{ID: Venus, Name: "Venus", Kind: KindPlanet}
	// EarthBody is the Earth body.
	EarthBody = Body{ID: Earth, Name: "Earth", Kind: KindPlanet}
	// MarsBody is the Mars body.
	MarsBody = Body{ID: Mars, Name: "Mars", Kind: KindPlanet}
	// JupiterBody is the Jupiter body.
	JupiterBody = Body{ID: Jupiter, Name: "Jupiter", Kind: KindPlanet}
	// SaturnBody is the Saturn body.
	SaturnBody = Body{ID: Saturn, Name: "Saturn", Kind: KindPlanet}
	// UranusBody is the Uranus body.
	UranusBody = Body{ID: Uranus, Name: "Uranus", Kind: KindPlanet}
	// NeptuneBody is the Neptune body.
	NeptuneBody = Body{ID: Neptune, Name: "Neptune", Kind: KindPlanet}
)

// Bodies is a utility list of all major Solar System bodies as concrete structs.
//
//nolint:gochecknoglobals // IAU body registry — immutable catalog data
var Bodies = []Body{
	SunBody, MoonBody, MercuryBody, VenusBody, EarthBody,
	MarsBody, JupiterBody, SaturnBody, UranusBody, NeptuneBody,
}
