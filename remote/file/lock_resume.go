package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SourceETagSuffix names the sidecar recording the source ETag a partially
// downloaded object was fetched under.
//
// # A sidecar, where this used to be object metadata
//
// gocloud gave every driver a metadata map, and the ETag rode in it. io/fs has
// no such thing, and inventing one would mean an extension interface every
// backend had to implement to be usable as a cache — for a single string that
// only the resume path reads.
//
// A sidecar object costs one extra write on the partial-download path and
// nothing anywhere else, works on every backend including ones not written yet,
// and is inspectable: a user wondering why a resume was refused can read the
// file. The failure mode is a sidecar that outlives its partial, which
// [ResumePoint] handles the same way it handles a missing one — by starting
// over, which is what it would do anyway.
const SourceETagSuffix = ".etag"

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

// ErrNoExclusiveCreate reports a filesystem that cannot create a name only when
// it is absent, and so cannot host the download lock.
var ErrNoExclusiveCreate = errors.New("remote/file: filesystem cannot create exclusively")

// inProcess serialises lock acquisition within this process, one key at a
// time.
//
// Kept even though the cross-process primitive is now exact, because it is
// doing something the exclusive create is not: honouring a context while
// waiting. A goroutine blocked on a lock held by another goroutine in the same
// process would otherwise have to discover the release by polling, and the
// thing being waited for is a download that may legitimately run for minutes.
//
// It also used to be load-bearing for correctness rather than only efficiency.
// Both this function and the old file.Open documented that fileblob "guards it
// with a per-Bucket mutex", which was why one Bucket was shared per URL for the
// life of the process. That was not true of the pinned driver and may never
// have been: fileblob's bucket struct held no mutex, and the one that existed
// was built per writer inside NewTypedWriter, so contenders each locked their
// own. Measured — 8 goroutines sharing one Bucket, 200 rounds — 51 rounds had
// two or more contenders each believing they held the lock, and 2 had three
// (#245).
var inProcess = keyedSemaphore{held: make(map[string]chan struct{})}

// keyedSemaphore hands out exclusive access per key, honouring a context
// while waiting.
//
// A plain sync.Mutex would do the exclusion but not the waiting: a caller
// blocked in Lock cannot notice its own deadline.
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

