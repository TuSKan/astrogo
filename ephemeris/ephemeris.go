package ephemeris

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
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
	// Center names the origin a State's vectors are measured from.
	Center = core.Center
	// ID is an ephemeris ID.
	ID = core.ID
	// Source is an ephemeris source.
	Source = core.Source
	// Kind is an ephemeris kind.
	Kind = core.Kind
	// Body is an ephemeris body.
	Body = core.Body
)

// Reference frames and origins a [State] can be labelled with. The
// unspecified values are the zero values and assert nothing — see
// [core.State].
const (
	FrameUnspecified = core.FrameUnspecified
	FrameICRS        = core.FrameICRS
	FrameGCRS        = core.FrameGCRS
	FrameITRS        = core.FrameITRS
	FrameTEME        = core.FrameTEME

	CenterUnspecified = core.CenterUnspecified
	CenterGeocentre   = core.CenterGeocentre
	CenterBarycentre  = core.CenterBarycentre
	CenterHeliocentre = core.CenterHeliocentre
)

// Errors reported by [State.Require].
var (
	ErrWrongFrame  = core.ErrWrongFrame
	ErrWrongCenter = core.ErrWrongCenter
)

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
)

const (
	// KindStar is the kind for stars.
	KindStar = core.KindStar
	// KindPlanet is the kind for planets.
	KindPlanet = core.KindPlanet
	// KindMoon is the kind for moons.
	KindMoon = core.KindMoon
	// KindMinorBody is the kind for minor bodies.
	KindMinorBody = core.KindMinorBody
	// KindComet is the kind for comets.
	KindComet = core.KindComet
	// KindBarycenter is the kind for barycenters.
	KindBarycenter = core.KindBarycenter
	// KindSatellite is the kind for satellites.
	KindSatellite = core.KindSatellite
)

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

var (
	// SunBody is the body for the Sun.
	SunBody = core.SunBody
	// MoonBody is the body for the Moon.
	MoonBody = core.MoonBody
	// MercuryBody is the body for Mercury.
	MercuryBody = core.MercuryBody
	// VenusBody is the body for Venus.
	VenusBody = core.VenusBody
	// EarthBody is the body for Earth.
	EarthBody = core.EarthBody
	// MarsBody is the body for Mars.
	MarsBody = core.MarsBody
	// JupiterBody is the body for Jupiter.
	JupiterBody = core.JupiterBody
	// SaturnBody is the body for Saturn.
	SaturnBody = core.SaturnBody
	// UranusBody is the body for Uranus.
	UranusBody = core.UranusBody
	// NeptuneBody is the body for Neptune.
	NeptuneBody = core.NeptuneBody
	// Bodies is the array of all bodies.
	Bodies = core.Bodies
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
	// ErrSofaEpv00 is the error for when the sofa epv00 function fails.
	ErrSofaEpv00 = errors.New("eph: sofa epv00 failed")
	// ErrSofaPlan94 is the error for when the sofa plan94 function fails.
	ErrSofaPlan94 = errors.New("eph: sofa plan94 failed")
	// ErrUnsupportedBody is the error for when an unsupported body is provided.
	ErrUnsupportedBody = errors.New("eph: unsupported body for sofa provider")
)

// Satellite is the SGP4 orbit propagator for NORAD TLE data.
type Satellite = satellite.Satellite

// JPL is the JPL DE4xx numerical ephemeris provider.
type JPL = jpl.Provider

// ─── Kepler two-body propagator ───────────────────────────────────────────────

