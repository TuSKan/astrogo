package plan

import (
	"fmt"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"

	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// ── Visibility Detection ─────────────────────────────────────────────────────

type benchMock struct {
	c coord.ICRS
}

func (m benchMock) ICRS(_ epoch.Time) (coord.ICRS, error) {
	return m.c, nil
}

func (m benchMock) Name() string                              { return "mock" }
func (m benchMock) Position(_ epoch.Time) (coord.ICRS, error) { return m.c, nil }
func (m benchMock) GetDetails(_ *coord.Context, _ DetailOverrides) (*TargetDetails, error) {
	return &TargetDetails{}, nil
}

func BenchmarkVisibleIntervals(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.LatDeg(45), angle.LonDeg(0), 0)
	site, _ := NewSite("Test", loc.Lat(), loc.Lon(), loc.Height())
	obj := benchMock{c: coord.NewICRS(angle.Deg(0), angle.Deg(45))}
	start := epoch.FromJD(2460000.0, epoch.UTC)
	end := start.Add(unit.Days(1.0))

	for b.Loop() {
		if _, err := VisibleIntervals(obj, site, Window{Start: start, End: end}, unit.Minutes(10), angle.Deg(20)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkVisibleIntervals_1MinStep(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.LatDeg(45), angle.LonDeg(0), 0)
	site, _ := NewSite("Test", loc.Lat(), loc.Lon(), loc.Height())
	obj := benchMock{c: coord.NewICRS(angle.Deg(0), angle.Deg(45))}
	start := epoch.FromJD(2460000.0, epoch.UTC)
	end := start.Add(unit.Days(1.0))

	for b.Loop() {
		if _, err := VisibleIntervals(obj, site, Window{Start: start, End: end}, unit.Minutes(1), angle.Deg(20)); err != nil {
			b.Fatal(err)
		}
	}
}

// ── Event Solver ────────────────────────────────────────────────────────────

// BenchmarkOppositions is a year of Mars against the Sun on the analytical
// ephemeris: the scan of their alignment at the step its bound sets, and the
// one opposition it finds refined on the apparent places (#787).
func BenchmarkOppositions(b *testing.B) {
	prov := eph.Default()
	mars, sun := NewMars(prov), NewSun(prov)
	start := epoch.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(unit.Days(365))

	for b.Loop() {
		if _, err := Oppositions(mars, sun, Window{Start: start, End: end}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEventSolver_Visibility(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.LatDeg(45), angle.LonDeg(0), 0)
	site, _ := NewSite("Test", loc.Lat(), loc.Lon(), loc.Height())
	obj := NewStar("T", angle.Deg(0), angle.Deg(0))
	start := epoch.FromJD(2451545.0, epoch.UTC)
	end := start.Add(unit.Hours(24))
	solver := NewEventSolver(unit.Minutes(30), unit.Seconds(1))
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    obj,
		Observer:  site,
		Threshold: angle.Deg(20),
	}

	for b.Loop() {
		if _, err := solver.Find(spec, Window{Start: start, End: end}); err != nil {
			b.Fatal(err)
		}
	}
}

// ── ObservableWindows ───────────────────────────────────────────────────────

func BenchmarkObservableWindows(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.LatDeg(45), angle.LonDeg(0), 0)
	site, _ := NewSite("Test", loc.Lat(), loc.Lon(), loc.Height())
	obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))
	start := epoch.FromJD(2451545.0, epoch.UTC)
	end := start.Add(unit.Hours(12))
	constraints := []Constraint{Altitude{Threshold: angle.Deg(30)}}

	for b.Loop() {
		if _, err := ObservableWindows(obj, site, Window{Start: start, End: end}, unit.Minutes(5), constraints...); err != nil {
			b.Fatal(err)
		}
	}
}

// ── Scheduler Scaling ───────────────────────────────────────────────────────
// Measures scheduling cost as the number of blocks grows.

func makeBlocks(n int) []*Block {
	blocks := make([]*Block, n)
	for i := range n {
		blocks[i] = &Block{
			ID:       fmt.Sprintf("B%d", i),
			Target:   NewStar(fmt.Sprintf("T%d", i), angle.Deg(0), angle.Deg(0)),
			Duration: unit.Minutes(10),
			Priority: float64(n - i), // descending priority
		}
	}

	return blocks
}