// AcquireLock blocks until it holds an exclusive lock on cacheKey within fsys,
// or ctx is done. Call the returned release exactly once — defer it
// immediately, including on the caller's own error paths.
//
// Exclusion is in two layers. Within this process it is [inProcess], which
// makes waiting cancellable. Across processes it is [CreateExclFS], which
// astrogo's local backend implements with O_CREATE|O_EXCL inside an [os.Root].
//
// # This is the part that got stronger
//
// The cross-process layer used to be gocloud's WriterOptions.IfNotExist, which
// this code described as "on S3 a genuinely atomic conditional PUT, on fileblob
// a Stat-then-Rename that is best-effort and can admit a second holder". The
// gap was real and tracked as #241, and it forced a losing writer to be
// recognised by three different error codes — FailedPrecondition, the intended
// signal; Unknown, from a Windows "Access is denied" when the winner held the
// destination open; and NotFound, from the loser's own staging file being
// renamed away because both had picked the same name from a clock that does not
// advance on Windows.
//
// None of that survives. O_CREATE|O_EXCL is indivisible in the kernel, so there
// is exactly one way to lose and it is [fs.ErrExist]. The classifier, the
// staging lock this function had to take around its own lock write, and the
// residual race the caller double-checked for are all gone with it.
//
// The layering still matters for a reason beyond belt-and-braces: `go test
// ./...` runs each package as its own process and several of them want the same
// JPL kernel, so the cross-process case is the common one.
func AcquireLock(ctx context.Context, fsys fs.FS, cacheKey string) (release func(), err error) {
	lockKey := cacheKey + ".lock"
	delay := lockRetryDelayInitial

	bound := WithContext(ctx, fsys)

	xfs, ok := bound.(CreateExclFS)
	if !ok {
		return nil, fmt.Errorf("remote: lock %s: %w", lockKey, ErrNoExclusiveCreate)
	}

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

	for {
		// Checked here as well as in the backend, because this loop is where a
		// canceled caller must stop and the backend is not obliged to be the
		// one that notices. Every backend in this package does — see
		// [localFS.WithContext] — but a lock handed to a caller who has gone is
		// bad enough to be worth one comparison per attempt.
		//
		// The gocloud implementation got this for free, since every write took a
		// ctx, which is exactly why it is easy to lose in the move and why the
		// test for it exists.
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("remote: wait for lock %s: %w", lockKey, err)
		}

		w, cerr := xfs.CreateExcl(lockKey)

		switch {
		case cerr == nil:
			closeErr := w.Close()
			if closeErr != nil {
				return nil, fmt.Errorf("remote: create lock %s: %w", lockKey, closeErr)
			}

			// release runs from the caller's defer, possibly after ctx was
			// canceled, and must still delete the lock — otherwise it leaks
			// until staleLockAge lets someone steal it.
			//
			// The in-process slot is handed back after the object is gone, not
			// before: releasing it first would let the next goroutine in this
			// process reach CreateExcl while the lock object is still there,
			// and spin until it is deleted.
			return func() {
				_ = Remove(context.WithoutCancel(ctx), fsys, lockKey)

				releaseInProcess()
			}, nil

		case errors.Is(cerr, fs.ErrExist):
			// Someone else got there first; wait below and try again.

		default:
			return nil, fmt.Errorf("remote: create lock %s: %w", lockKey, cerr)
		}

		if info, serr := fs.Stat(bound, lockKey); serr == nil &&
			time.Since(info.ModTime()) > staleLockAge {
			// Abandoned by a crashed holder; steal it next loop.
			_ = Remove(ctx, fsys, lockKey)
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
func ResumePoint(ctx context.Context, fsys fs.FS, cacheKey, sourceETag string) int64 {
	bound := WithContext(ctx, fsys)
	pKey := PartialKey(cacheKey)

	info, err := fs.Stat(bound, pKey)
	if err != nil || info.Size() <= 0 {
		return 0
	}

	if recorded := readETag(bound, pKey); recorded == "" || recorded != sourceETag {
		discardStaging(ctx, fsys, pKey, pKey)

		return 0
	}

	return info.Size()
}

// RecordedETag returns the source ETag recorded beside key when it was
// fetched, or "" when there is none.
//
// remote's cache-freshness check needs it, which is why this is exported where
// the rest of the sidecar handling is not.
func RecordedETag(ctx context.Context, fsys fs.FS, key string) string {
	return readETag(WithContext(ctx, fsys), key)
}

// ETag returns a validator for an object: a token that changes when the object
// does, used to decide whether a cached copy is still current and whether a
// partial download can be resumed onto.
//
// Two sources, in order.
//
// The server's own ETag, when there is one. [fs.FileInfo] has no such field and
// should not grow one — it is an HTTP concept that means nothing on a local
// disk — so the HTTP backend puts its response header in Sys, which is the
// standard library's own escape hatch for exactly this.
//
// Otherwise a weak validator synthesised from size and modification time. That
// is not a fallback invented here: it is precisely what gocloud's fileblob did
// for every local object, and it is what makes a file:// source resumable at
// all. It is weaker than a real ETag and says so in its shape — a file replaced
// with different content of the same length in the same nanosecond would not be
// noticed — which is why the strong one is preferred whenever offered.
//
// When there is neither, "" — meaning "cannot tell", which every caller here
// treats as "assume changed" rather than as "unchanged". A zero modification
// time is the case that produces it: a backend that reports no time at all has
// told us nothing, and a validator built from nothing would match itself
// forever.
func ETag(info fs.FileInfo) string {
	if header, ok := info.Sys().(http.Header); ok {
		if tag := strings.Trim(header.Get("ETag"), `"`); tag != "" {
			return tag
		}
	}

	mod := info.ModTime()
	if mod.IsZero() {
		return ""
	}

	return "W/" + strconv.FormatInt(info.Size(), 10) + "-" + strconv.FormatInt(mod.UnixNano(), 10)
}

// readETag returns the ETag recorded beside a staged object, or "".
func readETag(fsys fs.FS, key string) string {
	b, err := fs.ReadFile(fsys, key+SourceETagSuffix)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(b))
}

// WriteETag records sourceETag beside key, where [RecordedETag] reads it back.
// An empty ETag writes nothing, which reads back as "cannot tell".
//
// Exported because whether a cached object keeps its ETag is a policy question
// and policy lives in remote: only a Mutable endpoint's freshness check ever
// reads one, so an immutable kernel — the multi-gigabyte case — gets no sidecar
// at all. Inside this package it is used for the partial download, where resume
// needs it whatever the endpoint is.
func WriteETag(ctx context.Context, fsys fs.FS, key, sourceETag string) error {
	if sourceETag == "" {
		return nil
	}

	return WriteFile(ctx, fsys, key+SourceETagSuffix, strings.NewReader(sourceETag))
}

// StageAndPromote writes body into a staging object, validates it, and
// only then promotes it to cacheKey. Nothing a reader can observe at
// cacheKey is ever partial or unvalidated, and a transfer interrupted
// partway leaves a partial the next attempt can resume from.
//
// offset > 0 means body continues an existing partial: the two are
// concatenated into a separate staging key rather than written back over
// the partial while it is still open for reading, which Windows forbids.
func StageAndPromote(ctx context.Context, fsys fs.FS, cacheKey string,
	body io.Reader, offset int64, sourceETag string, validate func(io.Reader) error,
) error {
	bound := WithContext(ctx, fsys)
	pKey := PartialKey(cacheKey)
	writeKey := pKey
	src := body

	var existing io.ReadCloser

	if offset > 0 {
		r, err := bound.Open(pKey)
		if err != nil {
			return fmt.Errorf("remote: read partial %s: %w", pKey, err)
		}

		existing = r
		src = io.MultiReader(existing, body)
		writeKey = cacheKey + ".resume"
	}

	if err := writeStaged(ctx, fsys, writeKey, src, existing, sourceETag); err != nil {
		return err
	}

	if validate != nil {
		if err := validateStaged(ctx, fsys, writeKey, validate); err != nil {
			discardStaging(ctx, fsys, writeKey, pKey)

			return fmt.Errorf("remote: validate %s: %w", cacheKey, err)
		}
	}

	if err := promote(ctx, fsys, cacheKey, writeKey); err != nil {
		return err
	}

	discardStaging(ctx, fsys, writeKey, pKey)

	return nil
}

// promote copies the staged object to cacheKey.
//
// A copy rather than a rename because io/fs has no rename and astrogo's
// backends deliberately do not add one: the local backend's Create already
// renames its own staging file into place atomically, so this copy lands
// through that same promotion and a reader never sees a partial cacheKey. On a
// store with no rename at all — an object store — a copy is what a rename would
// have been anyway.
func promote(ctx context.Context, fsys fs.FS, cacheKey, writeKey string) error {
	r, err := WithContext(ctx, fsys).Open(writeKey)
	if err != nil {
		return fmt.Errorf("remote: read staging %s: %w", writeKey, err)
	}

	defer func() { _ = r.Close() }()

	if err := WriteFile(ctx, fsys, cacheKey, r); err != nil {
		return fmt.Errorf("remote: promote %s: %w", cacheKey, err)
	}

	return nil
}

// writeStaged copies src into writeKey, recording sourceETag beside it, and
// closes existing (the partial being resumed, if any) before returning so the
// caller can delete or rename it on Windows.
//
// # Why a failed copy still commits here
//
// [WriteFile] deliberately skips Close after a failed copy, so that a good
// object is never replaced by a truncated one. This path wants the opposite,
// and the distinction is what makes resuming possible: writeKey is a staging
// key that only the next attempt reads, and committing whatever arrived is
// exactly what a partial checkpoint is. On a resume writeKey is the separate
// ".resume" key, so a truncated commit there is inert — the untouched partial
// is what the next attempt resumes from.
func writeStaged(ctx context.Context, fsys fs.FS, writeKey string,
	src io.Reader, existing io.ReadCloser, sourceETag string,
) error {
	cfs, ok := WithContext(ctx, fsys).(CreateFS)
	if !ok {
		if existing != nil {
			_ = existing.Close()
		}

		return fmt.Errorf("remote: open staging %s: %w", writeKey, ErrReadOnly)
	}

	w, err := cfs.Create(writeKey)
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

	if err := WriteETag(ctx, fsys, writeKey, sourceETag); err != nil {
		return fmt.Errorf("remote: record etag for %s: %w", writeKey, err)
	}

	return nil
}

// validateStaged runs validate over the staged object's bytes, streaming
// rather than buffering so a multi-GB kernel needs no memory to check.
func validateStaged(ctx context.Context, fsys fs.FS, writeKey string, validate func(io.Reader) error) error {
	r, err := WithContext(ctx, fsys).Open(writeKey)
	if err != nil {
		return fmt.Errorf("read staging %s: %w", writeKey, err)
	}

	verr := validate(r)
	closeErr := r.Close()

	return errors.Join(verr, closeErr)
}

// discardStaging removes the staging objects and their ETag sidecars. Failures
// are ignored: a leftover is inert and the next successful attempt overwrites
// it.
func discardStaging(ctx context.Context, fsys fs.FS, writeKey, pKey string) {
	drop := func(key string) {
		_ = Remove(ctx, fsys, key)
		_ = Remove(ctx, fsys, key+SourceETagSuffix)
	}

	drop(writeKey)

	if writeKey != pKey {
		drop(pKey)
	}
}

// StagingSuffixes are the name suffixes this package appends to a cache key for
// its own bookkeeping. Exported so remote can recognise and skip them when it
// walks a cache directory; nothing else should need it.
var StagingSuffixes = []string{".lock", ".part", ".resume", SourceETagSuffix}

// IsStagingName reports whether name is one of this package's bookkeeping
// objects rather than a cached object in its own right.
func IsStagingName(name string) bool {
	base := path.Base(name)

	for _, suffix := range StagingSuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}

	return false
}
