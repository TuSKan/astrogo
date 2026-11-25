package plan

import (
	"errors"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// testEphemeris is a transit every 2.5 days lasting 3 hours, with an
// uncertainty whose epoch and period are correlated.
func testEphemeris() TransitEphemeris {
	return TransitEphemeris{
		Epoch:    epoch.FromJD(2461000.5, epoch.TDB),
		Period:   unit.Days(2.5),
		Duration: unit.Hours(3),
		Uncertainty: &TimingUncertainty{
			Epoch:       unit.Seconds(60),
			Period:      unit.Seconds(2),
			Correlation: 0.3,
		},
	}
}

// TestTransitsEnumeratesTheCyclesWhoseWindowsOverlap: the cycles returned are
// exactly those whose windows reach into the interval, negative ones before
// the epoch included, each at Epoch + n·Period with the uncertainty
// Var(Tn) = σ0² + 2n·ρ·σ0·σP + n²·σP² gives.
func TestTransitsEnumeratesTheCyclesWhoseWindowsOverlap(t *testing.T) {
	t.Parallel()

	e := testEphemeris()
	need := TransitWindow{Before: unit.Hours(1), After: unit.Hours(1), Sigmas: 3}

	got, err := e.Transits(Window{Start: e.Epoch.Add(unit.Days(-6)), End: e.Epoch.Add(unit.Days(6))}, need)
	if err != nil {
		t.Fatal(err)
	}

	cycles := make([]int, 0, len(got))
	for _, tr := range got {
		cycles = append(cycles, tr.Cycle)
	}

	if want := []int{-2, -1, 0, 1, 2}; !slices.Equal(cycles, want) {
		t.Fatalf("cycles %v, want %v", cycles, want)
	}

	for _, tr := range got {
		n := float64(tr.Cycle)

		if off := tr.Mid.Sub(e.Epoch).Seconds() - n*e.Period.Seconds(); math.Abs(off) > 1e-6 {
			t.Errorf("cycle %d: midpoint %.6f s from Epoch + n·Period", tr.Cycle, off)
		}

		if d := tr.End.Sub(tr.Start); math.Abs(d.Seconds()-e.Duration.Seconds()) > 1e-6 {
			t.Errorf("cycle %d: first to fourth contact is %v s, want the duration", tr.Cycle, d.Seconds())
		}

		want := math.Sqrt(60*60 + 2*n*0.3*60*2 + n*n*2*2)
		if math.Abs(tr.Sigma.Seconds()-want) > 1e-9 {
			t.Errorf("cycle %d: sigma %.9f s, want %.9f", tr.Cycle, tr.Sigma.Seconds(), want)
		}

		if tr.Flags != 0 {
			t.Errorf("cycle %d: flags %b, want none", tr.Cycle, tr.Flags)
		}
	}
}

// TestTransitsIncludesAWindowThatReachesIn: a transit whose midpoint lies
// outside the interval is returned while its window, baseline and margin
// included, reaches into it, and not once it does not.
func TestTransitsIncludesAWindowThatReachesIn(t *testing.T) {
	t.Parallel()

	e := testEphemeris()

	// Cycle 0's fourth contact is 1.5 h after Epoch; with an hour of baseline
	// and 3 × 60 s of margin its window ends at 2 h 33 min.
	need := TransitWindow{After: unit.Hours(1), Sigmas: 3}
	windowEnd := unit.Hours(1.5) + unit.Hours(1) + unit.Seconds(180)

	for _, tc := range []struct {
		name  string
		start unit.Duration
		want  bool
	}{
		{"a second before the window ends", windowEnd - unit.Seconds(1), true},
		{"a second after it ends", windowEnd + unit.Seconds(1), false},
	} {
		got, err := e.Transits(Window{Start: e.Epoch.Add(tc.start), End: e.Epoch.Add(unit.Days(1))}, need)
		if err != nil {
			t.Fatal(err)
		}

		if has := len(got) > 0 && got[0].Cycle == 0; has != tc.want {
			t.Errorf("an interval starting %s: cycle 0 returned %v, want %v", tc.name, has, tc.want)
		}
	}
}

// TestTransitsFlagWhatTheyAssume: a transit reports an ephemeris stating no
// uncertainty, one marked as varying, and a window reaching its neighbor's.
func TestTransitsFlagWhatTheyAssume(t *testing.T) {
	t.Parallel()

	e := testEphemeris()
	e.Uncertainty = nil
	e.TimingVariations = true

	got, err := e.Transits(Window{Start: e.Epoch, End: e.Epoch.Add(unit.Days(5))}, TransitWindow{Sigmas: 3})
	if err != nil {
		t.Fatal(err)
	}

	for _, tr := range got {
		if tr.Flags != TransitUncertaintyUnknown|TransitTimingVariations || tr.Sigma != 0 {
			t.Errorf("cycle %d: flags %b, sigma %v, want unknown uncertainty and timing variations, sigma 0",
				tr.Cycle, tr.Flags, tr.Sigma)
		}
	}

	// A 4-hour period with a 1-hour transit and 2 hours of baseline either
	// side: every window is 5 hours long, so each reaches its neighbors.
	short := TransitEphemeris{Epoch: e.Epoch, Period: unit.Hours(4), Duration: unit.Hours(1)}
	span := Window{Start: e.Epoch, End: e.Epoch.Add(unit.Hours(12))}

	got, err = short.Transits(span, TransitWindow{Before: unit.Hours(2), After: unit.Hours(2)})
	if err != nil {
		t.Fatal(err)
	}

	for _, tr := range got {
		if tr.Flags&TransitOverlapsNeighbor == 0 {
			t.Errorf("cycle %d: a window reaching its neighbors is not flagged", tr.Cycle)
		}
	}

	// The same ephemeris with no baseline leaves three hours between windows.
	got, err = short.Transits(span, TransitWindow{})
	if err != nil {
		t.Fatal(err)
	}

	for _, tr := range got {
		if tr.Flags&TransitOverlapsNeighbor != 0 {
			t.Errorf("cycle %d: flagged as overlapping with three hours between windows", tr.Cycle)
		}
	}
}

// TestTransitsRefusesWhatCannotBePredicted: an ephemeris or a window no
// transit can be predicted from is an error naming which.
func TestTransitsRefusesWhatCannotBePredicted(t *testing.T) {
	t.Parallel()

	w := Window{Start: testEphemeris().Epoch, End: testEphemeris().Epoch.Add(unit.Days(10))}

	for _, tc := range []struct {
		name   string
		modify func(*TransitEphemeris, *TransitWindow)
		want   error
	}{
		{"a zero period", func(e *TransitEphemeris, _ *TransitWindow) { e.Period = 0 }, ErrInvalidEphemeris},
		{"a negative period", func(e *TransitEphemeris, _ *TransitWindow) { e.Period = unit.Days(-1) }, ErrInvalidEphemeris},
		{"a period that is not a number", func(e *TransitEphemeris, _ *TransitWindow) {
			e.Period = unit.Seconds(math.NaN())
		}, ErrInvalidEphemeris},
		{"a duration as long as the period", func(e *TransitEphemeris, _ *TransitWindow) {
			e.Duration = e.Period
		}, ErrInvalidEphemeris},
		{"no duration", func(e *TransitEphemeris, _ *TransitWindow) { e.Duration = 0 }, ErrInvalidEphemeris},
		{"a negative uncertainty", func(e *TransitEphemeris, _ *TransitWindow) {
			e.Uncertainty.Epoch = unit.Seconds(-1)
		}, ErrInvalidEphemeris},
		{"a correlation above 1", func(e *TransitEphemeris, _ *TransitWindow) {
			e.Uncertainty.Correlation = 1.01
		}, ErrInvalidEphemeris},
		{"a correlation below -1", func(e *TransitEphemeris, _ *TransitWindow) {
			e.Uncertainty.Correlation = -1.01
		}, ErrInvalidEphemeris},
		{"a negative baseline", func(_ *TransitEphemeris, n *TransitWindow) { n.Before = unit.Hours(-1) }, ErrInvalidTransitWindow},
		{"a negative margin", func(_ *TransitEphemeris, n *TransitWindow) { n.Sigmas = -1 }, ErrInvalidTransitWindow},
		{"a margin outgrowing the period", func(e *TransitEphemeris, n *TransitWindow) {
			e.Uncertainty.Period = unit.Hours(1)
			n.Sigmas = 60
		}, ErrInvalidTransitWindow},
	} {
		e := testEphemeris()
		need := TransitWindow{Sigmas: 3}
		tc.modify(&e, &need)

		if _, err := e.Transits(w, need); !errors.Is(err, tc.want) {
			t.Errorf("%s: err %v, want %v", tc.name, err, tc.want)
		}
	}

	// A correlation of exactly ±1 is a valid, degenerate fit.
	e := testEphemeris()
	e.Uncertainty.Correlation = -1

	if _, err := e.Transits(w, TransitWindow{Sigmas: 3}); err != nil {
		t.Errorf("a correlation of -1: %v", err)
	}
}

// TestObserveTransitsAtTheGeometricAltitudeLimit: a target at declination δ
// never stands higher than 90° − |φ − δ| above a site at latitude φ, so no
// transit is observable above that and some are just below it, at sites
// north and south of the target and on the equator.
func TestObserveTransitsAtTheGeometricAltitudeLimit(t *testing.T) {
	t.Parallel()

	const dec = 20.0

	target := NewStar("δ = +20°", angle.Deg(140), angle.Deg(dec))

	// A period no multiple of which is near a sidereal day, so over three
	// weeks the transits fall at every hour angle.
	e := TransitEphemeris{Epoch: epoch.FromJD(2461000.5, epoch.TDB), Period: unit.Days(1.3712), Duration: unit.Hours(2)}
	w := Window{Start: e.Epoch, End: e.Epoch.Add(unit.Days(21))}

	for _, lat := range []float64{-50, -10, 0, 25, 60} {
		site, err := NewSite("φ", angle.LatDeg(lat), angle.LonDeg(-70), unit.Meters(0))
		if err != nil {
			t.Fatal(err)
		}

		highest := 90 - math.Abs(lat-dec)

		// Above the limit by more than refraction and the apparent place can
		// lift the target, and below it by enough to be reached.
		above, err := ObserveTransits(e, target, site, w, TransitWindow{}, Altitude{Threshold: angle.Deg(highest + 0.5)})
		if err != nil {
			t.Fatal(err)
		}

		for _, o := range above {
			if o.Coverage != TransitUnobservable || len(o.Observable) != 0 {
				t.Errorf("φ %+.0f°: cycle %d is %v above %.1f°, which a target at δ %+.0f° never reaches",
					lat, o.Predicted.Cycle, o.Coverage, highest+0.5, dec)
			}

			if len(o.Lost) != 1 || o.Lost[0].Window != o.Window || !strings.Contains(o.Lost[0].Reason, "altitude") {
				t.Errorf("φ %+.0f°: cycle %d lost %v, want its whole window to the altitude constraint",
					lat, o.Predicted.Cycle, o.Lost)
			}
		}

		below, err := ObserveTransits(e, target, site, w, TransitWindow{}, Altitude{Threshold: angle.Deg(highest - 3)})
		if err != nil {
			t.Fatal(err)
		}

		seen := 0

		for _, o := range below {
			if o.Coverage != TransitUnobservable {
				seen++
			}
		}

		if seen == 0 || len(below) != len(above) {
			t.Errorf("φ %+.0f°: %d of %d transits observable above %.1f°, want some of the same %d",
				lat, seen, len(below), highest-3, len(above))
		}
	}
}

// nightFixture is a transit ephemeris whose cycle 0 an observer at site sees
// centered on target's meridian passage in the night of 2026-12-01, and the
// passage it was built from.
func nightFixture(t *testing.T, site *Site, target *Star) (TransitEphemeris, epoch.Time) {
	t.Helper()

	search := Window{
		Start: epoch.FromJD(2461375.5, epoch.UTC), // 2026-12-01 00:00 UTC
		End:   epoch.FromJD(2461376.4, epoch.UTC),
	}

	events, err := NewEventSolver(unit.Minutes(15), unit.Seconds(0.001)).Find(EventSpec{
		Family: EventFamilyVisibility, Kind: EventTransit, Target: target, Observer: site,
	}, search)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) == 0 {
		t.Fatalf("no meridian passage of %s in %v", target.Name(), search)
	}

	culmination := events[0].Time

	ctx, err := siteContext(culmination, site)
	if err != nil {
		t.Fatal(err)
	}

	pos, err := target.Position(culmination)
	if err != nil {
		t.Fatal(err)
	}

	return TransitEphemeris{
		Epoch:    culmination.TDB().Add(ctx.BarycentricDelay(pos)),
		Period:   unit.Days(3.1),
		Duration: unit.Hours(2),
		Uncertainty: &TimingUncertainty{
			Epoch:  unit.Seconds(90),
			Period: unit.Seconds(5),
		},
	}, culmination
}

