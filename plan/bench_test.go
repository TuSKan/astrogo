package plan

import (
	"fmt"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"

	atime "github.com/TuSKan/astrogo/time"
)

// ── Visibility Detection ─────────────────────────────────────────────────────

type benchMock struct {
	c coord.ICRS
}

func (m benchMock) ICRS(_ atime.Time) (coord.ICRS, error) {
	return m.c, nil
}

func (m benchMock) Name() string                              { return "mock" }
func (m benchMock) Position(_ atime.Time) (coord.ICRS, error) { return m.c, nil }
func (m benchMock) GetDetails(_ *coord.Context, _ ...string) (*TargetDetails, error) {
	return &TargetDetails{}, nil
}

func BenchmarkVisibleIntervals(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := benchMock{c: coord.NewICRS(angle.Deg(0), angle.Deg(45))}
	start := atime.FromJD(2460000.0, atime.UTC)
	end := start.AddDays(1.0)

	b.ResetTimer()

	for range b.N {
		_, _ = VisibleIntervals(obj, site, start, end, 10*time.Minute, angle.Deg(20))
	}
}

func BenchmarkVisibleIntervals_1MinStep(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := benchMock{c: coord.NewICRS(angle.Deg(0), angle.Deg(45))}
	start := atime.FromJD(2460000.0, atime.UTC)
	end := start.AddDays(1.0)

	b.ResetTimer()

	for range b.N {
		_, _ = VisibleIntervals(obj, site, start, end, 1*time.Minute, angle.Deg(20))
	}
}

// ── Event Solver ────────────────────────────────────────────────────────────

func BenchmarkEventSolver_Visibility(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := NewStar("T", angle.Deg(0), angle.Deg(0))
	start := atime.FromJD(2451545.0, atime.UTC)
	end := start.Add(24 * atime.Hour)
	solver := NewEventSolver(30*atime.Minute, 1*atime.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    obj,
		Observer:  site,
		Threshold: angle.Deg(20),
	}

	b.ResetTimer()

	for range b.N {
		_, _ = solver.Find(spec, start, end)
	}
}

// ── ObservableWindows ───────────────────────────────────────────────────────

func BenchmarkObservableWindows(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := NewStar("T", angle.Hour(18.69), angle.Deg(0))
	start := atime.FromJD(2451545.0, atime.UTC)
	end := start.Add(12 * atime.Hour)
	constraints := []Constraint{Altitude{Threshold: angle.Deg(30)}}

	b.ResetTimer()

	for range b.N {
		_, _ = ObservableWindows(obj, start, end, 5*atime.Minute, site, constraints...)
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
			Duration: 10 * atime.Minute,
			Priority: float64(n - i), // descending priority
		}
	}

	return blocks
}

func benchScheduler(b *testing.B, n int, strategy Strategy) {
	b.Helper()

	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Bench", loc, angle.Zero(), nil)
	planner, _ := NewPlanner(site, nil)
	tm := &BasicTransitionModel{BaseSetup: 0}
	blocks := makeBlocks(n)

	start := atime.ZeroTime()
	window := Window{Start: start, End: start.Add(atime.Duration(n*15) * atime.Minute)}

	b.ResetTimer()

	for range b.N {
		_, _ = strategy.Schedule(planner, window, blocks, tm)
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

// ── Transit Estimate ────────────────────────────────────────────────────────

func BenchmarkTransitEstimate(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := benchMock{c: coord.NewICRS(angle.Deg(100), angle.Deg(20))}
	start := atime.FromJD(2460000.0, atime.UTC)
	end := start.AddDays(0.5)

	b.ResetTimer()

	for range b.N {
		_, _, _ = TransitEstimate(obj, site, start, end)
	}
}
