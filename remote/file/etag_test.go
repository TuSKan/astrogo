package file_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote/file"
)

// The validator machinery, which decides two things that both cost a
// multi-gigabyte download when they are wrong: whether a cached object is still
// current, and whether a partial one can be resumed onto.
//
// It replaced gocloud's per-object metadata map, which io/fs has no equivalent
// of, so none of it is inherited behaviour — all of it is new and all of it is
// load-bearing.

// stubInfo is an fs.FileInfo with whatever Sys and ModTime a case needs.
type stubInfo struct {
	size    int64
	modTime time.Time
	sys     any
}

func (s stubInfo) Name() string       { return "object.dat" }
func (s stubInfo) Size() int64        { return s.size }
func (s stubInfo) Mode() fs.FileMode  { return 0o444 }
func (s stubInfo) ModTime() time.Time { return s.modTime }
func (s stubInfo) IsDir() bool        { return false }
func (s stubInfo) Sys() any           { return s.sys }

// TestETagPrefersTheServersOwn walks the three answers, in the order the
// function tries them.
func TestETagPrefersTheServersOwn(t *testing.T) {
	t.Parallel()

	epoch := time.Unix(1_700_000_000, 0)

	strong := http.Header{}
	strong.Set("ETag", `"abc123"`)

	noTag := http.Header{}
	noTag.Set("Content-Type", "application/octet-stream")

	t.Run("a server ETag wins, unquoted", func(t *testing.T) {
		t.Parallel()

		got := file.ETag(stubInfo{size: 10, modTime: epoch, sys: strong})
		if got != "abc123" {
			t.Errorf("ETag = %q, want the server's %q with its quotes stripped", got, "abc123")
		}
	})

	t.Run("no server ETag synthesises a weak one", func(t *testing.T) {
		t.Parallel()

		got := file.ETag(stubInfo{size: 10, modTime: epoch, sys: noTag})
		if got == "" {
			t.Fatal("ETag is empty although size and modification time are both known")
		}

		if !strings.HasPrefix(got, "W/") {
			t.Errorf("ETag = %q, want a W/ prefix marking it weak", got)
		}

		// It has to change when either input does, or it validates nothing.
		bigger := file.ETag(stubInfo{size: 11, modTime: epoch, sys: noTag})
		if bigger == got {
			t.Error("a different size produced the same validator")
		}

		later := file.ETag(stubInfo{size: 10, modTime: epoch.Add(time.Second), sys: noTag})
		if later == got {
			t.Error("a different modification time produced the same validator")
		}
	})

	t.Run("nothing to go on is empty, not a validator that matches itself", func(t *testing.T) {
		t.Parallel()

		if got := file.ETag(stubInfo{size: 10}); got != "" {
			t.Errorf("ETag = %q for a zero modification time, want \"\" — a validator built "+
				"from nothing would report unchanged for ever", got)
		}
	})
}

// TestETagReadsARealHTTPResponse is the other half: the stub above asserts the
// logic, this asserts that the HTTP backend actually puts its header where ETag
// looks for it. A correct function reading a header nobody supplies is worth
// nothing.
func TestETagReadsARealHTTPResponse(t *testing.T) {
	t.Parallel()

	const tag = `"deadbeef"`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", tag)
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fsys, err := file.OpenFS(srv.URL + "/pub/")
	if err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat(fsys, "kernel.bsp")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	if got := file.ETag(info); got != "deadbeef" {
		t.Errorf("ETag = %q, want %q — the backend is not exposing its header through Sys",
			got, "deadbeef")
	}
}

// TestRecordedETagRoundTrips covers the sidecar that replaced object metadata.
func TestRecordedETagRoundTrips(t *testing.T) {
	t.Parallel()

	fsys := newFS(t)

	const (
		key = "jpl/de440s.bsp"
		tag = "abc123"
	)

	if got := file.RecordedETag(t.Context(), fsys, key); got != "" {
		t.Errorf("an unrecorded key read back %q, want \"\"", got)
	}

	if err := file.WriteETag(t.Context(), fsys, key, tag); err != nil {
		t.Fatalf("WriteETag: %v", err)
	}

	if got := file.RecordedETag(t.Context(), fsys, key); got != tag {
		t.Errorf("RecordedETag = %q, want %q", got, tag)
	}

	// An empty tag writes nothing rather than an empty sidecar, so "no
	// validator" and "a validator that is the empty string" stay the same
	// answer and neither leaves a file behind.
	const other = "iers/finals2000A.data"

	if err := file.WriteETag(t.Context(), fsys, other, ""); err != nil {
		t.Fatalf("WriteETag with no tag: %v", err)
	}

	if _, err := fs.Stat(fsys, other+file.SourceETagSuffix); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("an empty ETag left a sidecar behind (%v)", err)
	}
}

// TestIsStagingNameRecognizesBookkeeping covers the predicate remote uses to
// tell its own scaffolding from a cached object when it walks a cache.
func TestIsStagingNameRecognizesBookkeeping(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"jpl/de440s.bsp.lock",
		"jpl/de440s.bsp.part",
		"jpl/de440s.bsp.resume",
		"jpl/de440s.bsp.etag",
		"de440s.bsp.part",
	} {
		if !file.IsStagingName(name) {
			t.Errorf("IsStagingName(%q) = false, want true", name)
		}
	}

	for _, name := range []string{
		"jpl/de440s.bsp",
		"iers/finals2000A.data",
		"NGC.csv",
		// A name that merely contains a suffix rather than ending in one.
		"jpl/de440s.part.bsp",
	} {
		if file.IsStagingName(name) {
			t.Errorf("IsStagingName(%q) = true, want false — a real cached object", name)
		}
	}
}

