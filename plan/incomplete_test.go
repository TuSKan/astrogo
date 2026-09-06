package plan

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

var (
	errKernelUnreachable = errors.New("incomplete_test: kernel host unreachable")
	errBadRow            = errors.New("incomplete_test: malformed catalog row")
)

// TestSkipsNilWhenNothingWasSkipped is the case that matters most, and the
// easiest to get wrong: a complete result must return a nil error.
//
// If err() ever returned a non-nil ErrIncomplete for a clean run, every caller
// following the documented `errors.Is(err, ErrIncomplete)` pattern would treat
// every night as partial, and the signal would mean nothing.
func TestSkipsNilWhenNothingWasSkipped(t *testing.T) {
	t.Parallel()

	var s skips

	if err := s.err(); err != nil {
		t.Fatalf("a fresh collector reported %v, want nil", err)
	}

	// A nil reason is not a skip. Callers pass the error unconditionally
	// (dropped.add(stage, what, err) with no `if err != nil` guard), so this
	// is the common path, not an edge case.
	s.add("candidate", "Vega", nil)

	if err := s.err(); err != nil {
		t.Fatalf("a nil reason was recorded as a skip: %v", err)
	}
}

// TestSkipsWrapsErrIncompleteAndKeepsEveryReason checks both halves of the
// contract: the sentinel a caller branches on, and the individual causes
// underneath it, which is what makes the error worth reading rather than
// merely worth testing.
func TestSkipsWrapsErrIncompleteAndKeepsEveryReason(t *testing.T) {
	t.Parallel()

	var s skips

	s.add("small body", "433 Eros", errKernelUnreachable)
	s.add("bright source", "*sbdb.Provider", errBadRow)

	err := s.err()
	if err == nil {
		t.Fatal("two skips produced no error")
	}

	if !errors.Is(err, ErrIncomplete) {
		t.Errorf("err does not wrap ErrIncomplete: %v", err)
	}

	// errors.Join preserves every cause, not just the first — the point of
	// collecting rather than keeping errgroup's first-error-wins.
	if !errors.Is(err, errKernelUnreachable) {
		t.Errorf("the first skip's cause was lost: %v", err)
	}

	if !errors.Is(err, errBadRow) {
		t.Errorf("the second skip's cause was lost: %v", err)
	}

	// The name is the operative part for a human: "something was dropped" is
	// not actionable, "433 Eros was dropped" is.
	msg := err.Error()
	for _, want := range []string{"433 Eros", "*sbdb.Provider", "small body", "bright source"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error text does not name %q:\n%s", want, msg)
		}
	}
}

// TestSkipsIsConcurrencySafe runs add against the collector the way the
// gatherers do — from inside parallel.Map callbacks, several at once.
//
// Worth an explicit test rather than trusting the mutex by inspection: this is
// the one piece of shared mutable state VisibleTonight's concurrent stages
// write to, and -race only catches what actually races during the run.
func TestSkipsIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	const writers = 64

	var (
		s  skips
		wg sync.WaitGroup
	)

	wg.Add(writers)

	for range writers {
		go func() {
			defer wg.Done()

			s.add("candidate", "concurrent", errKernelUnreachable)
		}()
	}

	wg.Wait()

	err := s.err()
	if err == nil {
		t.Fatal("64 concurrent skips produced no error")
	}

	if got := strings.Count(err.Error(), "concurrent"); got != writers {
		t.Errorf("kept %d of %d skips — one was lost to a race", got, writers)
	}
}

// photometryFailure is a target that HAS a magnitude but cannot compute it —
// an asteroid whose SPK kernel went away between the window search and the
// photometry call.
//
// The compile-time assertion below is the point of the declaration: a fixture
// that misses the interface by one method would make the test vacuous rather
// than failing, since rawMagnitude's type assertion would simply not match and
// the object would fall through to "no published magnitude" — which is the
// very distinction under test.
var _ MagnitudeComputer = photometryFailure{}

type photometryFailure struct{ unreachableTarget }

func (photometryFailure) ApparentMagnitude(_ time.Time) (float64, error) {
	return 0, errKernelUnreachable
}

func (photometryFailure) ApparentMagnitudeCtx(_ time.Time, _ *coord.Context) (float64, error) {
	return 0, errKernelUnreachable
}

// noPublishedMagnitude implements neither magnitude interface — a real,
// ordinary catalog row with no photometry.
type noPublishedMagnitude struct{ unreachableTarget }

// TestRawMagnitudeSeparatesFailureFromAbsence covers the third instance of
// this defect inside VisibleTonight, and the least visible one.
//
// rawMagnitude fed a single bool into a magLimit comparison, so an object whose
// photometry FAILED was dropped by exactly the same branch as one that is too
// faint to make the cut. That is the #177 shape at its most deniable: the
// result is not merely short, it is short in the one field the caller asked to
// filter on.
func TestRawMagnitudeSeparatesFailureFromAbsence(t *testing.T) {
	t.Parallel()

	epoch := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.LocationUTC)

	// Photometry that failed: not a magnitude, and not an absence either.
	_, ok, err := rawMagnitude(photometryFailure{}, epoch)
	if ok {
		t.Error("a failed photometry call reported a usable magnitude")
	}

	if !errors.Is(err, errKernelUnreachable) {
		t.Errorf("rawMagnitude returned err = %v, want the underlying failure.\n"+
			"  Reported as a bare false, this object is dropped exactly like one too faint to qualify.", err)
	}

	// No published magnitude: a legitimate negative, and must stay one. If
	// this returned an error, every catalog row without photometry would
	// make the night read as incomplete.
	_, ok, err = rawMagnitude(noPublishedMagnitude{}, epoch)
	if ok {
		t.Error("an object with no published magnitude reported one")
	}

	if err != nil {
		t.Errorf("an absent magnitude was reported as a failure: %v", err)
	}
}
