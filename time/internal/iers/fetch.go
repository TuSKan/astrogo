package iers

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	fetchMu       sync.Mutex
	lastAttempt   time.Time         // wall-clock of last fetch attempt (success or failure)
	errLastFetch  error             // non-nil if the most recent attempt failed
	retryCooldown = 5 * time.Minute // minimum interval between fetch attempts
)

// EnsureLoaded makes a best-effort, at-most-one-attempt-per-cooldown-window
// attempt to populate the global EOP model before a lookup for mjd —
// mirroring the openngc.New()/jpl.NewProvider lazy-load contract used
// elsewhere in this codebase. It never logs; callers (time.lookupEOP)
// decide whether to warn-and-degrade or propagate the returned error.
//
// Skipped entirely once a caller has called RegisterModel directly (see
// modelIsExplicit): an explicit choice — including RegisterModel(ZeroModel{})
// for deterministic zero EOP — is authoritative, not something this lazy
// loader gets to silently override the moment an uncovered lookup happens
// to find a finals2000A file sitting in the cache directory.
//
// Order of attempts otherwise:
//  1. Fast path (no lock): the current model already covers mjd.
//  2. Under fetchMu (re-checked immediately after acquiring it):
//     [Loader.Cached] — whatever EOP data is already on disk, with no
//     network access and no consent check. The parsed table is registered
//     even if it does not cover mjd, since it is still the best available
//     data, and the attempt falls through to step 3 if it does not help.
//  3. If still uncovered and the retry cooldown has elapsed:
//     [Loader.Fetch], which is consent-gated.
//
// Both steps go through the registered [Loader] rather than reaching for
// a cache directory or an HTTP client directly. That is what lets this
// package, and astrogo/time above it, link neither — see [Loader]. With
// no loader registered the result is [ErrNoLoader], and the caller
// degrades to zero EOP exactly as it does when consent is absent.
//
// Safe for concurrent use: a mutex serialises attempts, and the coverage
// check is repeated inside the lock so a successful concurrent load is
// respected immediately.
func EnsureLoaded(mjd float64) error {
	if modelIsExplicit() {
		return nil
	}

	if covered(mjd) {
		return nil
	}

	fetchMu.Lock()
	defer fetchMu.Unlock()

	// Re-check after acquiring the lock — another goroutine may have
	// loaded successfully (or explicitly registered a model) while we
	// were waiting.
	if modelIsExplicit() || covered(mjd) {
		return nil
	}

	l := GetLoader()
	if l == nil {
		return ErrNoLoader
	}

	ctx := context.Background()

	if data, err := l.Cached(ctx); err == nil {
		if lastAttempt.IsZero() {
			lastAttempt = data.ModTime
		}

		if _, perr := parseAndRegister(data.Raw, SourceCache); perr == nil && covered(mjd) {
			return nil
		}
	}

	// Throttle retries so transient errors don't cause a request storm.
	if !lastAttempt.IsZero() && time.Since(lastAttempt) < retryCooldown {
		return errLastFetch // may be nil (successful) or the prior error
	}

	lastAttempt = time.Now()
	errLastFetch = fetch(ctx, l)

	return errLastFetch
}

// SetRetryCooldown sets the minimum interval EnsureLoaded waits between
// fetch attempts after a failure (0 disables throttling). The default is
// 5 minutes.
func SetRetryCooldown(d time.Duration) {
	fetchMu.Lock()
	defer fetchMu.Unlock()

	retryCooldown = d
}

// covered reports whether the current global model covers the given MJD.
func covered(mjd float64) bool {
	model := GetModel()
	if table, ok := model.(*Table); ok {
		_, err := table.EOP(mjd)
		return err == nil
	}

	return false
}

// parseAndRegister parses raw finals2000A bytes and, on success, registers
// the resulting Table as the global model — the shared core of both a
// network fetch and a raw on-disk cache read. Uses registerModelInternal,
// not RegisterModel: this is the lazy loader's own opportunistic
// registration, which must never override a caller's explicit choice (see
// RegisterModel's doc comment). source is SourceCache or SourceNetwork,
// recorded purely for EOPSource.
func parseAndRegister(data []byte, source string) (*Table, error) {
	table, err := ParseFinals2000A(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	registerModelInternal(table, source)

	return table, nil
}

// fetch is the shared core EnsureLoaded serializes on via fetchMu; it
// holds no lock itself.
func fetch(ctx context.Context, l Loader) error {
	data, err := l.Fetch(ctx)
	if err != nil {
		return fmt.Errorf("iers: fetch EOP data: %w", err)
	}

	table, err := parseAndRegister(data.Raw, SourceNetwork)
	if err != nil {
		return fmt.Errorf("iers: parse EOP data: %w", err)
	}

	lo, hi := table.Coverage()

	log.Printf("astrogo/iers: loaded EOP data: MJD %.0f–%.0f (%d records)", lo, hi, len(table.records))

	return nil
}
