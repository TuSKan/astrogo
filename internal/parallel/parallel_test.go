package parallel

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestMap_PreservesOrder(t *testing.T) {
	in := []int{5, 4, 3, 2, 1, 0}

	got, err := Map(in, 0, func(_ int, item int) (int, error) {
		return item * item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	want := []int{25, 16, 9, 4, 1, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestMap_IndexMatchesInputPosition(t *testing.T) {
	in := []string{"a", "b", "c", "d"}

	got, err := Map(in, 0, func(i int, item string) (string, error) {
		if item != in[i] {
			t.Errorf("f called with i=%d item=%q, want in[%d]=%q", i, item, i, in[i])
		}

		return item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	for i := range in {
		if got[i] != in[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], in[i])
		}
	}
}

func TestMap_EmptyInput(t *testing.T) {
	got, err := Map([]int(nil), 0, func(_ int, item int) (int, error) {
		t.Error("f should never be called for an empty input")

		return item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

var errMapTest = errors.New("parallel_test: injected failure")

func TestMap_PropagatesFirstError(t *testing.T) {
	in := []int{0, 1, 2, 3, 4}

	_, err := Map(in, 0, func(_ int, item int) (int, error) {
		if item == 2 {
			return 0, errMapTest
		}

		return item, nil
	})
	if !errors.Is(err, errMapTest) {
		t.Fatalf("Map error = %v, want errMapTest", err)
	}
}

// TestMap_SingleItem covers the degenerate n=1 case explicitly -- no
// concurrency to speak of, just confirms the plumbing doesn't assume a
// larger slice anywhere.
func TestMap_SingleItem(t *testing.T) {
	got, err := Map([]int{42}, 0, func(_ int, item int) (int, error) {
		return item + 1, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	if len(got) != 1 || got[0] != 43 {
		t.Errorf("got %v, want [43]", got)
	}
}

// TestMap_ExplicitLimitCapsConcurrency proves an explicit positive limit
// is actually enforced, not just accepted and ignored: with limit=1, no
// two calls to f may observe an in-progress sibling call.
func TestMap_ExplicitLimitCapsConcurrency(t *testing.T) {
	const n = 10

	in := make([]int, n)

	var active atomic.Int32

	_, err := Map(in, 1, func(_ int, item int) (int, error) {
		if got := active.Add(1); got > 1 {
			t.Errorf("active concurrent calls = %d, want <= 1 with limit=1", got)
		}

		defer active.Add(-1)

		return item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
}
