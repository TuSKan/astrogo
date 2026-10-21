package kepler

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
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
// registered body's own heliocentric StateAt result is made geocentric with
// the base provider's Sun: the same ephemeris places the orbit's Earth as
// places every other body the caller asks the base for.
func (p *Provider) State(id core.ID, t time.Time) (core.State, error) {
	el, ok := p.elements[id]
	if !ok {
		st, err := p.base.State(id, t)
		if err != nil {
			return core.State{}, fmt.Errorf("kepler: base provider: %w", err)
		}

		return st, nil
	}

	relPos, relVel, err := el.StateAt(t)
	if err != nil {
		return core.State{}, err
	}

	// StateAt returns the orbit about its central body. For heliocentric
	// elements that is already the heliocentric state; for a satellite it is
	// the offset from its parent, and the parent's own heliocentric position
	// has to be added before anything can be subtracted from it.
	//
	// The parent comes from the base provider rather than being propagated
	// here: it is a planet, the base already answers for planets, and
	// re-deriving it would be a second, worse ephemeris for a body this
	// package can simply ask about.
	heloPos, heloVel := relPos, relVel

	if central := el.CentralBody(); central.ID != core.Sun {
		parent, perr := p.base.State(central.ID, t)
		if perr != nil {
			return core.State{}, fmt.Errorf("kepler: central body %s: %w", central.ID, perr)
		}

		// The base answers geocentric, so the Earth term below would cancel
		// it out; add the parent's geocentric state and return directly.
		return core.State{
			Pos:    parent.Pos.Add(relPos),
			Vel:    parent.Vel.Add(relVel),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocenter,
		}, nil
	}

	// Geocentric is heliocentric plus the Sun's geocentric state, and the Sun
	// comes from the base, so the orbit is placed against the base's own
	// Earth. It used to come from SOFA's Epv00 whatever the base was, so a
	// JPL base placed the Sun from the kernel and the Earth from a model 4.0 km
	// rms and 11.7 km at worst away from it over 1900–2100 — 1.6″ for a body
	// at 0.01 AU, and two ephemerides inside one place (#810). The default
	// SOFA base's Sun is minus that same Epv00 heliocentric Earth, so a
	// Provider built without WithBase answers exactly what it did, to the bit.
	sun, err := p.base.State(core.Sun, t)
	if err != nil {
		return core.State{}, fmt.Errorf("kepler: the Sun from the base provider: %w", err)
	}

	if err := sun.Require(core.FrameICRS, core.CenterGeocenter); err != nil {
		return core.State{}, fmt.Errorf("kepler: the Sun from the base provider: %w", err)
	}

	return core.State{
		Pos:    heloPos.Add(sun.Pos),
		Vel:    heloVel.Add(sun.Vel),
		Frame:  core.FrameICRS,
		Center: core.CenterGeocenter,
	}, nil
}

// Close releases no resources — Provider holds no files or caches.
func (p *Provider) Close() error { return nil }

// PlutoElements are Pluto's classical heliocentric osculating elements at
// J2000.0, in the J2000 mean ecliptic frame.
//
// The T=0 row of E. M. Standish (JPL/Caltech Solar System Dynamics Group),
// "Keplerian Elements for Approximate Positions of the Major Planets",
// Table 1, valid 1800-2050.
//
// They live here rather than a layer up because the default base needs them:
// SOFA has no analytical Pluto, and without one a satellite registered about
// Pluto — Charon — cannot be placed at all, since the provider composes a
// satellite through its parent.
//
// It returns a copy, not a var, so that no importer can change Pluto's orbit
// for every other one in the process (#537).
func PlutoElements() Elements { return plutoElements }

