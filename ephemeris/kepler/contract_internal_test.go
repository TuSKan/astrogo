package kepler

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/oracle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// The numerical contract docs/kepler.md sets, held against the 256-bit oracle
// in internal/oracle.
//
// The anomaly bound is the measured worst case, 1.52 ulps, with a margin of
// under one ulp, so it fails on a regression of a single ulp's worth of care
// rather than waiting for a gross one. The state is not held to a fixed
// number of ulps, because it cannot be: its sensitivity to E grows as
// a/r = 1/(1 − e·cos E), a hundred at e = 0.99 near perihelion, so even the
// correctly rounded E puts the position several ulps out there. Its bound is
// the anomaly's contract carried through the exact sensitivity ∂state/∂E,
// plus contractStateULPs for the construction itself — of |r| for the
// position and of max(|v|, n·a) for the velocity, n·a being the speed on the
// circle of radius a, below which a near-parabolic orbit's speed at aphelion
// is set by π − E's own rounding.
const (
	contractAnomalyULPs = 2
	contractStateULPs   = 2
)

// TestTwoPiIsThreeExactParts holds the reduction's 2π to the oracle's: three
// float64s, each the rounding of what the earlier ones leave of 2π at 1356
// bits.
func TestTwoPiIsThreeExactParts(t *testing.T) {
	if want := oracle.TwoPi(); twoPi != want {
		t.Errorf("twoPi = %x, want %x", twoPi, want)
	}
}

// TestSolveKeplerContract holds SolveKepler and the state built from it to
// the contract over the oracle's sweep — thirteen eccentricities up to the
// last float64 below 1, M from 10⁻²⁰ to π either sign and phases a hair
// either side of whole revolutions out to 10¹² — thinned, not dropped, under
// -short: E within contractAnomalyULPs of the exact root, E(±0) = ±0 and
// E(−M) = −E(M) bit for bit, and the perifocal state within its bounds.
func TestSolveKeplerContract(t *testing.T) {
	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, e := range oracle.Eccentricities() {
		for _, z := range []float64{0, math.Copysign(0, -1)} {
			got, err := SolveKepler(angle.Rad(z), e)
			if err != nil || got.Radians() != 0 || math.Signbit(got.Radians()) != math.Signbit(z) {
				t.Errorf("e = %v: E(%v) = %v, %v; want the same zero", e, z, got.Radians(), err)
			}
		}

		for _, m := range oracle.MeanAnomalies(testing.Short()) {
			if m == 0 {
				continue
			}

			ea, err := SolveKepler(angle.Rad(m), e)
			if err != nil {
				t.Fatalf("e = %v, M = %v: %v", e, m, err)
			}

			got := ea.Radians()
			exactE := oracle.SolveExact(m, e)

			if rel, _ := oracle.ErrorOf(got, exactE); !(rel <= contractAnomalyULPs) {
				t.Errorf("e = %v, M = %v: E off by %.3g ulps, contract %d", e, m, rel, contractAnomalyULPs)
			}

			if neg, _ := SolveKepler(angle.Rad(-m), e); neg.Radians() != -got {
				t.Errorf("e = %v, M = %v: E(−M) = %v, −E(M) = %v", e, m, neg.Radians(), -got)
			}

			if math.Abs(m) > math.Pi {
				continue
			}

			el, err := NewElements(epoch, unit.AU(2.77), e, angle.Zero(), angle.Zero(), angle.Zero(), angle.Rad(m))
			if err != nil {
				t.Fatalf("e = %v, M = %v: %v", e, m, err)
			}

			pos, vel, err := el.perifocalClassical(0)
			if err != nil {
				t.Fatalf("e = %v, M = %v: %v", e, m, err)
			}

			a := el.semiMajorAxis.AU()
			aM := el.semiMajorAxis.Meters()
			n := math.Sqrt(el.CentralBody().GM/(aM*aM*aM)) * constants.Derived.JulianDaySeconds.Value

			wx, wy, wvx, wvy := oracle.StateFromE(a, e, n, exactE)
			dPos, dVel := oracle.Sensitivity(a, e, n, exactE)

			ef, _ := exactE.Float64()
			anomaly := contractAnomalyULPs * 0x1p-52 * math.Abs(ef)

			allowPos := dPos*anomaly + contractStateULPs*0x1p-52*oracle.Norm(wx, wy)
			if d := oracle.AbsErr(pos.X, pos.Y, wx, wy); !(d <= allowPos) {
				t.Errorf("e = %v, M = %v: position off by %.3g AU, contract %.3g AU", e, m, d, allowPos)
			}

			allowVel := dVel*anomaly + contractStateULPs*0x1p-52*math.Max(oracle.Norm(wvx, wvy), n*a)
			if d := oracle.AbsErr(vel.X, vel.Y, wvx, wvy); !(d <= allowVel) {
				t.Errorf("e = %v, M = %v: velocity off by %.3g AU/d, contract %.3g AU/d", e, m, d, allowVel)
			}
		}
	}
}

// TestSolveKeplerRefusesAnUnresolvablePhase: at |M| ≥ 2⁵² a float64 no longer
// resolves the phase to a radian, and a non-finite M has none; SolveKepler
// refuses both rather than return a number that means nothing. The largest
// float64 below 2⁵² is still solved.
func TestSolveKeplerRefusesAnUnresolvablePhase(t *testing.T) {
	for _, m := range []float64{0x1p52, -0x1p52, 1e300, math.Inf(1), math.Inf(-1), math.NaN()} {
		if _, err := SolveKepler(angle.Rad(m), 0.5); !errors.Is(err, ErrPhaseUnresolved) {
			t.Errorf("M = %v: err = %v, want ErrPhaseUnresolved", m, err)
		}
	}

	if _, err := SolveKepler(angle.Rad(math.Nextafter(0x1p52, 0)), 0.5); err != nil {
		t.Errorf("M just below 2⁵²: %v", err)
	}
}
