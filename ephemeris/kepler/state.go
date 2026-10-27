package kepler

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// stateVector is the state a set built by FromState was given: a position and
// velocity in the ICRF about the central body, in AU and AU/day.
type stateVector struct {
	pos, vel vector.Vec3
}

// FromState returns the two-body orbit through a state: pos and vel in the
// ICRF about central.ID at epoch, in AU and AU/day, under central.GM. Any
// conic.
//
// It is the form a state vector arrives in — a Horizons VECTORS answer, an
// orbit determination — and the one a barycentric refit produces (#803).
// Nothing passes through angles: the propagation starts from the state
// itself, so a state converted here and propagated by zero days is the state.
//
//   - An ellipse is propagated from the state directly, by universal
//     variables with the state's own r·v term.
//   - A hyperbola or a parabola goes through its perihelion. Far out on a
//     hyperbola the direct equation's terms grow like e^√|z| and cancel when
//     a step crosses perihelion; from perihelion nothing does. The perihelion
//     and its orientation come from the eccentricity vector v⃗×h⃗/μ − r̂, whose
//     terms stay near e + 1 and 1, where the textbook form subtracts terms
//     near v²·r/μ.
//
// The accessors report the orbit as published elements are: the J2000
// ecliptic angles, the semi-major axis or perihelion distance, the mean
// anomaly at epoch. They are derived for reporting and never read back.
//
// A state is osculating in the ICRF, so the satellite modifiers do not apply
// to it: [Elements.WithLaplacePlane], [Elements.WithSecularPrecession] and
// [Elements.WithPeriod] make [Elements.StateAt] fail. [Elements.WithCentralBody]
// re-derives the orbit through the same state under the new mass.
func FromState(epoch time.Time, pos, vel vector.Vec3, central CentralBody) (Elements, error) {
	el := Elements{epoch: epoch, central: central, state: &stateVector{pos: pos, vel: vel}}
	if err := el.Validate(); err != nil {
		return Elements{}, err
	}

	el.prepare()

	return el, nil
}

// validateStateForm is Validate for a set built by FromState.
func (el Elements) validateStateForm() error {
	pos, vel := el.state.pos, el.state.vel

	if !finite(pos.X+pos.Y+pos.Z) || !finite(vel.X+vel.Y+vel.Z) {
		return fmt.Errorf("%w: state %v, %v must be finite", ErrInvalidElements, pos, vel)
	}

	if gm := el.central.GM; !finite(gm) || gm <= 0 {
		return fmt.Errorf("%w: central body %v has mass parameter %v; a state needs a finite positive one",
			ErrInvalidElements, el.central.ID, gm)
	}

	if pos.Norm() == 0 {
		return fmt.Errorf("%w: a position at the central body itself", ErrInvalidElements)
	}

	if pos.Cross(vel).Norm() == 0 {
		return fmt.Errorf("%w: state %v, %v has no angular momentum: the motion is radial, "+
			"which no conic describes", ErrUnsupportedOrbit, pos, vel)
	}

	if el.plane != nil || el.precession.rate() != 0 || el.periodDays != 0 {
		return fmt.Errorf("%w: a state is osculating in the ICRF; a Laplace plane, a secular "+
			"precession or a period does not apply to it", ErrInvalidElements)
	}

	return nil
}

// prepareState is prepare for a set built by FromState: what the
// propagation needs, and the elements the accessors report, all from the
// state and the mass parameter.
func (el *Elements) prepareState() {
	pr := prepared{ok: true}

	day := constants.Derived.JulianDaySeconds.Value
	au := unit.AU(1).Meters()
	pr.mu = el.central.GM * day * day / (au * au * au)
	pr.sqrtMu = math.Sqrt(pr.mu)

	r0, v0 := el.state.pos, el.state.vel
	rn := r0.Norm()
	h := r0.Cross(v0)

	pr.alpha = 2/rn - v0.Dot(v0)/pr.mu

	// e⃗ = v⃗×h⃗/μ − r̂: its two terms stay near e + 1 and 1 however far out
	// the state is. See FromState.
	ev := v0.Cross(h).MulScalar(1 / pr.mu).Sub(r0.MulScalar(1 / rn))
	e := ev.Norm()
	hn := h.Norm()
	q := hn * hn / (pr.mu * (1 + e))

	w := h.MulScalar(1 / hn)

	// P along the eccentricity vector, Q completing the plane. A circle has
	// no perihelion: P is then the ascending node, as it is conventionally.
	p := ev.MulScalar(1 / e)
	if e == 0 {
		p = nodeDirection(w)
	}

	pr.p, pr.q = p, w.Cross(p)

	sigma := r0.Dot(v0) / pr.sqrtMu

	switch {
	case pr.alpha > 0:
		pr.period = 2 * math.Pi / math.Sqrt(pr.mu*pr.alpha*pr.alpha*pr.alpha)
	case e > 0:
		// The state's place measured from perihelion, for the perihelion
		// path: χ₀ in closed form, and its time from perihelion from the
		// left side of the perihelion equation, whose terms share χ₀'s sign.
		chi0 := sigma / e

		if pr.alpha < 0 {
			sqrtNegA := math.Sqrt(-pr.alpha)
			chi0 = math.Asinh(sigma*sqrtNegA/e) / sqrtNegA
		}

		_, _, _, c3 := stumpff(pr.alpha * chi0 * chi0)
		pr.tau = (q*chi0 + e*chi0*chi0*chi0*c3) / pr.sqrtMu
		pr.v0 = math.Sqrt(pr.mu * (1 + e) / q)
	}

	el.prep = pr
	el.eccentricity = e
	el.reportState(rn, sigma, w, p, q)
}

