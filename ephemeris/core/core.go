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
	"math"

	"github.com/TuSKan/astrogo/constants"
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
	// Deliberately not ICRS: a State that nobody labeled must not claim a
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

	// CenterGeocenter is the center of the Earth.
	CenterGeocenter

	// CenterBarycenter is the solar system barycenter.
	CenterBarycenter

	// CenterHeliocenter is the center of the Sun.
	CenterHeliocenter
)

func (c Center) String() string {
	switch c {
	case CenterUnspecified:
		return "unspecified"
	case CenterGeocenter:
		return "geocenter"
	case CenterBarycenter:
		return "barycenter"
	case CenterHeliocenter:
		return "heliocenter"
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
// An unspecified frame or center passes: this is a check against a wrong
// label, not a demand that every producer be labeled. Tightening it to
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

// kmPerAU is the number of kilometers in one Astronomical Unit.
//
// var, not const: constants.IAU.AstronomicalUnit is a struct, and Go does not
// permit selecting a struct field inside a constant expression.
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

// Distance returns the geocentric distance in AU.
func (s State) Distance() float64 { return s.Pos.Norm() }

// DistanceKm returns the geocentric distance in kilometers.
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

// ID is a NAIF integer code, and nothing else: the identifier JPL's kernels,
// Horizons and SPICE use for every body and point in the Solar System. The
// constants below are names for codes.
//
// It used to hold astrogo's own enumeration, 1 to 12, beside NAIF codes above
// it, and the two overlapped exactly where NAIF puts the system barycenters:
// a provider that did not know an astrogo number read it as a NAIF one, so
// Pluto (astrogo 9) was served as NAIF 9, the Pluto barycenter, by
// coincidence, while the Solar System barycenter (astrogo 12) was refused
// (#809). One integer, one meaning.
//
// # A planet's center and its system's barycenter are different points
//
// NAIF numbers both, and so does astrogo: [Jupiter] is 599, the planet, and
// [JupiterBarycenter] is 5, the barycenter of the planet and its moons. A
// planetary kernel — de440, de441 — holds the barycenters of the giant
// planets, because their satellite systems live in separate kernels (jup365,
// sat441, ura184, nep097), so the center of Jupiter needs one of those loaded
// as well. A provider answers the code it is asked for or refuses it; it
// never answers one for the other. The difference, measured against JPL
// Horizons over 2026:
//
//	Uranus    0.0497 arcsec
//	Jupiter   0.0324 arcsec
//	Saturn    0.0288 arcsec
//	Neptune   0.0093 arcsec
//	Mars      0.0000 arcsec   (Phobos and Deimos displace it by centimeters)
//
// Which one a planner observes is its choice, made visibly: plan's planets
// from Mars out are the barycenters (#253). Mercury and Venus have no moons,
// so their barycenters and their centers are one point under two codes.
//
// # Small bodies and satellites
//
// A numbered small body is [SmallBodyID] n, NAIF's 20000000 + n; an Earth
// satellite is [SatelliteID] of its NORAD catalog number, NAIF's
// −100000 − n. Neither overlaps another body.
type ID int32

// The Solar System's named points, by NAIF code.
const (
	// SolarSystemBarycenter is the barycenter of the Solar System, NAIF 0,
	// the origin every planetary kernel's chains end at.
	SolarSystemBarycenter ID = 0
	// MercuryBarycenter is NAIF 1, the same point as [Mercury].
	MercuryBarycenter ID = 1
	// VenusBarycenter is NAIF 2, the same point as [Venus].
	VenusBarycenter ID = 2
	// EarthMoonBarycenter is NAIF 3, 4,671 km from the Earth's center on
	// average.
	EarthMoonBarycenter ID = 3
	// MarsBarycenter is NAIF 4: Mars and its two moons, centimeters from
	// [Mars].
	MarsBarycenter ID = 4
	// JupiterBarycenter is NAIF 5, about 0.03 arcsec from [Jupiter] as seen
	// from Earth.
	JupiterBarycenter ID = 5
	// SaturnBarycenter is NAIF 6, about 0.03 arcsec from [Saturn].
	SaturnBarycenter ID = 6
	// UranusBarycenter is NAIF 7, about 0.05 arcsec from [Uranus].
	UranusBarycenter ID = 7
	// NeptuneBarycenter is NAIF 8, about 0.01 arcsec from [Neptune].
	NeptuneBarycenter ID = 8
	// PlutoBarycenter is NAIF 9: Pluto and Charon, whose barycenter lies
	// outside Pluto.
	PlutoBarycenter ID = 9
	// Sun is the Sun's center, NAIF 10.
	Sun ID = 10
	// Mercury is Mercury's center, NAIF 199.
	Mercury ID = 199
	// Venus is Venus's center, NAIF 299.
	Venus ID = 299
	// Moon is the Moon's center, NAIF 301.
	Moon ID = 301
	// Earth is the Earth's center, NAIF 399: the geocenter.
	Earth ID = 399
	// Mars is the center of Mars, NAIF 499.
	Mars ID = 499
	// Jupiter is the center of Jupiter, NAIF 599. A planetary kernel alone
	// does not have it; see [ID].
	Jupiter ID = 599
	// Saturn is the center of Saturn, NAIF 699.
	Saturn ID = 699
	// Uranus is the center of Uranus, NAIF 799.
	Uranus ID = 799
	// Neptune is the center of Neptune, NAIF 899.
	Neptune ID = 899
	// Pluto is the center of Pluto, NAIF 999.
	Pluto ID = 999
)

// Body is the center of the planet whose system barycenter id is — Jupiter
// (599) for [JupiterBarycenter] (5) — and id itself for anything else. It is
// for the questions a planet and its barycenter answer alike, its size and
// its brightness, not for placing either.
func (id ID) Body() ID {
	if id >= MercuryBarycenter && id <= PlutoBarycenter {
		return id*100 + 99
	}

	return id
}

// SmallBodyBase is NAIF's offset for numbered small bodies: asteroid n is
// SmallBodyBase + n.
//
// Untyped on purpose: it is an offset within the ID space rather than a body,
// so it has no place among the values a switch over an ID should handle.
const SmallBodyBase = 20000000

// smallBodySpan bounds the small-body block, matching NAIF's own allocation.
const smallBodySpan = 1000000

// SmallBodyID returns the identifier for the numbered small body n — 433 for
// Eros, 3200 for Phaethon — and whether n is a small-body number at all.
//
// It used to return 0 for an n outside NAIF's block, which then failed to
// resolve. Zero is the Solar System barycenter now, so the answer is a bool
// beside the ID rather than a value inside it.
func SmallBodyID(n int) (ID, bool) {
	if n <= 0 || n >= smallBodySpan {
		return 0, false
	}

	return SmallBodyBase + ID(n), true
}

// SmallBodyNumber reports the small-body number this ID stands for, and
// whether it is one at all.
func (id ID) SmallBodyNumber() (int, bool) {
	if id <= SmallBodyBase || id >= SmallBodyBase+smallBodySpan {
		return 0, false
	}

	return int(id - SmallBodyBase), true
}

// satelliteBase is NAIF's offset for an Earth satellite with no DSN code: the
// satellite with NORAD catalog number n is −satelliteBase − n.
const satelliteBase = 100000

// SatelliteID returns the identifier NAIF gives an Earth satellite with
// NORAD catalog number n, −100000 − n — −125544 for the ISS — and whether n
// is a catalog number at all.
func SatelliteID(norad int) (ID, bool) {
	if norad <= 0 || norad > math.MaxInt32-satelliteBase {
		return 0, false
	}

	return ID(-satelliteBase - norad), true
}

// SatelliteNumber reports the NORAD catalog number this ID stands for, and
// whether it is one at all.
func (id ID) SatelliteNumber() (int, bool) {
	if id >= -satelliteBase {
		return 0, false
	}

	return int(-id) - satelliteBase, true
}

// String returns the conventional name of the body identifier.
func (id ID) String() string {
	if name, ok := idNames[id]; ok {
		return name
	}

	if n, ok := id.SmallBodyNumber(); ok {
		return fmt.Sprintf("SmallBody(%d)", n)
	}

	if n, ok := id.SatelliteNumber(); ok {
		return fmt.Sprintf("Satellite(%d)", n)
	}

	return fmt.Sprintf("NAIF(%d)", int32(id))
}

var idNames = map[ID]string{
	SolarSystemBarycenter: "SolarSystemBarycenter",
	MercuryBarycenter:     "MercuryBarycenter",
	VenusBarycenter:       "VenusBarycenter",
	EarthMoonBarycenter:   "EarthMoonBarycenter",
	MarsBarycenter:        "MarsBarycenter",
	JupiterBarycenter:     "JupiterBarycenter",
	SaturnBarycenter:      "SaturnBarycenter",
	UranusBarycenter:      "UranusBarycenter",
	NeptuneBarycenter:     "NeptuneBarycenter",
	PlutoBarycenter:       "PlutoBarycenter",
	Sun:                   "Sun",
	Mercury:               "Mercury",
	Venus:                 "Venus",
	Moon:                  "Moon",
	Earth:                 "Earth",
	Mars:                  "Mars",
	Jupiter:               "Jupiter",
	Saturn:                "Saturn",
	Uranus:                "Uranus",
	Neptune:               "Neptune",
	Pluto:                 "Pluto",
}
