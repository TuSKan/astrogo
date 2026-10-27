package kepler

import (
	"fmt"
	"sync"

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
// plan.Asteroid's own helioGeometry) expects to be answerable, and which
// [Provider.Register] places about the Solar System barycenter to refit an
// orbit there. Defaults
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
// It also chooses the center the orbit is fitted about (#803). A
// heliocentric set is kept about the Sun, or refit through its state at the
// osculation epoch about the Solar System barycenter, with the Sun and every
// planet inside the body's distance as the central mass, whichever leaves the
// smaller perturbing acceleration there: the Sun for an asteroid inside
// Jupiter's orbit, the barycenter for a Centaur, a trans-Neptunian object or a
// long-period comet far out. [Provider.Locate] then reports which. A refit
// places the state about the barycenter through the base's Sun, and fails
// when the base cannot place the Sun there. A satellite's set, or one with a
// satellite modifier, is kept as given.
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

	fitted, err := fitCenter(el, p.base)
	if err != nil {
		return err
	}

	p.elements[id] = fitted

	return nil
}

// Locate returns id's state at t about the center its orbit is fitted
// about: the Sun or the Solar System barycenter for heliocentric elements, as
// Register chose, the parent planet for a satellite's
// ([Elements.CentralBody]). A body with no elements registered is the base
// provider's to locate.
//
// It used to answer geocentric, placing a heliocentric orbit against the
// base's Sun, and before #810 against SOFA's Epv00 whatever the base was.
// Located about its own center, the orbit meets the Earth, the Sun and any
// parent planet only where ephemeris.At composes it with the base's chains,
// so one ephemeris places them all.
func (p *Provider) Locate(id core.ID, t time.Time) (core.State, error) {
	el, ok := p.elements[id]
	if !ok {
		st, err := p.base.Locate(id, t)
		if err != nil {
			return core.State{}, fmt.Errorf("kepler: base provider: %w", err)
		}

		return st, nil
	}

	pos, vel, err := el.StateAt(t)
	if err != nil {
		return core.State{}, err
	}

	return core.State{Pos: pos, Vel: vel, Frame: core.FrameICRS, Center: el.CentralBody().ID}, nil
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
// SOFA-only core.Provider covering the Sun, the Earth, the Moon, the
// Earth-Moon barycenter, the planets' system barycenters, Pluto's from its
// elements, and the Solar System Barycenter. It answers each about the center
// its series computes it from:
//
//	Sun                           about the Solar System barycenter (Epv00)
//	Earth                         about the Sun (Epv00)
//	Moon, Earth-Moon barycenter   about the Earth (Moon98)
//	Mercury to Neptune            about the Sun (Plan94)
//	Pluto's barycenter            about the Sun ([PlutoElements])
//
// so the SOFA planets' geocentric states compose as they always were
// computed, Plan94's heliocentric planet less Epv00's heliocentric Earth.
//
// It lives here rather than in the ephemeris root package, which would
// cycle: ephemeris imports kepler to re-export Elements and NewFromElements.
type sofaBase struct {
	// last is the last Epv00 evaluation: the Sun and the Earth at one epoch
	// are two Locate calls and one series, the costliest SOFA evaluates
	// here. Held by value under mu, so keeping it allocates nothing; the
	// series itself is evaluated outside the lock.
	mu   sync.Mutex
	last epv00At
}

// epv00At is one Epv00 evaluation: the Earth's heliocentric and barycentric
// position and velocity at the TDB two-part date d1 + d2.
type epv00At struct {
	set      bool
	d1, d2   float64
	pvh, pvb [2][3]float64
}

func (s *sofaBase) Locate(id core.ID, t time.Time) (core.State, error) {
	tdb := t.TDB()
	d1, d2 := tdb.JDParts()

	state := func(pos, vel vector.Vec3, center core.ID) (core.State, error) {
		return core.State{Pos: pos, Vel: vel, Frame: core.FrameICRS, Center: center}, nil
	}

	switch id {
	case core.SolarSystemBarycenter:
		return state(vector.Vec3{}, vector.Vec3{}, core.SolarSystemBarycenter)

	case core.Sun:
		e, err := s.epv00(d1, d2)
		if err != nil {
			return core.State{}, err
		}

		// The Earth's barycentric vector less its heliocentric one.
		return state(
			vector.V3(e.pvb[0][0]-e.pvh[0][0], e.pvb[0][1]-e.pvh[0][1], e.pvb[0][2]-e.pvh[0][2]),
			vector.V3(e.pvb[1][0]-e.pvh[1][0], e.pvb[1][1]-e.pvh[1][1], e.pvb[1][2]-e.pvh[1][2]),
			core.SolarSystemBarycenter)

	case core.Earth:
		e, err := s.epv00(d1, d2)
		if err != nil {
			return core.State{}, err
		}

		return state(vector.V3(e.pvh[0][0], e.pvh[0][1], e.pvh[0][2]),
			vector.V3(e.pvh[1][0], e.pvh[1][1], e.pvh[1][2]), core.Sun)

	case core.Moon:
		pv := gofaext.Moon98(d1, d2)

		return state(vector.V3(pv[0][0], pv[0][1], pv[0][2]), vector.V3(pv[1][0], pv[1][1], pv[1][2]), core.Earth)

	case core.EarthMoonBarycenter:
		// The Moon's share of the pair's mass along the Moon's vector: exact
		// by definition, where Plan94's barycenter less SOFA's Earth was up
		// to 1,942 km off (#809).
		pv := gofaext.Moon98(d1, d2)
		mu := constants.Ephemeris.MoonGravitationalParameter.Value /
			constants.Ephemeris.EarthMoonGravitationalParameter.Value

		return state(vector.V3(pv[0][0], pv[0][1], pv[0][2]).MulScalar(mu),
			vector.V3(pv[1][0], pv[1][1], pv[1][2]).MulScalar(mu), core.Earth)

	case core.MercuryBarycenter, core.Mercury, core.VenusBarycenter, core.Venus,
		core.MarsBarycenter, core.JupiterBarycenter,
		core.SaturnBarycenter, core.UranusBarycenter, core.NeptuneBarycenter:
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

		return state(vector.V3(pv[0][0], pv[0][1], pv[0][2]), vector.V3(pv[1][0], pv[1][1], pv[1][2]), core.Sun)

	case core.PlutoBarycenter:
		// SOFA has no Pluto. Two-body propagation of the Standish elements
		// is an approximation, and documented as one — but it is the
		// difference between a Charon orbit that can be placed and one that
		// cannot be.
		pos, vel, err := PlutoElements().StateAt(t)
		if err != nil {
			return core.State{}, fmt.Errorf("kepler: pluto: %w", err)
		}

		return state(pos, vel, core.Sun)

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

// epv00 is Epv00 at d1 + d2, from the last evaluation when it was at the
// same date.
func (s *sofaBase) epv00(d1, d2 float64) (epv00At, error) {
	s.mu.Lock()
	e := s.last
	s.mu.Unlock()

	if e.set && e.d1 == d1 && e.d2 == d2 {
		return e, nil
	}

	pvh, pvb, status := gofaext.Epv00(d1, d2)
	if status < 0 {
		return epv00At{}, fmt.Errorf("%w: gofaext.Epv00 status %d", ErrSofaFailure, status)
	}

	e = epv00At{set: true, d1: d1, d2: d2, pvh: pvh, pvb: pvb}

	s.mu.Lock()
	s.last = e
	s.mu.Unlock()

	return e, nil
}