// plutoElements is the value PlutoElements returns, built once.
var plutoElements = func() Elements {
	el, err := NewElements(time.J2000(), unit.AU(39.48211675), 0.24882730,
		angle.Deg(17.14001206), angle.Deg(110.30393684),
		angle.Deg(113.76497945), angle.Deg(14.86012204))
	if err != nil {
		panic(fmt.Sprintf("kepler: built-in Pluto elements are invalid: %v", err))
	}

	return el
}()

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

	// Every named body is handled, Pluto included: it has no SOFA
	// analytical source and is propagated from [PlutoElements] instead.
	// The suppression that used to sit here existed only because it was
	// missing, and the linter noticed the moment it stopped being.
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
			Center: core.CenterGeocenter,
		}, nil

	case core.Moon:
		pv := gofaext.Moon98(d1, d2)

		return core.State{
			Pos:    vector.V3(pv[0][0], pv[0][1], pv[0][2]),
			Vel:    vector.V3(pv[1][0], pv[1][1], pv[1][2]),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocenter,
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
			Center: core.CenterGeocenter,
		}, nil

	case core.Earth:
		// The geocentre, from the geocentre.
		return core.State{Frame: core.FrameICRS, Center: core.CenterGeocenter}, nil

	case core.EarthMoonBarycenter:
		// The Moon's share of the pair's mass along the Moon's vector, as
		// ephemeris.Default answers it: exact by definition, where Plan94's
		// barycenter less SOFA's Earth was up to 1,942 km off (#809).
		pv := gofaext.Moon98(d1, d2)
		mu := constants.Ephemeris.MoonGravitationalParameter.Value /
			constants.Ephemeris.EarthMoonGravitationalParameter.Value

		return core.State{
			Pos:    vector.V3(pv[0][0], pv[0][1], pv[0][2]).MulScalar(mu),
			Vel:    vector.V3(pv[1][0], pv[1][1], pv[1][2]).MulScalar(mu),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocenter,
		}, nil

	case core.MercuryBarycenter, core.Mercury, core.VenusBarycenter, core.Venus,
		core.MarsBarycenter, core.JupiterBarycenter,
		core.SaturnBarycenter, core.UranusBarycenter, core.NeptuneBarycenter:
		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
		}

		// Plan94's planets are 1 to 8, its Mars to Neptune the system
		// barycenters, so those are what it answers for; the planets' own
		// centers are refused below rather than answered with their
		// barycenters (#809). Its 3 is the Earth-Moon barycenter, under
		// which the Earth used to be asked for and came back as the
		// barycenter.
		np := int(id)
		if id == core.Mercury || id == core.Venus {
			np = int(id / 100)
		}

		pv, status := gofaext.Plan94(d1, d2, np)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Plan94 status %d", ErrSofaFailure, status)
		}

		return core.State{
			Pos:    vector.V3(pv[0][0]-pvh[0][0], pv[0][1]-pvh[0][1], pv[0][2]-pvh[0][2]),
			Vel:    vector.V3(pv[1][0]-pvh[1][0], pv[1][1]-pvh[1][1], pv[1][2]-pvh[1][2]),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocenter,
		}, nil

	case core.PlutoBarycenter:
		// SOFA has no Pluto. Two-body propagation of the Standish elements
		// is an approximation, and documented as one — but it is the
		// difference between a Charon orbit that can be placed and one that
		// cannot be, and it is what ephemeris.Default() already offered a
		// layer up.
		pos, vel, perr := PlutoElements().StateAt(t)
		if perr != nil {
			return core.State{}, fmt.Errorf("kepler: pluto: %w", perr)
		}

		pvh, _, status := gofaext.Epv00(d1, d2)
		if status < 0 {
			return core.State{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
		}

		return core.State{
			Pos:    pos.Sub(vector.V3(pvh[0][0], pvh[0][1], pvh[0][2])),
			Vel:    vel.Sub(vector.V3(pvh[1][0], pvh[1][1], pvh[1][2])),
			Frame:  core.FrameICRS,
			Center: core.CenterGeocenter,
		}, nil

	case core.Mars, core.Jupiter, core.Saturn, core.Uranus, core.Neptune, core.Pluto:
		// The planets' own centers: SOFA's theory places their system
		// barycenters, and answering one for the other is what #809 removed.
		return core.State{}, fmt.Errorf("%w: %v; SOFA's theory places its system barycenter, %v",
			ErrUnsupportedBody, id, id/100)

	default:
		return core.State{}, fmt.Errorf("%w: %v", ErrUnsupportedBody, id)
	}
}

func (s *sofaBase) Close() error { return nil }
