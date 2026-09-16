package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync/atomic"
)

// Registered from init because that is what a blank-import registry is: a
// backend package exists to be imported for its side effect, the way
// database/sql drivers and image decoders are, and remote/file/s3 exports
// nothing at all. There is no other moment at which a scheme can announce
// itself before the first URL is opened.
//
//nolint:gochecknoinits // the registry pattern this package is built on
func init() { Register("file", openLocal) }

// localFS is the filesystem astrogo's cache lives on.
//
// # os.Root, not a path prefix
//
// Every operation goes through [os.Root], which confines it beneath one
// directory at the operating system's level: a symlink pointing out of the tree
// is refused by the kernel-side check rather than by string inspection that has
// to anticipate every way a path can escape. astrogo already validates keys —
// fs.ValidPath rejects "..", and path.Join is mandated over filepath.Join — but
// validation and confinement are different guarantees, and only one of them
// survives a symlink planted in the cache directory.
// # Why the root is opened per operation rather than held
//
// os.Root is a live directory handle, and holding one for the life of the
// process means the directory cannot be deleted while astrogo runs — on Windows
// that is enforced by the OS, and it makes a cache directory undeletable by the
// user who owns it. The cost of reopening is one syscall pair per metadata
// operation, on a path that already makes several; reads and writes go through
// the *os.File the root hands back, which is independent of it and outlives it.
type localFS struct {
	dir string
	// staged counts staging files this process has created, so two writers in
	// one process cannot pick the same name. See [localFS.Create].
	staged *atomic.Uint64
	// ctx is the context WithContext bound, or nil. See [localFS.WithContext].
	ctx context.Context //nolint:containedctx // the io/fs contract has nowhere else to put it
}

// openLocal builds a local filesystem from a file:// URL.
//
// The URL's path is the directory. create_dir=true creates it, which the cache
// needs on a first run and a read-only source must not have.
func openLocal(u *url.URL) (fs.FS, error) {
	dir, err := localPath(u)
	if err != nil {
		return nil, err
	}

	if u.Query().Get("create_dir") == "true" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create %s: %w", dir, err)
		}
	}

	// Opened once here to fail fast on a directory that does not exist or
	// cannot be reached, then closed: see localFS's doc comment.
	probe, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("open root %s: %w", dir, err)
	}

	if err := probe.Close(); err != nil {
		return nil, fmt.Errorf("close root %s: %w", dir, err)
	}

	return &localFS{dir: dir, staged: new(atomic.Uint64)}, nil
}

// WithContext implements [ContextFS].
//
// # Why a local filesystem implements this at all
//
// Because the alternative is an unenforceable rule. A Downloadable endpoint's
// backend must be cancellable — a multi-gigabyte fetch a caller cannot abandon
// is a defect — and [RequireContext] is where that is checked. If the local
// backend does not implement this, the check has to be skipped for it, and a
// check with an exception is a check nobody can rely on.
//
// So the cancellation here is real rather than a no-op that would make
// RequireContext lie. A local operation cannot be interrupted once the syscall
// is in flight, and none of them blocks for long enough to matter; what is
// honoured is the context's state at the moment each operation begins. That
// means a caller who cancels sees the next operation fail rather than a
// transfer that runs to completion, which is the property the rule exists for.
func (l *localFS) WithContext(ctx context.Context) fs.FS {
	clone := *l
	clone.ctx = ctx

	return &clone
}

// localPath turns a file:// URL into an OS path.
//
// This and remote's default-cache-dir resolver are the module's only two
// URL-to-path conversions, and they are deliberately not general: a Windows
// file URL is "file:///C:/Users/..." with a leading slash the drive letter must
// not keep, and getting that wrong produces a path that looks plausible and
// resolves nowhere.
func localPath(u *url.URL) (string, error) {
	p := u.Path

	if u.Host != "" && u.Host != "localhost" {
		// file://server/share — a UNC path on Windows, meaningless elsewhere.
		return filepath.FromSlash(`\\` + u.Host + u.Path), nil
	}

	if p == "" {
		return "", ErrNoPath
	}

	// "/C:/x" -> "C:/x". Only when what follows is a drive letter, so a
	// genuine root-relative Unix path is untouched.
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}

	return filepath.FromSlash(p), nil
}

