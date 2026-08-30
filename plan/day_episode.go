package plan

import (
	"fmt"
	"slices"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// episodeSearchWindow bounds how far Episode looks outside its own [from,
// to] argument for a missing rise or set before concluding there isn't one
// there — a target that's circumpolar (never sets) or permanently below
// the horizon (never rises). A year is comfortably longer than any real
// periodic visibility cycle this package computes events for.
const episodeSearchWindow = 366 * 24 * time.Hour

// initialSearchStepDays is searchEvent's first probe width. Deliberately
// small: the overwhelmingly common case is a rise or set within hours to
// a couple of days, and probing with episodeSearchWindow's full year from
// the start (an earlier version of this file did exactly that) turns
// every call into a multi-second solver sweep for no reason. The window
// doubles on each retry up to episodeSearchWindow, so the slow, wide
// sweep only happens for a target that genuinely doesn't cross the
// horizon within the first several doublings.
const initialSearchStepDays = 2.0

// maxEpisodeSearchSteps bounds how many times searchEvent doubles its
// probe window. Starting at initialSearchStepDays and doubling reaches
// episodeSearchWindow's ~366 days within 8 steps (2·2⁸ ≈ 512), so this
// stays generous without being unbounded — a pathological evaluator (or a
// genuine mathematical edge case this package hasn't anticipated) fails
// fast rather than looping for a very long time.
const maxEpisodeSearchSteps = 10

// dayEventsThreshold and dayEventsSpec are shared by DayEvents and Episode:
// both answer "when does this arbitrary Observable cross the site's true,
// elevation-corrected horizon" — the same convention plan.RiseSetThreshold
// documents and the generic-target rise/set solver at events.go already
// uses. Neither adds atmospheric refraction the way SunEvents/MoonEvents
// do for their specific bodies; a caller wanting that runs its own solver
// with site.SunRiseSetThreshold()/MoonRiseSetThreshold() instead.
func visibilityEvents(target Observable, site *Site, start, end time.Time) ([]Event, error) {
	return NewEventSolver(15*time.Minute, 1*time.Second).Find(EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    target,
		Observer:  site,
		Threshold: site.RiseSetThreshold(),
	}, start, end)
}

// DayEvents returns the first rise, set, and transit of target, as seen
// from site, within the local calendar day (loc's own midnight-to-midnight
// window) containing day — the day-indexed almanac-table view: "today's
// rise/set/transit" for a given date and timezone, as opposed to Episode's
// continuous-up-episode framing.
//
// Any of the three may come back nil if that event kind doesn't occur
// within the local day (a circumpolar or never-rises target, or a target
// that transits but doesn't cross the horizon at all that day) — this is
// not an error, just what that day looked like. loc's the timezone the
// calendar day is measured in; site's location (via site.Location()) is
// used for the geometry, and the two need not agree (a caller may
// legitimately want "Paris's calendar day" applied to "Mauna Kea's sky").
func DayEvents(day time.Time, loc *time.Location, target Observable, site *Site) (rise, set, transit *Event, err error) {
	if loc == nil {
		loc = time.LocationUTC
	}

	local := day.GoTime().In(loc)
	y, m, d := local.Date()

	start := time.FromGo(time.GoDate(y, m, d, 0, 0, 0, 0, loc))
	end := start.AddDays(1)

	events, err := visibilityEvents(target, site, start, end)
	if err != nil {
		return nil, nil, nil, err
	}

	for i := range events {
		switch events[i].Kind { //nolint:exhaustive // only rise/set/transit are relevant here
		case EventRise:
			if rise == nil {
				rise = &events[i]
			}
		case EventSet:
			if set == nil {
				set = &events[i]
			}
		case EventTransit:
			if transit == nil {
				transit = &events[i]
			}
		}
	}

	return rise, set, transit, nil
}

