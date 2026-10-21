package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Oppositions returns the oppositions of target to the Sun in the given
// interval: the instants their apparent geocentric ecliptic longitudes differ
// by 180° (see [EventOpposition]). One of target and other must be the Sun, in
// either order, or it returns [ErrOppositionNeedsSun]. With DE440s the
// oppositions of Mars in 2003 and 2027 and of Saturn in 2026 fall within a
// minute of JPL Horizons', and the Full Moon of 2026-09-26 within a second of
// Skyfield's.
//
// It does not search the apparent places. An opposition is an alignment: the
// target on the line from the Sun through the Earth, beyond the Earth. The
// search scans that alignment on one ephemeris read per body and instant, in
// the target's natural frame: about the Sun for a planet, an asteroid or a
// comet, where its heliocentric longitude meets the Earth's, and about the
// Earth for the Moon, or anything else inside the Earth's Hill sphere. A
// target that is not a [MovingBody] is taken to stand still on the sky, as a
// [Star], a [DeepSkyObject] and a [Constellation] do.
//
// Each step is the longest in which the longitude difference cannot reach an
// alignment: the states give its value and rate, and Kepler's second law
// bounds how fast the rate can change (see [longitudeCurvatureBound]). The
// steps close on an alignment as Newton's method does, and step over it once
// it is within the solver's tolerance. The scan places both bodies to first
// order in the light time, which leaves its alignment within a fraction of a
// second of the apparent opposition, and two evaluations of the apparent
// places then bracket that to the tolerance.
//
// Until #787 this sampled the apparent places of both bodies once a day.
// Over 2000–2040 the two find the same oppositions of Mars, Jupiter, Saturn,
// Uranus and Neptune, and over 2000–2004 the same Full Moons, within 0.3 s:
// see TestOppositionsMatchADailyScan.
func Oppositions(start, end time.Time, target, other Observable) ([]Event, error) {
	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventOpposition,
		Target: target,
		Other:  other,
	}
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	if err := checkInterval(start, end); err != nil {
		return nil, err
	}

	if isSun(target) {
		target, other = other, target
	}

	sun, ok := other.(MovingBody)
	if !ok || sun.EphID() != eph.Sun || isSun(target) {
		return nil, ErrOppositionNeedsSun
	}

	scan, err := newOppositionScan(target, sun, start)
	if err != nil {
		return nil, err
	}

	return scan.find(start, end, DefaultSolver())
}

// FullMoonOppositions returns the full moons (Sun-Moon Oppositions) in the given interval:
// the Moon's apparent ecliptic longitude 180° from the Sun's, which is
// [Oppositions] of the Moon. With DE440s the Full Moon of 2026-09-26 falls
// within a second of Skyfield's.
// For eclipses, use LunarEclipses, which decides each Full Moon by Earth's shadow.
func FullMoonOppositions(start, end time.Time, provider eph.Provider) ([]Event, error) {
	return Oppositions(start, end, NewMoon(provider), NewSun(provider))
}

// isSun reports whether o is the Sun.
func isSun(o Observable) bool {
	b, ok := o.(MovingBody)

	return ok && b.EphID() == eph.Sun
}

// Gravitational parameters in AU³/day², the units of an ephemeris state.
var (
	gmSun       = gmAUDay(constants.Ephemeris.SunGravitationalParameter)
	gmEarthMoon = gmAUDay(constants.Ephemeris.EarthMoonGravitationalParameter)
)

// gmAUDay converts a gravitational parameter from m³/s² to AU³/day².
func gmAUDay(c constants.Constant) float64 {
	day := constants.Derived.JulianDaySeconds.Value
	au := constants.IAU.AstronomicalUnit.Value

	return c.Value * day * day / (au * au * au)
}

// perturbationAllowance multiplies the two-body bound on how fast the
// longitude difference's rate can change. Each orbit is the osculating one,
// which other bodies bend. The largest bend measured is the Moon's pull on
// the Earth, which carries it about their barycentre each month and takes
// the Earth 13% past its bound; no planet and not the Moon passes its own
// (TestLongitudeCurvatureBoundHolds). Doubling leaves room to spare, and an
// alignment a step still oversteps is rooted in that step, not missed.
const perturbationAllowance = 2

// oppositionScan searches the alignments of one body with the Sun.
type oppositionScan struct {
	target Observable
	sun    MovingBody
	// body is the target's ephemeris; nil for a target fixed on the sky.
	body MovingBody
	// aboutEarth is the target's natural frame: about the Earth, or the Sun.
	aboutEarth bool
}