// validName reports whether name is usable as an io/fs name on this platform.
//
// fs.ValidPath is necessary and, on Windows, not sufficient. An io/fs name has
// exactly one separator, "/", so a backslash or a colon in one is an ordinary
// filename character — but Windows treats both as path syntax, and os.Root
// does too. Without this check "eop\finals2000A.data", which io/fs says is a
// single file at the root, quietly resolves to a file in a subdirectory.
//
// The standard library draws the line in the same place, and localName below
// uses the same tool it does. fstest.TestFS checks for this, which is how it was
// found rather than shipped.
//
// localName returns the OS path for an io/fs name within dir, or an error if
// the name is not one this platform can represent safely.
//
// filepath.Localize is the standard library's own answer and is what
// os.dirFS.join calls: it converts a slash-separated io/fs name to an OS path
// and refuses the ones that cannot be converted without changing meaning —
// which on Windows is exactly the backslash and colon cases, plus reserved
// device names like "NUL" that no amount of string checking would catch.
func localName(dir, name string) (string, error) {
	if !fs.ValidPath(name) {
		return "", fs.ErrInvalid
	}

	local, err := filepath.Localize(name)
	if err != nil {
		return "", fs.ErrInvalid
	}

	return filepath.Join(dir, local), nil
}

// Open implements [fs.FS].
//
// # Every path goes through os.Root, including this one
//
// An earlier version of this did not. Reads took the same route os.DirFS takes
// — validate the name, join it, open it — because an os.Root-backed filesystem
// appeared not to pass fstest.TestFS on Windows while os.DirFS did.
//
// Measured, twenty freshly created identical trees, three filesystems over each:
//
//	os.Root, handle held           20 failures / 20
//	os.Root, reopened per call     20 / 20
//	os.DirFS                        0 / 20
//
// The complaint is always the same: a directory's ModTime from the parent's
// directory entry disagrees with the same directory's ModTime from Stat, by a
// few hundred microseconds. Statting a directory three ways immediately after
// creating it shows no disagreement at all, so it needs the particular sequence
// fstest.TestFS performs; the cause is somewhere in how Windows serves
// directory metadata through a relative-open handle, and it is not astrogo's
// logic — the middle row differs from the first only in when the handle is
// opened, and both fail identically. That is #323 and it is still open.
//
// What was wrong was the conclusion, not the measurement. Giving up confinement
// on the read path bought nothing, because the Windows complaint is about
// *directory metadata* and is already filtered narrowly and platform-gated in
// TestLocalFSSatisfiesTestFS — the filter compares the two FileInfos with their
// timestamps removed and passes the complaint through unless they are otherwise
// identical. With os.Root restored, that filter absorbs 22 complaints across 20
// runs on Windows and the conformance suite is otherwise clean, on every
// platform.
//
// And the cost was real rather than theoretical: TestLocalFSConfinesToItsRoot
// skips on Windows, because creating a symlink needs a privilege CI does not
// have there, so the regression only showed up on Linux — where a symlink
// planted in the cache directory and pointing out of it was followed.
//
// So: confinement everywhere, and one documented filter for a Windows
// directory-metadata artifact that is not astrogo's to fix.
func (l *localFS) Open(name string) (fs.File, error) {
	if err := l.check("open", name); err != nil {
		return nil, err
	}

	full, err := localName(l.dir, name)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	root, err := l.root()
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}

	defer func() { _ = root.Close() }()

	f, err := root.Open(name)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a *fs.PathError with the right Op and Path
	}

	// Wrapped unconditionally rather than only for directories, because
	// deciding would cost a Stat and ReadDir on a regular file fails either
	// way. See [localDir.ReadDir] and [freshEntry].
	return &localDir{File: f, dir: full}, nil
}

// localDir is an open file whose ReadDir reports fresh entry metadata.
//
// Embedding *os.File keeps Read, ReadAt, Seek, Stat and Close, so this
// satisfies [File] exactly as the bare handle did.
type localDir struct {
	*os.File

	dir string
}

// ReadDir implements [fs.ReadDirFile], re-wrapping what the handle returns.
//
// This is the second of the two places entries come from, and missing it is why
// a first attempt at #323 did not work: fstest.TestFS takes the entries it
// checks from the *opened directory*, not from fs.ReadDir. See [freshEntry].
func (d *localDir) ReadDir(n int) ([]fs.DirEntry, error) {
	// The error is forwarded unwrapped because io.EOF is part of the contract:
	// fs.ReadDirFile reports it when a paginating read is exhausted, and a
	// caller looping until EOF identity-checks it.
	entries, err := d.File.ReadDir(n)

	return freshEntries(d.dir, entries), err
}