// TestObserveTransitsFromTwoSites is the issue's pair of fixtures: one transit
// and its baseline wholly observable from a southern site, centered on the
// target's meridian passage in a December night; partly observable when the
// ephemeris puts its egress after dawn; and not observable at all from a
// northern site where the target never climbs to the altitude asked for.
func TestObserveTransitsFromTwoSites(t *testing.T) {
	t.Parallel()

	south, err := NewSite("south", angle.LatDeg(-24.6), angle.LonDeg(-70.4), unit.Meters(2635))
	if err != nil {
		t.Fatal(err)
	}

	north, err := NewSite("north", angle.LatDeg(40), angle.LonDeg(-70.4), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	// Culminating at about 01:20 local time, near the zenith, in early
	// December, when morning twilight passes -12° near 04:10.
	target := NewStar("δ = -25°", angle.Deg(90), angle.Deg(-25))
	e, culmination := nightFixture(t, south, target)

	need := TransitWindow{Before: unit.Minutes(30), After: unit.Minutes(30), Sigmas: 3}
	constraints := []Constraint{Altitude{Threshold: angle.Deg(30)}, Sun{Threshold: angle.Deg(-12)}}
	night := Window{Start: culmination.Add(unit.Hours(-6)), End: culmination.Add(unit.Hours(6))}

	full := observeOne(t, e, target, south, night, need, constraints)

	if full.Coverage != TransitFull || !full.IngressObserved || !full.EgressObserved || len(full.Lost) != 0 {
		t.Fatalf("from the south: %v coverage, ingress %v, egress %v, lost %v; want all of it",
			full.Coverage, full.IngressObserved, full.EgressObserved, full.Lost)
	}

	if off := full.Mid.Sub(culmination).Seconds(); math.Abs(off) > 1e-6 {
		t.Errorf("the site midpoint is %.3g s from the meridian passage the ephemeris was built at", off)
	}

	if want := unit.Seconds(3 * 90); math.Abs((full.Margin - want).Seconds()) > 1e-9 {
		t.Errorf("margin %v s, want three sigmas of cycle 0, %v s", full.Margin.Seconds(), want.Seconds())
	}

	// The window is the transit widened by the margin and the baselines.
	if lead, tail := full.Ingress.Sub(full.Window.Start), full.Window.End.Sub(full.Egress); math.Abs(
		(lead-full.Margin-need.Before).Seconds()) > 1e-6 || math.Abs((tail-full.Margin-need.After).Seconds()) > 1e-6 {
		t.Errorf("the window starts %v s before ingress and ends %v s after egress, want margin and baseline, %v s",
			lead.Seconds(), tail.Seconds(), (full.Margin + need.Before).Seconds())
	}

	// An hour and a half later the transit and its margin still end some
	// fifteen minutes before morning twilight, but the baseline after it does
	// not: the transit is observed whole, and the observation is partial.
	shifted := e
	shifted.Epoch = e.Epoch.Add(unit.Hours(1.5))

	baselineLost := observeOne(t, shifted, target, south, night, need, constraints)

	if baselineLost.Coverage != TransitPartial || !baselineLost.IngressObserved || !baselineLost.EgressObserved {
		t.Fatalf("baseline into dawn: %v coverage, ingress %v, egress %v; want partial with both contacts",
			baselineLost.Coverage, baselineLost.IngressObserved, baselineLost.EgressObserved)
	}

	if n := len(baselineLost.Lost); n != 1 || !baselineLost.Lost[0].Start.After(baselineLost.Egress.Add(baselineLost.Margin)) {
		t.Fatalf("baseline into dawn: lost %v, want the baseline after egress and its margin alone", baselineLost.Lost)
	}

	// Two and a half hours later the egress, and the baseline after it, fall
	// in morning twilight while the target is still high.
	late := e
	late.Epoch = e.Epoch.Add(unit.Hours(2.5))

	partial := observeOne(t, late, target, south, night, need, constraints)

	if partial.Coverage != TransitPartial || !partial.IngressObserved || partial.EgressObserved {
		t.Fatalf("after dawn: %v coverage, ingress %v, egress %v; want partial, the ingress and not the egress",
			partial.Coverage, partial.IngressObserved, partial.EgressObserved)
	}

	if n := len(partial.Lost); n != 1 || !strings.Contains(partial.Lost[0].Reason, "sun altitude") ||
		partial.Lost[0].End != partial.Window.End {
		t.Fatalf("after dawn: lost %v, want the end of the window to the Sun", partial.Lost)
	}

	if last := partial.Observable[len(partial.Observable)-1]; last.End != partial.Lost[0].Start {
		t.Errorf("after dawn: the observable time ends at %v and the lost time starts at %v",
			last.End, partial.Lost[0].Start)
	}

	// From 40° N the target, at δ -25°, climbs no higher than 25°.
	none := observeOne(t, e, target, north, night, need, constraints)

	if none.Coverage != TransitUnobservable || none.IngressObserved || none.EgressObserved {
		t.Fatalf("from the north: %v coverage, ingress %v, egress %v; want none of it",
			none.Coverage, none.IngressObserved, none.EgressObserved)
	}

	altitudeLost := false

	for _, l := range none.Lost {
		if _, ok := l.Constraint.(Altitude); ok && l.Window == none.Window {
			altitudeLost = true
		}
	}

	if !altitudeLost {
		t.Errorf("from the north: lost %v, want the whole window to the altitude constraint", none.Lost)
	}
}

// observeOne is ObserveTransits returning its single cycle-0 transit.
func observeOne(
	t *testing.T, e TransitEphemeris, target Observable, site *Site, w Window, need TransitWindow, cs []Constraint,
) TransitObservation {
	t.Helper()

	got, err := ObserveTransits(e, target, site, w, need, cs...)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != 1 || got[0].Predicted.Cycle != 0 {
		t.Fatalf("%d transits, want cycle 0 alone: %v", len(got), got)
	}

	return got[0]
}

// TestObserveTransitsHoldsTheSiteInterval: which transits ObserveTransits
// returns is decided by their windows at the site, which the Rømer delay
// moves by about eight minutes from the barycenter's, either way. A window
// reaching into the interval only at the site is returned, and one that ends
// before it at the site is not, whatever the barycenter's window does.
func TestObserveTransitsHoldsTheSiteInterval(t *testing.T) {
	t.Parallel()

	site, err := NewSite("south", angle.LatDeg(-24.6), angle.LonDeg(-70.4), unit.Meters(2635))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name   string
		target *Star
		late   bool
	}{
		// In early December the Sun is near RA 247°, Dec -22°. Beside it the
		// observer is farther from the target than the barycenter is, and sees
		// each transit after it; opposite it, nearer, and before it.
		{"beside the Sun", NewStar("conjunction", angle.Deg(250), angle.Deg(-22)), true},
		{"opposite the Sun", NewStar("opposition", angle.Deg(70), angle.Deg(22)), false},
	} {
		e, culmination := nightFixture(t, site, tc.target)

		at := observeOne(t, e, tc.target, site,
			Window{Start: culmination.Add(unit.Hours(-6)), End: culmination.Add(unit.Hours(6))}, TransitWindow{}, nil)

		shift := at.Mid.Sub(at.Predicted.Mid).Minutes()
		if (tc.late && shift < 7) || (!tc.late && shift > -7) {
			t.Fatalf("%s: the site sees the transit %+.2f min from the barycenter, want about eight minutes %s",
				tc.name, shift, map[bool]string{true: "after", false: "before"}[tc.late])
		}

		// Beside the Sun, a minute before the site's window ends is minutes
		// after the barycenter's has: only the widened search finds it.
		inside := Window{Start: at.Window.End.Add(unit.Minutes(-1)), End: at.Window.End.Add(unit.Hours(6))}

		got, err := ObserveTransits(e, tc.target, site, inside, TransitWindow{})
		if err != nil {
			t.Fatal(err)
		}

		if len(got) == 0 || got[0].Predicted.Cycle != 0 {
			t.Errorf("%s: an interval starting a minute before the site's window ends: %d transits, want cycle 0",
				tc.name, len(got))
		}

		// Opposite the Sun, a minute after the site's window ends is minutes
		// before the barycenter's does: only the site's time excludes it.
		outside := Window{Start: at.Window.End.Add(unit.Minutes(1)), End: at.Window.End.Add(unit.Hours(6))}

		got, err = ObserveTransits(e, tc.target, site, outside, TransitWindow{})
		if err != nil {
			t.Fatal(err)
		}

		if len(got) != 0 {
			t.Errorf("%s: an interval starting a minute after the site's window ends: cycle %d returned",
				tc.name, got[0].Predicted.Cycle)
		}
	}
}

