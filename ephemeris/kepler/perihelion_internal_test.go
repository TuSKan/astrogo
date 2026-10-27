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

				_, _, _, s := stumpff(alpha * chi * chi)
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
// every anomaly one too, and StateAt says so: as a phase that cannot be
// resolved in the eccentric anomaly, which does not iterate, and as a
// universal anomaly that cannot converge.
func TestStateAtRefusesAnInstantThatIsNotANumber(t *testing.T) {
	epoch := time.FromJDParts(2460000.5, 0, time.TT)

	classical, err := NewElements(epoch, unit.AU(2.5), 0.2, angle.Deg(10), angle.Deg(20), angle.Deg(30), angle.Deg(40))
	if err != nil {
		t.Fatalf("NewElements: %v", err)
	}

	hyperbola, err := FromPerihelion(epoch, epoch, unit.AU(0.5), 1.3, angle.Deg(10), angle.Deg(20), angle.Deg(30))
	if err != nil {
		t.Fatalf("FromPerihelion: %v", err)
	}

	for _, c := range []struct {
		name string
		el   Elements
		want error
	}{
		{"NewElements", classical, ErrPhaseUnresolved},
		{"FromPerihelion", hyperbola, ErrKeplerNoConverge},
	} {
		pos, vel, err := c.el.StateAt(time.FromJD(math.NaN(), time.TT))
		if !errors.Is(err, c.want) {
			t.Errorf("%s: StateAt(NaN) = %v, %v, %v; want %v", c.name, pos, vel, err, c.want)
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
