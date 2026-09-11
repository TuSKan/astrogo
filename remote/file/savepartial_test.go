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

// errReadFailed stands for a source that stops mid-stream.
var errReadFailed = errors.New("the source stopped answering")

// failingReader fails after handing over some bytes, the way a connection
// dropped mid-transfer does.
type failingReader struct{ n int }

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n <= 0 {
		return 0, errReadFailed
	}

	n := min(len(p), r.n)
	r.n -= n

	for i := range n {
		p[i] = 'x'
	}

	return n, nil
}

// TestSavePartialReportsAFailedReadAndWritesNothing covers the case this
// function exists to survive: the body it is given stops partway.
//
// The failure has to surface rather than be recorded as a shorter partial,
// because a partial's length is exactly what ResumePoint later trusts to decide
// where the next attempt continues from.
func TestSavePartialReportsAFailedReadAndWritesNothing(t *testing.T) {
	t.Parallel()

	bucket := newBucket(t)

	err := file.SavePartial(t.Context(), bucket, "k.part", &failingReader{n: 512}, `"etag"`)
	if !errors.Is(err, errReadFailed) {
		t.Fatalf("SavePartial = %v, want the reader's own error", err)
	}

	if exists, _ := bucket.Exists(t.Context(), "k.part"); exists {
		t.Error("a partial was recorded for a body that failed to read; its length " +
			"would later be trusted as a resume offset")
	}
}
