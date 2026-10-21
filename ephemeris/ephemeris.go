package ephemeris

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// ─── Re-exported core types (users import "ephemeris", not "ephemeris/core") ─

type (
	// Provider is an ephemeris provider.
	Provider = core.Provider
	// State is an ephemeris state.
	State = core.State

	// Frame names the reference frame a State's vectors are expressed in.
	// See [core.State] for why the zero value asserts nothing.
	Frame = core.Frame
	// ID is an ephemeris ID.
	ID = core.ID
	// Source is an ephemeris source.
	Source = core.Source
)

// Reference frames a [State] can be labeled with. FrameUnspecified is the
// zero value and asserts nothing — see [core.State].
const (
	FrameUnspecified = core.FrameUnspecified
	FrameICRS        = core.FrameICRS
	FrameGCRS        = core.FrameGCRS
	FrameITRS        = core.FrameITRS
	FrameTEME        = core.FrameTEME
)

// ErrWrongFrame reports a chain of [core.Provider.Locate] answers that cannot be
// composed: one of them is not on the ICRS's axes.
var ErrWrongFrame = core.ErrWrongFrame

const (
	// Mercury is the identifier for Mercury.
	Mercury = core.Mercury
	// Venus is the identifier for Venus.
	Venus = core.Venus
	// Earth is the identifier for Earth.
	Earth = core.Earth
	// Mars is the identifier for Mars.
	Mars = core.Mars
	// Jupiter is the identifier for Jupiter.
	Jupiter = core.Jupiter
	// Saturn is the identifier for Saturn.
	Saturn = core.Saturn
	// Uranus is the identifier for Uranus.
	Uranus = core.Uranus
	// Neptune is the identifier for Neptune.
	Neptune = core.Neptune
	// Pluto is the identifier for Pluto.
	Pluto = core.Pluto
	// Moon is the identifier for the Moon.
	Moon = core.Moon
	// Sun is the identifier for the Sun.
	Sun = core.Sun
	// SolarSystemBarycenter is the identifier for the Solar System Barycenter.
	SolarSystemBarycenter = core.SolarSystemBarycenter
	// MercuryBarycenter is NAIF 1, the same point as Mercury.
	MercuryBarycenter = core.MercuryBarycenter
	// VenusBarycenter is NAIF 2, the same point as Venus.
	VenusBarycenter = core.VenusBarycenter
	// EarthMoonBarycenter is NAIF 3.
	EarthMoonBarycenter = core.EarthMoonBarycenter
	// MarsBarycenter is NAIF 4.
	MarsBarycenter = core.MarsBarycenter
	// JupiterBarycenter is NAIF 5.
	JupiterBarycenter = core.JupiterBarycenter
	// SaturnBarycenter is NAIF 6.
	SaturnBarycenter = core.SaturnBarycenter
	// UranusBarycenter is NAIF 7.
	UranusBarycenter = core.UranusBarycenter
	// NeptuneBarycenter is NAIF 8.
	NeptuneBarycenter = core.NeptuneBarycenter
	// PlutoBarycenter is NAIF 9.
	PlutoBarycenter = core.PlutoBarycenter
)

// SmallBodyID is the NAIF code of the numbered small body n; see
// [core.SmallBodyID].
func SmallBodyID(n int) (ID, bool) { return core.SmallBodyID(n) }

// SatelliteID is the NAIF code of the Earth satellite with NORAD catalog
// number n; see [core.SatelliteID].
func SatelliteID(norad int) (ID, bool) { return core.SatelliteID(norad) }

const (
	// Planets is the source for planets.
	Planets = core.Planets
	// SmallBody is the source for small bodies.
	SmallBody = core.SmallBody
	// Asteroids is the source for asteroids.
	Asteroids = core.Asteroids
	// Comets is the source for comets.
	Comets = core.Comets
	// Satellites is the source for satellites.
	Satellites = core.Satellites
	// Stations is the source for stations.
	Stations = core.Stations
	// Moons is the source for natural planetary satellites (e.g. Io,
	// Titan, Triton) — distinct from Satellites above, which is artificial
	// (TLE/SGP4-based) satellites.
	Moons = core.Moons
)

// ── Sentinel errors ──────────────────────────────────────────────────────────

