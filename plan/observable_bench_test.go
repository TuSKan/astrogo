package plan

import (
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// These benchmarks are the workloads an observing planner actually runs: rank
// many targets at one instant, and test targets against a night of
// constraints. They sit beside the event-solver and scheduler benchmarks in
// bench_test.go, which cover the sampled searches but never put a Sun, Moon or
// illumination constraint on a target, and never rank more than a handful.
//
// Every one is deterministic and offline: a fixed epoch rather than
// time.NowUTC, the analytical ephemeris, a site and a sky built here. Each
// checks its result once before the timed loop, because a benchmark whose call
// quietly fails or returns nothing reports a fast time for no work.

// benchSite is a mid-northern site, where both the stars and the planets below
// spend part of the benchmark's night above the horizon.
func benchSite(b *testing.B) *Site {
	b.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(2.17), angle.Deg(41.39), 0)
	if err != nil {
		b.Fatal(err)
	}

	site, err := NewSite("Bench", loc)
	if err != nil {
		b.Fatal(err)
	}

	return site
}

// benchEpoch is 22:00 UTC on the March equinox 2026, local night at benchSite.
// Fixed, so a result can be compared with the one before it.
func benchEpoch() time.Time {
	return time.Date(2026, time.March, 20, 22, 0, 0, 0, time.LocationUTC)
}

// benchStars is n stars spread over the sky: RA by the golden angle and
// declination by its conjugate, so the set never clusters and is the same on
// every run.
func benchStars(n int) []Observable {
	stars := make([]Observable, n)

	for i := range n {
		ra := math.Mod(float64(i)*137.50776405, 360)
		dec := -70 + 140*math.Mod(float64(i)*0.6180339887, 1)
		stars[i] = NewStar(fmt.Sprintf("S%d", i), angle.Deg(ra), angle.Deg(dec))
	}

	return stars
}

// benchPlanets are the moving bodies of a typical session: each costs an
// apparent place per evaluation, where a star costs a table lookup.
func benchPlanets() []Observable {
	prov := eph.Default()

	return []Observable{
		NewVenus(prov), NewMars(prov), NewJupiter(prov), NewSaturn(prov), NewMoon(prov),
	}
}

// altitudeOnly is the cheapest constraint set.
func altitudeOnly() []Constraint {
	return []Constraint{Altitude{Threshold: angle.Deg(20)}}
}

// nightChain is what a deep-sky session asks of every target at every step:
// high enough, astronomical night, away from the Moon, and the Moon dark
// enough. The Sun and Moon terms are properties of the instant, not of the
// target.
func nightChain() []Constraint {
	return []Constraint{
		Altitude{Threshold: angle.Deg(20)},
		Sun{Threshold: angle.Deg(-12)},
		MoonSep{Threshold: angle.Deg(30)},
		MoonIllum{Threshold: 0.8},
	}
}

