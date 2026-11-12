package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// A scheduling run keeps the Moon's place at each instant it scores, for the
// whole run (#959). These tests hold that the place it keeps is the one the
// process-wide cache gives, that it is kept rather than computed again, and
// that a schedule built through it is the schedule built without it, to the
// bit.

func sameVecBits(a, b vector.Vec3) bool {
	return math.Float64bits(a.X) == math.Float64bits(b.X) &&
		math.Float64bits(a.Y) == math.Float64bits(b.Y) &&
		math.Float64bits(a.Z) == math.Float64bits(b.Z)
}

func moonPlacesEpoch() time.Time {
	return time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)
}

// TestMoonPlacesAnswerAsTheProcessCache: over forty instants asked for twice,
// the run's record gives the vector the process-wide cache gives, and holds
// each instant once.
func TestMoonPlacesAnswerAsTheProcessCache(t *testing.T) {
	m := moonPlaces{}
	start := moonPlacesEpoch()

	for range 2 {
		for i := range 40 {
			at := start.Add(unit.Minutes(7.5 * float64(i)))

			got, err := m.at(at)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			want, err := getMoonGeocentricVec(at)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			if !sameVecBits(got, want) {
				t.Errorf("%v: the run's record gives %v, the process cache %v", at, got, want)
			}
		}
	}

	if len(m) != 40 {
		t.Errorf("the record holds %d instants after forty asked for twice, want 40", len(m))
	}
}

// TestMoonPlacesKeepWhatTheyComputed: an instant already in the record is
// answered from it, not computed again. The stored value is replaced by one no
// ephemeris gives, and the next answer must be that value.
func TestMoonPlacesKeepWhatTheyComputed(t *testing.T) {
	m := moonPlaces{}
	at := moonPlacesEpoch()

	if _, err := m.at(at); err != nil {
		t.Fatal(err)
	}

	sentinel := vector.V3(1, 2, 3)
	m[at] = sentinel

	got, err := m.at(at)
	if err != nil {
		t.Fatal(err)
	}

	if got != sentinel {
		t.Errorf("the second answer is %v: computed again rather than read from the record", got)
	}
}

// TestSchedulingWithTheRunsMoonIsBitIdentical: the default and the benchmark's
// swap schedules of forty blocks are the same, block for block and score for
// score by their bits, whether the Moon comes from the run's record or the
// process-wide cache. Forty is more than the cache holds, so the run without
// the record is the one that recomputes.
//
// makeBlocks puts every target at RA 0°, Dec 0°, near the Sun at the March
// equinox and so near the Moon a day after new. Within 30° of the Moon every
// score turns on its place at that instant, and a record that answered one
// instant with another's place would show in the bits; the test checks it is
// that close first.
func TestSchedulingWithTheRunsMoonIsBitIdentical(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatal(err)
	}

	site, err := NewSite("Moon places", loc)
	if err != nil {
		t.Fatal(err)
	}

	planner, err := NewPlanner(site, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := moonPlacesEpoch()

	moon, err := getMoonGeocentricVec(start)
	if err != nil {
		t.Fatal(err)
	}

	if sep := angle.Rad(angleBetweenVectors(moon, vector.V3(1, 0, 0))); sep.Degrees() >= 25 {
		t.Fatalf("the targets are %.1f° from the Moon: past 30° no score depends on its place, and the test would compare nothing", sep.Degrees())
	}

	const n = 40

	window := Window{Start: start, End: start.Add(unit.Minutes(n * 15))}
	transition := &BasicTransitionModel{}

	for _, tc := range []struct {
		name string
		run  func(sky *runSky) (*Schedule, error)
	}{
		{"swap, default passes", func(sky *runSky) (*Schedule, error) {
			return (&SwapOptimizedStrategy{}).schedule(planner, window, makeBlocks(n), transition,
				movingContext(site.Location(), site.Refraction()), sky)
		}},
		{"swap, three passes over Priority", func(sky *runSky) (*Schedule, error) {
			return (&SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 3}).schedule(planner, window,
				makeBlocks(n), transition, movingContext(site.Location(), site.Refraction()), sky)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := tc.run(nil)
			if err != nil {
				t.Fatal(err)
			}

			got, err := tc.run(newRunSky(true))
			if err != nil {
				t.Fatal(err)
			}

			if len(got.Blocks) != len(want.Blocks) || len(got.Unscheduled) != len(want.Unscheduled) {
				t.Fatalf("%d placed and %d unscheduled, want %d and %d",
					len(got.Blocks), len(got.Unscheduled), len(want.Blocks), len(want.Unscheduled))
			}

			if len(want.Blocks) == 0 {
				t.Fatal("nothing was placed: the schedule compares nothing")
			}

			for i, w := range want.Blocks {
				g := got.Blocks[i]

				if g.Block.ID != w.Block.ID || !g.Window.Start.Equal(w.Window.Start) || !g.Window.End.Equal(w.Window.End) ||
					g.SetupTime != w.SetupTime || math.Float64bits(g.Score) != math.Float64bits(w.Score) {
					t.Errorf("placement %d: %s %v–%v score %v, want %s %v–%v score %v", i,
						g.Block.ID, g.Window.Start, g.Window.End, g.Score,
						w.Block.ID, w.Window.Start, w.Window.End, w.Score)
				}
			}
		})
	}
}
