package iers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"
)

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
// Skipped entirely once a caller has called RegisterModel directly (see
// modelIsExplicit): an explicit choice — including RegisterModel(ZeroModel{})
// for deterministic zero EOP — is authoritative, not something this lazy
// loader gets to silently override the moment an uncovered lookup happens
// to find a finals2000A file sitting in the cache directory.
//
// Order of attempts otherwise:
//  1. Fast path (no lock): the current model already covers mjd.
//  2. Under fetchMu (re-checked immediately after acquiring it): read and
//     parse whatever finals2000A file already exists in the cache, with no
//     network access and no consent check — the same thing this package's
//     former LoadFS did, just from the standard cache location. This step
//     is necessary because remote.GetFile's own cache-hit path requires a
//     recorded Signature/ETag a hand-pre-seeded file never has; the parsed
//     table is registered even if it doesn't cover mjd — still the best
//     available data — and the attempt falls through to step 3 if it
//     doesn't help.
//  3. If still uncovered and the retry cooldown has elapsed: the existing
//     consent-gated fetch (remote.GetFile). NOTE: IERSFinals2000A is a
//     plain-HTTP KindFile endpoint, and remote/file has no HTTP backend
//     registered yet (see remote/file's own doc comment) — this step
//     currently always fails with a clear "no driver for scheme https"
//     error until that lands. Step 2 (reading a pre-seeded cache file)
//     still works fully offline in the meantime.
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

	ctx := context.Background()

	if bucket, key, err := CacheFile(ctx); err == nil {
		if attrs, aerr := bucket.Attributes(ctx, key); aerr == nil {
			if lastAttempt.IsZero() {
				lastAttempt = attrs.ModTime
			}

			if data, rerr := bucket.ReadAll(ctx, key); rerr == nil {
				if _, perr := parseAndRegister(data, SourceCache); perr == nil && covered(mjd) {
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
	errLastFetch = fetch(ctx)

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

// CacheFile returns the Bucket and key where downloaded EOP data is
// cached, under remote's cache directory for IERSFinals2000A.
func CacheFile(ctx context.Context) (bucket *file.Bucket, key string, err error) {
	bucket, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		return nil, "", fmt.Errorf("iers: %w", err)
	}

	return bucket, prefix + "finals2000A.data", nil
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
func fetch(ctx context.Context) error {
	// remote.GetFile reuses the cache untouched when the source's current
	// ETag shows the IERS bulletin hasn't changed since we last
	// downloaded it — a content check rather than a wall-clock expiration
	// window, since finals2000A is updated on IERS's own schedule, not
	// ours. WithValidate parses a fresh download before it's cached, so a
	// corrupt response never gets trusted as the new cache.
	bucket, key, err := remote.GetFile(ctx, remote.IERSFinals2000A, "finals2000A.all",
		remote.WithCacheName("finals2000A.data"),
		remote.WithValidate(func(r io.Reader) error {
			_, err := ParseFinals2000A(r)

			return err
		}))
	if err != nil {
		return fmt.Errorf("iers: fetch EOP data: %w", err)
	}

	data, err := bucket.ReadAll(ctx, key)
	if err != nil {
		return fmt.Errorf("iers: read EOP data: %w", err)
	}

	table, err := parseAndRegister(data, SourceNetwork)
	if err != nil {
		return fmt.Errorf("iers: parse EOP data: %w", err)
	}

	lo, hi := table.Coverage()

	log.Printf("astrogo/iers: loaded EOP data: MJD %.0f–%.0f (%d records)", lo, hi, len(table.records))

	return nil
}
