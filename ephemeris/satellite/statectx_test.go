package satellite_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func paranal(t *testing.T) *coord.Geodetic {
	t.Helper()

	site, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), unit.Meters(2635))
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return site
}

// TestStateCtxIsStateAtTheContextInstant: through a Context built at the
// instant, StateCtx is State — the same SGP4 answer, rotated by one route
// through the Context's celestial-to-intermediate matrix rather than the other
// through a fresh bias-precession-nutation matrix, which agree to rounding.
// Across a pass, so the instants are not all the element set's epoch.
func TestStateCtxIsStateAtTheContextInstant(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	site := paranal(t)
	start := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)

	// Rounding in two 3×3 rotations, relative: 1e-12 is 7 µm at the ISS.
	const tol = 1e-12

	for i := range 12 {
		at := start.Add(unit.Minutes(float64(10 * i)))
		ctx := coord.NewContext(at, site, atmosphere.Refraction{})

		want, err := sat.State(sat.ID(), at)
		if err != nil {
			t.Fatalf("State: %v", err)
		}

		got, err := sat.StateCtx(sat.ID(), ctx)
		if err != nil {
			t.Fatalf("StateCtx: %v", err)
		}

		if got.Frame != want.Frame || got.Center != want.Center {
			t.Errorf("%v: StateCtx is in %v about %v, State in %v about %v",
				at, got.Frame, got.Center, want.Frame, want.Center)
		}

		if d := got.Pos.Sub(want.Pos).Norm() / want.Pos.Norm(); d > tol {
			t.Errorf("%v: positions differ by %.3g of the radius, want ≤ %g", at, d, tol)
		}

		if d := got.Vel.Sub(want.Vel).Norm() / want.Vel.Norm(); d > tol {
			t.Errorf("%v: velocities differ by %.3g of the speed, want ≤ %g", at, d, tol)
		}
	}
}

// TestStateCtxRejectsUnexpectedID holds StateCtx to State's contract: this
// provider answers for its own satellite only.
func TestStateCtxRejectsUnexpectedID(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	ctx := coord.NewContext(time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC), paranal(t), atmosphere.Refraction{})

	if _, err := sat.StateCtx(eph.Sun, ctx); !errors.Is(err, satellite.ErrUnexpectedID) {
		t.Errorf("StateCtx(eph.Sun, ...) = %v, want ErrUnexpectedID", err)
	}
}