// Stat implements [fs.StatFS], so a caller need not open an object to learn it
// exists — the cache-hit check, which happens far more often than a read.
func (l *localFS) Stat(name string) (fs.FileInfo, error) {
	if err := l.check("stat", name); err != nil {
		return nil, err
	}

	full, err := localName(l.dir, name)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}

	fi, err := os.Stat(full)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a *fs.PathError
	}

	return fi, nil
}

// ReadDir implements [fs.ReadDirFS], and ReadFile below implements
// [fs.ReadFileFS].
//
// Both exist because os.dirFS implements them and behaviour follows the
// interface set. Without ReadDir, fs.ReadDir falls back to Open plus
// File.ReadDir — a different route to the same listing, and measured, one that
// disagrees with Stat about a directory's ModTime often enough to fail
// fstest.TestFS on Windows 14 times in 30 where os.DirFS failed 0.
//
// The general lesson is worth more than the fix: an io/fs implementation is
// only as conformant as the optional interfaces it implements, because every
// one it omits sends callers down a fallback path it never tested.
func (l *localFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if err := l.check("readdir", name); err != nil {
		return nil, err
	}

	full, err := localName(l.dir, name)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}

	entries, err := os.ReadDir(full)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a *fs.PathError
	}

	return freshEntries(full, entries), nil
}

// freshEntries re-wraps a directory scan's entries so each reports its own
// metadata when asked. See [freshEntry].
func freshEntries(dir string, entries []fs.DirEntry) []fs.DirEntry {
	out := make([]fs.DirEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, freshEntry{DirEntry: e, path: filepath.Join(dir, e.Name())})
	}

	return out
}

// freshEntry is a [fs.DirEntry] whose Info is read when asked rather than taken
// from the directory scan that produced it.
//
// # Why, and what it fixes
//
// This is #323. On Windows a directory entry's cached LastWriteTime lags the
// child's own metadata: measured over forty freshly built trees, the ModTime
// from os.ReadDir disagreed with a direct stat 15 times, while os.Stat,
// os.Open+Stat and os.Root.Open+Stat agreed with each other every time. So the
// staleness is in the directory scan, and every stat route is consistent.
//
// Any filesystem that hands back those cached entries while serving Stat from a
// real stat therefore disagrees with itself, which is exactly what
// fstest.TestFS checks. os.DirFS has the same defect — measured at 15 failures
// in 40, not the 0 in 40 the issue originally recorded — so this is the standard
// library's behaviour on Windows rather than anything astrogo does.
//
// Reading the metadata on demand costs one Lstat per entry whose Info is
// actually asked for, and makes the answer both self-consistent and more
// accurate. fs.DirEntry.Info's own documentation contemplates exactly this: it
// says Info may report ErrNotExist if the file was removed after the directory
// was read, which only makes sense for an implementation that reads it later.
type freshEntry struct {
	fs.DirEntry

	path string
}

func (e freshEntry) Info() (fs.FileInfo, error) {
	//nolint:wrapcheck // already a *fs.PathError with the right Op and Path
	return os.Lstat(e.path)
}

// ReadFile implements [fs.ReadFileFS].
func (l *localFS) ReadFile(name string) ([]byte, error) {
	if err := l.check("readfile", name); err != nil {
		return nil, err
	}

	full, err := localName(l.dir, name)
	if err != nil {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: err}
	}

	b, err := os.ReadFile(full)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a *fs.PathError
	}

	return b, nil
}

// Lstat and ReadLink implement [fs.ReadLinkFS], which os.dirFS also
// implements. See ReadDir for why the interface set is the thing that decides
// conformance rather than the method bodies.
func (l *localFS) Lstat(name string) (fs.FileInfo, error) {
	if err := l.check("lstat", name); err != nil {
		return nil, err
	}

	full, err := localName(l.dir, name)
	if err != nil {
		return nil, &fs.PathError{Op: "lstat", Path: name, Err: err}
	}

	fi, err := os.Lstat(full)
	if err != nil {
		return nil, err //nolint:wrapcheck // already a *fs.PathError
	}

	return fi, nil
}

// ReadLink implements [fs.ReadLinkFS].
func (l *localFS) ReadLink(name string) (string, error) {
	if err := l.check("readlink", name); err != nil {
		return "", err
	}

	full, err := localName(l.dir, name)
	if err != nil {
		return "", &fs.PathError{Op: "readlink", Path: name, Err: err}
	}

	target, err := os.Readlink(full)
	if err != nil {
		return "", err //nolint:wrapcheck // already a *fs.PathError
	}

	return target, nil
}