// Elements are classical heliocentric osculating orbital elements for
// two-body Keplerian propagation via NewFromElements — a lighter-weight,
// network-free alternative to a real SPK-kernel-backed Provider. See
// ephemeris/kepler's package doc for the full algorithm, reference
// frame, and accuracy/scope limitations (elliptical two-body only; no
// planetary perturbations, so accuracy drifts away from Elements.Epoch).
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
// elements are invalid — e.g. e >= 1, which two-body elliptical
// propagation cannot represent.
func NewElements(epoch time.Time, semiMajorAxis, eccentricity float64,
	inclination, ascendingNode, argPeriapsis, meanAnomaly angle.Angle,
) (Elements, error) {
	el, err := kepler.NewElements(epoch, semiMajorAxis, eccentricity, inclination, ascendingNode, argPeriapsis, meanAnomaly)
	if err != nil {
		return Elements{}, fmt.Errorf("eph: new elements: %w", err)
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
//	el, err := eph.NewElements(t, 2.77, 0.076, incl, node, argp, ma)
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
		var jplOpts []jpl.Option

		if !cfg.Start.IsZero() && !cfg.End.IsZero() {
			jplOpts = append(jplOpts, jpl.WithTimeInterval(cfg.Start, cfg.End))
		}

		p, err := jpl.NewProvider(ctx, source, kernel, jplOpts...)
		if err != nil {
			return nil, fmt.Errorf("ephemeris: new provider: %w", err)
		}

		for _, extra := range cfg.ExtraKernels {
			k, err := spk.CacheDownload(ctx, "planets/"+extra+".bsp")
			if err != nil {
				return nil, fmt.Errorf("ephemeris: cache kernel %s: %w", extra, err)
			}

			err = p.AddKernel(k)
			if err != nil {
				return nil, fmt.Errorf("ephemeris: add kernel: %w", err)
			}
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
// visible at the call site instead of resting on a lower layer's behaviour.
func Default() Provider {
	p := kepler.New(kepler.WithBase(&sofaProvider{}))
	if err := p.Register(Pluto, kepler.PlutoElements); err != nil {
		panic(fmt.Sprintf("ephemeris: failed to register built-in Pluto elements: %v", err))
	}

	return p
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// Position is a convenience helper that returns the geocentric position
// of a body at time t.
func Position(p Provider, id ID, t time.Time) (vector.Vec3, error) {
	st, err := p.State(id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("ephemeris: position: %w", err)
	}

	return st.Pos, nil
}

// Velocity is a convenience helper that returns the geocentric velocity
// of a body at time t.
func Velocity(p Provider, id ID, t time.Time) (vector.Vec3, error) {
	st, err := p.State(id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("ephemeris: velocity: %w", err)
	}

	return st.Vel, nil
}

const earthMeanRadiusKm = 6371.0

// Altitude returns the approximate altitude above the Earth's mean surface
// in kilometers. For satellites this gives orbital altitude (~400 km for ISS);
// for planets it gives geocentric distance minus Earth radius.
func Altitude(p Provider, id ID, t time.Time) (float64, error) {
	st, err := p.State(id, t)
	if err != nil {
		return 0, fmt.Errorf("ephemeris: altitude: %w", err)
	}

	return st.DistanceKm() - earthMeanRadiusKm, nil
}

// ApparentState returns the apparent state of a target: where it is seen from
// the Earth at obsTime, with both light time and annual aberration included.
// It iterates the light time by repeatedly polling the Provider at (t - tau).
//
// Apparent, not astrometric. The distinction matters and is easy to miss,
// because it does not come from the iteration but from what a Provider means
// by a geocentric state. State(target, t') is target(t') - earth(t'), so
// asking for it at the retarded epoch moves the observer back along with the
// target. The astrometric vector wants the opposite: the target retarded and
// the Earth left where it is. The two differ by earth(t) - earth(t - tau),
// which is v_earth * tau; and since tau is the range over c, that term is the
// range times v_earth/c, which is precisely the first-order aberration
// displacement and in the right direction.
//
// So this returns the apparent place, correctly. Do not apply aberration to
// the result — it is already there, and doubling it puts a planet at
// opposition some twenty arcseconds from where it is.
//
// [coord.Context.GeocentricToObserved] is the matching consumer: it adds
// diurnal parallax, the rotation into the local horizon and refraction, and
// deliberately does not touch aberration.
func ApparentState(p Provider, target ID, obsTime time.Time) (State, error) {
	st, err := p.State(target, obsTime)
	if err != nil {
		return State{}, fmt.Errorf("ephemeris: apparent state: %w", err)
	}

	tauDays := st.Pos.Norm() / 173.144632674
	for range 5 {
		retardedTime := obsTime.AddDays(-tauDays)

		st, err = p.State(target, retardedTime)
		if err != nil {
			return State{}, fmt.Errorf("ephemeris: apparent state retarded: %w", err)
		}

		tauDays = st.Pos.Norm() / 173.144632674
	}

	return st, nil
}

// ToICRS converts a geocentric Cartesian vector (in AU) to spherical ICRS coordinates.
func ToICRS(pos vector.Vec3) (coord.ICRS, error) {
	r := math.Sqrt(pos.X*pos.X + pos.Y*pos.Y + pos.Z*pos.Z)
	if r < 1e-12 {
		return coord.ICRS{}, ErrZeroVector
	}

	ra := math.Atan2(pos.Y, pos.X)
	dec := math.Asin(pos.Z / r)

	return coord.NewICRS(angle.Rad(ra).Wrap2Pi(), angle.Rad(dec)), nil
}

// ─── SOFA provider (analytical Sun/Moon) ─────────────────────────────────────

type sofaProvider struct{}

func (s *sofaProvider) State(id ID, t time.Time) (State, error) {
	tdb := t.TDB()
	d1, d2 := tdb.JDParts()

	switch id {
	case Sun:
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return State{}, ErrSofaEpv00
		}

		ph := pvh[0]
		vh := pvh[1]

		return State{
			Pos:    vector.Vec3{X: -ph[0], Y: -ph[1], Z: -ph[2]},
			Vel:    vector.Vec3{X: -vh[0], Y: -vh[1], Z: -vh[2]},
			Frame:  FrameICRS,
			Center: CenterGeocentre,
		}, nil

	case Moon:
		pv := gofaext.Moon98(d1, d2)

		return State{
			Pos:    vector.Vec3{X: pv[0][0], Y: pv[0][1], Z: pv[0][2]},
			Vel:    vector.Vec3{X: pv[1][0], Y: pv[1][1], Z: pv[1][2]},
			Frame:  FrameICRS,
			Center: CenterGeocentre,
		}, nil

	case SolarSystemBarycenter:
		_, pvb, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return State{}, ErrSofaEpv00
		}

		// pvb is Earth's barycentric position/velocity (SSB -> Earth), so
		// the SSB's own geocentric state (SSB - Earth) is its negation.
		return State{
			Pos:    vector.Vec3{X: -pvb[0][0], Y: -pvb[0][1], Z: -pvb[0][2]},
			Vel:    vector.Vec3{X: -pvb[1][0], Y: -pvb[1][1], Z: -pvb[1][2]},
			Frame:  FrameICRS,
			Center: CenterGeocentre,
		}, nil

	case Mercury, Venus, Earth, Mars, Jupiter, Saturn, Uranus, Neptune:
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return State{}, ErrSofaEpv00
		}

		var np int

		switch id {
		case Mercury:
			np = 1
		case Venus:
			np = 2
		case Earth:
			np = 3
		case Mars:
			np = 4
		case Jupiter:
			np = 5
		case Saturn:
			np = 6
		case Uranus:
			np = 7
		case Neptune:
			np = 8
		}

		pv, status := gofaext.Plan94(d1, d2, np)
		if status < 0 {
			return State{}, ErrSofaPlan94
		}

		return State{
			Pos: vector.Vec3{
				X: pv[0][0] - pvh[0][0],
				Y: pv[0][1] - pvh[0][1],
				Z: pv[0][2] - pvh[0][2],
			},
			Vel: vector.Vec3{
				X: pv[1][0] - pvh[1][0],
				Y: pv[1][1] - pvh[1][1],
				Z: pv[1][2] - pvh[1][2],
			},
			Frame:  FrameICRS,
			Center: CenterGeocentre,
		}, nil

	default:
		return State{}, ErrUnsupportedBody
	}
}

func (s *sofaProvider) Close() error { return nil }
