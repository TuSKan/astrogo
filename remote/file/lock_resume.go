package file

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"gocloud.dev/blob"
	"gocloud.dev/gcerrors"

	"github.com/TuSKan/astrogo/time"
)

// SourceETagKey is the blob metadata entry recording the source ETag a
// cached or partially-downloaded object was fetched under. It rides as
// object metadata, which every driver supports, rather than a sidecar
// object keyed by string suffix.
const SourceETagKey = "source-etag"

// staleLockAge bounds how long a lock is honored before a new acquirer
// treats it as abandoned by a crashed holder. Generous relative to any
// single download in this registry.
const staleLockAge = 30 * time.Minute

// AcquireLock's polling interval starts low so a short download is noticed
// almost immediately, and backs off so a multi-minute kernel transfer is
// not probed every 50ms for its whole duration.
const (
	lockRetryDelayInitial = 50 * time.Millisecond
	lockRetryDelayMax     = 2 * time.Second
)

// inProcess serialises lock acquisition within this process, one key at a
// time.
//
// This used to be delegated to fileblob, and both this function and
// [github.com/TuSKan/astrogo/remote/file.Open] documented that the driver
// "guards it with a per-Bucket mutex" — which is why file.Open shares one
// Bucket per URL for the life of the process.
//
// That is not true of the pinned driver and may never have been. fileblob's
// bucket struct holds no mutex at all, and the mutex that does appear is
// constructed per *writer*, inside NewTypedWriter, so each contender locks
// its own. What IfNotExist actually performs is an os.Stat followed by an
// os.Rename, with a window in between.
//
// Measured before this existed — 8 goroutines sharing one Bucket, 200
// rounds: 51 rounds in which two or more contenders each believed they held
// the lock, and 2 in which three did (#245).
var inProcess = keyedSemaphore{held: make(map[string]chan struct{})}

// keyedSemaphore hands out exclusive access per key, honouring a context
// while waiting.
//
// A plain sync.Mutex would do the exclusion but not the waiting: a caller
// blocked in Lock cannot notice its own deadline, and the thing being
// waited for here is a download that may legitimately run for minutes.
//
// Entries are never removed. The keys are cache keys — a few dozen kernels
// and bulletins across a process's life — so the map is bounded by what the
// process actually fetches, and reclaiming entries would need reference
// counting to no measurable end.
type keyedSemaphore struct {
	mu   sync.Mutex
	held map[string]chan struct{}
}

// acquire blocks until key is free or ctx is done, returning the release
// for the caller to run exactly once.
func (k *keyedSemaphore) acquire(ctx context.Context, key string) (release func(), err error) {
	k.mu.Lock()

	ch, ok := k.held[key]
	if !ok {
		ch = make(chan struct{}, 1)
		k.held[key] = ch
	}

	k.mu.Unlock()

	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("remote: wait for in-process lock %s: %w", key, ctx.Err())
	}
}

// AcquireLock blocks until it holds an exclusive lock on cacheKey within
// bucket, or ctx is done. Call the returned release exactly once — defer
// it immediately, including on the caller's own error paths.
//
// Exclusion is in two layers, because one of them is not reliable.
//
// Within this process it is [inProcess], an ordinary semaphore this package
// owns and can therefore trust. Across processes it is
// WriterOptions.IfNotExist, the create-if-absent primitive every Bucket
// exposes, so there is no backend-specific code here: on S3 a genuinely
// atomic conditional PUT, on fileblob a Stat-then-Rename that is best-effort
// and can admit a second holder. That residual race is bounded by the
// double-check GetFile performs after acquiring, and its consequences are
// what #241 tracks.
//
// The layering matters for a reason beyond belt-and-braces: `go test ./...`
// runs each package as its own process and several of them want the same JPL
// kernel, so the cross-process case is the common one and the in-process case
// is the one that used to be claimed and was not delivered.
func AcquireLock(ctx context.Context, bucket *Bucket, cacheKey string) (release func(), err error) {
	lockKey := cacheKey + ".lock"
	delay := lockRetryDelayInitial

	// Not re-wrapped: acquire already names the key and what was being
	// waited for, and a second layer saying the same thing makes the
	// message longer without making it more specific.
	releaseInProcess, err := inProcess.acquire(ctx, lockKey)
	if err != nil {
		return nil, err
	}

	// Every path out of the loop below that is not a successful acquire has
	// to hand the in-process slot back, or the next caller for this key
	// waits on a holder that no longer exists.
	defer func() {
		if err != nil {
			releaseInProcess()
		}
	}()

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
				//
				// The in-process slot is handed back after the object is
				// gone, not before: releasing it first would let the next
				// goroutine in this process reach IfNotExist while the lock
				// object is still there, and spin until it is deleted.
				return func() {
					_ = bucket.Delete(context.WithoutCancel(ctx), lockKey)

					releaseInProcess()
				}, nil
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

// PartialKey names the in-progress body for cacheKey.
func PartialKey(cacheKey string) string { return cacheKey + ".part" }

// ResumePoint reports how many bytes of cacheKey a previous attempt
// already fetched and can be safely reused, given the source's current
// ETag. It returns 0 — discarding any unusable leftover on the way — when
// there is no partial, the partial is empty, it recorded no ETag, or the
// source has changed since it was written.
func ResumePoint(ctx context.Context, bucket *Bucket, cacheKey, sourceETag string) int64 {
	pKey := PartialKey(cacheKey)

	attrs, err := bucket.Attributes(ctx, pKey)
	if err != nil || attrs.Size <= 0 {
		return 0
	}

	if recorded := attrs.Metadata[SourceETagKey]; recorded == "" || recorded != sourceETag {
		_ = bucket.Delete(ctx, pKey)

		return 0
	}

	return attrs.Size
}

// StageAndPromote writes body into a staging object, validates it, and
// only then promotes it to cacheKey. Nothing a reader can observe at
// cacheKey is ever partial or unvalidated, and a transfer interrupted
// partway leaves a partial the next attempt can resume from.
//
// offset > 0 means body continues an existing partial: the two are
// concatenated into a separate staging key rather than written back over
// the partial while it is still open for reading, which Windows forbids.
func StageAndPromote(ctx context.Context, bucket *Bucket, cacheKey string,
	body io.Reader, offset int64, sourceETag string, validate func(io.Reader) error,
) error {
	pKey := PartialKey(cacheKey)
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
func writeStaged(ctx context.Context, bucket *Bucket, writeKey string,
	src io.Reader, existing io.ReadCloser, sourceETag string,
) error {
	opts := &blob.WriterOptions{Metadata: map[string]string{SourceETagKey: sourceETag}}

	// AcquireLock protects one cache key, but fileblob's temporary filename
	// contains only the basename and a clock value. Different cache keys and
	// buckets can therefore collide on Windows, including with SavePartial.
	// Use the same basename lock as Save and SavePartial until Close commits
	// the staging object. This leaves the download's resume semantics intact.
	unlock := writeLock(bucket, writeKey)
	defer unlock()

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
func validateStaged(ctx context.Context, bucket *Bucket, writeKey string, validate func(io.Reader) error) error {
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
func discardStaging(ctx context.Context, bucket *Bucket, writeKey, pKey string) {
	_ = bucket.Delete(ctx, writeKey)

	if writeKey != pKey {
		_ = bucket.Delete(ctx, pKey)
	}
}