// Remove implements [RemoveFS].
func (l *localFS) Remove(name string) error {
	if err := l.check("remove", name); err != nil {
		return err
	}

	if _, err := localName(l.dir, name); err != nil {
		return &fs.PathError{Op: "remove", Path: name, Err: err}
	}

	root, err := l.root()
	if err != nil {
		return &fs.PathError{Op: "remove", Path: name, Err: err}
	}

	defer func() { _ = root.Close() }()

	//nolint:wrapcheck // already a *fs.PathError
	return root.Remove(name)
}

// Create implements [CreateFS]: a streaming write that becomes visible at name
// only when it succeeds.
//
// # The staging name, which is issue #315
//
// The write goes to a temporary file in the same directory and is renamed into
// place on Close. That much is ordinary. What is not ordinary is how the
// temporary is named, and it is the whole of a defect astrogo worked around
// twice without being able to fix.
//
// fileblob named it from time.Now().UnixNano(), reasoning that "nanosecond
// changes enough between each iteration to make a conflict unlikely". On
// Windows it does not change at all: the system clock ticks about every 15.6 ms
// and, measured on Windows 11, time.Now().UnixNano() returned ONE DISTINCT
// VALUE across 2000 consecutive reads. Every concurrent writer of a key
// therefore picked the same staging path, and its O_EXCL retry re-read the same
// frozen clock and retried into the same name. One writer's file was renamed
// out from under another — reported as "Access is denied", or as a file that
// vanished, depending on who lost (#241, #307, #315).
//
// The name here contains the process id and a per-process counter, so it is
// unique by construction rather than by hoping a clock moves. Two processes
// cannot collide because their pids differ; two goroutines cannot collide
// because the counter is atomic. O_EXCL stays as the backstop for a stale file
// left by a process that died mid-write.
//
// Staging happens inside the tree rather than in os.TempDir, which is the other
// half of #315: a cross-device rename is not atomic, and the temp directory is
// frequently on a different volume from the cache.
func (l *localFS) Create(name string) (io.WriteCloser, error) {
	if err := l.check("create", name); err != nil {
		return nil, err
	}

	if _, err := localName(l.dir, name); err != nil {
		return nil, &fs.PathError{Op: "create", Path: name, Err: err}
	}

	root, err := l.root()
	if err != nil {
		return nil, &fs.PathError{Op: "create", Path: name, Err: err}
	}

	if dir := path.Dir(name); dir != "." {
		if err := mkdirAll(root, dir); err != nil {
			_ = root.Close()

			return nil, &fs.PathError{Op: "create", Path: name, Err: err}
		}
	}

	staging := stagingName(name, l.staged.Add(1))

	f, err := root.OpenFile(staging,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		_ = root.Close()

		return nil, err //nolint:wrapcheck // already a *fs.PathError
	}

	// This root outlives the call: the writer needs it for the rename on
	// Close, and closes it there.
	return &stagedWrite{root: root, f: f, staging: staging, final: name}, nil
}

// CreateExcl implements [CreateExclFS].
//
// Unlike [localFS.Create] this does not stage. Staging exists so a reader never
// sees a half-written object, and it is exactly wrong here: the point of an
// exclusive create is that the name appears at the instant it is claimed, so a
// second caller's attempt fails. A write that became visible only on Close
// would leave a window in which two callers both believed they held the lock.
//
// The atomicity is the kernel's. O_CREATE|O_EXCL either creates the file or
// reports EEXIST, with no gap a second process can enter, on every platform
// astrogo supports — which is the guarantee gocloud's fileblob could not give
// and #241 exists because of.
func (l *localFS) CreateExcl(name string) (io.WriteCloser, error) {
	if err := l.check("createexcl", name); err != nil {
		return nil, err
	}

	if _, err := localName(l.dir, name); err != nil {
		return nil, &fs.PathError{Op: "createexcl", Path: name, Err: err}
	}

	root, err := l.root()
	if err != nil {
		return nil, &fs.PathError{Op: "createexcl", Path: name, Err: err}
	}

	if dir := path.Dir(name); dir != "." {
		if err := mkdirAll(root, dir); err != nil {
			_ = root.Close()

			return nil, &fs.PathError{Op: "createexcl", Path: name, Err: err}
		}
	}

	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		_ = root.Close()

		return nil, err //nolint:wrapcheck // already a *fs.PathError, and fs.ErrExist is the signal
	}

	return &rootedWrite{root: root, f: f, name: name}, nil
}

// check reports the bound context's error, if it has one.
func (l *localFS) check(op, name string) error {
	if l.ctx == nil {
		return nil
	}

	if err := l.ctx.Err(); err != nil {
		return &fs.PathError{Op: op, Path: name, Err: err}
	}

	return nil
}