var (
	// ErrTLERequired is the error for when TLE data is required.
	ErrTLERequired = errors.New("eph: Satellites source requires WithTLE option")
	// ErrNotImplemented is the error for when a source is not yet implemented.
	ErrNotImplemented = errors.New("eph: source not yet implemented")
	// ErrUnknownSource is the error for when an unknown source is provided.
	ErrUnknownSource = errors.New("eph: unknown source")
	// ErrZeroVector is the error for when a near-zero vector is provided.
	ErrZeroVector = errors.New("eph: cannot convert near-zero vector to ICRS")
	// ErrUnsupportedBody is the error [Default] returns for a body its SOFA
	// series do not place.
	ErrUnsupportedBody = kepler.ErrUnsupportedBody
	// ErrChainTooDeep reports a provider whose centres do not reach the Solar
	// System barycenter within a chain any kernel nests: one whose centres
	// loop.
	ErrChainTooDeep = errors.New("eph: chain of centres too deep")
)

// Satellite is the SGP4 orbit propagator for NORAD TLE data.
type Satellite = satellite.Satellite

// ─── Kepler two-body propagator ───────────────────────────────────────────────

// Elements are classical heliocentric osculating orbital elements for
// two-body Keplerian propagation via NewFromElements — a lighter-weight,
// network-free alternative to a real SPK-kernel-backed Provider. See
// ephemeris/kepler's package doc for the full algorithm, reference
// frame, and accuracy/scope limitations (two-body only, with no planetary
// perturbations, so accuracy drifts away from the epoch the elements
// osculate at).
type Elements = kepler.Elements

// KeplerOption configures a NewFromElements provider.
type KeplerOption = kepler.Option

// WithKeplerBase sets the Provider NewFromElements delegates to for any
// body other than the one it propagates from Elements — in particular
// Sun, which every consumer (magnitude/elongation computations) expects
// to be answerable. Defaults to a small internal SOFA-only source
// (Sun/Moon/planets, no kernel or network access) when not set.
func WithKeplerBase(p Provider) KeplerOption {
	return kepler.WithBase(p)
}

// NewElements constructs a validated set of classical heliocentric
// osculating orbital elements, referred to the J2000 ecliptic frame.
// Returns an error immediately (rather than deferring the failure to
// first use inside NewFromElements/NewMovingBodyProvider) when the
// elements are invalid — e.g. e >= 1, which a semi-major axis and mean
// anomaly cannot describe; [ElementsFromPerihelion] takes those.
func NewElements(epoch time.Time, semiMajorAxis unit.Length, eccentricity float64,
	inclination, ascendingNode, argPeriapsis, meanAnomaly angle.Angle,
) (Elements, error) {
	el, err := kepler.NewElements(epoch, semiMajorAxis, eccentricity, inclination, ascendingNode, argPeriapsis, meanAnomaly)
	if err != nil {
		return Elements{}, fmt.Errorf("eph: new elements: %w", err)
	}

	return el, nil
}

// ElementsFromPerihelion constructs a validated set of heliocentric
// osculating elements from a perihelion time, perihelion distance and
// eccentricity — the form the Minor Planet Center publishes comets in — for
// any e >= 0: ellipse, parabola or hyperbola. The angles are referred to the
// J2000 ecliptic frame. See [kepler.FromPerihelion].
func ElementsFromPerihelion(perihelionTime time.Time, perihelionDistance unit.Length, eccentricity float64,
	inclination, ascendingNode, argPeriapsis angle.Angle,
) (Elements, error) {
	el, err := kepler.FromPerihelion(perihelionTime, perihelionDistance, eccentricity,
		inclination, ascendingNode, argPeriapsis)
	if err != nil {
		return Elements{}, fmt.Errorf("eph: elements from perihelion: %w", err)
	}

	return el, nil
}

// NewMovingBodyProvider returns a generic Provider that answers a major
// body (Sun/Moon/Mercury-Neptune/SolarSystemBarycenter) via the same
// SOFA source Default() uses, and any small body whose elements have
// been registered onto it (see Provider.Register) via two-body
// Keplerian propagation — the standard "default ephemeris for a moving
// body" concept in this library: a Provider built with nothing
// registered answers every SOFA-covered body, and each Register call
// extends it, with no other change to how it's used. Unlike Default(),
// a fresh NewMovingBodyProvider() has *not* had Pluto registered — call
// Register(Pluto, kepler.PlutoElements) explicitly if it should be a
// registered body; the default base answers Pluto in any case, so a
// satellite of Pluto can be placed without it. opts configures the
// fallback base (default: the internal SOFA source); see WithKeplerBase.
func NewMovingBodyProvider(opts ...KeplerOption) *kepler.Provider {
	return kepler.New(opts...)
}

