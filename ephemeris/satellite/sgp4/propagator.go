package sgp4

import (
	"fmt"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Mode selects between the two operational conventions Vallado's code offers.
//
// # It has no effect on a near-Earth orbit
//
// The two modes differ in exactly two places: how sidereal time at epoch is
// computed, and how the node is normalised inside dpper's Lyddane branch. Both
// are deep space. Sidereal time reaches the model only through dscom, dsinit
// and dspace, and the near-Earth path never reads it — so for a period under
// 225 minutes the two modes are bit-identical, and this option is inert.
//
// Worth stating rather than leaving to be discovered: a caller who sets it on a
// low Earth orbit expecting a different answer will not get one, and that is
// correct rather than a bug. TestAFSPCModeIsADifferentAnswer asserts both
// halves — identical below the threshold, different above it.
//
// Where it does apply, the difference is a convention and not noise: output
// generated in one mode is reproduced only by the same mode.
type Mode int

const (
	// ModeImproved is Vallado's 'i', and the default. It uses the 1982 GMST
	// expression, which is what the reference states this package is measured
	// against were generated with.
	ModeImproved Mode = iota

	// ModeAFSPC is Vallado's 'a', preserving the original Space Command
	// convention. Choose it to reproduce output produced by AFSPC-mode code.
	ModeAFSPC
)

// String implements [fmt.Stringer].
func (m Mode) String() string {
	switch m {
	case ModeImproved:
		return "improved"
	case ModeAFSPC:
		return "AFSPC"
	default:
		return fmt.Sprintf("Mode(%d)", int(m))
	}
}

// Option configures a [Propagator]. See [WithGravity] and [WithMode].
type Option func(*config)

type config struct {
	gravity Gravity
	mode    Mode
}

// WithGravity selects the gravity model. The default is [WGS72], which is the
// model TLEs are fitted with; see [Gravity] before choosing anything else.
func WithGravity(g Gravity) Option { return func(c *config) { c.gravity = g } }

// WithMode selects the operational convention. The default is [ModeImproved].
func WithMode(m Mode) Option { return func(c *config) { c.mode = m } }

// Propagator holds one element set's initialised SGP4 coefficients.
//
// # Immutable, and therefore safe to share
//
// Every field is written once by [New] and only read afterwards, so any number
// of goroutines may call [Propagator.At] on one value at the same time without
// synchronisation. That is not how the model is usually implemented: the
// reference threads its state through a mutable record, and its deep-space
// resonance integrator deliberately carries `atime`/`xli`/`xni` between calls
// as a speed memo, which makes concurrent use of one satellite unsound.
//
// This package keeps that integrator's state in locals instead. It can, because
// the integration grid is anchored at t = 0 with a fixed step, so restarting
// from zero on every call produces bit-identical results to resuming — the
// carried state is a memo, not a path dependency. The cost is |tsince|/720
// steps for a resonant deep-space orbit: two for a day, 730 for a year.
type Propagator struct {
	el   Elements
	grav gravityModel
	mode Mode

	// From initl.
	noUnkozai            float64
	con41, con42         float64
	cosio, cosio2, sinio float64
	eccsq, omeosq, posq  float64
	ao, ainv, rp, rteosq float64
	gsto                 float64
	a, alta, altp        float64

	// Branch selection, both the model's own.
	isimp bool
	deep  bool

	// Secular and periodic coefficients.
	eta                        float64
	cc1, cc4, cc5              float64
	d2, d3, d4                 float64
	mdot, argpdot, nodedot     float64
	omgcof, xmcof, nodecf      float64
	t2cof, t3cof, t4cof, t5cof float64
	xlcof, aycof               float64
	delmo, sinmao              float64
	x1mth2, x7thm1             float64

	// Deep space. Both are zero for a near-Earth element set and neither is
	// read on that path.
	ds deepSpaceTerms
	rz resonanceTerms
}

// New initialises a propagator for el.
//
// The elements are validated ([Elements.Validate]) before anything is derived
// from them, so a returned *Propagator has already refused what it cannot
// propagate.
func New(el Elements, opts ...Option) (*Propagator, error) {
	cfg := config{gravity: WGS72, mode: ModeImproved}
	for _, opt := range opts {
		opt(&cfg)
	}

	if !cfg.gravity.valid() {
		return nil, fmt.Errorf("%w: unknown gravity model %v", ErrElements, cfg.gravity)
	}

	if cfg.mode != ModeImproved && cfg.mode != ModeAFSPC {
		return nil, fmt.Errorf("%w: unknown operation mode %v", ErrElements, cfg.mode)
	}

	if err := el.Validate(); err != nil {
		return nil, err
	}

	p := &Propagator{el: el, grav: constantsFor(cfg.gravity), mode: cfg.mode}

	// SGP4 carries its epoch as days from 0 January 1950, 0h. Formed from the
	// two-part Julian Date so the fraction survives: a single float64 JD near
	// 2.46e6 resolves to about 60 microseconds, and the whole point of taking a
	// float time argument is not to throw that away at the other end.
	jd1, jd2 := el.Epoch.UTC().JDParts()
	epoch1950 := (jd1 - jd1950) + jd2

	p.initl(epoch1950)
	p.initNearEarth()

	if p.deep {
		p.initDeepSpace(epoch1950)
	}

	return p, nil
}

// At returns the TEME position (km) and velocity (km/s) at tsince minutes from
// the element set's epoch. Negative tsince propagates backwards.
//
// # The time argument is a float, and that is the point
//
// SGP4's own argument is minutes-since-epoch as a real number, and taking it
// directly is what lets this package be compared against the reference states
// with no conversion at all. An implementation that offers only whole seconds
// forces its caller to recover the remainder by stepping along the velocity
// vector, and to know that the element epoch was truncated the same way — a
// subtlety that cost astrogo 5.94 km on Vallado's satellite 5 before its
// reference suite caught it.
//
// # An error may arrive with a usable state
//
// [ErrDecayed] and [ErrKeplerNotConverged] describe a result that exists and
// should not be trusted, so they are returned alongside the state that was
// computed. Every other error leaves both vectors zero. The distinction is
// stated here because `if err != nil { return }` is the right handling for both
// — a caller who wants to look at a decayed satellite's position has to ask for
// it deliberately.
func (p *Propagator) At(tsince float64) (pos, vel vector.Vec3, err error) {
	return p.evaluate(tsince)
}

// AtTime is [Propagator.At] for an absolute instant.
//
// t is converted to UTC first. SGP4 is defined against UTC, and the conversion
// is not a formality: a TT instant taken at face value lands 69.184 s late,
// which for a low Earth orbit is 530 km. [time.Time] is scale-aware precisely
// so this cannot be left to the caller.
func (p *Propagator) AtTime(t time.Time) (pos, vel vector.Vec3, err error) {
	// Two-part throughout, so a query far from epoch does not lose the
	// fraction to a subtraction of two large numbers.
	jd1, jd2 := t.UTC().JDParts()
	e1, e2 := p.el.Epoch.UTC().JDParts()

	tsince := ((jd1 - e1) + (jd2 - e2)) * 1440.0

	return p.At(tsince)
}

// Elements returns the element set this propagator was built from.
func (p *Propagator) Elements() Elements { return p.el }

// DeepSpace reports whether the element set takes SGP4's deep-space branch — a
// period of 225 minutes or more, which is the model's own threshold.
func (p *Propagator) DeepSpace() bool { return p.deep }

// SimplifiedDrag reports whether the element set takes SGP4's simplified drag
// model, which it does below a perigee altitude of 220 km and always in deep
// space. It is the model's own `isimp` branch.
func (p *Propagator) SimplifiedDrag() bool { return p.isimp }

// PerigeeAltitude returns the perigee altitude in km, as SGP4 itself computes
// it.
//
// Not the two-body value. A TLE's mean motion is a Kozai mean element and the
// model un-Kozai's it before deriving a semi-major axis, a correction worth
// about 1.4 km of perigee for a low orbit — the difference between reproducing
// the branch SGP4 takes and merely landing near it.
func (p *Propagator) PerigeeAltitude() float64 { return (p.rp - 1.0) * p.grav.radiusKM }

// ApogeeAltitude returns the apogee altitude in km, on the same basis as
// [Propagator.PerigeeAltitude].
func (p *Propagator) ApogeeAltitude() float64 { return p.alta * p.grav.radiusKM }
