package plan

import (
	"errors"
	"fmt"
	"sync"
)

// ErrIncomplete marks a result that is usable but shorter than it should be.
//
// # Why a result and an error together
//
// [VisibleTonight] queries several catalogues and fetches an ephemeris per
// candidate, and any of those can fail on its own. Failing the whole query
// because one kernel could not be fetched would be worse than useless — a
// night's plan is still a night's plan without one asteroid, and the caller
// asked for what is visible, not for a transaction.
//
// So it keeps skipping, and returns what it found. What it no longer does is
// keep quiet: a skip caused by a failure is collected, and the call returns a
// non-nil error wrapping this sentinel alongside the results.
//
//	objects, err := plan.VisibleTonight(ctx, site, night, 6.0, sources, prov)
//	if errors.Is(err, plan.ErrIncomplete) {
//	    // objects is valid, and something was dropped. err says what.
//	} else if err != nil {
//	    // nothing usable came back
//	}
//
// A caller who ignores the error gets exactly what they got before. A caller
// who checks it can tell a sky with two visible planets from a sky with two
// visible planets and an unreachable JPL — which reading the slice alone
// cannot.
//
// This replaces reporting the skips only through the logger. A log line tells
// a human something happened; it does not let the program decide, and deciding
// is the caller's job. #177 fixed six predicates that answered "no" when they
// meant "could not tell"; this is the same defect one layer up, where the
// answer was a short list.
var ErrIncomplete = errors.New("plan: result is incomplete")

// skips collects the reasons a result came back short.
//
// Concurrency-safe: the gatherers run under parallel.Map, and each records its
// own failures. parallel.Map's own error return cannot carry these — it is an
// errgroup, so it keeps only the first error and cancels the rest, which is
// the opposite of what a per-item skip needs.
type skips struct {
	mu   sync.Mutex
	errs []error
}

// add records one skip. A nil err is ignored, so callers need no guard.
func (s *skips) add(stage, what string, err error) {
	if err == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.errs = append(s.errs, fmt.Errorf("%s %q: %w", stage, what, err))
}

// err returns nil when nothing was skipped, or an error wrapping
// [ErrIncomplete] and every reason collected.
func (s *skips) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.errs) == 0 {
		return nil
	}

	return fmt.Errorf("%w: %w", ErrIncomplete, errors.Join(s.errs...))
}
