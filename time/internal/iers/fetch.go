package iers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	gofs "github.com/ungerik/go-fs"

	"github.com/TuSKan/astrogo/remote"
)

// ErrEOPHTTPStatus indicates an unexpected HTTP status from the IERS EOP download.
var ErrEOPHTTPStatus = errors.New("iers: EOP download returned unexpected status")

//nolint:gochecknoglobals // fetch rate-limiter state — guarded by sync.Mutex
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
// Order of attempts:
//  1. Fast path (no lock): the current model already covers mjd.
//  2. Under fetchMu (re-checked immediately after acquiring it): read and
//     parse whatever finals2000A file already exists on disk, with no
//     network access and no consent check — the same thing this package's
//     former LoadFS did, just from the standard cache path instead of an
//     arbitrary io/fs.FS. This step is necessary because remote.GetFile's
//     own cache-hit path requires a signature sidecar that a hand-pre-seeded
//     file never has (see fetch's doc comment); the parsed table is
//     registered even if it doesn't cover mjd — still the best available
//     data — and the attempt falls through to step 3 if it doesn't help.
//  3. If still uncovered and the retry cooldown has elapsed: the existing
//     consent-gated fetch (remote.GetFile: HEAD-probe reuse, or a fresh,
//     EnableDownloads-gated download).
//
// Safe for concurrent use: a mutex serialises attempts, and the coverage
// check is repeated inside the lock so a successful concurrent load is
// respected immediately.
func EnsureLoaded(mjd float64) error {
	if covered(mjd) {
		return nil
	}

	fetchMu.Lock()
	defer fetchMu.Unlock()

	// Re-check after acquiring the lock — another goroutine may have
	// loaded successfully while we were waiting.
	if covered(mjd) {
		return nil
	}

	if cacheFile, err := CacheFile(); err == nil {
		if info := cacheFile.Info(); info.Exists {
			if lastAttempt.IsZero() {
				lastAttempt = info.Modified
			}

			if data, rerr := cacheFile.ReadAll(); rerr == nil {
				if _, perr := parseAndRegister(data); perr == nil && covered(mjd) {
					return nil
				}
			}
		}
	}

	// Throttle retries so transient errors don't cause a request storm.
	if !lastAttempt.IsZero() && time.Since(lastAttempt) < retryCooldown {
		return errLastFetch // may be nil (successful) or the prior error
	}

	lastAttempt = time.Now()
	errLastFetch = fetch(context.Background())

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

// CacheFile returns the go-fs File where downloaded EOP data is cached,
// under remote.DataDir()/iers.
func CacheFile() (gofs.File, error) {
	dir, err := remote.CacheDir(remote.IERSFinals2000A)
	if err != nil {
		return "", fmt.Errorf("iers: %w", err)
	}

	return dir.Join("finals2000A.data"), nil
}

// parseAndRegister parses raw finals2000A bytes and, on success, registers
// the resulting Table as the global model — the shared core of both a
// network fetch and a raw on-disk cache read.
func parseAndRegister(data []byte) (*Table, error) {
	table, err := ParseFinals2000A(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	RegisterModel(table)

	return table, nil
}

// fetch is the shared core EnsureLoaded serializes on via fetchMu; it
// holds no lock itself.
func fetch(ctx context.Context) error {
	// remote.GetFile reuses the cache untouched when a HEAD probe shows the
	// IERS bulletin hasn't changed since we last downloaded it — a content
	// check rather than a wall-clock expiration window, since finals2000A
	// is updated on IERS's own schedule, not ours. WithValidate parses a
	// fresh download before it's cached, so a corrupt response never gets
	// trusted as the new cache.
	f, err := remote.GetFile(ctx, remote.IERSFinals2000A, "", remote.WithCacheName("finals2000A.data"), remote.WithValidate(func(b []byte) error {
		_, err := ParseFinals2000A(bytes.NewReader(b))
		return err
	}))
	if err != nil {
		var httpErr *remote.HTTPError
		if errors.As(err, &httpErr) {
			return fmt.Errorf("%w: %d", ErrEOPHTTPStatus, httpErr.StatusCode)
		}

		return fmt.Errorf("iers: fetch EOP data: %w", err)
	}

	data, err := f.ReadAll()
	if err != nil {
		return fmt.Errorf("iers: read EOP data: %w", err)
	}

	table, err := parseAndRegister(data)
	if err != nil {
		return fmt.Errorf("iers: parse EOP data: %w", err)
	}

	lo, hi := table.Coverage()

	log.Printf("astrogo/iers: loaded EOP data: MJD %.0f–%.0f (%d records)", lo, hi, len(table.records))

	return nil
}
