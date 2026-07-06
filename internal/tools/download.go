package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// ErrDownloadFailed indicates a download returned an unexpected HTTP status.
var ErrDownloadFailed = errors.New("download failed")

// downloadTimeout bounds the whole download (connect + transfer). Kernel
// files can be large (hundreds of MB), so this is generous compared to a
// typical API-call timeout — its purpose is only to prevent an indefinite
// hang on a stalled connection, not to cap legitimately slow transfers.
const downloadTimeout = 10 * time.Minute

// Download fetches a file from a URL and saves it to the target path.
func Download(url, path string) (err error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("download: mkdir: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download: new request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: HTTP do: %w", err)
	}
	defer func() {
		cerr := resp.Body.Close()
		if cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrDownloadFailed, resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), "download-*.tmp")
	if err != nil {
		return fmt.Errorf("download: create temp: %w", err)
	}

	tmpName := tmpFile.Name()

	// Ensure we don't leak the tmp file if something panics or fails early
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err = io.Copy(tmpFile, resp.Body); err != nil {
		return fmt.Errorf("download: copy: %w", err)
	}

	// Close explicitly before rename (critical for Windows)
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("download: close temp: %w", err)
	}

	// Atomically move the fully downloaded file into place
	if err := os.Rename(tmpName, path); err != nil {
		// On Windows, if multiple test suites run concurrently, another test
		// might have already downloaded and opened (locked) the file. If the file
		// exists and has a positive size, we can safely ignore the rename error.
		if stat, statErr := os.Stat(path); statErr == nil && stat.Size() > 0 {
			return nil
		}

		return fmt.Errorf("jpl: failed to finalize download atomic rename: %w", err)
	}

	return nil
}
