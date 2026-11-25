package plan

import (
	"fmt"
	"slices"

	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// TransitObservation is one predicted transit as an observer at a site sees
// it, and how much of the time it needs the site provides.
type TransitObservation struct {
	// Predicted is the prediction at the barycenter.
	Predicted PredictedTransit

	// Ingress, Mid and Egress are first contact, the midpoint and fourth
	// contact as their light reaches the site. Render one with .UTC().
	Ingress, Mid, Egress epoch.Time

	// Margin is the timing margin applied on each side, the window's Sigmas
	// times Predicted.Sigma: zero when the ephemeris states no uncertainty.
	Margin unit.Duration

	// Window is the time the observation needs: from Ingress less Margin
	// and the Before baseline to Egress plus Margin and the After baseline.
	Window Window

	// Observable is the parts of Window where every constraint passes, in
	// time order.
	Observable []Window

	// Lost is the parts of Window each constraint fails, with its reason, in
	// time order. Where two constraints fail at once, both are listed.
	Lost []LostInterval

	// Coverage is how much of Window, and of the transit within it, is
	// observable.
	Coverage TransitCoverage

	// IngressObserved and EgressObserved report whether the time around
	// first and fourth contact, Margin either side, is observable whole.
	// Without a margin, that is the instant of contact.
	IngressObserved, EgressObserved bool
}

// LostInterval is a stretch of a [TransitObservation]'s window that one
// constraint fails.
type LostInterval struct {
	Window

	// Constraint is the constraint that fails.
	Constraint Constraint

	// Reason is the constraint's own account of the failure, at the middle of
	// the interval.
	Reason string
}

// TransitCoverage is how much of the time a transit observation needs is
// observable.
type TransitCoverage int

const (
	// TransitUnobservable is a transit none of which, from first contact less
	// the margin to fourth contact plus it, is observable. Some of the
	// baselines may be.
	TransitUnobservable TransitCoverage = iota

	// TransitPartial is a transit of which some is observable, but not the
	// whole window, baselines and margins included.
	TransitPartial

	// TransitFull is a transit whose whole window, baselines and margins
	// included, is observable.
	TransitFull
)

// String names the coverage.
func (c TransitCoverage) String() string {
	switch c {
	case TransitUnobservable:
		return "unobservable"
	case TransitPartial:
		return "partial"
	case TransitFull:
		return "full"
	default:
		return fmt.Sprintf("TransitCoverage(%d)", int(c))
	}
}

// ObserveTransits returns the transits of e an observer at site can attempt
// within w, an interval of instants at the site, and what each offers under
// the constraints: when its light reaches the site, the window need asks for
// around it, the parts of that window every constraint passes, and the parts
// each one fails, with its reason.
//
// A transit is returned when its window reaches into w, wherever its midpoint
// falls. Its light reaches the site up to about 8.3 minutes before or after
// the barycenter, by the Rømer delay toward target, which
// [coord.Context.ArrivalAtObserver] takes out at target's position at the
// transit, proper motion applied.
//
// The constraints are the ones the rest of the package evaluates: [Altitude]
// for a minimum altitude, [Sun] for a twilight threshold, [Airmass],
// [MoonSep] and [MoonIllum]. With none, every window is observable. Each is
// sampled once a minute across the window, and each change refined to the
// instant, so a stretch shorter than a minute can go unseen.
//
// The errors are those of [TransitEphemeris.Transits], [ErrNilTarget],
// [ErrNilSite], and a constraint's own.
func ObserveTransits(
	e TransitEphemeris,
	target Observable,
	site *Site,
	w Window,
	need TransitWindow,
	constraints ...Constraint,
) ([]TransitObservation, error) {
	switch {
	case target == nil:
		return nil, ErrNilTarget
	case site == nil:
		return nil, ErrNilSite
	}

	if err := checkInterval(w.Start, w.End); err != nil {
		return nil, err
	}

	// A transit reaches the site within the largest Rømer delay of reaching
	// the barycenter, so the barycentric search is widened by it, and what it
	// finds is held to w once at the site.
	transits, err := e.Transits(Window{Start: w.Start.Add(-maxRomerDelay), End: w.End.Add(maxRomerDelay)}, need)
	if err != nil {
		return nil, err
	}

	ctx, err := siteContext(w.Start, site)
	if err != nil {
		return nil, err
	}

	out := make([]TransitObservation, 0, len(transits))

	for _, tr := range transits {
		pos, err := target.Position(tr.Mid)
		if err != nil {
			return nil, fmt.Errorf("plan: %s at cycle %d: %w", target.Name(), tr.Cycle, err)
		}

		o := TransitObservation{
			Predicted: tr,
			Ingress:   ctx.ArrivalAtObserver(pos, tr.Start),
			Mid:       ctx.ArrivalAtObserver(pos, tr.Mid),
			Egress:    ctx.ArrivalAtObserver(pos, tr.End),
			Margin:    unit.Seconds(need.Sigmas * tr.Sigma.Seconds()),
		}

		o.Window = Window{
			Start: o.Ingress.Add(-o.Margin - need.Before),
			End:   o.Egress.Add(o.Margin + need.After),
		}

		if o.Window.End.Before(w.Start) || o.Window.Start.After(w.End) {
			continue
		}

		if err := o.cover(target, site, constraints); err != nil {
			return nil, err
		}

		out = append(out, o)
	}

	return out, nil
}

// cover fills in what of o's window the constraints leave observable, what
// each takes, and the coverage that makes.
func (o *TransitObservation) cover(target Observable, site *Site, constraints []Constraint) error {
	o.Observable = []Window{o.Window}

	for _, c := range constraints {
		passing, err := ObservableWindows(target, site, o.Window, transitScanStep, c)
		if err != nil {
			return err
		}

		o.Observable = Intersect(o.Observable, passing)

		for _, gap := range Subtract([]Window{o.Window}, passing) {
			mid := gap.Start.Add(gap.End.Sub(gap.Start) / 2)

			res, err := c.Check(target, site, mid)
			if err != nil {
				return fmt.Errorf("plan: %T at cycle %d: %w", c, o.Predicted.Cycle, err)
			}

			reason := res.Reason
			if reason == "" {
				reason = fmt.Sprintf("%T fails", c)
			}

			o.Lost = append(o.Lost, LostInterval{Window: gap, Constraint: c, Reason: reason})
		}
	}

	slices.SortStableFunc(o.Lost, func(a, b LostInterval) int {
		switch {
		case a.Start.Before(b.Start):
			return -1
		case a.Start.After(b.Start):
			return 1
		default:
			return 0
		}
	})

	transit := Window{Start: o.Ingress.Add(-o.Margin), End: o.Egress.Add(o.Margin)}

	switch {
	case covered(o.Observable, o.Window):
		o.Coverage = TransitFull
	case len(Intersect(o.Observable, []Window{transit})) > 0:
		o.Coverage = TransitPartial
	default:
		o.Coverage = TransitUnobservable
	}

	o.IngressObserved = covered(o.Observable, Window{Start: o.Ingress.Add(-o.Margin), End: o.Ingress.Add(o.Margin)})
	o.EgressObserved = covered(o.Observable, Window{Start: o.Egress.Add(-o.Margin), End: o.Egress.Add(o.Margin)})

	return nil
}

// maxRomerDelay is more than the Rømer delay can be: the observer's distance
// from the barycenter, at most 1.02 au, in light time, is 509 seconds.
var maxRomerDelay = unit.Seconds(520)

// transitScanStep is how often ObserveTransits samples each constraint. Each
// change is refined to the instant by bisection, so the step only sets the
// shortest stretch that can go unseen, and a minute is below what any
// observing sequence plans in.
var transitScanStep = unit.Minutes(1)

// covered reports whether one of the windows holds all of w, which may be an
// instant.
func covered(windows []Window, w Window) bool {
	for _, in := range windows {
		if !in.Start.After(w.Start) && !in.End.Before(w.End) {
			return true
		}
	}

	return false
}
