package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	gofs "github.com/ungerik/go-fs"
)

// staleLockAge bounds how long a lock file is honored before a new acquirer
// treats it as abandoned — a crashed process, an OS-killed CI runner —
// rather than waiting on it forever. Generous relative to any single
// download in this registry (DefaultDownloadTimeout and every registered
// Endpoint.DownloadTimeout are well under this).
const staleLockAge = 30 * time.Minute

// lockRetryDelay bounds acquireLock's polling interval: it starts at the
// low end (so a short download, the common case in this test suite, is
// noticed almost immediately) and backs off exponentially to the high end
// (so a multi-minute kernel download isn't hammered with an OpenFile
// syscall every 50ms for its whole duration).
const (
	lockRetryDelayInitial = 50 * time.Millisecond
	lockRetryDelayMax     = 2 * time.Second
)

// lockPath returns dest's advisory-lock sidecar path, following the same
// "dest + suffix" convention resume.go's partialFor/validatorFor use.
func lockPath(dest gofs.File) gofs.File { return dest + ".lock" }

// acquireLock blocks until it holds an exclusive, cross-process lock on
// dest, or ctx is done. The returned release func must be called exactly
// once, even on an error path from the caller's own work — defer it
// immediately.
//
// This exists because dest.Exists() then dest.OpenWriter() is not atomic:
// without a lock, two callers racing to fill the same missing cache entry
// both decide "not cached yet" and both download, and writePartial's
// FIXED-name .part file means they corrupt each other's write rather than
// merely wasting bandwidth (which is all Save's random-temp-then-rename
// path risks). This is a real, not hypothetical, race — `go test ./...`
// runs each package as its own OS process, and several packages fetch the
// same shared JPL kernel, so this is a cross-PROCESS race, not just a
// cross-goroutine one; an in-package sync.Mutex cannot fix it.
//
// Implemented via O_CREATE|O_EXCL, which every platform this repo supports
// maps to a single atomic create-if-absent syscall — no new dependency,
// same portability story as the rest of this package.
//
// ok is false when dest has no local filesystem path (a non-local gofs
// backend, e.g. a future s3:// CacheDir) — there is no os.OpenFile to lock
// against there, so locking is skipped and the caller proceeds unlocked,
// exactly the behavior this whole package had before this file existed.
// Every CacheDir this codebase resolves today is local (see DataDir's doc
// comment), so this is a documented gap for a backend that does not exist
// yet, not a silent one for a backend in current use.
func acquireLock(ctx context.Context, dest gofs.File) (release func(), ok bool, err error) {
	lock := lockPath(dest)

	path := lock.LocalPath()
	if path == "" {
		return func() {}, false, nil
	}

	if err := dest.Dir().MakeAllDirs(); err != nil {
		return nil, false, fmt.Errorf("remote: mkdir for lock %s: %w", path, err)
	}

	delay := lockRetryDelayInitial

	for {
		f, ferr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if ferr == nil {
			_ = f.Close()

			return func() { _ = os.Remove(path) }, true, nil
		}

		// os.IsPermission alongside ErrExist: on Windows, a create racing
		// another goroutine's os.Remove of the SAME lock file (the release
		// path below) observably returns ERROR_ACCESS_DENIED rather than
		// ERROR_FILE_EXISTS while the delete is settling — a real,
		// live-reproduced Windows quirk (this test failed intermittently
		// on a real Windows run before this check was added, not a
		// theorized case), not a genuine permissions problem: nothing else
		// in this loop ever holds the lock file open, and the directory
		// was already proven writable by the MakeAllDirs call above. Both
		// cases mean the same thing here — "still contended, try again."
		if !errors.Is(ferr, os.ErrExist) && !os.IsPermission(ferr) {
			return nil, false, fmt.Errorf("remote: create lock %s: %w", path, ferr)
		}

		if info, statErr := os.Stat(path); statErr == nil && time.Since(info.ModTime()) > staleLockAge {
			_ = os.Remove(path) // abandoned by a crashed holder — steal it next loop
		}

		select {
		case <-ctx.Done():
			return nil, false, fmt.Errorf("remote: wait for lock %s: %w", path, ctx.Err())
		case <-time.After(delay):
		}

		if delay *= 2; delay > lockRetryDelayMax {
			delay = lockRetryDelayMax
		}
	}
}
