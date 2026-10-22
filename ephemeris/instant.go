package ephemeris

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Instant is one epoch of a [Provider]: the states it locates at that epoch,
// composed into a state about any center ([Instant.Geometric]) and into the
// places an observer sees ([Instant.Astrometric], [Instant.Apparent]).
//
// It remembers what it has located, so a body asked for twice at one epoch,
// the Earth above all, is evaluated once. Build one per epoch with [At] and
// ask it everything that epoch needs; it is not safe for concurrent use, and
// it is cheap enough to build that sharing one is never the point.
type Instant struct {
	p    Provider
	t    time.Time
	n    int
	memo [instantMemo]located
}

// instantMemo is how many bodies an Instant remembers: the Sun, the Earth and
// the Moon, the Earth-Moon barycenter, every planetary system's barycenter and
// center, and room besides. Past it an Instant locates again rather than
// remembering, which costs time and changes no answer.
const instantMemo = 24

// located is one remembered answer of [github.com/TuSKan/astrogo/ephemeris/core.Provider.Locate].
type located struct {
	id ID
	st State
}

// chainDepth bounds a chain of centers. The deepest a kernel nests is a
// satellite of a planet: the satellite, its planet's barycenter, the Solar
// System barycenter. A chain this long is a provider whose centers loop.
const chainDepth = 8

// At returns the Instant of p at t.
func At(p Provider, t time.Time) *Instant { return &Instant{p: p, t: t} }

// Time returns the epoch of the Instant.
func (in *Instant) Time() time.Time { return in.t }

// Geometric returns the state of target about center at the Instant's epoch:
// r_target(t) − r_center(t), both at TDB t, with no light time.
//
// It walks the two chains of centers the provider names, alternately, to the
// first body on both, and differences the two sums below it. A body is never
// taken through a center it does not need: the Moon about the Earth in a JPL
// kernel is the Moon about the Earth-Moon barycenter less the Earth about it,
// and never passes through the Solar System barycenter an AU away, which is
// where the rounding of a geocentric state composed through it would be.
//
// The axes are the ICRS's. A state located in the GCRS composes with the
// rest, since the GCRS shares the ICRS's axes; one on any other axes is
// refused with [ErrWrongFrame].
func (in *Instant) Geometric(target, center ID) (State, error) {
	var (
		tIDs, cIDs     [chainDepth + 1]ID
		tSts, cSts     [chainDepth]State
		nT, nC         = 1, 1
		frame          = FrameUnspecified
		mixed, advance bool
	)

	tIDs[0], cIDs[0] = target, center

	sum := func(sts []State) (pos, vel vector.Vec3) {
		for _, st := range sts {
			pos, vel = pos.Add(st.Pos), vel.Add(st.Vel)
		}

		return pos, vel
	}

	for {
		// The latest body on either chain may already be on the other.
		j := indexOf(cIDs[:nC], tIDs[nT-1])
		i := nT - 1

		if j < 0 {
			if i = indexOf(tIDs[:nT], cIDs[nC-1]); i >= 0 {
				j = nC - 1
			}
		}

		if j >= 0 {
			tp, tv := sum(tSts[:i])
			cp, cv := sum(cSts[:j])

			if mixed || frame == FrameUnspecified {
				frame = FrameICRS
			}

			return State{Pos: tp.Sub(cp), Vel: tv.Sub(cv), Frame: frame, Center: center}, nil
		}

		// Step the chain whose turn it is, or the one that has not reached the
		// root; both at the root would have met above.
		ids, sts, n := &tIDs, &tSts, &nT
		if (advance && cIDs[nC-1] != SolarSystemBarycenter) || tIDs[nT-1] == SolarSystemBarycenter {
			ids, sts, n = &cIDs, &cSts, &nC
		}

		advance = !advance

		if *n > chainDepth {
			return State{}, fmt.Errorf("%w: from %v about %v", ErrChainTooDeep, target, center)
		}

		st, err := in.locate(ids[*n-1])
		if err != nil {
			return State{}, err
		}

		switch {
		case frame == FrameUnspecified:
			frame = st.Frame
		case st.Frame != frame:
			mixed = true
		}

		sts[*n-1] = st
		ids[*n] = st.Center
		*n++
	}
}

// indexOf is the position of id in ids, or −1.
func indexOf(ids []ID, id ID) int {
	for i, x := range ids {
		if x == id {
			return i
		}
	}

	return -1
}

// Observer is where an astrometric or apparent place is seen from. The zero
// value is the geocentre.
type Observer struct {
	// pos and vel are the observer's offset from the geocentre, on the ICRS's
	// axes, in AU and AU/day.
	pos, vel vector.Vec3
}

// Geocenter is the observer at the center of the Earth.
func Geocenter() Observer { return Observer{} }