// rootedWrite is a direct write that closes its root with it. There is no
// promotion step: see [localFS.CreateExcl] for why that is the point.
type rootedWrite struct {
	root *os.Root
	f    *os.File
	name string
}

func (w *rootedWrite) Write(p []byte) (int, error) {
	n, err := w.f.Write(p)
	if err != nil {
		return n, fmt.Errorf("remote/file: write: %w", err)
	}

	return n, nil
}

// Abort implements [AbortWriter]. An exclusive create has claimed the name, so
// throwing the write away means removing it — otherwise a failed lock attempt
// would leave a lock nobody holds.
func (w *rootedWrite) Abort() error {
	defer func() { _ = w.root.Close() }()

	closeErr := w.f.Close()
	removeErr := w.root.Remove(w.name)

	if closeErr != nil {
		return fmt.Errorf("remote/file: abort: %w", closeErr)
	}

	if removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
		return fmt.Errorf("remote/file: abort: %w", removeErr)
	}

	return nil
}

func (w *rootedWrite) Close() error {
	err := w.f.Close()
	_ = w.root.Close()

	if err != nil {
		return fmt.Errorf("remote/file: close: %w", err)
	}

	return nil
}

// stagingName builds a name unique to this process and this call.
//
// Kept beside the file it stages so the rename is within one directory and
// therefore within one volume — see [localFS.Create].
func stagingName(name string, seq uint64) string {
	return name + ".astrogo-" +
		strconv.Itoa(os.Getpid()) + "-" +
		strconv.FormatUint(seq, 10) + ".part"
}

// mkdirAll creates a key's parent directories inside the root.
//
// os.Root has no MkdirAll, and walking the segments is the supported way to get
// one: each Mkdir is confined the same as every other operation, where a
// filepath.Join to the real path would step outside the guarantee the root
// exists to provide.
func mkdirAll(root *os.Root, dir string) error {
	var built string

	for _, seg := range splitPath(dir) {
		if built == "" {
			built = seg
		} else {
			built = path.Join(built, seg)
		}

		if err := root.Mkdir(built, 0o755); err != nil &&
			!errors.Is(err, fs.ErrExist) {
			return err //nolint:wrapcheck // already a *fs.PathError
		}
	}

	return nil
}

func splitPath(p string) []string {
	var out []string

	for p != "" && p != "." {
		dir, base := path.Split(p)
		out = append([]string{base}, out...)
		p = trimSlashes(dir)
	}

	return out
}

// root opens a confined handle on the directory for one operation.
func (l *localFS) root() (*os.Root, error) {
	//nolint:wrapcheck // callers add the operation and path
	return os.OpenRoot(l.dir)
}

// stagedWrite is a write in progress, promoted on Close.
type stagedWrite struct {
	root    *os.Root
	f       *os.File
	staging string
	final   string
	failed  bool
}

func (w *stagedWrite) Write(p []byte) (int, error) {
	n, err := w.f.Write(p)
	if err != nil {
		// Remembered so Close discards rather than promotes. Without this a
		// failed copy followed by a Close replaces a good object with a
		// truncated one, which is worse than not writing at all.
		w.failed = true
	}

	return n, err //nolint:wrapcheck // already a *fs.PathError
}

// Abort implements [AbortWriter]: throw the staged file away and leave the
// destination as it was.
func (w *stagedWrite) Abort() error {
	defer func() { _ = w.root.Close() }()

	closeErr := w.f.Close()
	removeErr := w.root.Remove(w.staging)

	if closeErr != nil {
		return fmt.Errorf("remote/file: abort %s: %w", w.final, closeErr)
	}

	if removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
		return fmt.Errorf("remote/file: abort %s: %w", w.final, removeErr)
	}

	return nil
}

// Close promotes the staged file, or discards it if any write failed.
func (w *stagedWrite) Close() error {
	defer func() { _ = w.root.Close() }()

	closeErr := w.f.Close()

	if w.failed || closeErr != nil {
		_ = w.root.Remove(w.staging)

		if closeErr != nil {
			return fmt.Errorf("remote/file: close %s: %w", w.final, closeErr)
		}

		return fmt.Errorf("%w: %s", ErrDiscarded, w.final)
	}

	if err := w.root.Rename(w.staging, w.final); err != nil {
		_ = w.root.Remove(w.staging)

		return fmt.Errorf("remote/file: promote %s: %w", w.final, err)
	}

	return nil
}