// BenchmarkRankObservables ranks a list of targets at one instant, the call
// behind "what should I observe now". The work is per target, so the sizes show
// how it scales; the mixed list shows what a moving body adds.
func BenchmarkRankObservables(b *testing.B) {
	site := benchSite(b)
	t := benchEpoch()

	mixed := append(benchStars(95), benchPlanets()...)

	for _, tc := range []struct {
		name string
		objs []Observable
	}{
		{"stars-100", benchStars(100)},
		{"stars-1000", benchStars(1000)},
		{"mixed-100", mixed},
	} {
		b.Run(tc.name, func(b *testing.B) {
			constraints := altitudeOnly()

			ranked, err := RankObservables(tc.objs, t, site, constraints...)
			if err != nil {
				b.Fatal(err)
			}

			if len(ranked) == 0 {
				b.Fatal("nothing ranked: the benchmark would time an empty result")
			}

			b.ReportAllocs()

			for b.Loop() {
				if _, err := RankObservables(tc.objs, t, site, constraints...); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkScorerScore is one target scored at one instant, with a Context
// built per call and with one supplied. The difference is the Apco13 solve a
// Scorer pays per target when nothing is shared.
func BenchmarkScorerScore(b *testing.B) {
	site := benchSite(b)
	t := benchEpoch()
	shared := coord.NewContext(t, site.Location(), site.Refraction())

	prov := eph.Default()
	targets := []struct {
		name string
		obj  Observable
	}{
		{"star", NewStar("S", angle.Hour(10.5), angle.Deg(20))},
		{"jupiter", NewJupiter(prov)},
	}

	for _, target := range targets {
		for _, tc := range []struct {
			name string
			ctx  *coord.Context
		}{
			{"new-context", nil},
			{"shared-context", shared},
		} {
			b.Run(target.name+"/"+tc.name, func(b *testing.B) {
				scorer := Scorer{Site: site, Context: tc.ctx, Constraints: altitudeOnly()}

				// A target that is down scores zero at once, before the
				// urgency and Moon terms: timing that would time the early
				// out, not a score.
				score, err := scorer.Score(target.obj, t)
				if err != nil {
					b.Fatal(err)
				}

				if score <= 0 {
					b.Fatalf("%s scored %v at the benchmark epoch: it must be up for the full score to run", target.name, score)
				}

				b.ReportAllocs()

				for b.Loop() {
					if _, err := scorer.Score(target.obj, t); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// nightWindow is twelve hours across the benchmark's night, long enough to hold
// dusk and dawn and short enough to be quick.
func nightWindow() (start, end time.Time) {
	start = time.Date(2026, time.March, 20, 16, 0, 0, 0, time.LocationUTC)

	return start, start.Add(unit.Hours(12))
}

// BenchmarkObservableWindowsChain is one target tested against a night at the
// cadence the planner defaults to, under the cheapest constraint set and under
// the full chain. A star pays the constraints; a moving body pays them and its
// own place at every step.
func BenchmarkObservableWindowsChain(b *testing.B) {
	site := benchSite(b)
	start, end := nightWindow()
	step := unit.Minutes(10)

	targets := []struct {
		name string
		obj  Observable
		// seen is whether the target is up during the night, so an empty
		// result means the benchmark is broken rather than the sky quiet.
		seen bool
	}{
		{"star", NewStar("S", angle.Hour(10.5), angle.Deg(20)), true},
		{"jupiter", NewJupiter(eph.Default()), true},
	}

	sets := []struct {
		name        string
		constraints []Constraint
	}{
		{"altitude", altitudeOnly()},
		{"chain", nightChain()},
	}

	for _, target := range targets {
		for _, set := range sets {
			b.Run(target.name+"/"+set.name, func(b *testing.B) {
				windows, err := ObservableWindows(target.obj, start, end, step, site, set.constraints...)
				if err != nil {
					b.Fatal(err)
				}

				if target.seen && len(windows) == 0 {
					b.Fatal("no window: the benchmark would time a target that never passes")
				}

				b.ReportAllocs()

				for b.Loop() {
					if _, err := ObservableWindows(target.obj, start, end, step, site, set.constraints...); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkObservableWindowsManyTargets is twenty targets tested against the
// same night. The Sun and the Moon are the same at an instant for every one of
// them, so this is where a constraint that recomputes them per target shows.
func BenchmarkObservableWindowsManyTargets(b *testing.B) {
	site := benchSite(b)
	start, end := nightWindow()
	step := unit.Minutes(10)
	stars := benchStars(20)

	for _, set := range []struct {
		name        string
		constraints []Constraint
	}{
		{"altitude", altitudeOnly()},
		{"chain", nightChain()},
	} {
		b.Run(set.name, func(b *testing.B) {
			run := func() (windows int, err error) {
				for _, star := range stars {
					w, err := ObservableWindows(star, start, end, step, site, set.constraints...)
					if err != nil {
						return 0, fmt.Errorf("%s: %w", star.Name(), err)
					}

					windows += len(w)
				}

				return windows, nil
			}

			windows, err := run()
			if err != nil {
				b.Fatal(err)
			}

			if windows == 0 {
				b.Fatal("no window for any target: the benchmark would time nothing")
			}

			b.ReportAllocs()

			for b.Loop() {
				if _, err := run(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
