package ephemeris

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// ─── Re-exported core types (users import "ephemeris", not "ephemeris/core") ─

type (
	Provider = core.Provider
	State    = core.State
	ID       = core.ID
	Source   = core.Source
	Kind     = core.Kind
	Body     = core.Body
)

const (
	Mercury               = core.Mercury
	Venus                 = core.Venus
	Earth                 = core.Earth
	Mars                  = core.Mars
	Jupiter               = core.Jupiter
	Saturn                = core.Saturn
	Uranus                = core.Uranus
	Neptune               = core.Neptune
	Pluto                 = core.Pluto
	Moon                  = core.Moon
	Sun                   = core.Sun
	SolarSystemBarycenter = core.SolarSystemBarycenter
)

const (
	KindStar       = core.KindStar
	KindPlanet     = core.KindPlanet
	KindMoon       = core.KindMoon
	KindMinorBody  = core.KindMinorBody
	KindComet      = core.KindComet
	KindBarycenter = core.KindBarycenter
	KindSatellite  = core.KindSatellite
)

const (
	Planets    = core.Planets
	SmallBody  = core.SmallBody
	Asteroids  = core.Asteroids
	Comets     = core.Comets
	Satellites = core.Satellites
	Stations   = core.Stations
)

var (
	SunBody     = core.SunBody
	MoonBody    = core.MoonBody
	MercuryBody = core.MercuryBody
	VenusBody   = core.VenusBody
	EarthBody   = core.EarthBody
	MarsBody    = core.MarsBody
	JupiterBody = core.JupiterBody
	SaturnBody  = core.SaturnBody
	UranusBody  = core.UranusBody
	NeptuneBody = core.NeptuneBody
	Bodies      = core.Bodies
)

// ── Sentinel errors ──────────────────────────────────────────────────────────

var (
	ErrTLERequired    = errors.New("eph: Satellites source requires WithTLE option")
	ErrNotImplemented = errors.New("eph: source not yet implemented")
	ErrUnknownSource  = errors.New("eph: unknown source")
	ErrZeroVector     = errors.New("eph: cannot convert near-zero vector to ICRS")
	ErrSofaEpv00      = errors.New("eph: sofa epv00 failed")
	ErrSofaPlan94     = errors.New("eph: sofa plan94 failed")
	ErrUnsupportedBody = errors.New("eph: unsupported body for sofa provider")
)

// Satellite is the SGP4 orbit propagator for NORAD TLE data.
type Satellite = satellite.Satellite

// JPL is the JPL DE4xx numerical ephemeris provider.
type JPL = jpl.Provider

// ─── Options ─────────────────────────────────────────────────────────────────

// Option configures provider construction.
type Option func(*config)

type config struct {
	Start        time.Time
	End          time.Time
	DataDir      string
	TLEName      string
	TLELine1     string
	TLELine2     string
	ExtraKernels []string
}

// WithDataDir sets the local cache directory for downloaded kernels.
func WithDataDir(dir string) Option {
	return func(c *config) { c.DataDir = dir }
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
//	p, err := eph.NewProvider(eph.Planets, "de442")
//	p, err := eph.NewProvider(eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
//	p, err := eph.NewProvider(eph.SmallBody, "433", eph.WithTimeInterval(start, end))
//	p, err := eph.NewProvider(eph.Satellites, "ISS", eph.WithTLE(l1, l2))
func NewProvider(source Source, kernel string, opts ...Option) (Provider, error) {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}

	switch source {
	case Planets, SmallBody, Asteroids, Comets:
		var jplOpts []jpl.Option
		if cfg.DataDir != "" {
			jplOpts = append(jplOpts, jpl.WithDataDir(cfg.DataDir))
		}

		if !cfg.Start.IsZero() && !cfg.End.IsZero() {
			jplOpts = append(jplOpts, jpl.WithTimeInterval(cfg.Start, cfg.End))
		}

		p, err := jpl.NewProvider(source, kernel, jplOpts...)
		if err != nil {
			return nil, err
		}

		for _, extra := range cfg.ExtraKernels {
			k, err := spk.CacheDownload("planets/"+extra+".bsp", p.DataDir)
			if err != nil {
				return nil, err
			}

			if err = p.AddKernel(k); err != nil {
				return nil, err
			}
		}

		return p, nil

	case Satellites:
		if cfg.TLELine1 == "" || cfg.TLELine2 == "" {
			return nil, ErrTLERequired
		}

		cfg.TLEName = kernel

		return satellite.NewFromTLE(cfg.TLEName, cfg.TLELine1, cfg.TLELine2)

	case Stations:
		return nil, fmt.Errorf("%w: Stations", ErrNotImplemented)

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownSource, source)
	}
}

// ─── Default SOFA Provider ───────────────────────────────────────────────────

// Default returns a SOFA-based ephemeris provider for the Sun and Moon.
func Default() Provider {
	return &sofaProvider{}
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// Position is a convenience helper that returns the geocentric position
// of a body at time t.
func Position(p Provider, id ID, t time.Time) (vector.Vec3, error) {
	st, err := p.State(id, t)
	if err != nil {
		return vector.Vec3{}, err
	}

	return st.Pos, nil
}

// Velocity is a convenience helper that returns the geocentric velocity
// of a body at time t.
func Velocity(p Provider, id ID, t time.Time) (vector.Vec3, error) {
	st, err := p.State(id, t)
	if err != nil {
		return vector.Vec3{}, err
	}

	return st.Vel, nil
}

const earthMeanRadiusKm = 6371.0

// Altitude returns the approximate altitude above the Earth's mean surface
// in kilometres. For satellites this gives orbital altitude (~400 km for ISS);
// for planets it gives geocentric distance minus Earth radius.
func Altitude(p Provider, id ID, t time.Time) (float64, error) {
	st, err := p.State(id, t)
	if err != nil {
		return 0, err
	}

	return st.DistanceKm() - earthMeanRadiusKm, nil
}

// ApparentState calculates the rigorously light-time delayed (retarded) geometric state
// of a target by repeatedly polling the ephemeris Provider at (t - tau).
func ApparentState(p Provider, target ID, obsTime time.Time) (State, error) {
	st, err := p.State(target, obsTime)
	if err != nil {
		return State{}, err
	}

	tauDays := st.Pos.Norm() / 173.144632674
	for range 5 {
		retardedTime := obsTime.AddDays(-tauDays)

		st, err = p.State(target, retardedTime)
		if err != nil {
			return State{}, err
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
			Pos: vector.Vec3{X: -ph[0], Y: -ph[1], Z: -ph[2]},
			Vel: vector.Vec3{X: -vh[0], Y: -vh[1], Z: -vh[2]},
		}, nil

	case Moon:
		pv := gofaext.Moon98(d1, d2)

		return State{
			Pos: vector.Vec3{X: pv[0][0], Y: pv[0][1], Z: pv[0][2]},
			Vel: vector.Vec3{X: pv[1][0], Y: pv[1][1], Z: pv[1][2]},
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
		}, nil

	default:
		return State{}, ErrUnsupportedBody
	}
}

func (s *sofaProvider) Close() error { return nil }
