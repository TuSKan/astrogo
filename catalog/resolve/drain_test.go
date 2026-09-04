package resolve_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

var errBroke = errors.New("upstream broke")

// seq builds an iterator yielding each value, then err if it is non-nil.
func seq(values []string, err error) resolve.SeqIterator[string] {
	return func(yield func(string, error) bool) {
		for _, v := range values {
			if !yield(v, nil) {
				return
			}
		}

		if err != nil {
			yield("", err)
		}
	}
}

// TestDrainReturnsTheError is the property Drain exists for.
//
// Four providers previously carried their own copy of this loop and every copy
// discarded the error — three with a bare `if err == nil { append }` and one
// with a log.Printf first. The caller then saw an empty slice and reported
// "not found", which is how a CDS outage became a confident "no such object".
func TestDrainReturnsTheError(t *testing.T) {
	t.Parallel()

	got, err := resolve.Drain(seq(nil, errBroke), 10)
	if !errors.Is(err, errBroke) {
		t.Errorf("err = %v, want errBroke", err)
	}

	if got != nil {
		t.Errorf("got %v, want nil alongside an error", got)
	}
}

// TestDrainDiscardsPartialResultsWithTheError pins the deliberate choice not
// to return rows taken before a failure.
//
// A short answer that looks complete is exactly how the original defect
// produced wrong results rather than visible ones: a caller checking only
// len() would treat a truncated page as the whole catalog. Returning nil
// forces the error to be handled.
func TestDrainDiscardsPartialResultsWithTheError(t *testing.T) {
	t.Parallel()

	got, err := resolve.Drain(seq([]string{"a", "b"}, errBroke), 10)
	if !errors.Is(err, errBroke) {
		t.Fatalf("err = %v, want errBroke", err)
	}

	if got != nil {
		t.Errorf("got %v, want nil — a partial page must not be returned as if complete", got)
	}
}

func TestDrainCollectsAndRespectsLimit(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		items []string
		limit int
		want  int
	}{
		{"under the limit", []string{"a", "b"}, 10, 2},
		{"exactly the limit", []string{"a", "b", "c"}, 3, 3},
		{"stops at the limit", []string{"a", "b", "c", "d"}, 2, 2},
		{"zero limit collects everything", []string{"a", "b", "c"}, 0, 3},
		{"negative limit collects everything", []string{"a", "b"}, -1, 2},
		{"empty is not an error", nil, 10, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolve.Drain(seq(c.items, nil), c.limit)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != c.want {
				t.Errorf("collected %d items (%v), want %d", len(got), got, c.want)
			}
		})
	}
}

// TestDrainEmptyIsNotAnError separates "asked and matched nothing" from
// "could not ask" at the lowest level, which is the distinction the whole
// Provider interface is built on.
func TestDrainEmptyIsNotAnError(t *testing.T) {
	t.Parallel()

	got, err := resolve.Drain(seq(nil, nil), 10)
	if err != nil {
		t.Errorf("err = %v, want nil — an empty result is an answer", err)
	}

	if len(got) != 0 {
		t.Errorf("got %v, want empty", got)
	}
}