// NewFromElements returns a Provider that answers id via two-body
// Keplerian propagation of el, and every other body via opts'
// WithKeplerBase (or the internal SOFA default) — dropping into
// plan.NewAsteroid/NewComet/NewGenericBody exactly like any
// SPK-kernel-backed provider from NewProvider, with no new plan type
// needed. Convenience for the common single-body case; for several
// Kepler-propagated bodies sharing one provider/base, use
// NewMovingBodyProvider directly and register each body on it.
//
//	el, err := eph.NewElements(t, unit.AU(2.77), 0.076, incl, node, argp, ma)
//	p, err := eph.NewFromElements(eph.ID(2000001), el) // 1 Ceres
func NewFromElements(id ID, el Elements, opts ...KeplerOption) (Provider, error) {
	p := NewMovingBodyProvider(opts...)
	if err := p.Register(id, el); err != nil {
		return nil, fmt.Errorf("eph: new from elements: %w", err)
	}

	return p, nil
}

// ─── Options ─────────────────────────────────────────────────────────────────

// Option configures provider construction.
type Option func(*config)

type config struct {
	Start        time.Time
	End          time.Time
	TLEName      string
	TLELine1     string
	TLELine2     string
	ExtraKernels []string
}

// WithTimeInterval restricts the ephemeris coverage window (for small-body SPK).
func WithTimeInterval(start, end time.Time) Option {
	return func(c *config) { c.Start = start; c.End = end }
}

// WithKernel adds an extra SPK kernel to load after the primary one.
// Multiple WithKernel options can be chained.
//
//	p := eph.NewProvider(eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
func WithKernel(name string) Option {
	return func(c *config) { c.ExtraKernels = append(c.ExtraKernels, name) }
}

// WithTLE provides raw TLE lines for satellite construction.
func WithTLE(line1, line2 string) Option {
	return func(c *config) {
		c.TLELine1 = line1
		c.TLELine2 = line2
	}
}

// ─── Factory ─────────────────────────────────────────────────────────────────

// NewProvider creates an ephemeris provider for the given source and kernel.
//
//	p, err := eph.NewProvider(ctx, eph.Planets, "de442")
//	p, err := eph.NewProvider(ctx, eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
//	p, err := eph.NewProvider(ctx, eph.SmallBody, "433", eph.WithTimeInterval(start, end))
//	p, err := eph.NewProvider(ctx, eph.Satellites, "ISS", eph.WithTLE(l1, l2))
func NewProvider(ctx context.Context, source Source, kernel string, opts ...Option) (Provider, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	switch source {
	case Planets, SmallBody, Asteroids, Comets, Moons:
		// Through the registered backend rather than by importing one. This
		// package knows SOFA and a source vocabulary; the kernel half reaches
		// remote and its network stack, and a build that never asks for a
		// kernel should not link it. See ephemeris/core/kernel.go and #112.
		p, err := core.KernelProvider(ctx, core.KernelRequest{
			Source:       source,
			Kernel:       kernel,
			Start:        cfg.Start,
			End:          cfg.End,
			ExtraKernels: cfg.ExtraKernels,
		})
		if err != nil {
			// Wrapped, not replaced: errors.Is against core.ErrNoKernelBackend
			// is how a caller distinguishes "this build has no kernel support"
			// from "that kernel could not be read", and the message already
			// names the import to add.
			return nil, fmt.Errorf("ephemeris: %w", err)
		}

		return p, nil

	case Satellites:
		if cfg.TLELine1 == "" || cfg.TLELine2 == "" {
			return nil, ErrTLERequired
		}

		cfg.TLEName = kernel

		sat, err := satellite.NewFromTLE(cfg.TLEName, cfg.TLELine1, cfg.TLELine2)
		if err != nil {
			return nil, fmt.Errorf("ephemeris: new TLE: %w", err)
		}

		return sat, nil

	case Stations:
		return nil, fmt.Errorf("%w: Stations", ErrNotImplemented)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownSource, source)
	}
}

// ─── Default SOFA Provider ───────────────────────────────────────────────────

