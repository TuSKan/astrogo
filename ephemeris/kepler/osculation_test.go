package kepler_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestFromPerihelionOsculatesAtItsEpoch: the osculation epoch is reported
// and is where the mean anomaly is given, and it moves nothing else (#803).
// The orbit is still propagated from its perihelion time, so the same orbit
// osculating at perihelion places the comet identically, bit for bit — it is
// the same arithmetic. And the mean anomaly it reports at the epoch is the
// one that, given to the asteroid form, traces the same ellipse.
func TestFromPerihelionOsculatesAtItsEpoch(t *testing.T) {
	t.Parallel()

	tp := time.FromJDParts(2460581.240845175775, 0, time.TDB)
	epoch := time.FromJDParts(2461305.5, 0, time.TDB) // 724 days after perihelion

	const (
		q = 1.3
		e = 0.6
	)

	incl, node, argp := angle.Deg(28.5), angle.Deg(141.2), angle.Deg(-63.9)

	atPerihelion, err := kepler.FromPerihelion(tp, tp, unit.AU(q), e, incl, node, argp)
	if err != nil {
		t.Fatal(err)
	}

	osculating, err := kepler.FromPerihelion(epoch, tp, unit.AU(q), e, incl, node, argp)
	if err != nil {
		t.Fatal(err)
	}

	if !osculating.Epoch().Equal(epoch) || !atPerihelion.Epoch().Equal(tp) {
		t.Errorf("Epoch = %v and %v, want the osculation epochs %v and %v",
			osculating.Epoch(), atPerihelion.Epoch(), epoch, tp)
	}

	if m := atPerihelion.MeanAnomaly(); m != 0 {
		t.Errorf("MeanAnomaly at perihelion = %v, want 0", m)
	}

	// n·(epoch − T), reduced to (−π, π].
	a := q / (1 - e)
	n := math.Sqrt(muAUDay() / (a * a * a))
	wantM := math.Remainder(n*epoch.Sub(tp).Days(), 2*math.Pi)

	if d := math.Remainder(osculating.MeanAnomaly().Radians()-wantM, 2*math.Pi); math.Abs(d) > 1e-12 {
		t.Errorf("MeanAnomaly = %v rad, want n·(epoch − T) = %v", osculating.MeanAnomaly().Radians(), wantM)
	}

	asteroid, err := kepler.NewElements(epoch, unit.AU(a), e, incl, node, argp, osculating.MeanAnomaly())
	if err != nil {
		t.Fatal(err)
	}

	for _, days := range []float64{-3000, -1, 0, 1, 365, 3000} {
		when := epoch.Add(unit.Days(days))

		got, gotVel, err := osculating.StateAt(when)
		if err != nil {
			t.Fatal(err)
		}

		same, sameVel, err := atPerihelion.StateAt(when)
		if err != nil {
			t.Fatal(err)
		}

		if got != same || gotVel != sameVel {
			t.Errorf("%+g days on: the osculation epoch moved the comet, %v against %v", days, got, same)
		}

		// Two arithmetic paths, held to each other's rounding.
		classical, _, err := asteroid.StateAt(when)
		if err != nil {
			t.Fatal(err)
		}

		if d := got.Sub(classical).Norm(); d > 1e-12*classical.Norm() {
			t.Errorf("%+g days on: %v AU from the asteroid form at the reported mean anomaly", days, d)
		}
	}
}

// TestFromPerihelionReportsTheHyperbolicMeanAnomaly: on a hyperbola the mean
// anomaly at the epoch is n·(epoch − T) with n = √(μ/|a|³), Danby's.
func TestFromPerihelionReportsTheHyperbolicMeanAnomaly(t *testing.T) {
	t.Parallel()

	tp := time.FromJDParts(2460581.240845175775, 0, time.TDB)
	epoch := tp.Add(unit.Days(-133))

	const (
		q = 0.39143
		e = 1.000095
	)

	el, err := kepler.FromPerihelion(epoch, tp, unit.AU(q), e, angle.Deg(139.1), angle.Deg(21.6), angle.Deg(308.5))
	if err != nil {
		t.Fatal(err)
	}

	a := q / (e - 1)
	want := math.Sqrt(muAUDay()/(a*a*a)) * -133

	if got := el.MeanAnomaly().Radians(); math.Abs(got-want) > 1e-12*math.Abs(want) {
		t.Errorf("MeanAnomaly = %v rad, want %v", got, want)
	}
}