// TestMemFSWritePath covers the in-memory backend's own writes.
//
// mem:// exists precisely because fstest.MapFS cannot be written to, so its
// write path being untested would leave the one thing it is for unverified.
func TestMemFSWritePath(t *testing.T) {
	t.Parallel()

	fsys, err := file.OpenFS("mem://writepath")
	if err != nil {
		t.Fatal(err)
	}

	xfs, ok := fsys.(file.CreateExclFS)
	if !ok {
		t.Fatal("mem:// does not implement CreateExclFS, so it cannot host the download lock")
	}

	// Exclusive create: the name is claimed the instant it succeeds, so a
	// second attempt loses.
	w, err := xfs.CreateExcl("cache.lock")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := xfs.CreateExcl("cache.lock"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("a second CreateExcl gave %v, want fs.ErrExist", err)
	}

	if _, err := io.WriteString(w, "held"); err != nil {
		t.Fatal(err)
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if got, err := fs.ReadFile(fsys, "cache.lock"); err != nil || string(got) != "held" {
		t.Errorf("the lock object reads back as (%q, %v), want %q", got, err, "held")
	}

	// Remove frees it.
	rfs, ok := fsys.(file.RemoveFS)
	if !ok {
		t.Fatal("mem:// does not implement RemoveFS")
	}

	if err := rfs.Remove("cache.lock"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if err := rfs.Remove("cache.lock"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("removing a name twice gave %v, want fs.ErrNotExist", err)
	}

	// Abort on an exclusive create gives the name back, so an abandoned lock
	// attempt does not block every later acquirer until staleLockAge.
	w2, err := xfs.CreateExcl("cache.lock")
	if err != nil {
		t.Fatal(err)
	}

	aw, ok := w2.(file.AbortWriter)
	if !ok {
		t.Fatalf("%T does not implement AbortWriter", w2)
	}

	if err := aw.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	if _, err := fs.Stat(fsys, "cache.lock"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after Abort the lock is still there (%v)", err)
	}

	// And Abort on a buffered write publishes nothing.
	cfs, ok := fsys.(file.CreateFS)
	if !ok {
		t.Fatal("mem:// does not implement CreateFS")
	}

	w3, err := cfs.Create("kernel.bsp")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := io.WriteString(w3, "half a kernel"); err != nil {
		t.Fatal(err)
	}

	abortable, ok := w3.(file.AbortWriter)
	if !ok {
		t.Fatalf("%T does not implement AbortWriter", w3)
	}

	if err := abortable.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}

	if err := w3.Close(); err != nil {
		t.Fatalf("Close after Abort: %v", err)
	}

	if got, err := fs.ReadFile(fsys, "kernel.bsp"); err == nil && len(got) != 0 {
		t.Errorf("an aborted write published %q", got)
	}
}

// TestMemFSHonoursItsContext covers ContextFS on the in-memory backend, which
// implements it for the same reason the local one does: a check with an
// exception is one nobody can rely on.
func TestMemFSHonoursItsContext(t *testing.T) {
	t.Parallel()

	fsys, err := file.OpenFS("mem://ctx")
	if err != nil {
		t.Fatal(err)
	}

	if err := file.WriteFile(t.Context(), fsys, "k.bsp", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}

	bound, err := file.RequireContext(t.Context(), fsys)
	if err != nil {
		t.Fatalf("RequireContext: %v", err)
	}

	if _, err := fs.ReadFile(bound, "k.bsp"); err != nil {
		t.Fatalf("a bound filesystem cannot read: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	dead, err := file.RequireContext(ctx, fsys)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := fs.ReadFile(dead, "k.bsp"); err == nil {
		t.Error("a cancelled context still read from mem://")
	}
}

// TestMemDirCannotBeReadAsAFile pins the one thing an open directory must
// refuse. fs.File gives a directory a Read method it cannot honour, and
// returning zero bytes and io.EOF instead of an error would make a caller that
// mistook a prefix for an object see an empty file rather than a mistake.
func TestMemDirCannotBeReadAsAFile(t *testing.T) {
	t.Parallel()

	fsys, err := file.OpenFS("mem://dirread")
	if err != nil {
		t.Fatal(err)
	}

	if err := file.WriteFile(t.Context(), fsys, "kernels/de440s.bsp",
		strings.NewReader("bytes")); err != nil {
		t.Fatal(err)
	}

	d, err := fsys.Open("kernels")
	if err != nil {
		t.Fatalf("Open a directory: %v", err)
	}

	defer func() { _ = d.Close() }()

	if _, err := d.Read(make([]byte, 8)); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("reading a directory gave %v, want fs.ErrInvalid", err)
	}

	// It is still a directory in every other respect.
	info, err := d.Stat()
	if err != nil {
		t.Fatal(err)
	}

	if !info.IsDir() {
		t.Error("Stat on an open directory does not report a directory")
	}
}
