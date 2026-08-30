package file

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// payload is deterministic and longer than several chunks at the sizes the
// tests use, so a wrong chunk offset shows up as wrong bytes rather than a
// short read.
func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + i%26)
	}

	return b
}

func seedBucket(t *testing.T, key string, data []byte) *Bucket {
	t.Helper()

	bucket, err := Open(context.Background(), mustLocalURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := Save(context.Background(), bucket, key, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	return bucket
}

func TestReaderAtMatchesBytesReaderAcrossChunkBoundaries(t *testing.T) {
	const chunk = 64

	data := payload(chunk*4 + 7) // deliberately not a whole number of chunks
	bucket := seedBucket(t, "obj", data)

	ra, err := NewReaderAt(context.Background(), bucket, "obj", WithChunkSize(chunk), WithCachedChunks(2))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}
	defer func() { _ = ra.Close() }()

	if ra.Size() != int64(len(data)) {
		t.Fatalf("Size() = %d, want %d", ra.Size(), len(data))
	}

	want := bytes.NewReader(data)

	// Every (offset, length) pair that straddles, aligns with, or falls
	// inside a chunk, compared against the standard library's own
	// io.ReaderAt so both the bytes and the (n, err) pair must agree.
	for off := 0; off <= len(data); off++ {
		for _, size := range []int{1, chunk - 1, chunk, chunk + 1, chunk * 3} {
			gotBuf, wantBuf := make([]byte, size), make([]byte, size)

			gotN, gotErr := ra.ReadAt(gotBuf, int64(off))
			wantN, wantErr := want.ReadAt(wantBuf, int64(off))

			if gotN != wantN || !errors.Is(gotErr, wantErr) {
				t.Fatalf("ReadAt(len=%d, off=%d) = (%d, %v), want (%d, %v)", size, off, gotN, gotErr, wantN, wantErr)
			}

			if !bytes.Equal(gotBuf[:gotN], wantBuf[:wantN]) {
				t.Fatalf("ReadAt(len=%d, off=%d) bytes mismatch", size, off)
			}
		}
	}
}

func TestReaderAtEdgeCases(t *testing.T) {
	data := payload(100)
	bucket := seedBucket(t, "obj", data)

	ra, err := NewReaderAt(context.Background(), bucket, "obj", WithChunkSize(32))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}

	if n, err := ra.ReadAt(nil, 0); n != 0 || err != nil {
		t.Errorf("ReadAt(empty) = (%d, %v), want (0, nil)", n, err)
	}

	if n, err := ra.ReadAt(make([]byte, 4), int64(len(data))); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt at EOF = (%d, %v), want (0, io.EOF)", n, err)
	}

	if _, err := ra.ReadAt(make([]byte, 4), -1); err == nil {
		t.Error("expected an error for a negative offset")
	}

	if err := ra.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := ra.ReadAt(make([]byte, 4), 0); !errors.Is(err, ErrReaderAtClosed) {
		t.Errorf("ReadAt after Close = %v, want ErrReaderAtClosed", err)
	}
}

func TestReaderAtConcurrentReads(t *testing.T) {
	const chunk = 16

	data := payload(chunk * 8)
	bucket := seedBucket(t, "obj", data)

	// Two resident chunks against eight, so readers race on eviction too,
	// not just on a warm cache.
	ra, err := NewReaderAt(context.Background(), bucket, "obj", WithChunkSize(chunk), WithCachedChunks(2))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}
	defer func() { _ = ra.Close() }()

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func(off int) {
			defer wg.Done()

			for range 50 {
				buf := make([]byte, chunk)
				if _, err := ra.ReadAt(buf, int64(off*chunk)); err != nil {
					t.Errorf("ReadAt(off=%d): %v", off*chunk, err)

					return
				}

				if !bytes.Equal(buf, data[off*chunk:(off+1)*chunk]) {
					t.Errorf("ReadAt(off=%d) returned wrong bytes", off*chunk)

					return
				}
			}
		}(i)
	}

	wg.Wait()
}

// The point of this package's design: random access works over a source
// that has no OS path at all. httpblob is registered by default, so an
// http:// bucket needs no opt-in import.
func TestReaderAtOverHTTPBucket(t *testing.T) {
	data := payload(1000)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "obj", time.GoTime{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	bucket, err := Open(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("Open http bucket: %v", err)
	}

	ra, err := NewReaderAt(context.Background(), bucket, "obj", WithChunkSize(128))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}
	defer func() { _ = ra.Close() }()

	if ra.Size() != int64(len(data)) {
		t.Fatalf("Size() = %d, want %d", ra.Size(), len(data))
	}

	got := make([]byte, 300)
	if _, err := ra.ReadAt(got, 500); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}

	if !bytes.Equal(got, data[500:800]) {
		t.Error("ReadAt over an http:// bucket returned wrong bytes")
	}
}

func TestSaveThenReadAllOverHTTPBucketIsReadOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "read-only", http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	bucket, err := Open(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatalf("Open http bucket: %v", err)
	}

	if err := Save(context.Background(), bucket, "k", strings.NewReader("x")); err == nil {
		t.Error("expected writing to an http:// bucket to fail")
	}
}
