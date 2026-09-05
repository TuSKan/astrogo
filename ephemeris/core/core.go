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
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Errors reported by [State.Require].
var (
	// ErrWrongFrame marks a state expressed in a different reference frame
	// from the one the caller needs.
	ErrWrongFrame = errors.New("ephemeris: state is in the wrong reference frame")

	// ErrWrongCenter marks a state measured from a different origin.
	ErrWrongCenter = errors.New("ephemeris: state is measured from the wrong origin")
)

// ─── Provider ────────────────────────────────────────────────────────────────

// Frame names the reference frame a [State]'s vectors are expressed in.
//
// It exists because "ICRS-like" is not a specification. That was the whole of
// the frame contract on State until this type: a comment, hedged, on a struct
// that several providers fill in differently. [github.com/TuSKan/astrogo/ephemeris/jpl]
// and the SOFA analytical provider produce ICRS; the SGP4 provider in
// ephemeris/satellite converts TEME to GCRS and produces that. Those differ by
// frame bias — about 23 milliarcseconds — and nothing in the type distinguished
// them, so a value from one could be used where the other was meant and remain
// mathematically valid while being physically wrong.
type Frame uint8

// The frames astrogo's providers actually produce.
const (
	// FrameUnspecified is the zero value, and asserts nothing.
	//
	// Deliberately not ICRS: a State that nobody labelled must not claim a
	// frame it was never checked against. A caller that needs certainty asks
	// with [State.Require] and gets an error rather than a guess.
	FrameUnspecified Frame = iota

	// FrameICRS is the International Celestial Reference System, the frame
	// JPL kernels and the SOFA analytical series are expressed in.
	FrameICRS

	// FrameGCRS is the Geocentric Celestial Reference System, which
	// ephemeris/satellite produces after converting SGP4's TEME output.
	FrameGCRS

	// FrameITRS is the International Terrestrial Reference System, which
	// rotates with the Earth.
	FrameITRS

	// FrameTEME is True Equator Mean Equinox, SGP4's own output frame.
	FrameTEME
)

func (f Frame) String() string {
	switch f {
	case FrameUnspecified:
		return "unspecified"
	case FrameICRS:
		return "ICRS"
	case FrameGCRS:
		return "GCRS"
	case FrameITRS:
		return "ITRS"
	case FrameTEME:
		return "TEME"
	default:
		return fmt.Sprintf("Frame(%d)", uint8(f))
	}
}

// Center names the origin a [State]'s vectors are measured from.
type Center uint8

// The origins astrogo's providers actually use.
const (
	// CenterUnspecified is the zero value, and asserts nothing. See
	// [FrameUnspecified] for why that is not the same as assuming a default.
	CenterUnspecified Center = iota

	// CenterGeocentre is the centre of the Earth.
	CenterGeocentre

	// CenterBarycentre is the solar system barycentre.
	CenterBarycentre

	// CenterHeliocentre is the centre of the Sun.
	CenterHeliocentre
)

func (c Center) String() string {
	switch c {
	case CenterUnspecified:
		return "unspecified"
	case CenterGeocentre:
		return "geocentre"
	case CenterBarycentre:
		return "barycentre"
	case CenterHeliocentre:
		return "heliocentre"
	default:
		return fmt.Sprintf("Center(%d)", uint8(c))
	}
}

// State represents the kinematic state of a celestial body.
//
// Units stay contractual — AU and AU/day — because putting them in the type
// would cost the hot path more than it is worth. Frame and origin do not:
// they are one byte each, they are read once at a boundary rather than in a
// loop, and getting them wrong produces a number that looks entirely
// reasonable.
type State struct {
	Pos vector.Vec3 // position in AU, in Frame, measured from Center
	Vel vector.Vec3 // velocity in AU/day, in Frame, measured from Center

	// Frame and Center describe the vectors above. Both zero values assert
	// nothing rather than defaulting, so a provider that has not been taught
	// to label its output says so instead of claiming ICRS geocentric.
	Frame  Frame
	Center Center
}

// Require reports whether the state is expressed in the frame and origin the
// caller needs, and returns an error naming the mismatch when it is not.
//
// An unspecified frame or centre passes: this is a check against a wrong
// label, not a demand that every producer be labelled. Tightening it to
// reject unspecified would turn "we do not know" into a failure at every call
// site that has not been updated, which is a migration and not a safeguard.
func (s State) Require(frame Frame, center Center) error {
	if frame != FrameUnspecified && s.Frame != FrameUnspecified && s.Frame != frame {
		return fmt.Errorf("%w: state is %s, caller needs %s", ErrWrongFrame, s.Frame, frame)
	}

	if center != CenterUnspecified && s.Center != CenterUnspecified && s.Center != center {
		return fmt.Errorf("%w: state is %s, caller needs %s", ErrWrongCenter, s.Center, center)
	}

	return nil
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
	// Moons is the source type for natural planetary satellite ephemeris —
	// NAIF's per-planet SPK kernels (e.g. jup365.bsp, sat441.bsp), distinct
	// from Satellites above (which is artificial, TLE/SGP4-based).
	Moons Source = "moons" // JPL/NAIF planetary satellite SPK
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

// SmallBodyBase is the offset that separates numbered small bodies from the
// named ones, and is NAIF's own convention for asteroid SPK IDs.
//
// It exists because the two namespaces used to overlap. A small body was
// identified by its bare number, so asteroid 4 Vesta and [Mars] were both
// ID 4 — as were Ceres and [Mercury], Pallas and [Venus], Juno and [Earth],
// and every asteroid numbered up to [SolarSystemBarycenter] at 12. A provider
// holding a planetary kernel resolved the planet and silently dropped the
// asteroid, which took four of the five largest asteroids out of reach with
// nothing to say why.
//
// Untyped on purpose: it is an offset within the ID space rather than a body,
// so it has no place among the values a switch over an ID should handle.
const SmallBodyBase = 20000000

// smallBodySpan bounds the small-body block, matching NAIF's own allocation.
const smallBodySpan = 1000000

// SmallBodyID returns the identifier for the numbered small body n — 433 for
// Eros, 3200 for Phaethon — placed clear of the named bodies.
//
// This is the form [Provider.SupportedBodies] reports. A bare core.ID(433)
// still resolves, so existing callers keep working, but it cannot be
// enumerated without ambiguity and is not what a new caller should write.
//
// Returns 0 for an n outside NAIF's small-body block, which is not a valid
// body identifier and will fail to resolve rather than aliasing another body.
func SmallBodyID(n int) ID {
	if n <= 0 || n >= smallBodySpan {
		return 0
	}

	return SmallBodyBase + ID(n)
}

// SmallBodyNumber reports the small-body number this ID stands for, and
// whether it is one at all.
func (id ID) SmallBodyNumber() (int, bool) {
	if id <= SmallBodyBase || id >= SmallBodyBase+smallBodySpan {
		return 0, false
	}

	return int(id - SmallBodyBase), true
}

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
		if n, ok := id.SmallBodyNumber(); ok {
			return fmt.Sprintf("SmallBody(%d)", n)
		}

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
var Bodies = []Body{
	SunBody, MoonBody, MercuryBody, VenusBody, EarthBody,
	MarsBody, JupiterBody, SaturnBody, UranusBody, NeptuneBody,
}