// isAboveHorizon reports whether target's altitude at t, as seen from
// site, is above site.RiseSetThreshold() — the single-instant check
// Episode needs to tell whether [from, to] starts already inside an
// up-episode.
func isAboveHorizon(target Observable, site *Site, t time.Time) (bool, error) {
	pos, err := target.Position(t)
	if err != nil {
		return false, fmt.Errorf("plan: position: %w", err)
	}

	altaz, err := coord.NewContext(t, site.Location(), site.Refraction()).ICRSToAltAz(pos)
	if err != nil {
		return false, fmt.Errorf("plan: ICRS to AltAz: %w", err)
	}

	return altaz.Alt() > site.RiseSetThreshold(), nil
}

// searchEvent looks for the nearest event of kind k at-or-after from
// (forward) or at-or-before from (backward). It starts with a short probe
// (initialSearchStepDays) and doubles the window on each retry, up to
// maxSpanDays total and maxEpisodeSearchSteps attempts, so the common
// case — a rise or set within hours to a couple of days — resolves in one
// small solver call, while a target that genuinely doesn't cross the
// horizon for a long stretch still terminates instead of searching
// forever. A nil, nil return means none was found within that bound — the
// circumpolar/never-rises case — not an error.
func searchEvent(target Observable, site *Site, from time.Time, k EventKind, forward bool, maxSpanDays float64) (*Event, error) {
	edge := from
	step := initialSearchStepDays

	for range maxEpisodeSearchSteps {
		if step > maxSpanDays {
			step = maxSpanDays
		}

		var start, end time.Time

		if forward {
			start, end = edge, edge.AddDays(step)
		} else {
			start, end = edge.AddDays(-step), edge
		}

		events, err := visibilityEvents(target, site, start, end)
		if err != nil {
			return nil, fmt.Errorf("plan: search event: %w", err)
		}

		if forward {
			for i := range events {
				if events[i].Kind == k {
					return &events[i], nil
				}
			}

			edge = end
		} else {
			for i := range slices.Backward(events) {
				if events[i].Kind == k {
					return &events[i], nil
				}
			}

			edge = start
		}

		if step >= maxSpanDays {
			break
		}

		step *= 2
	}

	//nolint:nilnil // documented above: nil, nil means "not found within
	// maxSpanDays" (circumpolar/never-rises), not an error -- Episode and
	// its own doc comment already treat this as the expected, non-failure
	// outcome for those targets.
	return nil, nil
}

// Episode returns the single continuous up-episode — rise, then the set
// that ends it — framing [from, to]: unlike a plain windowed event search,
// it reaches outside the interval as needed so rise is always before set
// and the pair always describes one real, continuous period the target
// was above the horizon, never two unrelated episodes stitched together.
//
// If target is already above the horizon at from, rise is searched for
// BEFORE from (the episode is already in progress); otherwise the next
// rise at or after from is used, which may itself fall after to. set is
// then the first set at or after that rise, which may likewise fall after
// to — this is the concrete fix for the naive "first set after today's
// rise" mistake near a long (e.g. ~40h, near-full-Moon) up-episode: that
// set is still THIS episode's, not an unrelated later one, however far
// past to it lands. The search itself starts small (see searchEvent) and
// only grows as needed, but never gives up before covering at least
// [from, to]'s own span (widened past episodeSearchWindow's default when
// to is further out than that) — so a target that never sets
// (circumpolar) or never rises within that bound returns rise == set ==
// nil with no error, not a failure.
//
// See DayEvents for the different question ("what happened on this
// calendar day") this is deliberately not answering.
func Episode(from, to time.Time, target Observable, site *Site) (rise, set *Event, err error) {
	up, err := isAboveHorizon(target, site, from)
	if err != nil {
		return nil, nil, err
	}

	maxSpanDays := episodeSearchWindow.Hours() / 24
	if span := to.Sub(from); span > episodeSearchWindow {
		maxSpanDays = span.Hours() / 24
	}

	rise, err = searchEvent(target, site, from, EventRise, !up, maxSpanDays)
	if err != nil {
		return nil, nil, err
	}

	if rise == nil {
		return nil, nil, nil
	}

	set, err = searchEvent(target, site, rise.Time, EventSet, true, maxSpanDays)
	if err != nil {
		return nil, nil, err
	}

	return rise, set, nil
}
