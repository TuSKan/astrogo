package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestHoursUntilSetInterpolatesBetweenProbes is #554. estimateHoursUntilSet
// probes the altitude at +0.5, 1, 2, 4 and 8 h and interpolates across the
// probe interval the target sets in. It used to interpolate from now to the
// first probe below the horizon instead, ignoring the probes the target was
// still up at, so a star that had just risen came out about to set: 1.57 h
// against a true 7.07 h, quadrupling its urgency.
//
// The property is that the estimate lies between the two probes bracketing
// the true set, which linear interpolation across them guarantees whatever
// the altitude curve; the true set is the window's end from VisibleIntervals.
func TestHoursUntilSetInterpolatesBetweenProbes(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Deg(45), 0)
	if err != nil {
		t.Fatal(err)
	}

	site, err := NewSite("45N", loc)
	if err != nil {
		t.Fatal(err)
	}

	// Dec −30° culminates at 15° from 45° N: up about seven hours, and still
	// rising for the first half of them, which is the case the old
	// interpolation got wrong.
	star := mockObject{pos: coord.NewICRS(angle.Deg(100), angle.Deg(-30))}
	day := time.Date(2024, time.June, 15, 0, 0, 0, 0, time.LocationUTC)

	windows, err := VisibleIntervals(star, site, day, day.Add(unit.Days(1)), 5*time.Minute, angle.Zero())
	if err != nil || len(windows) != 1 {
		t.Fatalf("reference day: %d windows, %v; want one", len(windows), err)
	}

	w := windows[0].Window
	probes := []float64{0, 0.5, 1, 2, 4, 8}

	for _, after := range []float64{0.25, 0.5, 1, 2, 3, 5} {
		tm := w.Start.Add(unit.Hours(after))

		ctx := coord.NewContext(tm, site.Location(), site.Refraction())

		pos, err := star.Position(tm)
		if err != nil {
			t.Fatal(err)
		}

		aa, err := observedAltAz(star, tm, ctx, pos)
		if err != nil {
			t.Fatal(err)
		}

		got := estimateHoursUntilSet(star, tm, ctx, aa.Alt().Degrees())
		want := w.End.Sub(tm).Hours()

		lo, hi := probes[0], probes[len(probes)-1]
		for i := 1; i < len(probes); i++ {
			if probes[i] >= want {
				lo, hi = probes[i-1], probes[i]

				break
			}
		}

		if got < lo || got > hi {
			t.Errorf("rise+%.2f h: estimate %.2f h, outside the probes [%g, %g] h that bracket the true %.2f h",
				after, got, lo, hi, want)
		}

		// Linear across a probe gap of up to four hours: an hour is loose
		// enough for the curvature, and the old error was five and a half.
		if math.Abs(got-want) > 1 {
			t.Errorf("rise+%.2f h: estimate %.2f h, true %.2f h", after, got, want)
		}
	}
}