// reportState sets the elements the accessors report for a set built by
// FromState: the semi-major axis or the perihelion distance, the J2000
// ecliptic angles and the mean anomaly at epoch. None of it is read by the
// propagation.
func (el *Elements) reportState(rn, sigma float64, w, p vector.Vec3, q float64) {
	pr, e := el.prep, el.eccentricity

	el.semiMajorAxis, el.perihelion = 0, 0
	if pr.alpha > 0 {
		el.semiMajorAxis = unit.AU(1 / pr.alpha)
	} else {
		el.perihelion = unit.AU(q)
	}

	// The orientation in the J2000 ecliptic, where published angles live.
	we := w.RotateX(-elementsEclipticObliquity)
	pe := p.RotateX(-elementsEclipticObliquity)

	node := nodeDirection(we)

	el.inclination = angle.Rad(math.Atan2(math.Hypot(we.X, we.Y), we.Z))
	el.ascendingNode = angle.Rad(math.Atan2(node.Y, node.X))
	el.argPeriapsis = angle.Rad(math.Atan2(pe.Dot(we.Cross(node)), pe.Dot(node)))

	switch {
	case pr.alpha > 0:
		// e·cos E = 1 − r/a and e·sin E = σ₀/√a, so E needs no division by e,
		// and M = E − e·sin E.
		sqrtA := 1 / math.Sqrt(pr.alpha)
		esinE := sigma / sqrtA
		eccAnomaly := math.Atan2(esinE, 1-rn*pr.alpha)

		if e == 0 {
			// A circle's anomaly is measured from P, the node.
			eccAnomaly = math.Atan2(el.state.pos.Dot(pr.q), el.state.pos.Dot(pr.p))
		}

		el.meanAnomaly = angle.Rad(eccAnomaly - esinE)
	case pr.alpha < 0:
		// The hyperbolic mean anomaly, n·(t − T): Danby's convention.
		el.meanAnomaly = angle.Rad(math.Sqrt(-pr.mu*pr.alpha*pr.alpha*pr.alpha) * pr.tau)
	default:
		el.meanAnomaly = 0
	}
}

// nodeDirection is the ascending node of the plane whose pole is w, in w's
// frame: the unit vector along ẑ × ŵ. An orbit in the reference plane has
// none, and the x axis stands in for it.
func nodeDirection(w vector.Vec3) vector.Vec3 {
	n := vector.V3(-w.Y, w.X, 0)
	if nn := n.Norm(); nn > 0 {
		return n.MulScalar(1 / nn)
	}

	return vector.V3(1, 0, 0)
}

// stateAt propagates a set built by FromState dtDays from its epoch: an
// ellipse from the state, an open orbit from its perihelion. See FromState.
func (el Elements) stateAt(dtDays float64) (pos, vel vector.Vec3, err error) {
	pr := el.prep

	if pr.alpha > 0 {
		return el.fromStateDirect(dtDays)
	}

	posPf, velPf, err := el.perifocalUniversal(pr.tau + dtDays)
	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	pos, vel = el.orient(posPf, velPf, 0)

	return pos, vel, nil
}