// TestObserveTransitsRefusesNoTargetOrSite: a nil target or site is the
// package's sentinel for it.
func TestObserveTransitsRefusesNoTargetOrSite(t *testing.T) {
	t.Parallel()

	e := testEphemeris()
	w := Window{Start: e.Epoch, End: e.Epoch.Add(unit.Days(3))}

	site, err := NewSite("s", angle.LatDeg(0), angle.LonDeg(0), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ObserveTransits(e, nil, site, w, TransitWindow{}); !errors.Is(err, ErrNilTarget) {
		t.Errorf("no target: err %v, want ErrNilTarget", err)
	}

	if _, err := ObserveTransits(e, NewStar("s", angle.Deg(0), angle.Deg(0)), nil, w, TransitWindow{}); !errors.Is(err, ErrNilSite) {
		t.Errorf("no site: err %v, want ErrNilSite", err)
	}
}

// TestTransitCoverageString names each coverage.
func TestTransitCoverageString(t *testing.T) {
	t.Parallel()

	for c, want := range map[TransitCoverage]string{
		TransitUnobservable: "unobservable", TransitPartial: "partial", TransitFull: "full", 7: "TransitCoverage(7)",
	} {
		if got := c.String(); got != want {
			t.Errorf("%d: %q, want %q", int(c), got, want)
		}
	}
}
