package keplerlab_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// elementsObliquity is the obliquity of the J2000 ecliptic published
// elements are referred to, 84381.448″ — kepler's own
// elementsEclipticObliquity (ephemeris/kepler/kepler.go), restated here
// because it is unexported.
const elementsObliquity = 84381.448 * math.Pi / 648000

// preparedOrbit is a prototype of a prepared two-body orbit: everything in
// kepler.Elements that does not depend on the epoch, computed once — the
// mean motion, the semi-minor axis, and the orientation as the two unit
// vectors P and Q along which the perifocal x and y lie in the ICRF.
type preparedOrbit struct {
	epoch          time.Time
	m0, n, a, b, e float64
	px, py, pz     float64
	qx, qy, qz     float64
}

func prepare(el kepler.Elements) preparedOrbit {
	a := el.SemiMajorAxis().AU()
	aM := el.SemiMajorAxis().Meters()
	e := el.Eccentricity()

	o := preparedOrbit{
		epoch: el.Epoch(),
		m0:    el.MeanAnomaly().Radians(),
		n:     math.Sqrt(el.CentralBody().GM/(aM*aM*aM)) * 86400,
		a:     a,
		b:     a * math.Sqrt((1-e)*(1+e)),
		e:     e,
	}

	// The columns of Rx(ε)·Rz(Ω)·Rx(i)·Rz(ω) applied to x̂ and ŷ.
	so, co := math.Sincos(el.AscendingNode().Radians())
	si, ci := math.Sincos(el.Inclination().Radians())
	sw, cw := math.Sincos(el.ArgPeriapsis().Radians())
	se, ce := math.Sincos(elementsObliquity)

	ecl := func(x, y float64) (float64, float64, float64) {
		// Rz(ω), then Rx(i), then Rz(Ω), in the ecliptic frame.
		x1, y1 := cw*x-sw*y, sw*x+cw*y
		y2, z2 := ci*y1, si*y1

		return co*x1 - so*y2, so*x1 + co*y2, z2
	}

	toEq := func(x, y, z float64) (float64, float64, float64) {
		return x, ce*y - se*z, se*y + ce*z
	}

	o.px, o.py, o.pz = toEq(ecl(1, 0))
	o.qx, o.qy, o.qz = toEq(ecl(0, 1))

	return o
}

// stateAt is the state at t: the phase from one fused multiply-add and an
// exact reduction, the stable Markley solve, the stable construction, and
// two vectors scaled and added.
func (o *preparedOrbit) stateAt(t time.Time) (pos, vel vector.Vec3) {
	m := reduceExact(math.FMA(o.n, t.Sub(o.epoch).Days(), o.m0))
	x, y, vx, vy := stableState(o.a, o.e, o.n, markleyStable(m, o.e))

	return vector.Vec3{X: o.px*x + o.qx*y, Y: o.py*x + o.qy*y, Z: o.pz*x + o.qz*y},
		vector.Vec3{X: o.px*vx + o.qx*vy, Y: o.py*vx + o.qy*vy, Z: o.pz*vx + o.qz*vy}
}

// ceresLike is a main-belt orbit with every angle non-zero, so the
// orientation is exercised in full. The values are illustrative, not Ceres's
// published elements.
func ceresLike(tb testing.TB) kepler.Elements {
	tb.Helper()

	el, err := kepler.NewElements(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		unit.AU(2.77), 0.0785, angle.Deg(10.6), angle.Deg(80.3), angle.Deg(73.6), angle.Deg(291.4))
	if err != nil {
		tb.Fatalf("elements: %v", err)
	}

	return el
}

// TestPreparedOrbitAgrees: the prototype is the same two-body problem as
// kepler.Elements.StateAt, so over two revolutions the two agree to the
// rounding of their different arithmetic — a bound, not bit identity, since
// the arithmetic differs.
func TestPreparedOrbitAgrees(t *testing.T) {
	el := ceresLike(t)
	o := prepare(el)

	var worst float64

	for d := range 3400 { // about two revolutions, daily
		at := el.Epoch().Add(unit.Days(float64(d)))

		want, wantV, err := el.StateAt(at)
		if err != nil {
			t.Fatalf("StateAt: %v", err)
		}

		got, gotV := o.stateAt(at)

		worst = math.Max(worst, got.Sub(want).Norm()/want.Norm())
		worst = math.Max(worst, gotV.Sub(wantV).Norm()/wantV.Norm())
	}

	t.Logf("prepared orbit against StateAt over two revolutions: %.3g relative at worst", worst)

	// A few ulps of each, compounded through the orientation: 1e-14 is
	// fifty ulps, far above either path's rounding and far below any error
	// in the formulas, which would be of order the quantities themselves.
	if worst > 1e-14 {
		t.Errorf("the prepared orbit differs from StateAt by %.3g relative", worst)
	}
}
