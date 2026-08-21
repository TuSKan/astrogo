package plan_test

import (
	"math/rand/v2"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/plan"
	astrotime "github.com/TuSKan/astrogo/time"
)

// base is an arbitrary fixed epoch; the algebra does not care which.
func base() astrotime.Time {
	return astrotime.FromGo(gotime.Date(2026, 8, 21, 0, 0, 0, 0, gotime.UTC))
}

// win builds a window from two hour offsets.
func win(fromHours, toHours float64) plan.Window {
	b := base()

	return plan.Window{
		Start: b.Add(astrotime.Duration(fromHours * float64(astrotime.Hour))),
		End:   b.Add(astrotime.Duration(toHours * float64(astrotime.Hour))),
	}
}

// randomSet builds a set of windows with a deterministic generator, so a
// failure is reproducible from the seed printed with it.
func randomSet(r *rand.Rand, n int) []plan.Window {
	// Two days, so a generated set spans several windows with plenty of
	// overlap between them.
	const span = 48.0

	out := make([]plan.Window, 0, n)

	for range n {
		start := r.Float64() * span
		// Lengths include zero, because a zero-duration window is exactly the
		// degenerate case the touching-counts-as-overlapping convention makes
		// reachable, and callers do produce them (a rise and set solved to the
		// same instant).
		length := r.Float64() * span / 4

		out = append(out, win(start, start+length))
	}

	return out
}

// total is TotalDuration in hours, for readable failure messages.
func totalHours(ws []plan.Window) float64 {
	return float64(plan.TotalDuration(ws)) / float64(astrotime.Hour)
}

// Union must produce a normalized set: sorted, pairwise disjoint, covering
// exactly what it was given and nothing more.
func TestUnionNormalizes(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2))

	for trial := range 500 {
		in := randomSet(r, 1+r.IntN(8))
		got := plan.Union(in)

		for i := 1; i < len(got); i++ {
			if got[i].Start.Before(got[i-1].Start) {
				t.Fatalf("trial %d: Union returned an unsorted set at %d", trial, i)
			}

			// Disjoint and not merely touching: Union's own convention
			// collapses touching windows, so a boundary shared between two
			// output windows means the merge did not run to completion.
			if !got[i].Start.After(got[i-1].End) {
				t.Fatalf("trial %d: Union left windows %d and %d overlapping or touching: %v..%v then %v..%v",
					trial, i-1, i,
					got[i-1].Start.ToGo(), got[i-1].End.ToGo(),
					got[i].Start.ToGo(), got[i].End.ToGo())
			}
		}

		// Idempotent: normalizing an already normalized set changes nothing.
		if again := plan.Union(got); !sameSet(again, got) {
			t.Fatalf("trial %d: Union is not idempotent", trial)
		}

		// Every input instant must survive, and no instant may be invented.
		for _, w := range in {
			if !coveredBy(w, got) {
				t.Fatalf("trial %d: Union dropped coverage of %v..%v",
					trial, w.Start.ToGo(), w.End.ToGo())
			}
		}
	}
}

// Intersect must be commutative, must never report time neither side had, and
// must not report an overlap where the two sets share no duration.
func TestIntersectLaws(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(3, 4))

	for trial := range 500 {
		a := randomSet(r, 1+r.IntN(6))
		b := randomSet(r, 1+r.IntN(6))

		ab := plan.Intersect(a, b)
		ba := plan.Intersect(b, a)

		if !sameSet(plan.Union(ab), plan.Union(ba)) {
			t.Fatalf("trial %d: Intersect is not commutative: %v vs %v", trial, ab, ba)
		}

		// The intersection is never larger than either side.
		if ta, tab := totalHours(a), totalHours(ab); tab > ta+1e-9 {
			t.Fatalf("trial %d: the intersection is %g hours against %g in a", trial, tab, ta)
		}

		if tb, tab := totalHours(b), totalHours(ab); tab > tb+1e-9 {
			t.Fatalf("trial %d: the intersection is %g hours against %g in b", trial, tab, tb)
		}

		// Intersecting a set with itself returns the set.
		if !sameSet(plan.Union(plan.Intersect(a, a)), plan.Union(a)) {
			t.Fatalf("trial %d: a set does not intersect itself to itself", trial)
		}

		// Every reported instant must be in both.
		for _, w := range ab {
			if !coveredBy(w, plan.Union(a)) || !coveredBy(w, plan.Union(b)) {
				t.Fatalf("trial %d: Intersect reported %v..%v, which is not in both sets",
					trial, w.Start.ToGo(), w.End.ToGo())
			}
		}
	}
}

