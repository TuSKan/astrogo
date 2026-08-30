package remote

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"
)

// sourceETagKey is the blob metadata entry recording the source ETag a
// cached or partially-downloaded object was fetched under. It rides as
// object metadata, which every driver supports, rather than a sidecar
// object keyed by string suffix.
const sourceETagKey = "source-etag"

// staleLockAge bounds how long a lock is honored before a new acquirer
// treats it as abandoned by a crashed holder. Generous relative to any
// single download in this registry.
const staleLockAge = 30 * time.Minute

// acquireLock's polling interval starts low so a short download is noticed
// almost immediately, and backs off so a multi-minute kernel transfer is
// not probed every 50ms for its whole duration.
const (
	lockRetryDelayInitial = 50 * time.Millisecond
	lockRetryDelayMax     = 2 * time.Second
)

// acquireLock blocks until it holds an exclusive lock on cacheKey within
// bucket, or ctx is done. Call the returned release exactly once — defer
// it immediately, including on the caller's own error paths.
//
// It is built on WriterOptions.IfNotExist, the create-if-absent primitive
// every Bucket exposes, so there is no backend-specific code here. On S3
// that is a genuinely atomic conditional PUT. fileblob implements it as
// Stat-then-Rename under a per-Bucket mutex, which is why remote/file.Open
// returns one shared Bucket per URL: that makes the lock exclusive within
// a process. Across processes on a local cache it remains best-effort,
// bounded by the double-check GetFile performs after acquiring.
func acquireLock(ctx context.Context, bucket *file.Bucket, cacheKey string) (release func(), err error) {
	lockKey := cacheKey + ".lock"
	delay := lockRetryDelayInitial

	// gcerrors.Unknown counts as contention alongside FailedPrecondition:
	// a losing writer on Windows surfaces a raw "Access is denied", which
	// fileblob maps to Unknown. A genuinely unrelated write failure (disk
	// full, permissions) then surfaces as a ctx deadline rather than
	// immediately, but is never silently swallowed.
	isContention := func(code gcerrors.ErrorCode) bool {
		return code == gcerrors.FailedPrecondition || code == gcerrors.Unknown
	}

	for {
		w, werr := bucket.NewWriter(ctx, lockKey, &blob.WriterOptions{IfNotExist: true})
		if werr == nil {
			_, writeErr := io.WriteString(w, "locked")
			closeErr := w.Close()

			switch {
			case writeErr == nil && closeErr == nil:
				// release runs from the caller's defer, possibly after ctx
				// was cancelled, and must still delete the lock — otherwise
				// it leaks until staleLockAge lets someone steal it.
				return func() { _ = bucket.Delete(context.WithoutCancel(ctx), lockKey) }, nil
			case isContention(gcerrors.Code(closeErr)):
				// Someone won the race between NewWriter and Close.
			default:
				return nil, fmt.Errorf("remote: create lock %s: %w", lockKey, cmp.Or(writeErr, closeErr))
			}
		} else if !isContention(gcerrors.Code(werr)) {
			return nil, fmt.Errorf("remote: create lock %s: %w", lockKey, werr)
		}

		if attrs, aerr := bucket.Attributes(ctx, lockKey); aerr == nil && time.Since(attrs.ModTime) > staleLockAge {
			_ = bucket.Delete(ctx, lockKey) // abandoned by a crashed holder; steal it next loop
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("remote: wait for lock %s: %w", lockKey, ctx.Err())
		case <-time.After(delay):
		}

		if delay *= 2; delay > lockRetryDelayMax {
			delay = lockRetryDelayMax
		}
	}
}

// partialKey names the in-progress body for cacheKey.
func partialKey(cacheKey string) string { return cacheKey + ".part" }

// resumePoint reports how many bytes of cacheKey a previous attempt
// already fetched and can be safely reused, given the source's current
// ETag. It returns 0 — discarding any unusable leftover on the way — when
// there is no partial, the partial is empty, it recorded no ETag, or the
// source has changed since it was written.
func resumePoint(ctx context.Context, bucket *file.Bucket, cacheKey, sourceETag string) int64 {
	pKey := partialKey(cacheKey)

	attrs, err := bucket.Attributes(ctx, pKey)
	if err != nil || attrs.Size <= 0 {
		return 0
	}

	if recorded := attrs.Metadata[sourceETagKey]; recorded == "" || recorded != sourceETag {
		_ = bucket.Delete(ctx, pKey)

		return 0
	}

	return attrs.Size
}

