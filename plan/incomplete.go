package plan

import (
	"errors"
	"fmt"
	"sync"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// ErrIncomplete marks a result that is usable but shorter than it should be.
//
// [VisibleTonight] queries several catalogs and fetches an ephemeris per
// candidate, and any of those can fail on its own. It skips a candidate it
// could not evaluate and returns what it found, with a non-nil error wrapping
// this sentinel that names each skip and why:
//
//	objects, err := plan.VisibleTonight(ctx, site, night, 6.0, sources, prov)
//	if errors.Is(err, plan.ErrIncomplete) {
//	    // objects is valid, and something was dropped. err says what.
//	} else if err != nil {
//	    // nothing usable came back
//	}
//
// The results returned with it are valid. The error is what tells a sky with
// two visible planets from a sky with two visible planets and an unreachable
// JPL, which the slice alone cannot.
//
// It is [resolve.ErrIncomplete], the value catalog.Resolver.Search returns
// with partial matches too, so one errors.Is check covers a short result from
// either package.
var ErrIncomplete = resolve.ErrIncomplete

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
