package plan

import (
	"slices"

	"github.com/TuSKan/astrogo/time"
)

// Window represents a contiguous time interval.
type Window struct {
	Start time.Time
	End   time.Time
}

// Duration returns the duration of the window as a standard time.Duration.
func (w Window) Duration() time.Duration {
	return w.End.Sub(w.Start)
}

// Overlaps reports whether w and o share any instant, including a shared
// boundary — w.End == o.Start or o.End == w.Start count as overlapping,
// with zero shared duration. This inclusive convention is what lets Union's
// merge step reuse Overlaps directly: two windows that merely touch
// coalesce into one contiguous window instead of being reported as
// separated by an infinitesimal, physically meaningless gap.
func (w Window) Overlaps(o Window) bool {
	return !w.End.Before(o.Start) && !o.End.Before(w.Start)
}

// Intersect returns the sub-window w and o have in common, and whether they
// overlap at all (see Overlaps for what counts). A shared boundary with no
// shared duration still reports ok == true, with the result's Start == End
// — a caller taking .Duration() of the result gets 0, the correct answer,
// with no separate touching-vs-overlapping check of its own required.
func (w Window) Intersect(o Window) (Window, bool) {
	if !w.Overlaps(o) {
		return Window{}, false
	}

	start := w.Start
	if o.Start.After(start) {
		start = o.Start
	}

	end := w.End
	if o.End.Before(end) {
		end = o.End
	}

	return Window{Start: start, End: end}, true
}

// Union merges ws into a normalized set: sorted by Start, with every pair
// of overlapping or touching windows (see Overlaps) collapsed into one. The
// result is pairwise disjoint and sorted ascending — the form Intersect,
// Subtract, and TotalDuration all reduce their own inputs to first, so a
// caller never has to pre-sort or pre-merge a window set by hand before
// passing it to any of them.
func Union(ws []Window) []Window {
	if len(ws) == 0 {
		return nil
	}

	sorted := slices.Clone(ws)
	slices.SortFunc(sorted, func(a, b Window) int {
		switch {
		case a.Start.Before(b.Start):
			return -1
		case a.Start.After(b.Start):
			return 1
		default:
			return 0
		}
	})

	merged := make([]Window, 0, len(sorted))
	cur := sorted[0]

	for _, w := range sorted[1:] {
		if !cur.Overlaps(w) {
			merged = append(merged, cur)
			cur = w

			continue
		}

		if w.End.After(cur.End) {
			cur.End = w.End
		}
	}

	return append(merged, cur)
}

// Intersect returns the windows common to both a and b — e.g. the times a
// target clears the horizon (ObservableWindows) that also fall inside some
// other already-computed set of windows. Both inputs are normalized via
// Union first, so overlapping or unsorted windows within either set are not
// the caller's problem; the result is itself normalized.
//
// Implementation note: since Union leaves both slices sorted and pairwise
// disjoint, a single forward sweep (advance whichever window ends first)
// finds every pairwise intersection in one pass — the standard two-pointer
// interval-list-intersection algorithm.
func Intersect(a, b []Window) []Window {
	ua, ub := Union(a), Union(b)

	var out []Window

	i, j := 0, 0
	for i < len(ua) && j < len(ub) {
		if iw, ok := ua[i].Intersect(ub[j]); ok {
			out = append(out, iw)
		}

		if ua[i].End.Before(ub[j].End) {
			i++
		} else {
			j++
		}
	}

	return out
}

// Subtract returns the portions of a not covered by any window in b — the
// concrete answer to "of the time this target is observable, how much has
// some other body below the horizon": a = ObservableWindows(target, ...),
// b = VisibleIntervals(other, ...). Both inputs are normalized via Union
// first, matching Intersect's convention.
func Subtract(a, b []Window) []Window {
	ua, ub := Union(a), Union(b)

	var out []Window

	for _, wa := range ua {
		remaining := []Window{wa}

		for _, wb := range ub {
			if !wa.Overlaps(wb) {
				continue
			}

			var next []Window
			for _, r := range remaining {
				next = append(next, subtractOne(r, wb)...)
			}

			remaining = next
		}

		out = append(out, remaining...)
	}

	return out
}

// subtractOne removes wb's overlap with w, if any. It returns w unchanged
// (as the sole element) when they don't overlap, nil when wb fully covers
// w, or one or two remaining pieces for a partial overlap (two when wb
// sits strictly inside w, splitting it in half).
func subtractOne(w, wb Window) []Window {
	iw, ok := w.Intersect(wb)
	if !ok {
		return []Window{w}
	}

	var pieces []Window

	if iw.Start.After(w.Start) {
		pieces = append(pieces, Window{Start: w.Start, End: iw.Start})
	}

	if iw.End.Before(w.End) {
		pieces = append(pieces, Window{Start: iw.End, End: w.End})
	}

	return pieces
}

// TotalDuration sums ws's windows' durations, after normalizing via Union
// so overlapping input — e.g. two independently-computed window sets
// concatenated by hand without merging — isn't double-counted.
func TotalDuration(ws []Window) time.Duration {
	var total time.Duration

	for _, w := range Union(ws) {
		total += w.Duration()
	}

	return total
}
