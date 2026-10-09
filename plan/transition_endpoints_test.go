package plan

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Transition was comparable as TransitionContext, so a caller may compare
// two with ==; this would not compile if it stopped being so.
func TestTransitionStaysComparable(t *testing.T) {
	t.Parallel()

	plain := Transition{}
	pointed := Transition{ToAltAz: coord.NewAltAz(angle.Deg(30), angle.Deg(120))}

	if plain == pointed {
		t.Error("two Transitions with different ToAltAz compare equal")
	}
}

// transitionSite is the Paranal site the transition tests share.
func transitionSite(t *testing.T) *Site {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("s", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return site
}

// A target the strategy can place but cannot turn into alt/az fails the
// transition from either end, rather than slewing from a zero position.
func TestNewTransitionReportsAnEndItCannotObserve(t *testing.T) {
	t.Parallel()

	site := transitionSite(t)
	star := &Block{ID: "star", Target: NewStar("a", angle.Hour(5.5), angle.Deg(-5))}
	moving := &Block{ID: "moving", Target: errMovingBody{}}
	start := time.Date(2026, time.March, 20, 1, 0, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		name     string
		from, to *Block
	}{
		{"from AltAz", moving, star},
		{"to AltAz", star, moving},
	} {
		_, err := NewTransition(c.from, c.to, start, start, site)
		if !errors.Is(err, errMovingBodyFails) || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s: NewTransition returned %v, want the %s failure", c.name, err, c.name)
		}
	}
}

// TestTransitionEndpointsAreTheFullContextAnswer holds the two positions a
// TransitionModel is handed to a full coord.NewContext at each one's own
// instant, from NewTransition and from a strategy's moving Context alike.
//
// Both read the two ends through one moving Context, so the order matters:
// FromAltAz has to be taken before the Context moves to ToTime. Three minutes
// apart, a position read at the wrong instant is 45′ off, and the bound here
// is SetTime's ≲0.1″. The strategy's Context is left forty minutes away first,
// as constraint checks leave it. The slew BasicTransitionModel prices from
// the positions is held to the one full Contexts give.
func TestTransitionEndpointsAreTheFullContextAnswer(t *testing.T) {
	t.Parallel()

	site := transitionSite(t)
	from := &Block{ID: "from", Target: NewStar("a", angle.Hour(5.5), angle.Deg(-5))}
	to := &Block{ID: "to", Target: NewStar("b", angle.Hour(8.1), angle.Deg(-40))}
	model := &BasicTransitionModel{SlewRate: 2}
	start := time.Date(2026, time.March, 20, 1, 0, 0, 0, time.LocationUTC)

	ctxAt := movingContext(site.Location(), site.Refraction())
	ctxAt(start.Add(unit.Minutes(40)))

	builders := []struct {
		name  string
		build func(toTime time.Time) (Transition, error)
	}{
		{"NewTransition", func(toTime time.Time) (Transition, error) {
			return NewTransition(from, to, start, toTime, site)
		}},
		{"a strategy's moving Context", func(toTime time.Time) (Transition, error) {
			return newTransition(from, to, start, toTime, site, ctxAt)
		}},
	}

	for _, toTime := range []time.Time{start, start.Add(unit.Minutes(3))} {
		wantFrom, wantTo := fullContextAltAz(t, from, start, site), fullContextAltAz(t, to, toTime, site)

		want, err := model.Overhead(Transition{FromBlock: from, ToBlock: to, FromAltAz: wantFrom, ToAltAz: wantTo})
		if err != nil {
			t.Fatalf("Overhead with full Contexts: %v", err)
		}

		if want <= 0 {
			t.Fatalf("no slew between two targets 40° apart: %v", want)
		}

		for _, b := range builders {
			tr, err := b.build(toTime)
			if err != nil {
				t.Fatalf("%s: %v", b.name, err)
			}

			if d := altAzSeparationArcsec(tr.FromAltAz, wantFrom); d > 0.1 {
				t.Errorf("%s, ToTime %v: FromAltAz is %.3f″ from a full Context at FromTime", b.name, toTime, d)
			}

			if d := altAzSeparationArcsec(tr.ToAltAz, wantTo); d > 0.1 {
				t.Errorf("%s, ToTime %v: ToAltAz is %.3f″ from a full Context at ToTime", b.name, toTime, d)
			}

			got, err := model.Overhead(tr)
			if err != nil {
				t.Fatalf("%s: Overhead: %v", b.name, err)
			}

			if d := math.Abs((got - want).Seconds()); d > 1e-3 {
				t.Errorf("%s, ToTime %v: slew %v, %v with full Contexts", b.name, toTime, got, want)
			}
		}
	}
}

// fullContextAltAz is b's target at the instant at, observed from a fresh
// coord.Context.
func fullContextAltAz(t *testing.T, b *Block, at time.Time, site *Site) coord.AltAz {
	t.Helper()

	pos, err := b.Target.Position(at)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	aa, err := observedAltAz(b.Target, at, coord.NewContext(at, site.Location(), site.Refraction()), pos)
	if err != nil {
		t.Fatalf("observedAltAz: %v", err)
	}

	return aa
}

// BenchmarkGreedyStrategy_10Slew is BenchmarkGreedyStrategy_10 with the
// transition model slewing, so the two positions every transition carries are
// in the measurement: on main before #485 the slew's own Contexts were ~86% of
// it.
func BenchmarkGreedyStrategy_10Slew(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Bench", loc)
	planner, _ := NewPlanner(site, nil)
	tm := &BasicTransitionModel{BaseSetup: 0, SlewRate: 2}
	blocks := makeBlocks(10)
	start := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)
	window := Window{Start: start, End: start.Add(unit.Minutes(150))}

	for b.Loop() {
		_, _ = (&GreedyStrategy{}).Schedule(planner, window, blocks, tm)
	}
}
