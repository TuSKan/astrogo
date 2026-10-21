package keplerlab_test

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// exactState is the perifocal state for the float64 inputs, at prec: the
// exact root E of Kepler's equation for m, then x = a(cos E − e),
// y = a√(1−e²)·sin E and their time derivatives with mean motion n.
func exactState(a, e, n, m float64) (x, y, vx, vy *big.Float) {
	return exactStateFromE(a, e, n, solveExact(m, e))
}

// stableState is the proposed state construction from E: every quantity that
// cancels near perihelion as e → 1 written as a sum of same-signed terms.
//
//	1 − cos E   = sin²E / (1 + cos E)      while cos E > 0
//	x           = a·((1 − e) − (1 − cos E))
//	r           = a·((1 − e) + e·(1 − cos E))
//	b           = a·√((1 − e)(1 + e))
//	Ė           = n·a / r
func stableState(a, e, n, bigE float64) (x, y, vx, vy float64) {
	s, c := math.Sincos(bigE)

	omc := 1 - c
	if c > 0 {
		omc = s * s / (1 + c)
	}

	delta := 1 - e
	b := a * math.Sqrt(delta*(1+e))

	x = a * (delta - omc)
	y = b * s

	edot := n * a / (a * (delta + e*omc))

	return x, y, -a * s * edot, b * c * edot
}

// distErr is |(gx, gy) − (wx, wy)| / |(wx, wy)| in units of 2⁻⁵².
func distErr(gx, gy float64, wx, wy *big.Float) float64 {
	return absErr(gx, gy, wx, wy) / norm(wx, wy) / 0x1p-52
}

// TestStateAccuracy measures the whole path from elements to a state, on
// elements evaluated at their own epoch so that M is exactly the given one:
// kepler.Elements.StateAt as it ships, against the proposed solver and
// construction. Production's orientation with zero angles is the identity
// followed by a rotation about X, so its perifocal y is hypot(Y, Z) with Y's
// sign. The table is logged; the proposal's bound is asserted in
// TestProposedContract.
func TestStateAccuracy(t *testing.T) {
	const aAU = 2.77

	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	type row struct{ pos, vel, velScale, atE, atM float64 }

	worst := map[string]map[string]*row{"production": {}, "proposed": {}}

	note := func(who, region string, pos, vel, velScale, e, m float64) {
		w := worst[who][region]
		if w == nil {
			w = &row{}
			worst[who][region] = w
		}

		if pos > w.pos {
			w.pos, w.atE, w.atM = pos, e, m
		}

		w.vel = math.Max(w.vel, vel)
		w.velScale = math.Max(w.velScale, velScale)
	}

	for _, e := range eccentricities {
		for _, m := range meanAnomalies(testing.Short()) {
			if m == 0 || math.Abs(m) > math.Pi {
				continue
			}

			el, err := kepler.NewElements(epoch, unit.AU(aAU), e, angle.Zero(), angle.Zero(), angle.Zero(), angle.Rad(m))
			if err != nil {
				t.Fatalf("e = %v, M = %v: %v", e, m, err)
			}

			// The a and n production itself uses, so that only the solver
			// and the construction differ.
			a := el.SemiMajorAxis().AU()
			aM := el.SemiMajorAxis().Meters()
			n := math.Sqrt(el.CentralBody().GM/(aM*aM*aM)) * 86400

			wx, wy, wvx, wvy := exactState(a, e, n, m)
			region := regionOf(m, e)

			pos, vel, err := el.StateAt(epoch)
			if err != nil {
				t.Fatalf("e = %v, M = %v: StateAt: %v", e, m, err)
			}

			// The velocity's scale: the speed itself, or n·a — the speed on
			// the circle of radius a — where that is larger. Near aphelion
			// of a near-parabolic orbit the speed is orders of magnitude
			// below n·a and is set by π − E, which carries E's own float64
			// rounding whatever solved for it; judged against itself there,
			// any solver looks wrong.
			fvx, _ := wvx.Float64()
			fvy, _ := wvy.Float64()
			scale := math.Max(math.Hypot(fvx, fvy), n*a) * 0x1p-52

			py := math.Copysign(math.Hypot(pos.Y, pos.Z), pos.Y)
			pvy := math.Copysign(math.Hypot(vel.Y, vel.Z), vel.Y)
			note("production", region, distErr(pos.X, py, wx, wy), distErr(vel.X, pvy, wvx, wvy),
				absErr(vel.X, pvy, wvx, wvy)/scale, e, m)

			x, y, vx, vy := stableState(a, e, n, markleyStable(m, e))
			note("proposed", region, distErr(x, y, wx, wy), distErr(vx, vy, wvx, wvy),
				absErr(vx, vy, wvx, wvy)/scale, e, m)
		}
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "\n%-11s %-34s %14s %14s %14s  %s\n",
		"path", "region", "position", "velocity", "velocity", "worst position at")
	fmt.Fprintf(&sb, "%-11s %-34s %14s %14s %14s\n", "", "", "ulps of |r|", "ulps of |v|", "of max(|v|,n·a)")

	for _, who := range []string{"production", "proposed"} {
		for _, r := range regions[:4] {
			if w := worst[who][r]; w != nil {
				fmt.Fprintf(&sb, "%-11s %-34s %14.3g %14.3g %14.3g  e=%.17g M=%.17g\n",
					who, r, w.pos, w.vel, w.velScale, w.atE, w.atM)
			}
		}
	}

	t.Log(sb.String())
}

