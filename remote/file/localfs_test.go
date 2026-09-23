package file_test

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/fstest"

	"github.com/TuSKan/astrogo/remote/file"
)

// errPrivilegeNotHeld is Windows' ERROR_PRIVILEGE_NOT_HELD, which is what
// os.Symlink returns for an account outside developer mode.
//
// Written as a number because there is no portable name for it: golang.org/x/sys
// has one and is not a dependency here. Worth the number rather than a broader
// predicate, because the broader one does not work — measured, errors.Is against
// fs.ErrPermission is false for this errno, so a test written the obvious way
// would have skipped on every symlink failure while appearing to check for one.
const errPrivilegeNotHeld = syscall.Errno(1314)

// localURL builds a file:// URL for dir, the way remote's own resolver does.
func localURL(t *testing.T, dir string) string {
	t.Helper()

	slash := filepath.ToSlash(dir)
	if slash == "" || slash[0] != '/' {
		slash = "/" + slash // Windows drive-letter paths are not "/"-rooted
	}

	return "file://" + slash + "?create_dir=true"
}

// TestLocalFSSatisfiesTestFS is the acceptance bar for any backend here.
//
// fstest.TestFS is strict in ways a hand-written suite is not: it checks that
// Open refuses invalid paths, that ReadDir is sorted and consistent with Stat,
// that Glob and Sub agree with a walk, that opening a directory works, and that
// a file's content is the same read twice. A backend that passes it behaves
// like every other filesystem in Go, which is the entire reason for building on
// io/fs rather than on an interface only astrogo implements.
func TestLocalFSSatisfiesTestFS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	want := map[string]string{
		"kernels/de440s.bsp":   "not really a kernel",
		"kernels/naif0012.tls": "leap seconds",
		"eop/finals2000A.data": "earth orientation",
		"top.txt":              "at the root",
	}

	for name, body := range want {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	fsys, err := file.OpenFS(localURL(t, dir))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}

	// fstest.TestFS directly, with nothing filtered.
	//
	// This used to run behind a filter that let one complaint through: a
	// Windows discrepancy between a directory's ModTime as the parent's
	// directory scan reported it and as a stat reported it. That was #323, and
	// it is fixed at the source rather than tolerated — see [freshEntry] in
	// localfs.go. Measured before and after over forty freshly built trees:
	// 15 failures in 40 before, 0 in 40 after.
	if err := fstest.TestFS(fsys, names...); err != nil {
		t.Errorf("fstest.TestFS: %v", err)
	}
}

// TestLocalFSWriteRoundTrip covers the three interfaces astrogo adds to io/fs.
func TestLocalFSWriteRoundTrip(t *testing.T) {
	t.Parallel()

	fsys, err := file.OpenFS(localURL(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	cfs, ok := fsys.(file.CreateFS)
	if !ok {
		t.Fatal("the local filesystem does not implement CreateFS, so the cache cannot be written")
	}

	const (
		name = "jpl/planets/de440s.bsp"
		body = "kernel bytes"
	)

	w, err := cfs.Create(name)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Nothing is visible until Close: a reader that arrives mid-write must see
	// no object rather than half of one.
	if _, err := fs.Stat(fsys, name); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the object is visible before Close (%v); staging is not doing its job", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := fs.ReadFile(fsys, name)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != body {
		t.Errorf("read back %q, wrote %q", got, body)
	}

	// And no staging file is left behind.
	entries, err := fs.ReadDir(fsys, "jpl/planets")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if strings.Contains(e.Name(), ".astrogo-") {
			t.Errorf("a staging file survived promotion: %s", e.Name())
		}
	}

	rfs, ok := fsys.(file.RemoveFS)
	if !ok {
		t.Fatal("the local filesystem does not implement RemoveFS")
	}

	if err := rfs.Remove(name); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := fs.Stat(fsys, name); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after Remove, Stat returned %v, want fs.ErrNotExist", err)
	}
}

