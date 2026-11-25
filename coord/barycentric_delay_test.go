package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// delaySite is a mid-latitude site for the delay tests.
func delaySite(t *testing.T) *coord.Geodetic {
	t.Helper()

	site, err := coord.NewGeodetic(angle.LatDeg(-30.17), angle.LonDeg(-70.81), unit.Meters(2207))
	if err != nil {
		t.Fatal(err)
	}

	return site
}

// TestBarycentricDelayDoesNotDependOnHowTheContextGotThere: a Context moved to
// an instant gives the delay one built there gives, whether SetTime held its
// state (minutes away), interpolated it (hours away) or rebuilt it (days
// away). The state the transforms use between evaluations is placed for
// directions, and read for the delay it would put the observer thousands of
// kilometers from where it is.
func TestBarycentricDelayDoesNotDependOnHowTheContextGotThere(t *testing.T) {
	t.Parallel()

	site := delaySite(t)
	t0 := epoch.FromJD(2461045.25, epoch.UTC)
	target := coord.NewICRS(angle.Deg(30), angle.Deg(10))

	moved, err := coord.NewContext(t0, site, atmosphere.Refraction{})
	if err != nil {
		t.Fatal(err)
	}

	for _, offset := range []unit.Duration{unit.Minutes(10), unit.Hours(6), unit.Hours(18), unit.Days(3)} {
		at := t0.Add(offset)
		moved.SetTime(at)

		fresh, err := coord.NewContext(at, site, atmosphere.Refraction{})
		if err != nil {
			t.Fatal(err)
		}

		got, want := moved.BarycentricDelay(target).Seconds(), fresh.BarycentricDelay(target).Seconds()
		if math.Abs(got-want) > 1e-9 {
			t.Errorf("%v after the build: a moved Context's delay %.9f s, a fresh one's %.9f s, %.3g s apart",
				offset, got, want, got-want)
		}
	}
}

// TestArrivalAtObserverInvertsTheDelay: the arrival instant plus the delay
// there is the barycentric time asked for, to a nanosecond, for targets around
// the sky and through a year, and the Context is left at that instant.
func TestArrivalAtObserverInvertsTheDelay(t *testing.T) {
	t.Parallel()

	site := delaySite(t)

	ctx, err := coord.NewContext(epoch.FromJD(2461000.5, epoch.UTC), site, atmosphere.Refraction{})
	if err != nil {
		t.Fatal(err)
	}

	for day := 0.0; day < 365; day += 36.5 {
		bary := epoch.FromJD(2461000.5+day, epoch.TDB)

		for _, target := range []coord.ICRS{
			coord.NewICRS(angle.Deg(0), angle.Deg(0)),
			coord.NewICRS(angle.Deg(120), angle.Deg(-45)),
			coord.NewICRS(angle.Deg(270), angle.Deg(66.56)),
			coord.NewICRS(angle.Deg(359.99), angle.Deg(-89)),
		} {
			at := ctx.ArrivalAtObserver(target, bary)

			if !ctx.Time().Equal(at) && math.Abs(ctx.Time().Sub(at).Seconds()) > 1e-9 {
				t.Fatalf("day %.1f: the Context is at %v, not the arrival %v", day, ctx.Time(), at)
			}

			back := at.TDB().Add(ctx.BarycentricDelay(target))
			if miss := back.Sub(bary).Seconds(); math.Abs(miss) > 1e-9 {
				t.Errorf("day %.1f, %v: the arrival plus its delay misses the barycentric time by %.3g s",
					day, target, miss)
			}
		}
	}
}

// TestBarycentricDelayIsAProjection: the delay is the observer's barycentric
// position projected on the direction, so opposite directions give opposite
// delays, it never exceeds the observer's distance from the barycenter, about
// 1.02 au, in light time, and through a year a target in the ecliptic sees it
// swing nearly that far either way.
func TestBarycentricDelayIsAProjection(t *testing.T) {
	t.Parallel()

	site := delaySite(t)

	ctx, err := coord.NewContext(epoch.FromJD(2461000.5, epoch.UTC), site, atmosphere.Refraction{})
	if err != nil {
		t.Fatal(err)
	}

	const limit = 1.02 * 499.004784 // seconds of light time

	toward := coord.NewICRS(angle.Deg(0), angle.Deg(0))
	away := coord.NewICRS(angle.Deg(180), angle.Deg(0))

	lowest, highest := math.Inf(1), math.Inf(-1)

	for day := 0.0; day < 365.25; day++ {
		ctx.SetTime(epoch.FromJD(2461000.5+day, epoch.UTC))

		d, opposite := ctx.BarycentricDelay(toward).Seconds(), ctx.BarycentricDelay(away).Seconds()

		if math.Abs(d+opposite) > 1e-9 {
			t.Errorf("day %.0f: delays toward and away from one direction, %.6f and %.6f s, are not opposite",
				day, d, opposite)
		}

		if math.Abs(d) > limit {
			t.Errorf("day %.0f: delay %.3f s exceeds the observer's distance from the barycenter, %.3f s",
				day, d, limit)
		}

		lowest, highest = min(lowest, d), max(highest, d)
	}

	// The ecliptic's 23.4° tilt from the equator puts RA 0, Dec 0 on it, so
	// the full orbit sweeps past: nearly ±499 s, less the Earth's distance
	// below 1 au near perihelion.
	if highest < 480 || lowest > -480 {
		t.Errorf("an ecliptic target's delay ranged over [%.1f, %.1f] s through a year, want nearly ±499 s",
			lowest, highest)
	}
}
