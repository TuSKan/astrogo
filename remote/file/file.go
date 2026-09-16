// Package file is astrogo's file-access layer, built on [io/fs].
//
// It is internal to remote and is not importable from outside remote/ —
// TestSubpackagesAreNotImportedDirectly enforces that. Everything here is
// re-exported from remote, which is where a caller reaches it, and which is
// also the only place that answers whether bytes may move at all. What is
// left here is moving them.
//
// # Why io/fs and not a storage abstraction
//
// This package used to be a thin naming layer over gocloud.dev/blob: one type,
// Bucket, aliased to *blob.Bucket. That worked, and it cost 388 packages and an
// unconditional OpenTelemetry dependency for a library whose storage needs are
// open, read a range, write, delete.
//
// The standard library already has the read half, and every Go programmer
// already knows it. A file is an [fs.File]; a missing one is [fs.ErrNotExist],
// checked with errors.Is exactly as it is against os, embed and zip; a name is
// an [fs.ValidPath], which is the unrooted, slash-separated, no-dot-elements
// rule CLAUDE.md already mandated and path.Join already produced. The write
// half io/fs deliberately does not define (golang/go#45757), so astrogo defines
// the smallest set it needs, in the standard library's own single-method idiom
// — see [CreateFS], [CreateExclFS], [RemoveFS] and [ContextFS] in fsys.go.
//
// # Addressing
//
// A filesystem URL plus a name, and nothing here is assumed local: the cache
// and every source may live on S3, GCS, Azure, SFTP or anywhere a backend is
// registered. Names are always "/"-separated (use path.Join, never
// filepath.Join) and no API here takes an OS filesystem path. The module's one
// path-to-URL conversion is remote's own default-cache-dir resolver, which has
// to start from os.UserCacheDir and is deliberately not general.
//
// Scheme dispatch is [OpenFS] over this package's own registry, populated from
// each backend's init. The backends every astrogo build needs are compiled in —
// file://, http://, https:// and mem:// — and any further scheme is an opt-in
// blank import of its own subpackage, which is why adding one needs no change
// here.
package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
)

// WriteFile streams r into fsys at name.
//
// The filesystem must implement [CreateFS]; one that does not — the HTTP
// backend, and any read-only source — is refused with [ErrReadOnly] rather than
// failing somewhere less obvious.
//
// # Streaming, and the Close that is skipped
//
// io.Copy rather than io.ReadAll because a DE440 kernel is three gigabytes and
// astrogo's whole download path exists so that one never has to fit in memory.
//
// On a copy failure Close is deliberately not called, and this is the same
// hazard the gocloud implementation documented: an io.WriteCloser's Close does
// not know whether an earlier Write failed, and a staged writer's Close commits
// by renaming into place regardless — so closing after a failed copy replaces a
// good object with a truncated one. This package's own backends handle it
// properly: every writer here implements [AbortWriter], so a failed copy throws
// the staged write away and releases its handles. A backend that does not is
// left unclosed, which is what the gocloud implementation did on every backend
// and which leaks rather than corrupts.
func WriteFile(ctx context.Context, fsys fs.FS, name string, r io.Reader) error {
	cfs, ok := WithContext(ctx, fsys).(CreateFS)
	if !ok {
		return fmt.Errorf("remote/file: write %s: %w", name, ErrReadOnly)
	}

	w, err := cfs.Create(name)
	if err != nil {
		return fmt.Errorf("remote/file: create %s: %w", name, err)
	}

	if _, err := io.Copy(w, r); err != nil {
		abort(w)

		return fmt.Errorf("remote/file: write %s: %w", name, err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("remote/file: close %s: %w", name, err)
	}

	return nil
}

// abort throws away a write in progress.
//
// A writer that can be aborted is, which releases its handles and leaves the
// destination untouched. One that cannot is simply not closed — the gocloud-era
// behaviour, and the reason [AbortWriter] exists: skipping Close is the only
// way to avoid committing a truncated object, and it leaks whatever the writer
// was holding.
func abort(w io.WriteCloser) {
	if aw, ok := w.(AbortWriter); ok {
		_ = aw.Abort()
	}
}

// Remove deletes name from fsys.
//
// A filesystem that cannot delete is refused with [ErrReadOnly], and a name
// that is not there reports [fs.ErrNotExist] — so a caller clearing a stale
// object writes errors.Is rather than learning a second vocabulary.
func Remove(ctx context.Context, fsys fs.FS, name string) error {
	rfs, ok := WithContext(ctx, fsys).(RemoveFS)
	if !ok {
		return fmt.Errorf("remote/file: remove %s: %w", name, ErrReadOnly)
	}

	if err := rfs.Remove(name); err != nil {
		return fmt.Errorf("remote/file: remove %s: %w", name, err)
	}

	return nil
}

// Exists reports whether name is present on fsys.
//
// A convenience over fs.Stat for the one question every cache-hit path asks.
// An error that is not [fs.ErrNotExist] is returned rather than folded into
// false: "not cached" and "the store is broken" are different answers, and
// treating the second as the first re-downloads a kernel every time and reports
// nothing.
func Exists(ctx context.Context, fsys fs.FS, name string) (bool, error) {
	_, err := fs.Stat(WithContext(ctx, fsys), name)

	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	default:
		return false, fmt.Errorf("remote/file: stat %s: %w", name, err)
	}
}
