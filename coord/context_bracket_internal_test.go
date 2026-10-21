package coord

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func bracketFixture(t *testing.T) (*Geodetic, atmosphere.Refraction, time.Time) {
	t.Helper()

	site, err := NewGeodetic(angle.Deg(15), angle.Deg(50), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return site, atmosphere.StandardRefraction(), time.Date(2026, time.June, 15, 12, 0, 0, 0, time.LocationUTC)
}

// TestSetTimeEvaluatesTheSeriesOnceADay is #790's cost in the terms that
// decide it: a Context walked through three days of one-minute steps
// evaluates the series at the instant it was built and once a day after, four
// instants in all, where it used to evaluate them every hour, 72 times. The
// instants are read from the nodes SetTime keeps, not from a counter.
func TestSetTimeEvaluatesTheSeriesOnceADay(t *testing.T) {
	site, atm, base := bracketFixture(t)

	ctx := NewContext(base, site, atm)

	var at []time.Time

	note := func(n *node) {
		for _, a := range at {
			if a.Equal(n.at) {
				return
			}
		}

		at = append(at, n.at)
	}

	for i := 0; i <= 3*24*60; i++ {
		ctx.SetTime(base.Add(unit.Minutes(float64(i))))

		for k := range ctx.nnodes {
			note(&ctx.nodes[k])
		}
	}

	if len(at) != 4 {
		t.Errorf("three days of minute steps evaluated the series at %d instants, want 4: %v", len(at), at)
	}

	for i, a := range at {
		if want := base.Add(unit.Days(float64(i))); !a.Equal(want) {
			t.Errorf("evaluation %d at %v, want %v: a day apart from the build", i, a, want)
		}
	}
}

// TestSetTimeHoldsAQuarterHour: within holdFor of where it was built, either
// way, SetTime evaluates nothing and keeps the build's one node; a minute
// beyond, it evaluates the bracket's far end, a day out on the side it moved.
func TestSetTimeHoldsAQuarterHour(t *testing.T) {
	site, atm, base := bracketFixture(t)

	ctx := NewContext(base, site, atm)
	ctx.SetTime(base.Add(holdFor))
	ctx.SetTime(base.Add(-holdFor))

	if ctx.nnodes != 1 || !ctx.nodes[0].at.Equal(base) {
		t.Fatalf("within a quarter hour, SetTime holds %d node(s), the first at %v; want the build's alone",
			ctx.nnodes, ctx.nodes[0].at)
	}

	ctx.SetTime(base.Add(-holdFor - unit.Minutes(1)))

	if ctx.nnodes != 2 || !ctx.nodes[0].at.Equal(base.Add(-bracket)) || !ctx.nodes[1].at.Equal(base) {
		t.Errorf("past the hold, going back: nodes %v and %v, want a bracket ending at the build",
			ctx.nodes[0].at, ctx.nodes[1].at)
	}
}