// TestFailedWriteLeavesTheObjectAlone is the property that makes staging worth
// having at all.
//
// The old layer had to skip Close after a failed copy, because gocloud's writer
// promoted regardless of whether an earlier Write had failed — so closing after
// a failure replaced a good object with a truncated one, and the workaround was
// to leak the temporary instead. Here Close is always safe to call: it promotes
// on success and discards on failure.
func TestFailedWriteLeavesTheObjectAlone(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	fsys, err := file.OpenFS(localURL(t, dir))
	if err != nil {
		t.Fatal(err)
	}

	cfs := fsys.(file.CreateFS) //nolint:forcetypeassert // asserted by the test above

	const name = "cache/object.dat"

	// A good object first.
	w, err := cfs.Create(name)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := io.WriteString(w, "the good one"); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// Now a write that fails partway. Closing the underlying file out from
	// under the writer is the most direct way to make Write fail without
	// reaching into unexported state.
	w2, err := cfs.Create(name)
	if err != nil {
		t.Fatal(err)
	}

	if f, ok := w2.(interface{ Write([]byte) (int, error) }); ok {
		_, _ = f.Write([]byte("partial"))
	}

	// Remove the staging file underneath, so the promotion cannot succeed.
	staged, _ := filepath.Glob(filepath.Join(dir, "cache", "*.astrogo-*"))
	for _, s := range staged {
		_ = os.Remove(s)
	}

	if err := w2.Close(); err == nil {
		t.Error("Close reported success after its staging file had vanished")
	}

	got, err := fs.ReadFile(fsys, name)
	if err != nil {
		t.Fatalf("the original object is gone after a failed write: %v", err)
	}

	if string(got) != "the good one" {
		t.Errorf("the original object was replaced by a failed write: %q", got)
	}
}

// TestConcurrentWritersInOneProcess is half of #315.
//
// fileblob named its staging file from time.Now().UnixNano(), and on Windows
// that clock does not advance — one distinct value across 2000 consecutive
// reads, measured. Every concurrent writer picked the same path and its O_EXCL
// retry re-read the same frozen clock, so one writer's file was renamed out
// from under another. astrogo serialised writers in-process to work around it.
//
// The staging name here carries a per-process atomic counter, so this needs no
// lock. Eight goroutines, forty rounds, is the shape that reproduced the
// original failure 51 times in 320.
func TestConcurrentWritersInOneProcess(t *testing.T) {
	t.Parallel()

	fsys, err := file.OpenFS(localURL(t, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	cfs := fsys.(file.CreateFS) //nolint:forcetypeassert // asserted above

	const (
		writers = 8
		rounds  = 40
		name    = "shared/object.dat"
	)

	for round := range rounds {
		var (
			wg   sync.WaitGroup
			errs = make([]error, writers)
		)

		for i := range writers {
			wg.Add(1)

			go func(i int) {
				defer wg.Done()

				w, cerr := cfs.Create(name)
				if cerr != nil {
					errs[i] = cerr

					return
				}

				if _, werr := io.WriteString(w, "written by everyone"); werr != nil {
					errs[i] = werr

					return
				}

				errs[i] = w.Close()
			}(i)
		}

		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, writer %d: %v.\n"+
					"  Concurrent writers of one key must not interfere. If the staging name "+
					"has gone back to being derived from a clock, this is #315 again.",
					round, i, err)
			}
		}

		// And the object is whole, not a mixture.
		got, rerr := fs.ReadFile(fsys, name)
		if rerr != nil {
			t.Fatalf("round %d: %v", round, rerr)
		}

		if string(got) != "written by everyone" {
			t.Fatalf("round %d: object is %q", round, got)
		}
	}
}

