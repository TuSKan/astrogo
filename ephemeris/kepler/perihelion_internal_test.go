package kepler

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestSolveUniversalConvergesAcrossTheConics sweeps the universal Kepler
// solver over the shapes and ranges FromPerihelion accepts: e from a circle
// to 50, q from 0.01 AU to 40, and Δt across half a period of each ellipse
// or out to 2,700 years for an open orbit. Every solve must converge, and to
// a residual at the level of the arithmetic.
//
// It is the one test that reaches the corners the bracket exists for. The
// real comets elsewhere converge from a good start either way; here a
// near-parabolic hyperbola far out sends Newton's first step past the
// bracket, and a strong hyperbola far out is where the equation grows like
// sinh.
func TestSolveUniversalConvergesAcrossTheConics(t *testing.T) {
	auMeters := unit.AU(1).Meters()
	day := constants.Derived.JulianDaySeconds.Value
	mu := constants.IAU.SunGravitationalParameter.Value * day * day / (auMeters * auMeters * auMeters)
	sqrtMu := math.Sqrt(mu)

	// Measured worst over this grid: 1.7e-14, at e = 1.0001.
	const tol = 1e-13

	for _, e := range []float64{0, 0.3, 0.7, 0.9, 0.97, 0.99, 0.999, 0.9999, 1, 1.0001, 1.01, 1.2, 2, 5, 50} {
		for _, q := range []float64{0.01, 0.3, 1, 5, 40} {
			alpha := (1 - e) / q

			span := 1e6 // days, for an orbit that does not close
			if alpha > 0 {
				span = math.Pi / math.Sqrt(mu*alpha*alpha*alpha)
			}

			for i := -100; i <= 100; i++ {
				dt := span * float64(i) / 100

				chi, err := solveUniversal(q, e, alpha, sqrtMu, dt)
				if err != nil {
					t.Fatalf("e = %v, q = %v, Δt = %v d: %v", e, q, dt, err)
				}

				if dt == 0 {
					continue
				}

				_, s := stumpff(alpha * chi * chi)
				residual := math.Abs(e*chi*chi*chi*s+q*chi-sqrtMu*dt) / (sqrtMu * math.Abs(dt))

				if !(residual <= tol) {
					t.Errorf("e = %v, q = %v, Δt = %v d: χ = %v leaves a relative residual of %.2g",
						e, q, dt, chi, residual)
				}
			}
		}
	}
}

// TestStateAtRefusesAnInstantThatIsNotANumber checks both paths fail rather
// than hand back a position of NaNs. An instant that is not a number makes
// every anomaly one too; Kepler's equation cannot be solved for it, in either
// form, and StateAt says so.
func TestStateAtRefusesAnInstantThatIsNotANumber(t *testing.T) {
	epoch := time.FromJDParts(2460000.5, 0, time.TT)

	classical, err := NewElements(epoch, unit.AU(2.5), 0.2, angle.Deg(10), angle.Deg(20), angle.Deg(30), angle.Deg(40))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	hyperbola, err := FromPerihelion(epoch, unit.AU(0.5), 1.3, angle.Deg(10), angle.Deg(20), angle.Deg(30))
	if err != nil {
		t.Fatalf("FromPerihelion: %v", err)
	}

	for name, el := range map[string]Elements{"NewElements": classical, "FromPerihelion": hyperbola} {
		pos, vel, err := el.StateAt(time.FromJD(math.NaN(), time.TT))
		if !errors.Is(err, ErrKeplerNoConverge) {
			t.Errorf("%s: StateAt(NaN) = %v, %v, %v; want ErrKeplerNoConverge", name, pos, vel, err)
		}
	}
}

// TestValidateRefusesABadPerihelionDistance checks the perihelion form's own
// validation, which FromPerihelion pre-empts for its arguments but Validate
// exists to repeat for a value built any other way.
func TestValidateRefusesABadPerihelionDistance(t *testing.T) {
	for _, q := range []float64{-1, math.NaN(), math.Inf(1)} {
		el := Elements{perihelion: unit.AU(q), eccentricity: 1}

		if err := el.Validate(); !errors.Is(err, ErrInvalidElements) {
			t.Errorf("q = %v: Validate = %v, want ErrInvalidElements", q, err)
		}
	}
}

// TestStumpffSeriesMeetsTheClosedForm checks the switch at |z| = 0.1, where
// the Maclaurin series hands over to the closed forms: the two must agree on
// either side of it, for both signs of z, to within the closed forms' own
// cancellation, about 6ε/|z|.
func TestStumpffSeriesMeetsTheClosedForm(t *testing.T) {
	closed := func(z float64) (c, s float64) {
		if z > 0 {
			sz := math.Sqrt(z)

			return (1 - math.Cos(sz)) / z, (sz - math.Sin(sz)) / (sz * sz * sz)
		}

		sz := math.Sqrt(-z)

		return (math.Cosh(sz) - 1) / -z, (math.Sinh(sz) - sz) / (sz * sz * sz)
	}

	for _, z := range []float64{0.0999999, -0.0999999, 0.05, -0.05} {
		cs, ss := stumpff(z)
		cc, sc := closed(z)

		limit := 10 * 2.2e-16 / math.Abs(z)
		if math.Abs(cs-cc)/cc > limit || math.Abs(ss-sc)/sc > limit {
			t.Errorf("z = %v: series C, S = %.17g, %.17g; closed forms %.17g, %.17g", z, cs, ss, cc, sc)
		}
	}

	// And at zero, the limits 1/2 and 1/6 exactly.
	if c, s := stumpff(0); c != 0.5 || s != 1.0/6 {
		t.Errorf("C(0), S(0) = %v, %v; want 1/2 and 1/6", c, s)
	}
}
