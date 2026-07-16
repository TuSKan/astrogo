package remote

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	gofs "github.com/ungerik/go-fs"
)

// Save writes r fully to dest, creating dest's parent directory first —
// the one place astrogo persists an arbitrary stream to a cache location,
// whatever its source (a download response body, a decoded API payload,
// computed data). On the local filesystem the write is atomic (temp file +
// rename, via writeAtomicReader); a non-local go-fs destination is written
// through directly, since atomic rename is generally unavailable there.
func Save(r io.Reader, dest gofs.File) error {
	if err := dest.Dir().MakeAllDirs(); err != nil {
		return fmt.Errorf("remote: mkdir %s: %w", dest.Dir(), err)
	}

	if local := dest.LocalPath(); local != "" {
		if err := writeAtomicReader(r, local); err != nil {
			return fmt.Errorf("remote: write %s: %w", dest, err)
		}

		return nil
	}

	w, err := dest.OpenWriter()
	if err != nil {
		return fmt.Errorf("remote: open writer %s: %w", dest, err)
	}

	defer func() { _ = w.Close() }()

	if _, err := io.Copy(w, r); err != nil {
		return fmt.Errorf("remote: write %s: %w", dest, err)
	}

	return nil
}

// writeAtomicReader streams body into a temp file next to path and
// atomically renames it into place — never leaving a partial file at
// path. Used by Save for the local-filesystem destination path (go-fs's
// own WriteAll doesn't do this: it truncates in place).
func writeAtomicReader(body io.Reader, path string) (err error) {
	tmpFile, err := os.CreateTemp(filepath.Dir(path), "astrogo-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	tmpName := tmpFile.Name()

	// Ensure we don't leak the tmp file if something panics or fails early.
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpName)
	}()

	if _, err = io.Copy(tmpFile, body); err != nil {
		return fmt.Errorf("copy: %w", err)
	}

	// Close explicitly before rename (critical for Windows).
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp: %w", err)
	}

	// Atomically move the fully written file into place.
	if err := os.Rename(tmpName, path); err != nil {
		// On Windows, if multiple processes run concurrently, another one
		// might have already written and opened (locked) the file. If the
		// file exists and has a positive size, the rename loss is harmless.
		if stat, statErr := os.Stat(path); statErr == nil && stat.Size() > 0 {
			return nil
		}

		return fmt.Errorf("finalize atomic rename: %w", err)
	}

	return nil
}