// TestConcurrentWritersAcrossProcesses is the other half of #315, and the half
// no in-process lock could ever reach.
//
// remote/file/writelock.go's own doc comment states the boundary: "Two
// processes writing the same key into the same bucket directory. That is what
// remote's cross-process lock object is for, and it is a separate mechanism
// with its own limits." The staging collision was underneath that mechanism,
// which is why it survived two attempts to fix it.
//
// This runs the test binary again as a child, twice, both writing one key.
func TestConcurrentWritersAcrossProcesses(t *testing.T) {
	if dir := os.Getenv("ASTROGO_STAGING_CHILD"); dir != "" {
		stagingChild(t, dir)

		return
	}

	t.Parallel()

	dir := t.TempDir()

	const children = 4

	var (
		wg   sync.WaitGroup
		out  = make([][]byte, children)
		errs = make([]error, children)
	)

	for i := range children {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			cmd := exec.CommandContext(t.Context(), os.Args[0],
				"-test.run=TestConcurrentWritersAcrossProcesses", "-test.v")

			cmd.Env = append(os.Environ(), "ASTROGO_STAGING_CHILD="+dir)

			out[i], errs[i] = cmd.CombinedOutput()
		}(i)
	}

	wg.Wait()

	for i := range children {
		if errs[i] != nil {
			t.Errorf("child %d failed: %v\n%s", i, errs[i], out[i])
		}
	}

	fsys, err := file.OpenFS(localURL(t, dir))
	if err != nil {
		t.Fatal(err)
	}

	got, err := fs.ReadFile(fsys, "shared/object.dat")
	if err != nil {
		t.Fatalf("after %d concurrent processes the object is unreadable: %v", children, err)
	}

	if string(got) != "written by everyone" {
		t.Errorf("the object is %q, so one process promoted a partial write", got)
	}

	// No staging file left behind by any of them.
	entries, err := fs.ReadDir(fsys, "shared")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if strings.Contains(e.Name(), ".astrogo-") {
			t.Errorf("a staging file survived: %s", e.Name())
		}
	}
}

// stagingChild is the body the re-executed test binary runs.
func stagingChild(t *testing.T, dir string) {
	t.Helper()

	fsys, err := file.OpenFS(localURL(t, dir))
	if err != nil {
		t.Fatal(err)
	}

	cfs, ok := fsys.(file.CreateFS)
	if !ok {
		t.Fatal("no CreateFS")
	}

	for range 25 {
		w, cerr := cfs.Create("shared/object.dat")
		if cerr != nil {
			t.Fatalf("create: %v", cerr)
		}

		if _, werr := io.WriteString(w, "written by everyone"); werr != nil {
			t.Fatalf("write: %v", werr)
		}

		if cerr := w.Close(); cerr != nil {
			t.Fatalf("close: %v", cerr)
		}
	}
}

// TestLocalFSConfinesToItsRoot is what os.Root buys over a path prefix.
//
// fs.ValidPath already refuses "..", so the string cases below are belt and
// braces. The one that matters is the symlink: a link planted inside the cache
// directory pointing outside it defeats every check that works on the name
// alone, and is refused here by the kernel-side confinement instead.
func TestLocalFSConfinesToItsRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	outside := t.TempDir()

	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("not yours"), 0o644); err != nil {
		t.Fatal(err)
	}

	fsys, err := file.OpenFS(localURL(t, dir))
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{
		"../secret.txt",
		"a/../../secret.txt",
		"/etc/passwd",
		"",
	} {
		if _, err := fsys.Open(name); err == nil {
			t.Errorf("Open(%q) succeeded", name)
		}
	}

	// The symlink case. Not every platform lets an unprivileged process create
	// one — Windows without developer mode — so a failure to set it up is a
	// skip rather than a pass.
	link := filepath.Join(dir, "escape")
	if err := os.Symlink(outside, link); err != nil {
		// Only the one failure that means this account may not make symlinks.
		// Measured on Windows: os.Symlink returns ERROR_PRIVILEGE_NOT_HELD, and
		// errors.Is against fs.ErrPermission is FALSE for it, so the obvious
		// predicate would never have fired. fs.ErrPermission is kept beside it
		// for the platforms where that is the answer. Anything else — a missing
		// target, a name already taken, a full disk — is this test's own setup
		// being wrong, which is worth seeing.
		if errors.Is(err, errPrivilegeNotHeld) || errors.Is(err, fs.ErrPermission) {
			t.Skipf("this account cannot create a symlink: %v", err)
		}

		t.Fatalf("os.Symlink: %v", err)
	}

	if _, err := fsys.Open("escape/secret.txt"); err == nil {
		t.Error("a symlink out of the root was followed; os.Root is not confining")
	}
}