// newOppositionScan picks the target's natural frame from its place at start:
// about the Earth if it is inside the Earth's Hill sphere, where the Earth's
// pull on it outweighs the Sun's tide, and about the Sun otherwise.
func newOppositionScan(target Observable, sun MovingBody, start time.Time) (*oppositionScan, error) {
	o := &oppositionScan{target: target, sun: sun}

	body, ok := target.(MovingBody)
	if !ok {
		return o, nil
	}

	o.body = body

	st, err := body.Provider().State(body.EphID(), start)
	if err != nil {
		return nil, fmt.Errorf("events: target position: %w", err)
	}

	sunSt, err := sun.Provider().State(eph.Sun, start)
	if err != nil {
		return nil, fmt.Errorf("events: other position: %w", err)
	}

	hill := sunSt.Pos.Norm() * math.Cbrt(gmEarthMoon/(3*gmSun))
	o.aboutEarth = st.Pos.Norm() < hill

	return o, nil
}

// oppositionSample is the scan's view of one instant.
type oppositionSample struct {
	// phi is the target's ecliptic longitude about its centre less the
	// Earth's about the Sun, in radians within (−π, π]: zero when the three
	// are aligned.
	phi float64
	// rate is phi's rate, and curv a bound on how fast that can change, in
	// radians per day and per day².
	rate, curv float64
	// beyond reports whether the target is farther from the Sun than the
	// Earth, in the ecliptic, which makes an alignment an opposition rather
	// than an inferior conjunction.
	beyond bool
}

// sample evaluates the scan at t, on the ecliptic of date.
//
// phi is taken from the first-order apparent places of both bodies (see
// firstOrderPosition): the longitudes of two geocentric vectors a and b
// differ by 180° exactly when a − b points the way −b does and is the longer
// of the two, whatever a and b are, so the alignment this scans for is the
// apparent opposition to first order. rate and curv are the geometric
// orbits', which the light time moves by parts in ten thousand.
func (o *oppositionScan) sample(t time.Time) (oppositionSample, error) {
	tt1, tt2 := t.TT().JDParts()
	rm := gofaext.Ecm06(tt1, tt2)
	ecliptic := func(v vector.Vec3) vector.Vec3 {
		return vector.Vec3{
			X: rm[0][0]*v.X + rm[0][1]*v.Y + rm[0][2]*v.Z,
			Y: rm[1][0]*v.X + rm[1][1]*v.Y + rm[1][2]*v.Z,
			Z: rm[2][0]*v.X + rm[2][1]*v.Y + rm[2][2]*v.Z,
		}
	}

	sunSt, err := o.sun.Provider().State(eph.Sun, t)
	if err != nil {
		return oppositionSample{}, fmt.Errorf("events: other position: %w", err)
	}

	// The Earth about the Sun is the geocentric Sun reversed.
	b := ecliptic(firstOrderPosition(sunSt))
	lonE := math.Atan2(-b.Y, -b.X)
	rS, vS := ecliptic(sunSt.Pos), ecliptic(sunSt.Vel)
	rateE := longitudeRate(rS, vS)
	curvE := longitudeCurvatureBound(rS.MulScalar(-1), vS.MulScalar(-1), gmSun+gmEarthMoon)

	if o.body == nil {
		p, err := o.target.Position(t)
		if err != nil {
			return oppositionSample{}, fmt.Errorf("events: target position: %w", err)
		}

		a := ecliptic(p.ToUnitVector())

		return oppositionSample{
			phi:    math.Remainder(math.Atan2(a.Y, a.X)-lonE, 2*math.Pi),
			rate:   -rateE,
			curv:   perturbationAllowance * curvE,
			beyond: true,
		}, nil
	}

	st, err := o.body.Provider().State(o.body.EphID(), t)
	if err != nil {
		return oppositionSample{}, fmt.Errorf("events: target position: %w", err)
	}

	a := ecliptic(firstOrderPosition(st))
	r, v, mu := ecliptic(st.Pos), ecliptic(st.Vel), gmEarthMoon

	if !o.aboutEarth {
		a = a.Sub(b)
		r, v, mu = r.Sub(rS), v.Sub(vS), gmSun
	}

	return oppositionSample{
		phi:    math.Remainder(math.Atan2(a.Y, a.X)-lonE, 2*math.Pi),
		rate:   longitudeRate(r, v) - rateE,
		curv:   perturbationAllowance * (longitudeCurvatureBound(r, v, mu) + curvE),
		beyond: o.aboutEarth || a.X*a.X+a.Y*a.Y > b.X*b.X+b.Y*b.Y,
	}, nil
}

