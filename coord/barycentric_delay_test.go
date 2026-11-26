package coord_test

import (
	"math"
	"testing"
	"time"

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

// TestArrivalAtObserverMatchesSwarthmoreTransitFinder compares
// ArrivalAtObserver with an independent implementation: the Swarthmore
// Transit Finder (astro.swarthmore.edu/transits), queried on 2026-11-25 for
// WASP-4 b at its "European Southern Observatory: Paranal" site, with UTC
// output. For each of six transits it published first contact, midpoint and
// fourth contact as BJD_TDB, to 1e-4 day, and as UTC, to the minute; given
// its BJD_TDB, astrogo must land within the two roundings, 30 s plus 4.32 s,
// of its UTC.
//
// The September transits carry the largest Rømer delay WASP-4 sees, about
// +6.8 minutes, and the December ones under a minute: a correction left out
// or reversed misses the first set by most of seven minutes.
func TestArrivalAtObserverMatchesSwarthmoreTransitFinder(t *testing.T) {
	t.Parallel()

	site, err := coord.NewGeodetic(angle.LatDeg(-24.625), angle.LonDeg(-70.403333), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	// WASP-4 as the Transit Finder lists it, 23:34:15.10 -42:03:42.41.
	wasp4 := coord.NewICRS(angle.Deg((23+34/60.0+15.10/3600)*15), angle.Deg(-(42 + 3/60.0 + 42.41/3600)))

	const tolerance = 30 + 0.5e-4*86400 // seconds: the UTC minute, and the BJD's last digit

	events := []struct {
		day string     // the UTC date of the transit
		bjd [3]float64 // BJD_TDB - 2450000 at first contact, midpoint, fourth contact
		utc [3]string  // the same, UTC, as hh:mm
	}{
		{"2026-09-02", [3]float64{11285.8661, 11285.9113, 11285.9566}, [3]string{"08:39", "09:44", "10:49"}},
		{"2026-09-05", [3]float64{11288.5426, 11288.5878, 11288.6330}, [3]string{"00:53", "01:59", "03:04"}},
		{"2026-09-06", [3]float64{11289.8808, 11289.9260, 11289.9712}, [3]string{"09:00", "10:06", "11:11"}},
		{"2026-11-27", [3]float64{11371.5129, 11371.5582, 11371.6034}, [3]string{"00:17", "01:23", "02:28"}},
		{"2026-12-01", [3]float64{11375.5276, 11375.5728, 11375.6181}, [3]string{"00:39", "01:44", "02:49"}},
		{"2026-12-05", [3]float64{11379.5423, 11379.5875, 11379.6327}, [3]string{"01:01", "02:06", "03:11"}},
	}

	ctx, err := coord.NewContext(epoch.FromJD(2461285.9, epoch.UTC), site, atmosphere.Refraction{})
	if err != nil {
		t.Fatal(err)
	}

	worst := 0.0

	for _, ev := range events {
		for i, b := range ev.bjd {
			whole := math.Floor(b)
			bary := epoch.FromJDParts(2450000+whole, b-whole, epoch.TDB)

			published, err := time.Parse("2006-01-02 15:04", ev.day+" "+ev.utc[i])
			if err != nil {
				t.Fatal(err)
			}

			off := ctx.ArrivalAtObserver(wasp4, bary).Sub(epoch.FromGo(published)).Seconds()
			worst = max(worst, math.Abs(off))

			if math.Abs(off) > tolerance {
				t.Errorf("%s %s: BJD_TDB %.4f arrives at %+.1f s from the Transit Finder's %s UTC, beyond its %.1f s of rounding",
					ev.day, []string{"first contact", "midpoint", "fourth contact"}[i], 2450000+b, off, ev.utc[i], tolerance)
			}
		}
	}

	t.Logf("worst disagreement with the Transit Finder over 18 contacts: %.1f s, its rounding %.1f s", worst, tolerance)
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
