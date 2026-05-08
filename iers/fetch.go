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
)

const (
	// finalsURL is the IERS data center URL for the current finals2000A.all file.
	finalsURL = "https://datacenter.iers.org/data/9/finals2000A.all"

	// staleDays is the maximum age of the cached file before re-downloading.
	staleDays = 7

	// fetchTimeout is the HTTP timeout for downloading the file.
	fetchTimeout = 30 * time.Second
)

var fetchOnce sync.Once

// FetchIfStale downloads fresh IERS EOP data if the embedded table
// doesn't cover the requested MJD. The downloaded file is cached to
// iers/data/finals2000A.data relative to the module root (or the
// working directory if the module root is not available).
//
// This function is safe for concurrent use: only one download will
// be attempted regardless of how many goroutines call it.
//
// The downloaded data is parsed and registered globally via RegisterModel,
// replacing the previous (potentially stale) embedded data.
func FetchIfStale(mjd float64) error {
	model := GetModel()

	// Check if current model covers the requested epoch.
	if table, ok := model.(*Table); ok {
		if _, err := table.EOP(mjd); err == nil {
			return nil // embedded data is fresh enough
		}
	}

	var fetchErr error
	fetchOnce.Do(func() {
		fetchErr = doFetch()
	})
	return fetchErr
}

// CachePath returns the path where downloaded EOP data is cached.
func CachePath() string {
	return filepath.Join("iers", "data", "finals2000A.data")
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