// stageAndPromote writes body into a staging object, validates it, and
// only then promotes it to cacheKey. Nothing a reader can observe at
// cacheKey is ever partial or unvalidated, and a transfer interrupted
// partway leaves a partial the next attempt can resume from.
//
// offset > 0 means body continues an existing partial: the two are
// concatenated into a separate staging key rather than written back over
// the partial while it is still open for reading, which Windows forbids.
func stageAndPromote(ctx context.Context, bucket *file.Bucket, cacheKey string,
	body io.Reader, offset int64, sourceETag string, validate func(io.Reader) error,
) error {
	pKey := partialKey(cacheKey)
	writeKey := pKey
	src := body

	var existing io.ReadCloser

	if offset > 0 {
		r, err := bucket.NewReader(ctx, pKey, nil)
		if err != nil {
			return fmt.Errorf("remote: read partial %s: %w", pKey, err)
		}

		existing = r
		src = io.MultiReader(existing, body)
		writeKey = cacheKey + ".resume"
	}

	if err := writeStaged(ctx, bucket, writeKey, src, existing, sourceETag); err != nil {
		return err
	}

	if validate != nil {
		if err := validateStaged(ctx, bucket, writeKey, validate); err != nil {
			discardStaging(ctx, bucket, writeKey, pKey)

			return fmt.Errorf("remote: validate %s: %w", cacheKey, err)
		}
	}

	if err := bucket.Copy(ctx, cacheKey, writeKey, nil); err != nil {
		return fmt.Errorf("remote: promote %s: %w", cacheKey, err)
	}

	discardStaging(ctx, bucket, writeKey, pKey)

	return nil
}

// writeStaged copies src into writeKey, recording sourceETag as metadata,
// and closes existing (the partial being resumed, if any) before
// returning so the caller can delete or rename it on Windows.
//
// Close is called even when the copy failed. driver.Writer is only an
// io.WriteCloser and fileblob's Close commits whatever arrived — which is
// exactly what a partial checkpoint needs, since writeKey is a staging
// key that only the next attempt reads. On a resume, writeKey is the
// separate ".resume" key, so a truncated commit there is inert: the
// untouched partial is what the next attempt resumes from.
func writeStaged(ctx context.Context, bucket *file.Bucket, writeKey string,
	src io.Reader, existing io.ReadCloser, sourceETag string,
) error {
	opts := &blob.WriterOptions{Metadata: map[string]string{sourceETagKey: sourceETag}}

	w, err := bucket.NewWriter(ctx, writeKey, opts)
	if err != nil {
		if existing != nil {
			_ = existing.Close()
		}

		return fmt.Errorf("remote: open staging %s: %w", writeKey, err)
	}

	_, copyErr := io.Copy(w, src)
	closeErr := w.Close()

	if existing != nil {
		_ = existing.Close()
	}

	if copyErr != nil || closeErr != nil {
		return fmt.Errorf("remote: write staging %s: %w", writeKey, errors.Join(copyErr, closeErr))
	}

	return nil
}

// validateStaged runs validate over the staged object's bytes, streaming
// rather than buffering so a multi-GB kernel needs no memory to check.
func validateStaged(ctx context.Context, bucket *file.Bucket, writeKey string, validate func(io.Reader) error) error {
	r, err := bucket.NewReader(ctx, writeKey, nil)
	if err != nil {
		return fmt.Errorf("read staging %s: %w", writeKey, err)
	}

	verr := validate(r)
	closeErr := r.Close()

	return errors.Join(verr, closeErr)
}

// discardStaging removes the staging objects. Failures are ignored: a
// leftover is inert and the next successful attempt overwrites it.
func discardStaging(ctx context.Context, bucket *file.Bucket, writeKey, pKey string) {
	_ = bucket.Delete(ctx, writeKey)

	if writeKey != pKey {
		_ = bucket.Delete(ctx, pKey)
	}
}