// TestPhysicalOrbits puts the state errors in kilometres for orbits that
// exist: a main-belt asteroid, a Halley-type comet, and two long-period
// comets near perihelion, each from elements in the form NewElements takes.
// Kilometres from an exact solution of the same two-body problem: this is
// arithmetic error, not model error.
func TestPhysicalOrbits(t *testing.T) {
	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	kmPerAU := unit.AU(1).Meters() / 1e3

	var sb strings.Builder

	fmt.Fprintf(&sb, "\n%-38s %-10s %16s %16s\n", "orbit", "M", "production, km", "proposed, km")

	for _, o := range []struct {
		name string
		q, e float64 // perihelion distance (AU), eccentricity
	}{
		{"main belt (q 2.55 AU, e 0.08)", 2.55, 0.08},
		{"Halley-type (q 0.59 AU, e 0.967)", 0.59, 0.967},
		{"long-period (q 0.5 AU, e 0.9999)", 0.5, 0.9999},
		{"near-parabolic (q 1 AU, e 0.999999)", 1, 0.999999},
	} {
		for _, m := range []float64{1e-9, 1e-6, 1e-3, 0.1, 3} {
			el, err := kepler.NewElements(epoch, unit.AU(o.q/(1-o.e)), o.e, angle.Zero(), angle.Zero(), angle.Zero(), angle.Rad(m))
			if err != nil {
				t.Fatalf("%s: %v", o.name, err)
			}

			a := el.SemiMajorAxis().AU()
			aM := el.SemiMajorAxis().Meters()
			n := math.Sqrt(el.CentralBody().GM/(aM*aM*aM)) * 86400

			wx, wy, _, _ := exactState(a, o.e, n, m)

			pos, _, err := el.StateAt(epoch)
			if err != nil {
				t.Fatalf("%s: %v", o.name, err)
			}

			py := math.Copysign(math.Hypot(pos.Y, pos.Z), pos.Y)
			x, y, _, _ := stableState(a, o.e, n, markleyStable(m, o.e))

			fmt.Fprintf(&sb, "%-38s %-10.0e %16.3g %16.3g\n", o.name, m,
				absErr(pos.X, py, wx, wy)*kmPerAU, absErr(x, y, wx, wy)*kmPerAU)
		}
	}

	t.Log(sb.String())
}
