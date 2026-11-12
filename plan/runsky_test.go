package plan

import (
	"math"
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// A scheduling run keeps the Sun and Moon its constraints read at each instant
// it asks about, for the whole run (#913), as #959 keeps the score's Moon.
// These tests hold that what it keeps is what the constraints compute, that it
// is kept rather than computed again, that a caller's own ephemeris is never
// answered from it, and that a schedule built through it is the schedule built
// without it, to the bit.

func runSkySite(t *testing.T) *Site {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(2.17), angle.Deg(41.39), 0)
	if err != nil {
		t.Fatal(err)
	}

	site, err := NewSite("Run sky", loc)
	if err != nil {
		t.Fatal(err)
	}

	return site
}

// runSkyStart is the evening of 27 March 2026 at runSkySite, with the Moon
// past first quarter and up through the evening, so MoonSep has a Moon in the
// sky to keep targets away from.
func runSkyStart() time.Time {
	return time.Date(2026, time.March, 27, 17, 0, 0, 0, time.LocationUTC)
}

func sameAngleBits(a, b angle.Angle) bool {
	return math.Float64bits(a.Radians()) == math.Float64bits(b.Radians())
}

func sameICRSBits(a, b coord.ICRS) bool {
	return sameAngleBits(a.RA(), b.RA()) && sameAngleBits(a.Dec(), b.Dec())
}

// TestRunSkyAnswersAsTheConstraintsDo: the first time the run asks about an
// instant, its record gives the Sun's altitude, the Moon's place and its
// illuminated fraction exactly as computing them afresh from the same Context
// does. Every later time it gives that same answer, to the bit, and it holds
// each instant once.
//
// Later answers are compared with the first, not with a fresh computation. The
// run's Context interpolates between brackets placed where it began (#790), so
// an instant reached along another path comes out different in the last bits:
// 1.9e-9 rad on the Sun's altitude at one instant here, with the record left
// out. Answering it once is the point:
// every target in the run then sees one sky.
func TestRunSkyAnswersAsTheConstraintsDo(t *testing.T) {
	site := runSkySite(t)
	ctxAt := movingContext(site.Location(), site.Refraction())
	sky := newRunSky(false)
	start := runSkyStart()

	var none *runSky

	type first struct {
		alt  angle.Angle
		moon coord.ICRS
		frac float64
	}

	firsts := map[time.Time]first{}

	for pass := range 2 {
		for i := range 40 {
			at := start.Add(unit.Minutes(7.5 * float64(i)))
			ctx := ctxAt(at)

			if pass == 1 {
				f := firsts[at]
				alt, _ := sky.sunAltitude(at, ctx)
				moon, _ := sky.moonTopocentric(at, ctx)
				frac, _ := sky.moonIllumination(at)

				if !sameAngleBits(alt, f.alt) || !sameICRSBits(moon, f.moon) || math.Float64bits(frac) != math.Float64bits(f.frac) {
					t.Errorf("%v: asked again, the record moves from its first answer by %g rad (Sun), %g and %g rad (Moon), %g (lit)",
						at, alt.Radians()-f.alt.Radians(), moon.RA().Radians()-f.moon.RA().Radians(),
						moon.Dec().Radians()-f.moon.Dec().Radians(), frac-f.frac)
				}

				continue
			}

			gotAlt, err := sky.sunAltitude(at, ctx)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			wantAlt, err := none.sunAltitude(at, ctx)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			if !sameAngleBits(gotAlt, wantAlt) {
				t.Errorf("%v: the record gives the Sun at %v, computing it %v", at, gotAlt, wantAlt)
			}

			gotMoon, err := sky.moonTopocentric(at, ctx)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			wantMoon, err := none.moonTopocentric(at, ctx)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			if !sameICRSBits(gotMoon, wantMoon) {
				t.Errorf("%v: the record gives the Moon at %v, computing it %v", at, gotMoon, wantMoon)
			}

			gotFrac, err := sky.moonIllumination(at)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			wantFrac, err := none.moonIllumination(at)
			if err != nil {
				t.Fatalf("%v: %v", at, err)
			}

			if math.Float64bits(gotFrac) != math.Float64bits(wantFrac) {
				t.Errorf("%v: the record gives the Moon %v lit, computing it %v", at, gotFrac, wantFrac)
			}

			firsts[at] = first{alt: gotAlt, moon: gotMoon, frac: gotFrac}
		}
	}

	for name, n := range map[string]int{"Sun": len(sky.sunAlt), "Moon": len(sky.moonTopo), "illumination": len(sky.moonIllum)} {
		if n != 40 {
			t.Errorf("the record holds the %s at %d instants after forty asked for twice, want 40", name, n)
		}
	}
}

