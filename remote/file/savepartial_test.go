package file_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

// errStoreBroken stands for any failure that is not a missing key.
var errStoreBroken = errors.New("the store is on fire")

// TestIsNotFoundSeparatesAMissFromAFailure is the distinction every
// cache-before-fetch path in the module turns on.
//
// It matters in both directions and neither is loud. A miss read as a failure
// turns a first run into an error; a failure read as a miss re-downloads a
// multi-gigabyte kernel on every call and reports nothing.
func TestIsNotFoundSeparatesAMissFromAFailure(t *testing.T) {
	t.Parallel()

	bucket, err := file.Open(t.Context(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	_, missing := bucket.ReadAll(t.Context(), "never-written.dat")
	if !file.IsNotFound(missing) {
		t.Errorf("IsNotFound(%v) = false, want true for a key never written", missing)
	}

	if file.IsNotFound(nil) {
		t.Error("IsNotFound(nil) = true; a success is not a miss")
	}

	if file.IsNotFound(errStoreBroken) {
		t.Error("IsNotFound reported true for an unrelated error; a broken store would read as a cold cache")
	}
}

// TestSavePartialRecordsTheSourceETag covers the one write in this package
// that carries metadata, and the reason it exists: without the recorded ETag,
// ResumePoint cannot tell a resumable partial from a stale one and discards it.
func TestSavePartialRecordsTheSourceETag(t *testing.T) {
	t.Parallel()

	bucket, err := file.Open(t.Context(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	const (
		key     = "kernel.bsp.part"
		content = "the first half"
		etag    = `"abc123"`
	)

	if err := file.SavePartial(t.Context(), bucket, key, strings.NewReader(content), etag); err != nil {
		t.Fatalf("SavePartial: %v", err)
	}

	attrs, err := bucket.Attributes(t.Context(), key)
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if got := attrs.Metadata[file.SourceETagKey]; got != etag {
		t.Errorf("recorded source ETag = %q, want %q — ResumePoint reads this to decide "+
			"whether the partial still matches the source", got, etag)
	}

	got, err := bucket.ReadAll(t.Context(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

// TestSavePartialWithoutAnETagWritesNoMetadata keeps the empty case from
// recording an empty ETag, which ResumePoint would compare against the
// source's real one and treat as a mismatch — discarding a partial that was
// simply written by a source serving no ETag at all.
func TestSavePartialWithoutAnETagWritesNoMetadata(t *testing.T) {
	t.Parallel()

	bucket, err := file.Open(t.Context(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := file.SavePartial(t.Context(), bucket, "k.part", strings.NewReader("x"), ""); err != nil {
		t.Fatalf("SavePartial: %v", err)
	}

	attrs, err := bucket.Attributes(t.Context(), "k.part")
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if got, ok := attrs.Metadata[file.SourceETagKey]; ok {
		t.Errorf("recorded a source ETag of %q for a write that had none", got)
	}
}
