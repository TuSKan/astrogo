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

// FetchIfStale downloads fresh IERS EOP data if the current model
// doesn't cover the requested MJD. The downloaded file is cached under
// remote.DataDir()/iers (default: the user's OS cache directory, e.g.
// ~/.cache/astrogo/iers/ on Linux, %LocalAppData%/astrogo/iers/ on Windows).
//
// This function is safe for concurrent use: a mutex serialises
// download attempts, and the coverage check is repeated inside the
// lock so a successful concurrent fetch is respected immediately.
//
// After a failed attempt, retries are throttled to once per 5 minutes
// to avoid hammering the IERS server on transient errors.
//
// The downloaded data is parsed and registered globally via RegisterModel,
// replacing the previous (potentially stale) embedded data.
//
// Calling this function is itself the download consent (the endpoint is
// remote.IERSFinals2000A, ~3.7 MB); it still respects remote.SetOffline
// and remote.Disable.
func FetchIfStale(mjd float64) error {
	// Fast path (no lock): current model already covers this epoch.
	if covered(mjd) {
		return nil
	}

	fetchMu.Lock()
	defer fetchMu.Unlock()

	// Re-check after acquiring the lock — another goroutine may have
	// fetched successfully while we were waiting.
	if covered(mjd) {
		return nil
	}

	// Seed the cooldown timer from the on-disk cache if we haven't
	// attempted a fetch yet in this process.
	if lastAttempt.IsZero() {
		if cacheFile, err := CacheFile(); err == nil {
			if info := cacheFile.Info(); info.Exists {
				lastAttempt = info.Modified
			}
		}
	}

	// Throttle retries so transient errors don't cause a request storm.
	if !lastAttempt.IsZero() && time.Since(lastAttempt) < retryCooldown {
		return errLastFetch // may be nil (successful) or the prior error
	}

	lastAttempt = time.Now()
	errLastFetch = doFetch(context.Background())

	return errLastFetch
}

// FetchNow downloads and registers fresh IERS EOP data immediately,
// bypassing the coverage, staleness, and cooldown checks that FetchIfStale
// applies (a fresh on-disk cache file is still reused). Use it at service
// startup or from a scheduled refresh job.
//
// Calling this function is itself the download consent; it still respects
// remote.SetOffline and remote.Disable(remote.IERSFinals2000A).
func FetchNow(ctx context.Context) error {
	fetchMu.Lock()
	defer fetchMu.Unlock()

	lastAttempt = time.Now()
	errLastFetch = doFetch(ctx)

	return errLastFetch
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

func doFetch(ctx context.Context) error {
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

	table, err := ParseFinals2000A(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("iers: parse EOP data: %w", err)
	}

	RegisterModel(table)
	lo, hi := table.Coverage()

	log.Printf("astrogo/iers: loaded EOP data: MJD %.0f–%.0f (%d records)", lo, hi, len(table.records))

	return nil
}
