package plan_test

import (
	"math/rand/v2"
	"testing"

	"github.com/TuSKan/astrogo/plan"
)

// TestAnInvertedWindowIsEmpty: a window ending before it starts holds no
// instant. Until #421 Subtract returned it whole from under a window covering
// it, TotalDuration counted it as −1 h, and Union returned it beside a window
// it overlapped, in a result documented as pairwise disjoint.
func TestAnInvertedWindowIsEmpty(t *testing.T) {
	t.Parallel()

	inverted := win(1, 0) // 01:00 to 00:00
	cover := win(-2, 4)
	other := win(-1, 0.5)

	if got := plan.Subtract([]plan.Window{inverted}, []plan.Window{cover}); len(got) != 0 {
		t.Errorf("Subtract(inverted, covering) = %v, want nothing", got)
	}

	if got := plan.TotalDuration([]plan.Window{inverted}); got != 0 {
		t.Errorf("TotalDuration(inverted) = %v, want 0", got)
	}

	if got := plan.Union([]plan.Window{inverted, other}); !sameSet(got, []plan.Window{other}) {
		t.Errorf("Union(inverted, other) = %v, want just the other", got)
	}

	if got := plan.Union([]plan.Window{inverted}); got != nil {
		t.Errorf("Union(inverted) = %v, want nil", got)
	}

	if got := plan.Intersect([]plan.Window{inverted}, []plan.Window{cover}); len(got) != 0 {
		t.Errorf("Intersect(inverted, covering) = %v, want nothing", got)
	}

	for _, pair := range [][2]plan.Window{{inverted, cover}, {cover, inverted}} {
		if pair[0].Overlaps(pair[1]) {
			t.Errorf("%v overlaps %v; an inverted window overlaps nothing", pair[0], pair[1])
		}

		if iw, ok := pair[0].Intersect(pair[1]); ok {
			t.Errorf("%v.Intersect(%v) = %v, true; want false", pair[0], pair[1], iw)
		}
	}

	// Taking away nothing leaves everything.
	if got := plan.Subtract([]plan.Window{cover}, []plan.Window{inverted}); !sameSet(got, []plan.Window{cover}) {
		t.Errorf("Subtract(covering, inverted) = %v, want the covering window whole", got)
	}

	// Duration still says what the window is.
	if got := inverted.Duration(); got >= 0 {
		t.Errorf("inverted.Duration() = %v, want negative", got)
	}
}

// TestInvertedWindowsChangeNothing: mixing inverted windows into random sets
// must leave every set operation's answer as it was without them.
func TestInvertedWindowsChangeNothing(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(421, 7))

	withInverted := func(ws []plan.Window) []plan.Window {
		out := append([]plan.Window(nil), ws...)

		for range 1 + r.IntN(3) {
			start := r.Float64() * 48
			out = append(out, win(start, start-r.Float64()*12-1e-6))
		}

		r.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })

		return out
	}

	for trial := range 500 {
		a, b := randomSet(r, 1+r.IntN(6)), randomSet(r, 1+r.IntN(6))
		ai, bi := withInverted(a), withInverted(b)

		if !sameSet(plan.Union(ai), plan.Union(a)) {
			t.Fatalf("trial %d: Union changed", trial)
		}

		if !sameSet(plan.Intersect(ai, bi), plan.Intersect(a, b)) {
			t.Fatalf("trial %d: Intersect changed", trial)
		}

		if !sameSet(plan.Subtract(ai, bi), plan.Subtract(a, b)) {
			t.Fatalf("trial %d: Subtract changed", trial)
		}

		if got, want := plan.TotalDuration(ai), plan.TotalDuration(a); got != want {
			t.Fatalf("trial %d: TotalDuration %v, want %v", trial, got, want)
		}
	}
}
