package file

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// OpenFS must hand back the same filesystem for the same URL.
//
// The reason has changed and is worth recording, because the old one was
// wrong. This used to be load-bearing for correctness: fileblob was believed to
// guard its IfNotExist precondition with a per-Bucket mutex, so the download
// lock was only exclusive within a process if every caller shared one Bucket
// per directory. It had no such mutex. The lock is now O_CREATE|O_EXCL, which
// needs no sharing to be exclusive.
//
// What remains is ordinary: one handle per URL avoids re-resolving a backend on
// every fetch, and the filesystems here are safe for concurrent use so sharing
// is free.
func TestOpenFSReusesOneFilesystemPerURL(t *testing.T) {
	urlA := mustLocalURL(t, t.TempDir())
	urlB := mustLocalURL(t, t.TempDir())

	first, err := OpenFS(urlA)
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	again, err := OpenFS(urlA)
	if err != nil {
		t.Fatalf("OpenFS (second): %v", err)
	}

	if first != again {
		t.Error("OpenFS returned a different filesystem for the same URL")
	}

	other, err := OpenFS(urlB)
	if err != nil {
		t.Fatalf("OpenFS other: %v", err)
	}

	if other == first {
		t.Error("OpenFS returned the same filesystem for two different URLs")
	}
}

// An unregistered scheme must say which ones are registered, because the answer
// is almost always a missing blank import and a bare "unknown scheme" does not
// suggest that.
func TestOpenUnregisteredSchemeNamesTheRegisteredOnes(t *testing.T) {
	_, err := OpenFS("nosuchscheme://bucket")
	if err == nil {
		t.Fatal("expected an error for an unregistered scheme")
	}

	if !errors.Is(err, ErrNoScheme) {
		t.Errorf("error is %v, want it to wrap ErrNoScheme", err)
	}

	if !strings.Contains(err.Error(), "file") {
		t.Errorf("the error does not list the registered schemes, so it does not hint at "+
			"the missing import that usually causes it: %v", err)
	}
}

// The schemes every astrogo build must carry without an opt-in import.
func TestDefaultSchemesRegistered(t *testing.T) {
	got := Schemes()

	for _, scheme := range []string{"file", "http", "https", "mem"} {
		if !slices.Contains(got, scheme) {
			t.Errorf("scheme %q not registered; available: %v", scheme, got)
		}
	}
}

func TestWriteFileWritesFullContent(t *testing.T) {
	fsys, err := OpenFS(mustLocalURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	const want = "the quick brown fox jumps over the lazy dog"

	if err := WriteFile(t.Context(), fsys, "fox.txt", strings.NewReader(want)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := fs.ReadFile(fsys, "fox.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// A write that fails partway must leave whatever was at the name alone.
//
// This used to rest on gocloud's driver contract. It now rests on this
// package's own staging: the local backend writes to a uniquely named file
// beside the target and renames it into place only on a clean Close, so a
// failed copy never reaches the name at all.
func TestWriteFileFailedWriteLeavesPriorObjectUntouched(t *testing.T) {
	fsys, err := OpenFS(mustLocalURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	if err := WriteFile(t.Context(), fsys, "k", strings.NewReader("original")); err != nil {
		t.Fatalf("WriteFile (seed): %v", err)
	}

	failing := io.MultiReader(strings.NewReader("partial-"), &errAfterReader{})
	if err := WriteFile(t.Context(), fsys, "k", failing); err == nil {
		t.Fatal("expected WriteFile to fail for a reader that errors mid-stream")
	}

	got, err := fs.ReadFile(fsys, "k")
	if err != nil {
		t.Fatalf("ReadFile after failed WriteFile: %v", err)
	}

	if string(got) != "original" {
		t.Errorf("content after failed overwrite = %q, want unchanged %q", got, "original")
	}
}

// A read-only backend must refuse a write with ErrReadOnly rather than
// panicking on a type assertion or failing with something less specific.
func TestWriteFileRefusesAReadOnlyFilesystem(t *testing.T) {
	fsys, err := OpenFS("https://example.invalid/pub/")
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	err = WriteFile(t.Context(), fsys, "k", strings.NewReader("x"))
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("writing to an HTTP filesystem returned %v, want ErrReadOnly", err)
	}
}

// Exists must separate a miss from a failure, which is the distinction every
// cache-hit path in the module depends on.
func TestExistsSeparatesAMissFromAFailure(t *testing.T) {
	fsys, err := OpenFS(mustLocalURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	ok, err := Exists(t.Context(), fsys, "not-there.txt")
	if err != nil {
		t.Errorf("a missing object reported an error (%v); a miss is (false, nil)", err)
	}

	if ok {
		t.Error("a missing object reported as present")
	}

	if err := WriteFile(t.Context(), fsys, "there.txt", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}

	if ok, err := Exists(t.Context(), fsys, "there.txt"); err != nil || !ok {
		t.Errorf("Exists on a present object = (%v, %v), want (true, nil)", ok, err)
	}
}

// errAfterReader always returns an error on Read — used to simulate a
// source that fails mid-stream.
type errAfterReader struct{}

func (errAfterReader) Read([]byte) (int, error) { return 0, errFakeReadFailure }

var errFakeReadFailure = bytes.ErrTooLarge // any stable, distinguishable error value

func mustLocalURL(t *testing.T, dir string) string {
	t.Helper()

	return testutil.FileURL(t, dir)
}
