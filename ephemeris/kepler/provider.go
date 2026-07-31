package kepler

import (
	"fmt"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// config holds Provider construction options.
type config struct {
	base core.Provider
}

// Option configures a Provider at construction time.
type Option func(*config)

// WithBase sets the core.Provider a Provider delegates to for any body
// ID other than the one it propagates from Elements — in particular for
// core.Sun, which every consumer (magnitude/elongation computations,
// plan.Asteroid's own helioGeometry) expects to be answerable. Defaults
// to a small internal SOFA-only source (Sun/Moon/planets, no kernel or
// network access) when not set.
func WithBase(p core.Provider) Option {
	return func(c *config) { c.base = p }
}

// Provider adapts a single body's Elements into a core.Provider,
// dropping into plan.NewAsteroid/NewComet/NewGenericBody exactly like
// any SPK-kernel-backed provider — no new plan type is needed for a
// Kepler-propagated body to become a full Observable.
type Provider struct {
	id   core.ID
	el   Elements
	base core.Provider
}

// New returns a Provider that answers id via Keplerian propagation of el,
// and every other body via opts' WithBase (or the internal SOFA default).
func New(id core.ID, el Elements, opts ...Option) (*Provider, error) {
	if err := el.Validate(); err != nil {
		return nil, err
	}

	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.base == nil {
		cfg.base = &sofaBase{}
	}

	return &Provider{id: id, el: el, base: cfg.base}, nil
}

// State returns id's geocentric state at t — the propagated Elements'
// body if id matches, otherwise delegated to the configured base
// provider. core.Provider.State is contractually geocentric, so the
// propagated body's own heliocentric StateAt result is converted via
// Earth's heliocentric position from gofaext.Epv00, matching how every
// other core.Provider in this codebase (ephemeris's own sofaProvider)
// derives a geocentric state from heliocentric SOFA/JPL data.
func (p *Provider) State(id core.ID, t time.Time) (core.State, error) {
	if id != p.id {
		st, err := p.base.State(id, t)
		if err != nil {
			return core.State{}, fmt.Errorf("kepler: base provider: %w", err)
		}

		return st, nil
	}

	heloPos, heloVel, err := p.el.StateAt(t)
	if err != nil {
		return core.State{}, err
	}

	tdb := t.TDB()
	d1, d2 := tdb.JDParts()

	pvh, _, status := gofaext.Epv00(d1, d2)
	if status < 0 {
		return core.State{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
	}

	earthPos := vector.V3(pvh[0][0], pvh[0][1], pvh[0][2])
	earthVel := vector.V3(pvh[1][0], pvh[1][1], pvh[1][2])

	return core.State{
		Pos: heloPos.Sub(earthPos),
		Vel: heloVel.Sub(earthVel),
	}, nil
}

// Close releases no resources — Provider holds no files or caches.
func (p *Provider) Close() error { return nil }

// sofaBase is the default WithBase delegate: a small, offline,
// SOFA-only core.Provider covering the Sun, Moon, and the eight major
// planets — deliberately duplicated from ephemeris's own unexported
// sofaProvider rather than importing the ephemeris root package, which
// would cycle (ephemeris imports kepler to re-export Elements/
// NewFromElements).
type sofaBase struct{}

func (s *sofaBase) State(id core.ID, t time.Time) (core.State, error) {
	tdb := t.TDB()
	d1, d2 := tdb.JDParts()

	//nolint:exhaustive // Pluto/SolarSystemBarycenter have no SOFA
	// (gofaext.Epv00/Plan94) analytical source — mirrors ephemeris's own
	// sofaProvider, which has the same intentional gap; the default case
	// below returns ErrUnsupportedBody for both.
	switch id {
	case core.Sun:
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
		}

		return core.State{
			Pos: vector.V3(-pvh[0][0], -pvh[0][1], -pvh[0][2]),
			Vel: vector.V3(-pvh[1][0], -pvh[1][1], -pvh[1][2]),
		}, nil

	case core.Moon:
		pv := gofaext.Moon98(d1, d2)

		return core.State{
			Pos: vector.V3(pv[0][0], pv[0][1], pv[0][2]),
			Vel: vector.V3(pv[1][0], pv[1][1], pv[1][2]),
		}, nil

	case core.Mercury, core.Venus, core.Earth, core.Mars,
		core.Jupiter, core.Saturn, core.Uranus, core.Neptune:
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
		}

		np := map[core.ID]int{
			core.Mercury: 1, core.Venus: 2, core.Earth: 3, core.Mars: 4,
			core.Jupiter: 5, core.Saturn: 6, core.Uranus: 7, core.Neptune: 8,
		}[id]

		pv, status := gofaext.Plan94(d1, d2, np)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Plan94 status %d", ErrSofaFailure, status)
		}

		return core.State{
			Pos: vector.V3(pv[0][0]-pvh[0][0], pv[0][1]-pvh[0][1], pv[0][2]-pvh[0][2]),
			Vel: vector.V3(pv[1][0]-pvh[1][0], pv[1][1]-pvh[1][1], pv[1][2]-pvh[1][2]),
		}, nil

	default:
		return core.State{}, fmt.Errorf("%w: %v", ErrUnsupportedBody, id)
	}
}

func (s *sofaBase) Close() error { return nil }
