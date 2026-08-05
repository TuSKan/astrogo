package plan

import (
	stdtime "time"

	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// t0 is an arbitrary fixed base instant every test below builds windows
// relative to via hour offsets, so test cases read as plain integers
// (h(0, 2) instead of two separately-constructed time.Time values).
var t0 = time.FromJD(2451545.0, time.UTC) //nolint:gochecknoglobals // test fixture, not production state

// h builds a Window [t0+startH, t0+endH] hours from the fixture base.
func h(startH, endH float64) Window {
	return Window{
		Start: t0.Add(stdtime.Duration(startH * float64(stdtime.Hour))),
		End:   t0.Add(stdtime.Duration(endH * float64(stdtime.Hour))),
	}
}

// ── Overlaps ─────────────────────────────────────────────────────────────

func TestWindowOverlaps(t *testing.T) {
	cases := []struct {
		name string
		a, b Window
		want bool
	}{
		{"disjoint, a before b", h(0, 1), h(2, 3), false},
		{"disjoint, b before a", h(2, 3), h(0, 1), false},
		{"touching, a.End == b.Start", h(0, 1), h(1, 2), true},
		{"touching, b.End == a.Start", h(1, 2), h(0, 1), true},
		{"partial overlap", h(0, 2), h(1, 3), true},
		{"full containment", h(0, 4), h(1, 2), true},
		{"identical", h(0, 2), h(0, 2), true},
		{"same start, different end", h(0, 1), h(0, 2), true},
		{"same end, different start", h(0, 2), h(1, 2), true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Overlaps(c.b); got != c.want {
				t.Errorf("%v.Overlaps(%v) = %v, want %v", c.a, c.b, got, c.want)
			}
			// Overlaps must be symmetric.
			if got := c.b.Overlaps(c.a); got != c.want {
				t.Errorf("%v.Overlaps(%v) = %v, want %v (symmetric case)", c.b, c.a, got, c.want)
			}
		})
	}
}

// ── Intersect (method) ──────────────────────────────────────────────────

func TestWindowIntersect(t *testing.T) {
	t.Run("no overlap returns ok=false", func(t *testing.T) {
		_, ok := h(0, 1).Intersect(h(2, 3))
		if ok {
			t.Error("expected ok=false for disjoint windows")
		}
	})

	t.Run("touching returns zero-duration ok=true", func(t *testing.T) {
		iw, ok := h(0, 1).Intersect(h(1, 2))
		if !ok {
			t.Fatal("expected ok=true for touching windows")
		}

		if !iw.Start.Equal(iw.End) {
			t.Errorf("expected zero-duration intersection, got %v to %v", iw.Start, iw.End)
		}

		if d := iw.Duration(); d != 0 {
			t.Errorf("expected 0 duration, got %v", d)
		}
	})

	t.Run("partial overlap", func(t *testing.T) {
		iw, ok := h(0, 2).Intersect(h(1, 3))
		if !ok {
			t.Fatal("expected overlap")
		}

		want := h(1, 2)
		if !iw.Start.Equal(want.Start) || !iw.End.Equal(want.End) {
			t.Errorf("got [%v, %v], want [%v, %v]", iw.Start, iw.End, want.Start, want.End)
		}
	})

	t.Run("full containment yields the smaller window", func(t *testing.T) {
		iw, ok := h(0, 4).Intersect(h(1, 2))
		if !ok {
			t.Fatal("expected overlap")
		}

		want := h(1, 2)
		if !iw.Start.Equal(want.Start) || !iw.End.Equal(want.End) {
			t.Errorf("got [%v, %v], want [%v, %v]", iw.Start, iw.End, want.Start, want.End)
		}
	})

	t.Run("symmetric", func(t *testing.T) {
		a, b := h(0, 2), h(1, 3)
		iw1, ok1 := a.Intersect(b)

		iw2, ok2 := b.Intersect(a)
		if ok1 != ok2 || !iw1.Start.Equal(iw2.Start) || !iw1.End.Equal(iw2.End) {
			t.Errorf("Intersect should be symmetric: %v/%v vs %v/%v", iw1, ok1, iw2, ok2)
		}
	})
}

