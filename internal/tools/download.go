package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// Download fetches a file from a URL and saves it to the target path.
func Download(url, path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jpl: download failed with status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(path), "download-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	// Ensure we don't leak the tmp file if something panics or fails early
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err = io.Copy(tmpFile, resp.Body); err != nil {
		return err
	}

	// Close explicitly before rename (critical for Windows)
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Atomically move the fully downloaded file into place
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("jpl: failed to finalize download atomic rename: %w", err)
	}

	return nil
}