// Site is the observer at ctx's site, at ctx's epoch: its geocentric position
// and the velocity the Earth's rotation gives it ([coord.Context.ObsVec],
// [coord.Context.ObsVel]). Ask an [Instant] at the same epoch, ctx.Time().
//
// An apparent place seen from a Site is SOFA's topocentric place: the light
// time to the site itself, and the aberration of the site's whole velocity,
// annual and diurnal at once. [coord.Context.TopocentricToObserved] then takes
// it to the horizon:
//
//	app, err := ephemeris.At(p, ctx.Time()).Apparent(ephemeris.Moon, ephemeris.Site(ctx))
//	altaz := ctx.TopocentricToObserved(app.Pos)
//
// The geocentric place with parallax applied afterward
// ([coord.Context.GeocentricToObserved]) differs from it by up to 0.035″ for
// the Moon (docs/ephemeris.md §5.3).
func Site(ctx *coord.Context) Observer {
	return Observer{pos: ctx.ObsVec(), vel: ctx.ObsVel()}
}

// Astrometric returns the astrometric state of target seen by obs, as SOFA
// and JPL Horizons' quantity 1 define it: the target at the epoch its light
// left, t − τ, less the observer at t,
//
//	r_target(t − τ) − r_observer(t),  τ solving |·| = c·τ,
//
// with the velocity v_target(t − τ) − v_observer(t). The target's whole
// chain is evaluated at t − τ, exactly; only the observer stays at t. No
// aberration and no light deflection: those are [Instant.Apparent]'s.
//
// The velocity omits the factor 1 − dτ/dt, which is 1 within |ṙ|/c ≤ 10⁻⁴.
// The light time is solved in TDB without the Shapiro delay, a few hundred
// microseconds at worst: meters of motion, microarcseconds of direction.
// Center is the Earth.
func (in *Instant) Astrometric(target ID, obs Observer) (State, error) {
	var a astrometric
	if err := in.astrometric(target, obs, &a); err != nil {
		return State{}, err
	}

	return State{Pos: a.pos, Vel: a.vel, Frame: FrameICRS, Center: Earth}, nil
}

// astrometric is [Instant.Astrometric]'s solution, with what
// [Instant.Apparent] goes on to need: the observer's barycentric state at t,
// and the Instant at the epoch the light left, for the Sun's place then.
type astrometric struct {
	pos, vel vector.Vec3 // the astrometric state
	ob, ov   vector.Vec3 // the observer, barycentric, at t
	then     Instant     // at t − τ
}

// Apparent returns the apparent state of target seen by obs, in SOFA's order
// (iauAtciq): the astrometric direction ([Instant.Astrometric]), deflected by
// the Sun's gravity with the true geometry (iauLd), then aberrated by the
// observer's barycentric velocity (iauAb), relativistically. The range and the
// velocity are the astrometric ones: deflection and aberration bend a ray,
// not a motion. Center is the Earth.
//
// The deflection takes the Sun to the target at t − τ, where the light left
// it, and the Sun to the observer at t, where it arrives; SOFA's iauLdsun
// limiter keeps it finite inside the solar disc. The Sun's own light is not
// deflected.
//
// Until #803 astrogo took the apparent place as the geocentric state at the
// retarded epoch, which carries aberration to first order because the Earth
// moves back with the target. Against Horizons' quantity 2 that scattered
// 9.8 mas in right ascension, and SOFA's order 1.7 mas.
func (in *Instant) Apparent(target ID, obs Observer) (State, error) {
	var a astrometric
	if err := in.astrometric(target, obs, &a); err != nil {
		return State{}, err
	}

	sun, err := in.Geometric(Sun, SolarSystemBarycenter)
	if err != nil {
		return State{}, fmt.Errorf("ephemeris: the Sun for light deflection and aberration: %w", err)
	}

	// The Sun to the observer, and how far.
	e := a.ob.Sub(sun.Pos)
	em := e.Norm()
	dir := unitArray(a.pos)

	if target != Sun {
		q, err := a.then.Geometric(target, Sun)
		if err != nil {
			return State{}, fmt.Errorf("ephemeris: the Sun at the retarded epoch for light deflection: %w", err)
		}

		if q.Pos.Norm() > 0 && em > 0 {
			// iauLdsun's limiter, verbatim: 1e-6 over the squared distance,
			// floored so an observer inside one au does not get a larger
			// limit than one at the Earth.
			dir = gofaext.Ld(1.0, dir, unitArray(q.Pos), unitArray(e), em, 1e-6/math.Max(em*em, 1))
		}
	}

	// iauAb's last term is the Sun's gravitational potential at the
	// observer, over em; an observer at the Sun's center, which only a
	// synthetic provider can put there, has it dropped rather than infinite.
	if em == 0 {
		em = math.Inf(1)
	}

	v := a.ov.DivScalar(lightAUPerDay)
	bm1 := math.Sqrt(1 - v.Norm2())
	dir = gofaext.Ab(dir, [3]float64{v.X, v.Y, v.Z}, em, bm1)

	return State{
		Pos:    vector.V3(dir[0], dir[1], dir[2]).Unit().MulScalar(a.pos.Norm()),
		Vel:    a.vel,
		Frame:  FrameICRS,
		Center: Earth,
	}, nil
}

