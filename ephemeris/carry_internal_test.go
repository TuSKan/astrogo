package ephemeris

import (
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// roundingFloor is the disagreement between two evaluations of the same
// barycentric state on Default, in AU: SOFA resolves an epoch to 0.16 µs, which
// is 4.8 mm of the Earth's 30 km/s, and a carried state is compared with one
// evaluated at an epoch of its own, so twice that. Measured, two Epv00 calls
// 1.3 s apart disagree by 7.8 mm.
var roundingFloor = 9.6e-3 / constants.IAU.AstronomicalUnit.Value

// carryEpochs are the instants the carry tests sample: every six hours through
// 2026, every five days under -short.
func carryEpochs(t *testing.T) []time.Time {
	t.Helper()

	step := 0.25
	if testing.Short() {
		step = 5
	}

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	var out []time.Time
	for d := 0.0; d < 365; d += step {
		out = append(out, start.Add(unit.Days(d)))
	}

	return out
}

// TestCarriedBodyStaysWithinItsResidual: the Earth and the Sun carried back
// over the longest light time each is carried for land within what their
// models leave out, residual × τ²/2, of where Default puts them at t − τ.
//
// This is the bound the carry rests on, held at its own end, where a wrong
// model is meters out: an Earth carried without the Sun's pull misses by
// 15 m at 71.5 s, against a bound of 10 cm. Compared through a place, the same
// mistake would be 2.7 µas for the Moon, under the exact path's own rounding.
func TestCarriedBodyStaysWithinItsResidual(t *testing.T) {
	t.Parallel()

	p := Default()
	au := constants.IAU.AstronomicalUnit.Value
	day := constants.Derived.JulianDaySeconds.Value

	for _, b := range []struct {
		id       ID
		residual float64
		maxTau   float64
	}{
		{Earth, carryResidual(earthResidual), earthMaxTau},
		{Sun, carryResidual(sunResidual), sunMaxTau},
	} {
		tauS := b.maxTau * day
		bound := b.residual*tauS*tauS/2/au + roundingFloor

		var worst float64

		for _, at := range carryEpochs(t) {
			in := At(p, at)

			// The Sun's chain decides the carrier for the Sun; for the Earth,
			// the Moon is the target whose chain meets the Earth's there.
			target := b.id
			if b.id == Earth {
				target = Moon
			}

			c, ok, err := in.carrierFor(target)
			if err != nil {
				t.Fatal(err)
			}

			if !ok || c.id != b.id {
				t.Fatalf("%v: carrier %v (%v), want %v", target, c.id, ok, b.id)
			}

			pos, _ := c.at(b.maxTau)

			exact, err := At(p, at.Add(unit.Days(-b.maxTau))).Geometric(b.id, SolarSystemBarycenter)
			if err != nil {
				t.Fatal(err)
			}

			worst = math.Max(worst, pos.Sub(exact.Pos).Norm())
		}

		if worst > bound {
			t.Errorf("%v carried over %.1f s misses by %.3g m, past its bound of %.3g m",
				b.id, tauS, worst*au, bound*au)
		}

		t.Logf("%v carried over %.1f s: worst %.3g m, bound %.3g m", b.id, tauS, worst*au, bound*au)
	}
}

// angleBetween is the angle between a and b, by atan2: an acos floors at
// about 0.004″, far above what is measured here.
func angleBetween(a, b vector.Vec3) float64 {
	return math.Atan2(a.Cross(b).Norm(), a.Dot(b))
}

// TestCarriedPlaceIsTheExactPlace: through 2026, every apparent place the
// carry gives on Default is within carryTolerance of the exact path's, plus
// that path's own rounding seen at the target's distance (5.6 µas for the
// Moon at its nearest, under 0.1 µas for any planet). The Sun and the planets
// out to Saturn are always carried, Uranus while its light time is under the
// Sun's 2.60 h (it runs from 2.5 to 2.8 h), and Neptune never: its places
// agree bit for bit.
func TestCarriedPlaceIsTheExactPlace(t *testing.T) {
	t.Parallel()

	p := Default()

	for _, id := range []ID{
		Moon, Sun, MercuryBarycenter, VenusBarycenter, MarsBarycenter,
		JupiterBarycenter, SaturnBarycenter, UranusBarycenter, NeptuneBarycenter,
	} {
		var worst, worstBound float64

		for _, at := range carryEpochs(t) {
			carried, err := At(p, at).apparent(id, Geocenter(), true)
			if err != nil {
				t.Fatalf("%v: %v", id, err)
			}

			exact, err := At(p, at).apparent(id, Geocenter(), false)
			if err != nil {
				t.Fatalf("%v: %v", id, err)
			}

			bound := carryTolerance + roundingFloor/exact.Pos.Norm()
			if got := angleBetween(carried.Pos, exact.Pos); got > worst {
				worst, worstBound = got, bound
			}

			if got := angleBetween(carried.Pos, exact.Pos); got > bound {
				t.Fatalf("%v at %v: carried place %.4f µas from the exact one, bound %.4f µas",
					id, at, got*206264.806e6, bound*206264.806e6)
			}
		}

		t.Logf("%-20v worst %.3f µas (bound there %.3f µas)", id, worst*206264.806e6, worstBound*206264.806e6)
	}
}

// countingProvider records the epochs at which each body is located.
type countingProvider struct {
	Provider

	epochs map[ID]map[float64]bool
}

func (c *countingProvider) Locate(id ID, t time.Time) (State, error) {
	if c.epochs[id] == nil {
		c.epochs[id] = map[float64]bool{}
	}

	c.epochs[id][t.JD()] = true

	st, err := c.Provider.Locate(id, t)
	if err != nil {
		return State{}, fmt.Errorf("counting provider: %w", err)
	}

	return st, nil
}

// TestCarriedPlaceLocatesTheEarthAndSunOnce: an apparent place on Default
// locates the Earth and the Sun, the bodies behind every Epv00, at the
// observing epoch only when B is carried, and at the retarded epoch too when
// it is not. That is #791's saving: two Epv00 series per place become one.
func TestCarriedPlaceLocatesTheEarthAndSunOnce(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, time.June, 10, 21, 0, 0, 0, time.LocationUTC)

	for _, tc := range []struct {
		id     ID
		epochs int
	}{
		{Moon, 1}, {Sun, 1}, {MarsBarycenter, 1}, {SaturnBarycenter, 1},
		{NeptuneBarycenter, 2}, // its light time, 4.1 h, passes the Sun's bound
	} {
		cp := &countingProvider{Provider: Default(), epochs: map[ID]map[float64]bool{}}

		if _, err := At(cp, at).Apparent(tc.id, Geocenter()); err != nil {
			t.Fatalf("%v: %v", tc.id, err)
		}

		seen := map[float64]bool{}

		for _, b := range []ID{Earth, Sun} {
			for jd := range cp.epochs[b] {
				seen[jd] = true
			}
		}

		if len(seen) != tc.epochs {
			t.Errorf("%v: the Earth and the Sun located at %d epochs, want %d", tc.id, len(seen), tc.epochs)
		}
	}
}
