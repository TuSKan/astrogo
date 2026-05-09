package iers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TuSKan/astrogo/internal/cache"
)

const (
	// finalsURL is the IERS data center URL for the current finals2000A.all file.
	finalsURL = "https://datacenter.iers.org/data/9/finals2000A.all"

	// staleDays is the maximum age of the cached file before re-downloading.
	staleDays = 7

	// fetchTimeout is the HTTP timeout for downloading the file.
	fetchTimeout = 30 * time.Second
)

var (
	fetchMu       sync.Mutex
	lastAttempt   time.Time         // wall-clock of last fetch attempt (success or failure)
	lastFetchErr  error             // non-nil if the most recent attempt failed
	retryCooldown = 5 * time.Minute // minimum interval between fetch attempts
)

// FetchIfStale downloads fresh IERS EOP data if the current model
// doesn't cover the requested MJD. The downloaded file is cached under
// the user's OS cache directory (e.g. ~/.cache/astrogo/iers/ on Linux,
// %LocalAppData%/astrogo/iers/ on Windows).
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
		if info, err := os.Stat(CachePath()); err == nil {
			lastAttempt = info.ModTime()
		}
	}

	// Throttle retries so transient errors don't cause a request storm.
	if !lastAttempt.IsZero() && time.Since(lastAttempt) < retryCooldown {
		return lastFetchErr // may be nil (successful) or the prior error
	}

	lastAttempt = time.Now()
	lastFetchErr = doFetch()
	return lastFetchErr
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

// CachePath returns the absolute path where downloaded EOP data is cached.
// It uses the OS-standard user cache directory via [cache.Path].
func CachePath() string {
	p, err := cache.Path("iers", "finals2000A.data")
	if err != nil {
		// Fallback: temp directory (cache.Path already handles UserCacheDir
		// failures, so this is truly exceptional).
		return filepath.Join(os.TempDir(), "astrogo", "iers", "finals2000A.data")
	}
	return p
}

func doFetch() error {
	cachePath := CachePath()

	// Check if a sufficiently fresh cache file exists.
	if info, err := os.Stat(cachePath); err == nil {
		age := time.Since(info.ModTime())
		if age < staleDays*24*time.Hour {
			// Cache is fresh — load from disk instead of downloading.
			return loadFromDisk(cachePath)
		}
	}

	log.Printf("astrogo/iers: downloading fresh EOP data from %s", finalsURL)

	client := &http.Client{Timeout: fetchTimeout}
	resp, err := client.Get(finalsURL)
	if err != nil {
		return fmt.Errorf("iers: failed to download EOP data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("iers: EOP download returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("iers: failed to read EOP response: %w", err)
	}

	// Parse before writing to disk, so we don't cache corrupt data.
	table, err := ParseFinals2000A(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("iers: failed to parse downloaded EOP data: %w", err)
	}

	// Ensure directory exists and write cache.
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		log.Printf("astrogo/iers: warning: could not create cache dir: %v", err)
		// Still register the parsed data even if caching fails.
	} else if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		log.Printf("astrogo/iers: warning: could not write cache file: %v", err)
	}

	RegisterModel(table)
	min, max := table.Coverage()
	log.Printf("astrogo/iers: loaded fresh EOP data: MJD %.0f–%.0f (%d records)",
		min, max, len(table.records))
	return nil
}

func loadFromDisk(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("iers: failed to read cached EOP file: %w", err)
	}
	table, err := ParseFinals2000A(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("iers: failed to parse cached EOP data: %w", err)
	}
	RegisterModel(table)
	min, max := table.Coverage()
	log.Printf("astrogo/iers: loaded cached EOP data: MJD %.0f–%.0f (%d records)",
		min, max, len(table.records))
	return nil
}