// TestRunSkyKeepsWhatItComputed: an instant already in the record is answered
// from it, not computed again. Each stored value is replaced by one no
// ephemeris gives, and the next answer must be that value.
func TestRunSkyKeepsWhatItComputed(t *testing.T) {
	site := runSkySite(t)
	at := runSkyStart()
	ctx := movingContext(site.Location(), site.Refraction())(at)
	sky := newRunSky(false)

	if _, err := sky.sunAltitude(at, ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := sky.moonTopocentric(at, ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := sky.moonIllumination(at); err != nil {
		t.Fatal(err)
	}

	sentinelAlt := angle.Deg(123)
	sentinelMoon := coord.NewICRS(angle.Deg(1), angle.Deg(2))
	sentinelFrac := 7.0

	sky.sunAlt[at] = sentinelAlt
	sky.moonTopo[at] = sentinelMoon
	sky.moonIllum[at] = sentinelFrac

	if got, _ := sky.sunAltitude(at, ctx); got != sentinelAlt {
		t.Errorf("the Sun's second answer is %v: computed again rather than read from the record", got)
	}

	if got, _ := sky.moonTopocentric(at, ctx); got != sentinelMoon {
		t.Errorf("the Moon's second answer is %v: computed again rather than read from the record", got)
	}

	if got, _ := sky.moonIllumination(at); got != sentinelFrac {
		t.Errorf("the illumination's second answer is %v: computed again rather than read from the record", got)
	}
}

// TestMoonIllumWithItsOwnProviderIsNeverAnsweredFromTheRun: the record holds
// the default ephemeris's illuminated fraction, so a MoonIllum given its own
// Provider must compute with that Provider, whatever the record holds. A
// MoonIllum with none reads the record.
func TestMoonIllumWithItsOwnProviderIsNeverAnsweredFromTheRun(t *testing.T) {
	site := runSkySite(t)
	at := runSkyStart()
	ctx := movingContext(site.Location(), site.Refraction())(at)
	sky := newRunSky(false)

	const sentinel = 7.0

	sky.moonIllum[at] = sentinel

	star := NewStar("S", angle.Zero(), angle.Zero())

	own, err := MoonIllum{Threshold: 1, Provider: eph.Default()}.checkSky(star, at, ctx, sky)
	if err != nil {
		t.Fatal(err)
	}

	if own.Value == sentinel {
		t.Error("a MoonIllum with its own Provider was answered from the run's record of the default ephemeris")
	}

	byDefault, err := MoonIllum{Threshold: 1}.checkSky(star, at, ctx, sky)
	if err != nil {
		t.Fatal(err)
	}

	if byDefault.Value != sentinel {
		t.Errorf("a MoonIllum with no Provider gave %v, not the run's record", byDefault.Value)
	}
}

// TestSchedulingWithTheRunsSkyIsBitIdentical: greedy and swap schedules under
// the full night chain (altitude, Sun, MoonSep, MoonIllum) are the same, block
// for block and score for score by their bits, whether the constraints read
// the Sun and the Moon from the run's record or compute them every time.
//
// The window opens in daylight, so the Sun constraint decides where the night
// starts. The first target, the highest in priority, trails the Moon by 28.5°:
// MoonSep keeps it back until the Moon has drawn away past 30°, about 20:45
// UTC, and it sets below 20° by about 23:20. A record that answered one
// instant with another's sky would move one of those boundaries, and the bits
// would show it. A Moon held at an earlier instant would never let it go at
// all.
func TestSchedulingWithTheRunsSkyIsBitIdentical(t *testing.T) {
	site := runSkySite(t)
	start := runSkyStart()

	moon, err := NewMoon(eph.Default()).Position(start)
	if err != nil {
		t.Fatal(err)
	}

	// 30.5° of right ascension at the Moon's declination is about 28° of sky.
	trailing := NewStar("behind the Moon", angle.Deg(moon.RA().Degrees()-30.5), moon.Dec())
	moonSep := MoonSep{Threshold: angle.Deg(30)}

	for _, premise := range []struct {
		hours float64
		pass  bool
	}{{0, false}, {3, false}, {5, true}} {
		at := start.Add(unit.Hours(premise.hours))

		r, err := moonSep.Check(trailing, at, site)
		if err != nil {
			t.Fatal(err)
		}

		if r.Pass != premise.pass {
			t.Fatalf("%v: MoonSep gives the trailing target %v, want pass=%v: it no longer waits for the Moon",
				at, r, premise.pass)
		}
	}

	targets := append([]Observable{trailing}, benchStars(15)...)

	blocks := func() []*Block {
		bs := make([]*Block, len(targets))
		for i, tgt := range targets {
			bs[i] = &Block{ID: tgt.Name(), Target: tgt, Duration: 20 * time.Minute, Priority: float64(len(targets) - i)}
		}

		return bs
	}

	planner, err := NewPlanner(site, nightChain())
	if err != nil {
		t.Fatal(err)
	}

	window := Window{Start: start, End: start.Add(unit.Hours(10))}
	transition := &BasicTransitionModel{}

	for _, tc := range []struct {
		name string
		run  func(sky *runSky) (*Schedule, error)
	}{
		{"greedy", func(sky *runSky) (*Schedule, error) {
			return (&GreedyStrategy{}).schedule(planner, window, blocks(), transition,
				movingContext(site.Location(), site.Refraction()), sky)
		}},
		{"swap over priority", func(sky *runSky) (*Schedule, error) {
			return (&SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 3}).schedule(planner, window,
				blocks(), transition, movingContext(site.Location(), site.Refraction()), sky)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			want, err := tc.run(nil)
			if err != nil {
				t.Fatal(err)
			}

			got, err := tc.run(newRunSky(tc.name != "greedy"))
			if err != nil {
				t.Fatal(err)
			}

			if !slices.ContainsFunc(want.Blocks, func(b ScheduledBlock) bool { return b.Block.ID == trailing.Name() }) {
				t.Fatal("the target behind the Moon was not placed: MoonSep's boundary decides nothing")
			}

			if len(got.Blocks) != len(want.Blocks) || len(got.Unscheduled) != len(want.Unscheduled) {
				t.Fatalf("%d placed and %d unscheduled, want %d and %d",
					len(got.Blocks), len(got.Unscheduled), len(want.Blocks), len(want.Unscheduled))
			}

			for i := range want.Blocks {
				g, w := got.Blocks[i], want.Blocks[i]
				if g.Block.ID != w.Block.ID || !g.Window.Start.Equal(w.Window.Start) || !g.Window.End.Equal(w.Window.End) ||
					math.Float64bits(g.Score) != math.Float64bits(w.Score) {
					t.Errorf("block %d: %s %v–%v score %v, want %s %v–%v score %v",
						i, g.Block.ID, g.Window.Start, g.Window.End, g.Score, w.Block.ID, w.Window.Start, w.Window.End, w.Score)
				}
			}
		})
	}
}
