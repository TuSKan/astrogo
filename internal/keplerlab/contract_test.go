package keplerlab_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// The numerical contract docs/kepler.md proposes, held against the oracle
// for the proposal: Markley's starter and fifth-order correction on the
// stable residual, an exact reduction, and the stable state construction.
//
// The anomaly bound is the measured worst case with a margin of under one
// ulp, so it fails on a regression of a single ulp's worth of care rather
// than waiting for a gross one; the canary that removes the stable residual
// moves the worst case by twelve orders of magnitude.
//
// The state is not held to a fixed number of ulps, because it cannot be: its
// sensitivity to E grows as a/r = 1/(1 − e·cos E), a hundred at e = 0.99 near
// perihelion, so even the correctly rounded E puts the position several ulps
// out there. Its bound is the anomaly's contract carried through the exact
// sensitivity ∂state/∂E, plus contractStateULPs for the construction itself
// — of |r| for the position and of max(|v|, n·a) for the velocity.
const (
	contractAnomalyULPs = 2 // measured 1.52
	contractStateULPs   = 2 // the construction's own share
)

// TestProposedContract holds the proposal to the contract over the whole
// sweep: E within contractAnomalyULPs of the exact root, E(±0) = ±0 and
// E(−M) = −E(M) bit for bit, and the state within its bounds.
func TestProposedContract(t *testing.T) {
	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, e := range eccentricities {
		for _, z := range []float64{0, math.Copysign(0, -1)} {
			if got := markleyStable(z, e); got != 0 || math.Signbit(got) != math.Signbit(z) {
				t.Errorf("e = %v: E(%v) = %v, want the same zero", e, z, got)
			}
		}

		for _, m := range meanAnomalies(testing.Short()) {
			if m == 0 {
				continue
			}

			got := markleyStable(m, e)
			exactE := solveExact(m, e)

			if rel, _ := errorOf(got, exactE); !(rel <= contractAnomalyULPs) {
				t.Errorf("e = %v, M = %v: E off by %.3g ulps, contract %d", e, m, rel, contractAnomalyULPs)
			}

			if neg := markleyStable(-m, e); neg != -got {
				t.Errorf("e = %v, M = %v: E(−M) = %v, −E(M) = %v", e, m, neg, -got)
			}

			if math.Abs(m) > math.Pi {
				continue
			}

			el, err := kepler.NewElements(epoch, unit.AU(2.77), e, angle.Zero(), angle.Zero(), angle.Zero(), angle.Rad(m))
			if err != nil {
				t.Fatalf("e = %v, M = %v: %v", e, m, err)
			}

			a := el.SemiMajorAxis().AU()
			aM := el.SemiMajorAxis().Meters()
			n := math.Sqrt(el.CentralBody().GM/(aM*aM*aM)) * 86400

			wx, wy, wvx, wvy := exactStateFromE(a, e, n, exactE)
			x, y, vx, vy := stableState(a, e, n, got)

			ef, _ := exactE.Float64()
			dPos, dVel := stateSensitivity(a, e, n, exactE)
			anomaly := contractAnomalyULPs * 0x1p-52 * math.Abs(ef)

			allowPos := dPos*anomaly + contractStateULPs*0x1p-52*norm(wx, wy)
			if d := absErr(x, y, wx, wy); !(d <= allowPos) {
				t.Errorf("e = %v, M = %v: position off by %.3g AU, contract %.3g AU", e, m, d, allowPos)
			}

			allowVel := dVel*anomaly + contractStateULPs*0x1p-52*math.Max(norm(wvx, wvy), n*a)
			if d := absErr(vx, vy, wvx, wvy); !(d <= allowVel) {
				t.Errorf("e = %v, M = %v: velocity off by %.3g AU/d, contract %.3g AU/d", e, m, d, allowVel)
			}
		}
	}
}
