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

// Provider is a generic core.Provider for moving bodies: it answers
// State(id, t) via two-body Keplerian propagation for every id it has
// been given elements for (see Register), and delegates any other id to
// a base provider — by default the same offline SOFA source
// ephemeris.Default() uses (Sun/Moon/Mercury-Neptune). A Provider built
// with zero registered elements behaves exactly like that default
// source for major bodies; each Register call extends it to also
// answer one more small body, with no other change to how it's used —
// dropping into plan.NewAsteroid/NewComet/NewGenericBody exactly like
// any SPK-kernel-backed provider, no new plan type needed.
type Provider struct {
	elements map[core.ID]Elements
	base     core.Provider
}

// New returns a Provider with no small bodies registered yet — every
// body id is answered by opts' WithBase (or the internal SOFA default)
// until Register is called. Use New(...).Register(id, el) to build a
// single-body provider, or call Register repeatedly to share one
// provider (and its base) across several bodies.
func New(opts ...Option) *Provider {
	cfg := config{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.base == nil {
		cfg.base = &sofaBase{}
	}

	return &Provider{elements: make(map[core.ID]Elements), base: cfg.base}
}

// Register adds (or replaces) id's orbital elements on p, validating
// them first. A Provider with id already registered silently overwrites
// the prior entry — the same last-write-wins semantics a plain map has.
//
// Not safe to call concurrently with State — populate every body a
// Provider will ever answer before sharing it across concurrent
// readers (e.g. plan.VisibleTonight's candidate-gathering pipeline
// registers every Kepler-eligible candidate sequentially before its
// concurrent evaluation phase begins reading via State).
func (p *Provider) Register(id core.ID, el Elements) error {
	if err := el.Validate(); err != nil {
		return err
	}

	p.elements[id] = el

	return nil
}

// State returns id's geocentric state at t — the propagated Elements
// for id if registered, otherwise delegated to the configured base
// provider. core.Provider.State is contractually geocentric, so a
// registered body's own heliocentric StateAt result is converted via
// Earth's heliocentric position from gofaext.Epv00, matching how every
// other core.Provider in this codebase (ephemeris's own sofaProvider)
// derives a geocentric state from heliocentric SOFA/JPL data.
func (p *Provider) State(id core.ID, t time.Time) (core.State, error) {
	el, ok := p.elements[id]
	if !ok {
		st, err := p.base.State(id, t)
		if err != nil {
			return core.State{}, fmt.Errorf("kepler: base provider: %w", err)
		}

		return st, nil
	}

	heloPos, heloVel, err := el.StateAt(t)
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
		Pos:    heloPos.Sub(earthPos),
		Vel:    heloVel.Sub(earthVel),
		Frame:  core.FrameICRS,
		Center: core.CenterGeocentre,
	}, nil
}

// Close releases no resources — Provider holds no files or caches.
func (p *Provider) Close() error { return nil }

// sofaBase is the default WithBase delegate: a small, offline,
// SOFA-only core.Provider covering the Sun, Moon, the eight major
// planets, and the Solar System Barycenter — deliberately duplicated
// from ephemeris's own unexported sofaProvider rather than importing
// the ephemeris root package, which would cycle (ephemeris imports
// kepler to re-export Elements/NewFromElements). Pluto has no SOFA
// analytical source and is not covered here — ephemeris.Default()
// closes that gap by Register-ing Pluto's own elements on top of a
// Provider using this as its base, rather than teaching this SOFA-only
// type about a body SOFA itself has no model for.
type sofaBase struct{}

func (s *sofaBase) State(id core.ID, t time.Time) (core.State, error) {
	tdb := t.TDB()
	d1, d2 := tdb.JDParts()

	//nolint:exhaustive // Pluto has no SOFA (gofaext.Epv00/Plan94)
	// analytical source — mirrors ephemeris's own sofaProvider, which has
	// the same intentional gap; the default case below returns
	// ErrUnsupportedBody for it and anything else unnamed.
	switch id {
	case core.Sun:
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
		}

		return core.State{
			Pos:    vector.V3(-pvh[0][0], -pvh[0][1], -pvh[0][2]),
			Vel:    vector.V3(-pvh[1][0], -pvh[1][1], -pvh[1][2]),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocentre,
		}, nil

	case core.Moon:
		pv := gofaext.Moon98(d1, d2)

		return core.State{
			Pos:    vector.V3(pv[0][0], pv[0][1], pv[0][2]),
			Vel:    vector.V3(pv[1][0], pv[1][1], pv[1][2]),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocentre,
		}, nil

	case core.SolarSystemBarycenter:
		_, pvb, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
		}

		// pvb is Earth's barycentric position/velocity (SSB -> Earth), so
		// the SSB's own geocentric state (SSB - Earth) is its negation.
		return core.State{
			Pos:    vector.V3(-pvb[0][0], -pvb[0][1], -pvb[0][2]),
			Vel:    vector.V3(-pvb[1][0], -pvb[1][1], -pvb[1][2]),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocentre,
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
			Pos:    vector.V3(pv[0][0]-pvh[0][0], pv[0][1]-pvh[0][1], pv[0][2]-pvh[0][2]),
			Vel:    vector.V3(pv[1][0]-pvh[1][0], pv[1][1]-pvh[1][1], pv[1][2]-pvh[1][2]),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocentre,
		}, nil

	default:
		return core.State{}, fmt.Errorf("%w: %v", ErrUnsupportedBody, id)
	}
}

func (s *sofaBase) Close() error { return nil }