// locate is [github.com/TuSKan/astrogo/ephemeris/core.Provider.Locate] at the Instant's epoch, remembered.
func (in *Instant) locate(id ID) (State, error) {
	for i := range in.n {
		if in.memo[i].id == id {
			return in.memo[i].st, nil
		}
	}

	st, err := in.p.Locate(id, in.t)
	if err != nil {
		return State{}, fmt.Errorf("ephemeris: locate %v: %w", id, err)
	}

	if st.Frame != FrameICRS && st.Frame != FrameGCRS {
		return State{}, fmt.Errorf("%w: %v is located on %s axes, which do not compose with the ICRS", ErrWrongFrame, id, st.Frame)
	}

	if in.n < len(in.memo) {
		in.memo[in.n] = located{id: id, st: st}
		in.n++
	}

	return st, nil
}

// observer returns the observer's barycentric position and velocity at the
// Instant's epoch.
func (in *Instant) observer(obs Observer) (pos, vel vector.Vec3, err error) {
	earth, err := in.Geometric(Earth, SolarSystemBarycenter)
	if err != nil {
		return vector.Vec3{}, vector.Vec3{}, err
	}

	return earth.Pos.Add(obs.pos), earth.Vel.Add(obs.vel), nil
}

// astrometric solves the light time from target to obs into a.
//
// Newton's method on f(τ) = |r(t − τ) − r_obs(t)| − cτ, whose derivative the
// target's velocity gives: f'(τ) = −(c + p̂·v). The first guess already moves
// the target back along its velocity at t, so the one evaluation at the
// retarded epoch usually settles it; the target is then stepped from there to
// the light time Newton's step solves for, along its velocity at that epoch.
// That step leaves the target's acceleration over it, |a|·Δτ²/2, with |a|
// measured from its velocities at t and at the retarded epoch, and the loop
// stops when that is within |v|·lightTimeTolDays, the position the tolerance
// on τ stands for. A further evaluation is a full provider read at a new
// epoch — on the analytical ephemeris an Epv00, 34 µs — so stopping at the
// first one that is good enough is the point.
func (in *Instant) astrometric(target ID, obs Observer, a *astrometric) error {
	var err error

	if a.ob, a.ov, err = in.observer(obs); err != nil {
		return fmt.Errorf("ephemeris: the observer: %w", err)
	}

	now, err := in.Geometric(target, SolarSystemBarycenter)
	if err != nil {
		return fmt.Errorf("ephemeris: %v at the observing epoch: %w", target, err)
	}

	p0 := now.Pos.Sub(a.ob)
	if p0.Norm() < 1e-12 {
		return fmt.Errorf("%w: %v seen from its own place", ErrZeroVector, target)
	}

	// |p0 − v·τ| = cτ to first order in τ: the target moving at its velocity
	// at t.
	tau := p0.Norm() / (lightAUPerDay + p0.Unit().Dot(now.Vel))

	for range lightTimeMaxIter {
		a.then = Instant{p: in.p, t: in.t.Add(unit.Days(-tau))}

		// The light time of the epoch as represented, not as asked for: an
		// epoch near JD 2.46e6 resolves to about a tenth of a microsecond,
		// which is millimeters of the Moon, and the position read is the one
		// at the epoch the provider was given.
		tau = in.t.Sub(a.then.t).Days()

		st, err := a.then.Geometric(target, SolarSystemBarycenter)
		if err != nil {
			return fmt.Errorf("ephemeris: %v retarded by light time: %w", target, err)
		}

		p := st.Pos.Sub(a.ob)
		step := (p.Norm() - lightAUPerDay*tau) / (lightAUPerDay + p.Unit().Dot(st.Vel))

		a.pos, a.vel = p.Sub(st.Vel.MulScalar(step)), st.Vel.Sub(a.ov)

		acceleration := now.Vel.Sub(st.Vel).Norm() / tau
		if acceleration*step*step/2 <= st.Vel.Norm()*lightTimeTolDays || math.Abs(step) < lightTimeTolDays {
			break
		}

		tau += step
	}

	if a.pos.Norm() < 1e-12 {
		return fmt.Errorf("%w: %v seen from its own place", ErrZeroVector, target)
	}

	return nil
}