// Subtract must remove exactly the shared time and no more: what is left plus
// what was in common has to add back up to what was there to begin with.
func TestSubtractConservesTime(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(5, 6))

	for trial := range 500 {
		a := randomSet(r, 1+r.IntN(6))
		b := randomSet(r, 1+r.IntN(6))

		rest := plan.Subtract(a, b)
		common := plan.Intersect(a, b)

		if got, want := totalHours(rest)+totalHours(common), totalHours(a); absDiff(got, want) > 1e-9 {
			t.Fatalf("trial %d: %g hours left plus %g in common is %g, but a held %g",
				trial, totalHours(rest), totalHours(common), got, want)
		}

		// Nothing left over may lie in b.
		for _, w := range rest {
			if w.Duration() > 0 && overlapsAny(w, plan.Union(b)) {
				t.Fatalf("trial %d: Subtract left %v..%v, which is still covered by b",
					trial, w.Start.ToGo(), w.End.ToGo())
			}
		}

		// Subtracting a set from itself leaves nothing.
		if got := totalHours(plan.Subtract(a, a)); got > 1e-9 {
			t.Fatalf("trial %d: subtracting a set from itself left %g hours", trial, got)
		}

		// Subtracting nothing changes nothing.
		if !sameSet(plan.Union(plan.Subtract(a, nil)), plan.Union(a)) {
			t.Fatalf("trial %d: subtracting an empty set changed the input", trial)
		}
	}
}

// The degenerate cases, stated explicitly rather than left to the generator.
func TestWindowEdgeCases(t *testing.T) {
	t.Parallel()

	// Touching windows coalesce, which is the documented convention.
	if got := plan.Union([]plan.Window{win(0, 5), win(5, 10)}); len(got) != 1 ||
		!got[0].Start.Equal(win(0, 10).Start) || !got[0].End.Equal(win(0, 10).End) {
		t.Errorf("two touching windows unioned to %v, want the single window 0..10", got)
	}

	// An empty set is empty, not a single zero window.
	if got := plan.Union(nil); got != nil {
		t.Errorf("Union(nil) = %v, want nil", got)
	}

	if got := plan.Intersect(nil, nil); len(got) != 0 {
		t.Errorf("Intersect(nil, nil) = %v, want empty", got)
	}

	if got := plan.Subtract(nil, []plan.Window{win(0, 5)}); len(got) != 0 {
		t.Errorf("subtracting from nothing gave %v, want empty", got)
	}

	// A window fully covering another removes it entirely.
	if got := plan.Subtract([]plan.Window{win(2, 4)}, []plan.Window{win(0, 10)}); len(got) != 0 {
		t.Errorf("subtracting a covering window left %v, want nothing", got)
	}

	// A window strictly inside another splits it in two.
	got := plan.Subtract([]plan.Window{win(0, 10)}, []plan.Window{win(4, 6)})
	if len(got) != 2 {
		t.Fatalf("subtracting an interior window gave %d pieces, want 2", len(got))
	}

	if d := totalHours(got); absDiff(d, 8) > 1e-9 {
		t.Errorf("the two remaining pieces total %g hours, want 8", d)
	}

	// TotalDuration must not double count overlapping input.
	if d := totalHours([]plan.Window{win(0, 10), win(0, 10), win(5, 15)}); absDiff(d, 15) > 1e-9 {
		t.Errorf("overlapping windows totalled %g hours, want 15", d)
	}
}

// A zero-duration window is a real result — a target that rises and sets at
// the same instant — so the algebra has to absorb it rather than let it
// fragment a neighbouring window or invent an overlap.
func TestZeroDurationWindowsAreAbsorbed(t *testing.T) {
	t.Parallel()

	// Subtracting an instant from the middle of a window must not split it:
	// removing zero time cannot leave two pieces where there was one.
	if got := plan.Subtract([]plan.Window{win(0, 10)}, []plan.Window{win(5, 5)}); len(got) != 1 {
		t.Errorf("subtracting a zero-duration window split 0..10 into %d pieces, want 1", len(got))
	}

	// Two window sets that merely touch share no observable time, so
	// intersecting them must report none.
	if got := plan.Intersect([]plan.Window{win(0, 10)}, []plan.Window{win(10, 20)}); len(got) != 0 {
		t.Errorf("intersecting sets that only touch at an instant gave %v, want empty", got)
	}

	// A zero-duration window contributes nothing to a union's coverage.
	if d := totalHours(plan.Union([]plan.Window{win(0, 10), win(3, 3)})); absDiff(d, 10) > 1e-9 {
		t.Errorf("a zero-duration window changed the total to %g hours, want 10", d)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func absDiff(a, b float64) float64 {
	if a > b {
		return a - b
	}

	return b - a
}

// sameSet compares two normalized window sets.
func sameSet(a, b []plan.Window) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !a[i].Start.Equal(b[i].Start) || !a[i].End.Equal(b[i].End) {
			return false
		}
	}

	return true
}

// coveredBy reports whether every instant of w lies inside some window of set.
func coveredBy(w plan.Window, set []plan.Window) bool {
	if w.Duration() <= 0 {
		// A zero-duration window covers no instant, so nothing has to hold it.
		return true
	}

	cursor := w.Start

	for _, s := range set {
		if s.End.Before(cursor) || s.Start.After(cursor) {
			continue
		}

		if s.End.After(cursor) {
			cursor = s.End
		}
	}

	return !cursor.Before(w.End)
}

// overlapsAny reports whether w shares more than an instant with any of set.
func overlapsAny(w plan.Window, set []plan.Window) bool {
	for _, s := range set {
		if iw, ok := w.Intersect(s); ok && iw.Duration() > 0 {
			return true
		}
	}

	return false
}
