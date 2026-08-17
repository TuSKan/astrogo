package file

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"gocloud.dev/blob"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// Open must hand back the same *Bucket for the same URL: fileblob guards
// its IfNotExist precondition with a per-Bucket mutex, so remote's
// download lock is only exclusive within a process if every caller shares
// one Bucket per directory.
func TestOpenReusesOneBucketPerURL(t *testing.T) {
	urlA := mustLocalURL(t, t.TempDir())
	urlB := mustLocalURL(t, t.TempDir())

	first, err := Open(context.Background(), urlA)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	again, err := Open(context.Background(), urlA)
	if err != nil {
		t.Fatalf("Open (second): %v", err)
	}

	if first != again {
		t.Error("Open returned a different Bucket for the same URL")
	}

	other, err := Open(context.Background(), urlB)
	if err != nil {
		t.Fatalf("Open other: %v", err)
	}

	if other == first {
		t.Error("Open returned the same Bucket for two different URLs")
	}
}

func TestOpenUnregisteredScheme(t *testing.T) {
	if _, err := Open(context.Background(), "nosuchscheme://bucket"); err == nil {
		t.Fatal("expected an error for an unregistered scheme")
	}
}

// The two schemes every astrogo build must carry without an opt-in import.
func TestDefaultSchemesRegistered(t *testing.T) {
	for _, scheme := range []string{"file", "http", "https"} {
		if !blob.DefaultURLMux().ValidBucketScheme(scheme) {
			t.Errorf("scheme %q not registered; available: %v", scheme, blob.DefaultURLMux().BucketSchemes())
		}
	}
}

func TestSaveWritesFullContent(t *testing.T) {
	bucket, err := Open(context.Background(), mustLocalURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	const want = "the quick brown fox jumps over the lazy dog"

	if err := Save(context.Background(), bucket, "fox.txt", strings.NewReader(want)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := bucket.ReadAll(context.Background(), "fox.txt")
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

func TestSaveFailedWriteLeavesPriorObjectUntouched(t *testing.T) {
	// driver.Bucket.NewTypedWriter's contract guarantees a failed write
	// never clobbers whatever was previously at key — the property Save's
	// own doc comment relies on instead of a temp-file-then-rename dance.
	// Exercise it with a reader that fails partway through.
	bucket, err := Open(context.Background(), mustLocalURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := Save(context.Background(), bucket, "k", strings.NewReader("original")); err != nil {
		t.Fatalf("Save (seed): %v", err)
	}

	failing := io.MultiReader(strings.NewReader("partial-"), &errAfterReader{})
	if err := Save(context.Background(), bucket, "k", failing); err == nil {
		t.Fatal("expected Save to fail for a reader that errors mid-stream")
	}

	got, err := bucket.ReadAll(context.Background(), "k")
	if err != nil {
		t.Fatalf("ReadAll after failed Save: %v", err)
	}

	if string(got) != "original" {
		t.Errorf("content after failed overwrite = %q, want unchanged %q", got, "original")
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