// Default returns the standard offline ephemeris provider for every
// named body core.ID defines: the Sun, Moon, Mercury through Neptune,
// and the Solar System Barycenter via SOFA analytical models (no kernel
// or network access), plus Pluto — which SOFA has no analytical model
// for — via two-body Keplerian propagation from plutoElements. No named
// core.ID this library defines returns ErrUnsupportedBody from here.
//
// Pluto's accuracy is the same documented limitation as any other
// Kepler-backed body (see Elements' doc comment): two-body propagation
// ignores planetary perturbations, and Pluto's real motion is measurably
// perturbed by its 3:2 mean-motion resonance with Neptune, so it drifts
// away from J2000.0 faster than a typical asteroid does. This is an
// approximate offline position, not a claim of high-precision Pluto
// ephemeris — for that, use a real SPK-kernel-backed provider instead
// (NewProvider with Planets; de440s and later kernels carry Pluto).
//
// The concrete type returned is a *kepler.Provider with Pluto registered
// explicitly. Since kepler's own default base learned to answer Pluto from
// the same [kepler.PlutoElements], that registration is now belt and braces
// rather than the only thing closing the gap — it keeps Default's contract
// visible at the call site instead of resting on a lower layer's behavior.
func Default() Provider {
	p := kepler.New()
	if err := p.Register(PlutoBarycenter, kepler.PlutoElements()); err != nil {
		panic(fmt.Sprintf("ephemeris: failed to register built-in Pluto elements: %v", err))
	}

	return p
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// lightAUPerDay is the speed of light in astronomical units per day, the
// denominator of the light-time iteration below.
//
// Derived rather than written out. It was the literal 173.144632674 in both
// places it is used — a published value rounded to nine decimals, which is
// 1.4e-12 low against c and the AU as constants defines them.
//
// That rounding changes nothing, and provably so rather than approximately.
// At Neptune's distance it moves the light time by 21 nanoseconds, which is
// 2.4e-13 days; one ULP of a Julian Date at a modern epoch is 4.7e-10 days.
// The correction is about 1900 times smaller than the smallest difference the
// epoch can represent, so the retarded time below comes out bit-identical.
// Measured: the apparent position of Mars, Jupiter and Neptune is unchanged to
// the last bit.
//
// So this is a tidying, not a correction — but one that cannot go stale if
// either input is ever revised, which the two literals could.
//
// var, not const: both inputs are struct fields.
var lightAUPerDay = constants.SI2019.SpeedOfLight.Value *
	constants.Derived.JulianDaySeconds.Value / constants.IAU.AstronomicalUnit.Value

// unitArray is the [3]float64 form gofa's vector routines take.
func unitArray(v vector.Vec3) [3]float64 {
	u := v.Unit()

	return [3]float64{u.X, u.Y, u.Z}
}

// Light-time iteration limits.
//
// This used to run a flat five iterations. It converges geometrically with
// ratio v/c ~ 1e-4, so each pass buys four decimal digits and the fifth was
// refining a number that had stopped changing in double precision two passes
// earlier. Measured over 2026 — how far each iteration moves the answer, at
// worst, for the bodies with the fastest relative motion:
//
//	body      pass 1     pass 2      pass 3        pass 4+
//	Moon      0.759"     0.00000"    0"            0"
//	Sun      20.837"     0.00003"    0"            0"
//	Venus    44.762"     0.00121"    0.0000001"    0"
//	Mars     38.733"     0.00087"    0.0000000"    0"
//	Jupiter  29.008"     0.00208"    0.0000002"    0"
//	Neptune  24.346"     0.00214"    0.0000002"    0"
//
// That would justify a flat three, but a convergence test is both cheaper and
// self-explaining: the Moon settles after one pass and says so, where a fixed
// count cannot. The ceiling stays at five so a pathological provider still
// terminates.
//
// It matters because plan now takes this path for every ephemeris-backed
// target at every time step (see plan/apparent.go), where it used to call the
// provider once.
const (
	// lightTimeTolDays is ~0.9 microseconds of light time, and the position
	// a target moves in it: Instant.astrometric stops once its last step
	// along the target's velocity leaves less error than that.
	lightTimeTolDays = 1e-11

	// lightTimeMaxIter caps the loop. Nothing measured needs more than three
	// passes; this is the guard against a provider whose state is not a
	// continuous function of time.
	lightTimeMaxIter = 5
)

// ToICRS converts a geocentric Cartesian vector (in AU) to spherical ICRS coordinates.
func ToICRS(pos vector.Vec3) (coord.ICRS, error) {
	r := math.Sqrt(pos.X*pos.X + pos.Y*pos.Y + pos.Z*pos.Z)
	if r < 1e-12 {
		return coord.ICRS{}, ErrZeroVector
	}

	ra, dec := pos.ToSpherical()

	return coord.NewICRS(angle.Rad(ra), angle.Rad(dec)), nil
}
