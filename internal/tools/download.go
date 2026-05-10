package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// ErrDownloadFailed indicates a download returned an unexpected HTTP status.
var ErrDownloadFailed = errors.New("download failed")

// Download fetches a file from a URL and saves it to the target path.
func Download(url, path string) (err error) {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("download: mkdir: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
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
		return fmt.Errorf("jpl: failed to finalize download atomic rename: %w", err)
	}

	return nil
}