// TestLstatAndReadLinkSeeTheLinkItself covers [fs.ReadLinkFS], which the local
// backend implements and nothing else here does.
//
// The distinction is the whole point of the interface: Stat follows a symlink
// and reports the target, Lstat reports the link. A cache that cannot tell them
// apart cannot notice that one of its entries has been replaced by a pointer
// somewhere else — which is the same threat os.Root confinement addresses from
// the other side, and why both exist.
func TestLstatAndReadLinkSeeTheLinkItself(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	const (
		target  = "kernel.bsp"
		link    = "latest.bsp"
		content = "de440s bytes"
	)

	if err := os.WriteFile(filepath.Join(dir, target), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// Not every platform lets an unprivileged process create a symlink —
	// Windows without developer mode — so a failure to set it up is a skip
	// rather than a pass.
	if err := os.Symlink(filepath.Join(dir, target), filepath.Join(dir, link)); err != nil {
		// The same two conditions, and the same reasoning, as the symlink set
		// up in TestLocalFSConfinesToItsRoot above.
		if errors.Is(err, errPrivilegeNotHeld) || errors.Is(err, fs.ErrPermission) {
			t.Skipf("this account cannot create a symlink: %v", err)
		}

		t.Fatalf("os.Symlink: %v", err)
	}

	fsys, err := file.OpenFS(localURL(t, dir))
	if err != nil {
		t.Fatal(err)
	}

	rlFS, ok := fsys.(fs.ReadLinkFS)
	if !ok {
		t.Fatal("the local backend does not implement fs.ReadLinkFS")
	}

	got, err := rlFS.ReadLink(link)
	if err != nil {
		t.Fatalf("ReadLink: %v", err)
	}

	if filepath.Base(got) != target {
		t.Errorf("ReadLink(%q) = %q, want it to point at %q", link, got, target)
	}

	// Lstat describes the link.
	li, err := rlFS.Lstat(link)
	if err != nil {
		t.Fatalf("Lstat: %v", err)
	}

	if li.Mode()&fs.ModeSymlink == 0 {
		t.Errorf("Lstat(%q).Mode() = %v, want the symlink bit set", link, li.Mode())
	}

	// Stat follows it, so it describes the target and the two disagree.
	si, err := fs.Stat(fsys, link)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if si.Mode()&fs.ModeSymlink != 0 {
		t.Errorf("Stat(%q) reports a symlink; it should have followed the link", link)
	}

	if si.Size() != int64(len(content)) {
		t.Errorf("Stat(%q).Size() = %d, want the target's %d", link, si.Size(), len(content))
	}
}

// TestReadLinkRefusesWhatIsNotALink pins the error, because "not a link" and
// "not there" are different answers and a caller walking a cache has to tell
// them apart.
func TestReadLinkRefusesWhatIsNotALink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "plain.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	fsys, err := file.OpenFS(localURL(t, dir))
	if err != nil {
		t.Fatal(err)
	}

	rlFS, ok := fsys.(fs.ReadLinkFS)
	if !ok {
		t.Fatal("the local backend does not implement fs.ReadLinkFS")
	}

	if _, err := rlFS.ReadLink("plain.txt"); err == nil {
		t.Error("ReadLink on an ordinary file succeeded")
	}

	if _, err := rlFS.ReadLink("not-there.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadLink on a missing name gave %v, want fs.ErrNotExist", err)
	}

	// And an unrepresentable name is refused before it reaches the OS.
	if _, err := rlFS.ReadLink("../escape"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("ReadLink(%q) gave %v, want fs.ErrInvalid", "../escape", err)
	}
}