// find scans [start, end] for alignments and returns those that are
// oppositions, refined on the apparent places.
func (o *oppositionScan) find(start, end time.Time, solver Solver) ([]Event, error) {
	const turn = 2 * math.Pi

	tol := solver.Tolerance.Days()

	var events []Event

	keep := func(tr time.Time, s oppositionSample) error {
		if !s.beyond {
			return nil
		}

		ev, err := o.refine(tr, solver)
		if err != nil {
			return err
		}

		if !ev.Time.Before(start) && !ev.Time.After(end) {
			events = append(events, ev)
		}

		return nil
	}

	t := start

	s, err := o.sample(t)
	if err != nil {
		return nil, err
	}

	// u is phi unwrapped along the scan: an alignment is u at a multiple of
	// a turn.
	u := s.phi

	for {
		below := turn * math.Floor(u/turn)
		dDown, dUp := u-below, below+turn-u

		// An alignment the scan is certain to reach within the tolerance,
		// even curving away from it as fast as it can, root days ahead. On
		// one exactly, it is crossing it now.
		root, reached := 0.0, dDown == 0

		switch {
		case reached:
		case s.rate < 0 && dDown <= -s.rate*tol-s.curv*tol*tol/2:
			root, reached = dDown/-s.rate, true
		case s.rate > 0 && dUp <= s.rate*tol-s.curv*tol*tol/2:
			root, reached = dUp/s.rate, true
		}

		var step float64

		if reached {
			tr := t.Add(unit.Days(root))
			if tr.After(end) {
				return events, nil
			}

			if err := keep(tr, s); err != nil {
				return nil, err
			}

			// Step over it, to where the difference is surest to have left
			// it: it cannot turn back to it in under 2|rate|/curv, and is
			// farthest from it at half that. Nor may it reach the next
			// alignment, a turn away.
			speed := math.Abs(s.rate)
			step = root + math.Max(math.Min(speed/s.curv, reachTime(turn, speed, s.curv)), tol)
		} else {
			step = math.Min(reachTime(dUp, s.rate, s.curv), reachTime(dDown, -s.rate, s.curv))
			if math.IsNaN(step) || step <= 0 || math.IsInf(step, 1) {
				return nil, fmt.Errorf("%w: %s at %v", ErrOppositionScan, o.target.Name(), t)
			}
		}

		next := t.Add(unit.Days(step))
		if next.After(end) {
			next = end
		}

		if !next.After(t) {
			return events, nil
		}

		s2, err := o.sample(next)
		if err != nil {
			return nil, err
		}

		// Unwrap about where the rate predicts, which the curvature bound
		// keeps within half a turn of where it is.
		pred := u + s.rate*next.Sub(t).Days()
		u2 := pred + math.Remainder(s2.phi-pred, turn)

		// An alignment the step crossed, which the bound said it could not:
		// root it in the step rather than miss it. One the step landed on
		// exactly is the next one's to take.
		if below2 := turn * math.Floor(u2/turn); !reached && below2 != below && u2 != below2 {
			tr, sr, err := o.crossed(t, next, u, u2, s.rate, solver)
			if err != nil {
				return nil, err
			}

			if !tr.After(end) {
				if err := keep(tr, sr); err != nil {
					return nil, err
				}
			}
		}

		t, s, u = next, s2, u2
	}
}

// crossed roots the alignment a step from t1 to t2 crossed, u running from u1
// to u2 across a multiple of a turn, and returns it with the sample last taken
// there.
func (o *oppositionScan) crossed(t1, t2 time.Time, u1, u2, rate float64, solver Solver) (time.Time, oppositionSample, error) {
	const turn = 2 * math.Pi

	zero := turn * math.Floor(math.Max(u1, u2)/turn)

	var last oppositionSample

	g := func(t time.Time) (float64, error) {
		s, err := o.sample(t)
		if err != nil {
			return 0, err
		}

		last = s
		pred := u1 + rate*t.Sub(t1).Days()

		return pred + math.Remainder(s.phi-pred, turn) - zero, nil
	}

	tr, _, err := solver.findRootFrom(g, t1, t2, u1-zero, u2-zero)

	return tr, last, err
}