// fromStateDirect propagates an ellipse from its state by universal
// variables, χ solving
//
//	r₀·χ·c₁ + σ₀·χ²·c₂ + χ³·c₃ = √μ·Δt,   σ₀ = r⃗₀·v⃗₀/√μ,
//
// and the Lagrange coefficients
//
//	f = 1 − χ²·c₂/r₀              g = (r₀·χ·c₁ + σ₀·χ²·c₂)/√μ
//	ḟ = −√μ·χ·c₁/(r·r₀)           ġ = (r₀·c₀ + σ₀·χ·c₁)/r
//
// with g and ġ in the forms that do not subtract: Δt − χ³·c₃/√μ and
// 1 − χ²·c₂/r are the same quantities, and they cancel once r ≫ r₀, which is
// what cost the perihelion path 317 ulps before #842.
func (el Elements) fromStateDirect(dtDays float64) (pos, vel vector.Vec3, err error) {
	pr := el.prep
	r0, v0 := el.state.pos, el.state.vel

	// A closed orbit repeats, so fold the interval to within half a period of
	// the epoch, as the perihelion path does.
	dt := dtDays - pr.period*math.Round(dtDays/pr.period)

	rn := r0.Norm()
	sigma := r0.Dot(v0) / pr.sqrtMu

	chi, err := solveUniversalFrom(rn, sigma, pr.alpha, pr.sqrtMu*dt)
	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	c0, c1, c2, _ := stumpff(pr.alpha * chi * chi)

	chi2c2 := chi * chi * c2
	r := rn*c0 + sigma*chi*c1 + chi2c2

	f := 1 - chi2c2/rn
	g := (rn*chi*c1 + sigma*chi2c2) / pr.sqrtMu
	fdot := -pr.sqrtMu * chi * c1 / (r * rn)
	gdot := (rn*c0 + sigma*chi*c1) / r

	return r0.MulScalar(f).Add(v0.MulScalar(g)), r0.MulScalar(fdot).Add(v0.MulScalar(gdot)), nil
}

// solveUniversalFrom solves r₀·χ·c₁ + σ₀·χ²·c₂ + χ³·c₃ = target for χ, by
// Newton's method inside a bracket grown from below.
//
// The bracket starts at |target|/r₀, capped at 1/√|α|, and doubles until the
// equation changes sign. Started from the upper end instead, Newton's method
// on a function that grows like sinh descends about one unit of anomaly per
// step; and a first bracket built from √μ·Δt/r₀ alone crawled, in the
// 256-bit oracle this was measured against, where the Stumpff series was
// still unconverged and returned χ = 151,656 for a root of 12.2. The left
// side is odd in χ, so the bracket grows on target's side of zero.
func solveUniversalFrom(rn, sigma, alpha, target float64) (float64, error) {
	const (
		tolerance = 1e-13
		maxGrow   = 2000
		maxIter   = 400
	)

	if target == 0 {
		return 0, nil
	}

	eval := func(chi float64) (f, fp float64) {
		c0, c1, c2, c3 := stumpff(alpha * chi * chi)
		chi2 := chi * chi

		return rn*chi*c1 + sigma*chi2*c2 + chi2*chi*c3 - target, rn*c0 + sigma*chi*c1 + chi2*c2
	}

	start := math.Abs(target / rn)
	if alpha != 0 {
		start = math.Min(start, 1/math.Sqrt(math.Abs(alpha)))
	}

	edge, near := math.Copysign(start, target), 0.0

	for range maxGrow {
		f, _ := eval(edge)
		if f == 0 || (f > 0) == (target > 0) {
			break
		}

		near, edge = edge, 2*edge
	}

	lo, hi := near, edge
	if target < 0 {
		lo, hi = edge, near
	}

	chi := lo + (hi-lo)/2
	width := hi - lo

	for range maxIter {
		f, fp := eval(chi)
		if f == 0 {
			return chi, nil
		}

		if f < 0 {
			lo = chi
		} else {
			hi = chi
		}

		// A converged step is taken whatever the bracket says, as in
		// solveUniversal: at the root the iterate is one end of the bracket,
		// and a step of a few ulps cannot land inside it.
		step := f / fp
		next := chi - step

		if math.Abs(step) <= tolerance*math.Max(1, math.Abs(chi)) && finite(next) {
			return next, nil
		}

		if !(next > lo && next < hi) || hi-lo > width/2 {
			next = lo + (hi-lo)/2
		}

		width = hi - lo
		chi = next
	}

	return 0, fmt.Errorf("%w: universal anomaly from a state after %d iterations (r₀=%v, σ₀=%v, α=%v)",
		ErrKeplerNoConverge, maxIter, rn, sigma, alpha)
}
