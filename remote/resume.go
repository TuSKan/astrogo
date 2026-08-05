package remote

import (
	"fmt"
	"io"

	gofs "github.com/ungerik/go-fs"
)

// Sidecars kept next to a cache file while a download is in flight. Unlike
// Save's randomly-named temp file (removed on failure), these persist
// deliberately across a failed attempt so the next one can resume instead
// of re-fetching from zero — the difference that matters for the
// multi-hundred-megabyte sources this package serves.
const (
	// partSuffix names the in-progress body.
	partSuffix = ".part"
	// validatorSuffix names the ETag the partial body was fetched under.
	// Without it a partial cannot be proven to still match upstream, so
	// it is discarded rather than resumed.
	validatorSuffix = ".part.etag"
)

func partialFor(dest gofs.File) gofs.File   { return dest + partSuffix }
func validatorFor(dest gofs.File) gofs.File { return dest + validatorSuffix }

// resumePoint reports how many bytes of dest were already downloaded by a
// previous attempt and the ETag they were fetched under, for use as
// Range/If-Range. It returns (0, "") when there is nothing safely
// resumable — no partial, an empty partial, or a partial with no recorded
// validator — discarding any unusable leftovers on the way out so a stale
// partial can never silently corrupt a later download.
func resumePoint(dest gofs.File) (offset int64, validator string) {
	part := partialFor(dest)
	if !part.Exists() {
		// A validator with no body is orphaned; drop it.
		discardPartial(dest)

		return 0, ""
	}

	size := part.Size()
	if size <= 0 {
		discardPartial(dest)

		return 0, ""
	}

	etag, err := validatorFor(dest).ReadAll()
	if err != nil || len(etag) == 0 {
		discardPartial(dest)

		return 0, ""
	}

	return size, string(etag)
}

// discardPartial removes both sidecars, ignoring absent files.
func discardPartial(dest gofs.File) {
	_ = partialFor(dest).Remove()
	_ = validatorFor(dest).Remove()
}

// writePartial streams r into dest's partial file — appending when the
// server honored our Range request with 206, truncating when it answered
// 200 (no range support, or If-Range detected changed content). The
// validator is recorded first so an interrupted transfer still leaves
// enough behind to resume; an empty validator means the next attempt will
// start over rather than resume unsafely.
func writePartial(r io.Reader, dest gofs.File, appendMode bool, validator string) error {
	if err := dest.Dir().MakeAllDirs(); err != nil {
		return fmt.Errorf("remote: mkdir %s: %w", dest.Dir(), err)
	}

	if validator != "" {
		if err := validatorFor(dest).WriteAll([]byte(validator)); err != nil {
			return fmt.Errorf("remote: record resume validator for %s: %w", dest, err)
		}
	} else {
		// Nothing to prove a future partial still matches upstream.
		_ = validatorFor(dest).Remove()
	}

	part := partialFor(dest)

	var (
		w   io.WriteCloser
		err error
	)

	if appendMode {
		w, err = part.OpenAppendWriter()
	} else {
		w, err = part.OpenWriter()
	}

	if err != nil {
		return fmt.Errorf("remote: open partial %s: %w", part, err)
	}

	defer func() { _ = w.Close() }()

	if _, err := io.Copy(w, r); err != nil {
		// Deliberately leave the partial in place: that is the whole
		// point — the next attempt resumes from here.
		return fmt.Errorf("remote: write partial %s: %w", part, err)
	}

	return nil
}

// promotePartial moves a fully-downloaded partial into its final cache
// path and clears the validator sidecar, completing the download.
func promotePartial(dest gofs.File) error {
	if err := partialFor(dest).MoveTo(dest); err != nil {
		return fmt.Errorf("remote: finalize %s: %w", dest, err)
	}

	_ = validatorFor(dest).Remove()

	return nil
}