// TestAbortThrowsTheWriteAway covers [file.AbortWriter], which exists because
// an io.WriteCloser's Close cannot tell a finished write from an abandoned one.
//
// Two writers, two different obligations:
//
//   - A staged write (Create) commits on Close. Abort must leave whatever was
//     at the name untouched and remove the staging file, so a failed download
//     never replaces a good kernel with a truncated one.
//   - An exclusive create (CreateExcl) has already claimed the name at the
//     instant it succeeded. Abort must remove it, or an abandoned lock attempt
//     leaves a lock nobody holds and every later acquirer waits out staleLockAge.
//
// Both also have to release their handles. That is not decorative: skipping
// Close was the old way to avoid committing a partial write, and on Windows the
// resulting unclosed handle keeps the directory undeletable — which is how this
// surfaced, as a t.TempDir cleanup failure rather than as a wrong answer.
func TestAbortThrowsTheWriteAway(t *testing.T) {
	t.Parallel()

	t.Run("a staged write leaves the object alone", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		fsys, err := file.OpenFS(localURL(t, dir))
		if err != nil {
			t.Fatal(err)
		}

		if err := file.WriteFile(t.Context(), fsys, "kernel.bsp",
			strings.NewReader("original")); err != nil {
			t.Fatal(err)
		}

		cfs, ok := fsys.(file.CreateFS)
		if !ok {
			t.Fatal("the local backend does not implement CreateFS")
		}

		w, err := cfs.Create("kernel.bsp")
		if err != nil {
			t.Fatal(err)
		}

		aw, ok := w.(file.AbortWriter)
		if !ok {
			t.Fatalf("%T does not implement AbortWriter, so a failed copy must leak to "+
				"avoid corrupting the object", w)
		}

		if _, err := io.WriteString(w, "half a new kernel"); err != nil {
			t.Fatal(err)
		}

		if err := aw.Abort(); err != nil {
			t.Fatalf("Abort: %v", err)
		}

		got, err := fs.ReadFile(fsys, "kernel.bsp")
		if err != nil {
			t.Fatalf("ReadFile after Abort: %v", err)
		}

		if string(got) != "original" {
			t.Errorf("after Abort the object is %q, want the untouched %q", got, "original")
		}

		// And nothing is left beside it. A staging file that outlived its
		// writer would be both a leaked handle and a puzzle in the user's cache.
		entries, err := fs.ReadDir(fsys, ".")
		if err != nil {
			t.Fatal(err)
		}

		if len(entries) != 1 {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				names = append(names, e.Name())
			}

			t.Errorf("after Abort the directory holds %v, want only the object", names)
		}
	})

	t.Run("an exclusive create gives the name back", func(t *testing.T) {
		t.Parallel()

		fsys, err := file.OpenFS(localURL(t, t.TempDir()))
		if err != nil {
			t.Fatal(err)
		}

		xfs, ok := fsys.(file.CreateExclFS)
		if !ok {
			t.Fatal("the local backend does not implement CreateExclFS")
		}

		w, err := xfs.CreateExcl("cache.lock")
		if err != nil {
			t.Fatal(err)
		}

		// While it is held, a second attempt must lose — that is the whole
		// guarantee, and the reason the name has to come back on Abort.
		if _, err := xfs.CreateExcl("cache.lock"); !errors.Is(err, fs.ErrExist) {
			t.Errorf("a second CreateExcl gave %v, want fs.ErrExist", err)
		}

		if _, err := io.WriteString(w, "locked"); err != nil {
			t.Fatal(err)
		}

		aw, ok := w.(file.AbortWriter)
		if !ok {
			t.Fatalf("%T does not implement AbortWriter", w)
		}

		if err := aw.Abort(); err != nil {
			t.Fatalf("Abort: %v", err)
		}

		if _, err := fs.Stat(fsys, "cache.lock"); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after Abort the lock is still there (%v); a crashed acquirer would "+
				"block everyone until staleLockAge", err)
		}

		// So the name is free again.
		again, err := xfs.CreateExcl("cache.lock")
		if err != nil {
			t.Fatalf("CreateExcl after Abort: %v", err)
		}

		if err := again.Close(); err != nil {
			t.Fatal(err)
		}
	})
}