// ── Union ────────────────────────────────────────────────────────────────

func windowsEqual(a, b []Window) bool {
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

func TestUnion(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		if got := Union(nil); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("single window unchanged", func(t *testing.T) {
		got := Union([]Window{h(0, 1)})
		if !windowsEqual(got, []Window{h(0, 1)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("disjoint windows stay separate, sorted", func(t *testing.T) {
		got := Union([]Window{h(2, 3), h(0, 1)})
		if !windowsEqual(got, []Window{h(0, 1), h(2, 3)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("overlapping windows merge", func(t *testing.T) {
		got := Union([]Window{h(0, 2), h(1, 3)})
		if !windowsEqual(got, []Window{h(0, 3)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("touching windows merge", func(t *testing.T) {
		got := Union([]Window{h(0, 1), h(1, 2)})
		if !windowsEqual(got, []Window{h(0, 2)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("fully contained window absorbed", func(t *testing.T) {
		got := Union([]Window{h(0, 4), h(1, 2)})
		if !windowsEqual(got, []Window{h(0, 4)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("chain of overlaps merges into one", func(t *testing.T) {
		got := Union([]Window{h(4, 5), h(0, 1), h(1, 2), h(2, 3), h(3, 4)})
		if !windowsEqual(got, []Window{h(0, 5)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("mixed: some merge, some stay separate", func(t *testing.T) {
		got := Union([]Window{h(0, 1), h(0.5, 2), h(5, 6)})
		if !windowsEqual(got, []Window{h(0, 2), h(5, 6)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("result is idempotent", func(t *testing.T) {
		ws := []Window{h(2, 3), h(0, 1), h(0.5, 2.5)}
		once := Union(ws)

		twice := Union(once)
		if !windowsEqual(once, twice) {
			t.Errorf("Union not idempotent: %v vs %v", once, twice)
		}
	})

	t.Run("does not mutate its input", func(t *testing.T) {
		ws := []Window{h(2, 3), h(0, 1)}
		orig := append([]Window(nil), ws...)

		_ = Union(ws)
		if !windowsEqual(ws, orig) {
			t.Errorf("Union mutated its input slice: %v vs original %v", ws, orig)
		}
	})
}

// ── Intersect (free function) ───────────────────────────────────────────

func TestIntersectSets(t *testing.T) {
	t.Run("disjoint sets", func(t *testing.T) {
		got := Intersect([]Window{h(0, 1)}, []Window{h(2, 3)})
		if len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})

	t.Run("overlapping sets", func(t *testing.T) {
		got := Intersect([]Window{h(0, 2)}, []Window{h(1, 3)})
		if !windowsEqual(got, []Window{h(1, 2)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("identical sets", func(t *testing.T) {
		ws := []Window{h(0, 1), h(2, 3)}

		got := Intersect(ws, ws)
		if !windowsEqual(got, ws) {
			t.Errorf("got %v, want %v", got, ws)
		}
	})

	t.Run("multiple pairwise intersections", func(t *testing.T) {
		// a: [0,2] [4,6]   b: [1,5]
		// -> [1,2] [4,5]
		got := Intersect([]Window{h(0, 2), h(4, 6)}, []Window{h(1, 5)})
		if !windowsEqual(got, []Window{h(1, 2), h(4, 5)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("empty input yields empty output", func(t *testing.T) {
		if got := Intersect(nil, []Window{h(0, 1)}); len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})
}

// ── Subtract ─────────────────────────────────────────────────────────────

func TestSubtract(t *testing.T) {
	t.Run("b empty leaves a unchanged", func(t *testing.T) {
		got := Subtract([]Window{h(0, 2)}, nil)
		if !windowsEqual(got, []Window{h(0, 2)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("a empty stays empty", func(t *testing.T) {
		if got := Subtract(nil, []Window{h(0, 2)}); len(got) != 0 {
			t.Errorf("got %v", got)
		}
	})

	t.Run("no overlap leaves a unchanged", func(t *testing.T) {
		got := Subtract([]Window{h(0, 1)}, []Window{h(2, 3)})
		if !windowsEqual(got, []Window{h(0, 1)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("b fully covers a", func(t *testing.T) {
		got := Subtract([]Window{h(1, 2)}, []Window{h(0, 3)})
		if len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})

	t.Run("b strictly inside a splits it in two", func(t *testing.T) {
		got := Subtract([]Window{h(0, 4)}, []Window{h(1, 2)})
		if !windowsEqual(got, []Window{h(0, 1), h(2, 4)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("b overlaps a's start", func(t *testing.T) {
		got := Subtract([]Window{h(1, 3)}, []Window{h(0, 2)})
		if !windowsEqual(got, []Window{h(2, 3)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("b overlaps a's end", func(t *testing.T) {
		got := Subtract([]Window{h(0, 2)}, []Window{h(1, 3)})
		if !windowsEqual(got, []Window{h(0, 1)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("touching does not subtract anything", func(t *testing.T) {
		got := Subtract([]Window{h(0, 1)}, []Window{h(1, 2)})
		if !windowsEqual(got, []Window{h(0, 1)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("multiple b windows carve multiple gaps", func(t *testing.T) {
		// a: [0,10], b: [1,2] and [4,5] -> [0,1] [2,4] [5,10]
		got := Subtract([]Window{h(0, 10)}, []Window{h(1, 2), h(4, 5)})
		if !windowsEqual(got, []Window{h(0, 1), h(2, 4), h(5, 10)}) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("normalizes overlapping input in a and b first", func(t *testing.T) {
		// a given as two overlapping pieces that union to [0,10];
		// b given as two overlapping pieces that union to [2,6].
		a := []Window{h(0, 6), h(4, 10)}
		b := []Window{h(2, 5), h(3, 6)}

		got := Subtract(a, b)
		if !windowsEqual(got, []Window{h(0, 2), h(6, 10)}) {
			t.Errorf("got %v", got)
		}
	})
}

// ── TotalDuration ───────────────────────────────────────────────────────

func TestTotalDuration(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := TotalDuration(nil); got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("single window", func(t *testing.T) {
		got := TotalDuration([]Window{h(0, 2)})
		if got != 2*stdtime.Hour {
			t.Errorf("got %v, want 2h", got)
		}
	})

	t.Run("disjoint windows sum", func(t *testing.T) {
		got := TotalDuration([]Window{h(0, 1), h(2, 4)})
		if got != 3*stdtime.Hour {
			t.Errorf("got %v, want 3h", got)
		}
	})

	t.Run("overlapping windows are not double-counted", func(t *testing.T) {
		got := TotalDuration([]Window{h(0, 2), h(1, 3)})
		if got != 3*stdtime.Hour {
			t.Errorf("got %v, want 3h (union, not 4h)", got)
		}
	})
}

// ── Cross-function invariants ──────────────────────────────────────────
//
// For any a, b: Union(a)'s total duration must equal the sum of what
// Subtract(a, b) removes (Intersect(a, b)) plus what it leaves behind
// (Subtract(a, b) itself) -- the algebraic identity these four functions
// must jointly satisfy regardless of how a and b happen to overlap.

func TestSubtractIntersectPartitionUnion(t *testing.T) {
	cases := []struct {
		name string
		a, b []Window
	}{
		{"disjoint", []Window{h(0, 1)}, []Window{h(2, 3)}},
		{"touching", []Window{h(0, 1)}, []Window{h(1, 2)}},
		{"partial overlap", []Window{h(0, 2)}, []Window{h(1, 3)}},
		{"b inside a", []Window{h(0, 4)}, []Window{h(1, 2)}},
		{"a inside b", []Window{h(1, 2)}, []Window{h(0, 4)}},
		{"multi-window both sides", []Window{h(0, 2), h(5, 8)}, []Window{h(1, 6), h(7, 9)}},
		{"unsorted, overlapping inputs", []Window{h(3, 5), h(0, 2), h(1, 4)}, []Window{h(4, 6), h(0.5, 1)}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			total := TotalDuration(Union(c.a))
			removed := TotalDuration(Intersect(c.a, c.b))
			remaining := TotalDuration(Subtract(c.a, c.b))

			testutil.AssertNear(t, "removed+remaining vs total",
				float64(removed+remaining), float64(total), float64(stdtime.Microsecond))
		})
	}
}