func benchScheduler(b *testing.B, n int, strategy Strategy) {
	b.Helper()

	loc, _ := coord.NewGeodetic(angle.Latitude{}, angle.Longitude{}, 0)
	site, _ := NewSite("Bench", loc.Lat(), loc.Lon(), loc.Height())
	planner, _ := NewPlanner(site, nil)
	tm := &BasicTransitionModel{BaseSetup: 0}
	blocks := makeBlocks(n)

	// A real epoch, inside the EOP bulletin's coverage, rather than
	// epoch.ZeroTime.
	//
	// ZeroTime is FromJD(0) — 4713 BC — where no Earth Orientation
	// Parameters exist, and every constraint evaluation therefore took the
	// out-of-coverage path. A heap profile put 99.88% of this benchmark's
	// allocated bytes in epoch.Time.EOP rather than anywhere in scheduling,
	// so what it reported was the cost of missing EOP data (#246).
	//
	// Fixed epoch, not epoch.Now: a benchmark whose result depends on the
	// day it is run cannot be compared with the one before it.
	start := epoch.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC)
	window := Window{Start: start, End: start.Add(unit.Minutes(float64(n) * 15))}

	for b.Loop() {
		if _, err := strategy.Schedule(planner, window, blocks, tm); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGreedyStrategy_10(b *testing.B) {
	benchScheduler(b, 10, &GreedyStrategy{})
}

func BenchmarkGreedyStrategy_50(b *testing.B) {
	benchScheduler(b, 50, &GreedyStrategy{})
}

func BenchmarkGreedyStrategy_100(b *testing.B) {
	benchScheduler(b, 100, &GreedyStrategy{})
}

func BenchmarkPriorityStrategy_10(b *testing.B) {
	benchScheduler(b, 10, &PriorityStrategy{})
}

func BenchmarkPriorityStrategy_50(b *testing.B) {
	benchScheduler(b, 50, &PriorityStrategy{})
}

func BenchmarkPriorityStrategy_100(b *testing.B) {
	benchScheduler(b, 100, &PriorityStrategy{})
}

func BenchmarkSwapOptimized_10(b *testing.B) {
	benchScheduler(b, 10, &SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 3})
}

func BenchmarkSwapOptimized_50(b *testing.B) {
	benchScheduler(b, 50, &SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 3})
}

func BenchmarkSwapOptimized_100(b *testing.B) {
	benchScheduler(b, 100, &SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 3})
}

// BenchmarkSwapOptimizedDefault_100 is the strategy as its zero value configures
// it, twenty passes over a Priority seed: what a caller writing
// &SwapOptimizedStrategy{} runs. Each pass returns to every placement, which
// is where the Moon's place was computed again for instants the run already
// had (#959).
func BenchmarkSwapOptimizedDefault_100(b *testing.B) {
	benchScheduler(b, 100, &SwapOptimizedStrategy{})
}

// ── Transit Estimate ────────────────────────────────────────────────────────

func BenchmarkTransitEstimate(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.LatDeg(45), angle.LonDeg(0), 0)
	site, _ := NewSite("Test", loc.Lat(), loc.Lon(), loc.Height())
	obj := benchMock{c: coord.NewICRS(angle.Deg(100), angle.Deg(20))}
	start := epoch.FromJD(2460000.0, epoch.UTC)
	end := start.Add(unit.Days(0.5))

	for b.Loop() {
		if _, _, err := TransitEstimate(obj, site, Window{Start: start, End: end}); err != nil {
			b.Fatal(err)
		}
	}
}

// ── Fortnight Event Sweep (issue #10 repro) ─────────────────────────────────

// BenchmarkFortnightEvents mirrors an observing-planner workload (the shape
// reported in https://github.com/TuSKan/astrogo/issues/10): for each of 14
// days, compute sun rise/set/transit, the three twilight bands, and moon
// rise/set — the standard "14-night forecast" that made coord.NewContext
// ~65% of total CPU before solveVisibility amortized one full NewContext per
// ~hour of solve window instead of rebuilding one per sample and per
// bisection iteration, now with coord.Context.SetTime (see coord/context.go).
func BenchmarkFortnightEvents(b *testing.B) {
	g, err := coord.NewGeodetic(angle.LatDeg(41.3874), angle.LonDeg(2.1686), 0) // Barcelona
	if err != nil {
		b.Fatal(err)
	}

	tz, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		b.Fatal(err)
	}

	site, err := NewSite("bcn", g.Lat(), g.Lon(), g.Height(), WithTimeZone(tz))
	if err != nil {
		b.Fatal(err)
	}

	provider := eph.Default()
	start := epoch.Date(2026, 7, 20, 0, 0, 0, 0, tz)

	for b.Loop() {
		for d := range 14 {
			day := start.AddDate(0, 0, d)
			next := day.AddDate(0, 0, 1)

			if _, err := SunEvents(site, Window{Start: day, End: next}, provider); err != nil {
				b.Fatal(err)
			}

			// That evening's night, from local noon.
			for _, kind := range []TwilightKind{CivilTwilight, NauticalTwilight, AstronomicalTwilight} {
				if _, err := NightOf(site, day.Add(unit.Hours(12)), provider, kind); err != nil {
					b.Fatal(err)
				}
			}

			if _, err := MoonEvents(site, Window{Start: day, End: next}, provider); err != nil {
				b.Fatal(err)
			}
		}
	}
}