// refine brackets the apparent opposition about tr, an alignment of the
// first-order places, and roots it there. The two differ by the light time's
// second order and the Sun's deflection of the light, a fraction of a second
// of time, so a bracket of the solver's tolerance about tr seldom needs to
// grow; when it does, it widens on both sides.
func (o *oppositionScan) refine(tr time.Time, solver Solver) (Event, error) {
	exact := func(t time.Time) (float64, error) {
		pos1, err := o.target.Position(t)
		if err != nil {
			return 0, fmt.Errorf("events: target position: %w", err)
		}

		pos2, err := o.sun.Position(t)
		if err != nil {
			return 0, fmt.Errorf("events: other position: %w", err)
		}

		return eclipticDifference(EventOpposition, pos1, pos2, t), nil
	}

	half := solver.Tolerance / 2
	ta, tb := tr.Add(-half), tr.Add(half)

	fa, err := exact(ta)
	if err != nil {
		return Event{}, err
	}

	fb, err := exact(tb)
	if err != nil {
		return Event{}, err
	}

	for i := 0; sameSign(fa, fb); i++ {
		if i == solver.MaxIter {
			return Event{}, fmt.Errorf("%w: no apparent opposition about %v", ErrBracketingViolated, tr)
		}

		w := tb.Sub(ta)
		ta, tb = ta.Add(-w), tb.Add(w)

		if fa, err = exact(ta); err != nil {
			return Event{}, err
		}

		if fb, err = exact(tb); err != nil {
			return Event{}, err
		}
	}

	te, val, err := solver.findRootFrom(exact, ta, tb, fa, fb)
	if err != nil {
		return Event{}, err
	}

	return Event{Kind: EventOpposition, Time: te, Value: val}, nil
}

// reachTime is the least time in which a quantity d away, approached at rate
// b and accelerating toward it at most a, can arrive: the positive root of
// b·τ + a·τ²/2 = d, written so that neither sign of b cancels.
func reachTime(d, b, a float64) float64 {
	if a == 0 {
		if b > 0 {
			return d / b
		}

		return math.Inf(1)
	}

	disc := math.Sqrt(b*b + 2*a*d)
	if b >= 0 {
		return 2 * d / (b + disc)
	}

	return (disc - b) / a
}

// longitudeRate is the rate of the ecliptic longitude of r for a velocity v,
// in radians per unit of v's time.
func longitudeRate(r, v vector.Vec3) float64 {
	return (r.X*v.Y - r.Y*v.X) / (r.X*r.X + r.Y*r.Y)
}

// longitudeCurvatureBound bounds how fast the rate of ecliptic longitude can
// change, in radians per day², on the two-body orbit through the state (r, v)
// about a centre of gravitational parameter mu (AU, AU/day, AU³/day²).
//
// By Kepler's second law the longitude turns at L' = h_z/ρ², h_z the ecliptic
// component of the angular momentum and ρ the distance projected on the
// ecliptic, so L″ = −2·h_z·ρ'/ρ³. On an orbit of perihelion distance q,
// eccentricity e and inclination i:
//
//   - ρ = r·cos β is at least q·|cos i|, the latitude β never passing i;
//   - ρ' = r'·cos β − r·β'·sin β, where |r'| ≤ μe/h on any conic and
//     r·|β'·sin β| ≤ h·sin²i/(2ρ), from sin β = sin i·sin u and u' = h/r²;
//
// so |L″| ≤ μ·(2e·|cos i| + (1 + e)·sin²i) / (q³·|cos i|³): the planar
// 2μe/q³ when i is zero, and zero for a circle in the ecliptic. It holds for
// an orbit that is not closed too. An orbit through the ecliptic's pole has
// none, its longitude turning arbitrarily fast near the pole.
func longitudeCurvatureBound(r, v vector.Vec3, mu float64) float64 {
	hv := r.Cross(v)
	h2 := hv.Norm2()
	cosi := math.Abs(hv.Z) / math.Sqrt(h2)

	// The eccentricity vector, which keeps a small e to its rounding where
	// e² = 1 − h²/(μa) would leave it to the square root of that.
	e := v.Cross(hv).DivScalar(mu).Sub(r.Unit()).Norm()
	q := h2 / (mu * (1 + e))
	sin2i := 1 - cosi*cosi

	return mu * (2*e*cosi + (1+e)*sin2i) / (q * q * q * cosi * cosi * cosi)
}

// eclipticDifference is the geocentric ecliptic longitude of pos1 less pos2's
// on the ecliptic of date, in degrees within (−180, 180]: zero at a
// conjunction in longitude. For an opposition it is measured from 180°
// instead, so that it is zero there.
func eclipticDifference(kind EventKind, pos1, pos2 coord.ICRS, t time.Time) float64 {
	diff := wrap180(coord.ICRSToEcliptic(pos1, t).Lon().Degrees() - coord.ICRSToEcliptic(pos2, t).Lon().Degrees())

	if kind == EventOpposition {
		if diff > 0 {
			diff -= 180
		} else {
			diff += 180
		}
	}

	return diff
}
