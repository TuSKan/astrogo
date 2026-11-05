package mpcorb_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/catalog/mpcorb"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

const (
	// healFile is the comet file, the one the fixture rows come from.
	healFile = "CometEls.txt"

	// errorPage is what a server answering with an error page under a 200
	// puts where the orbit file should be. It holds no orbit row, so Read
	// skips every line of it and yields nothing at all.
	errorPage = "<html>503 Service Unavailable</html>"
)

// mpcorbCacheAndSource points remote.MPCORB's cache and source at fresh
// directories and consents to its downloads, for the rest of t. It returns the
// cache with the key healFile caches under, and the source.
func mpcorbCacheAndSource(t *testing.T) (cache remote.FS, key string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.MPCORB).Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.MPCORB, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.MPCORB)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.MPCORB)

	return cache, prefix + healFile, src
}

func put(t *testing.T, fsys remote.FS, key string, b []byte) {
	t.Helper()

	if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader(b)); err != nil {
		t.Fatalf("write %s: %v", key, err)
	}
}

func isCached(t *testing.T, fsys remote.FS, key string) bool {
	t.Helper()

	_, err := fs.Stat(fsys, key)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat %s: %v", key, err)
	}

	return err == nil
}

// truncatedGzip is the comet fixture compressed and cut before its trailer,
// which gzip detects only at the end of the stream.
func truncatedGzip(t *testing.T) []byte {
	t.Helper()

	var b bytes.Buffer

	zw := gzip.NewWriter(&b)
	if _, err := zw.Write([]byte(cometFixture())); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	return b.Bytes()[:b.Len()-4]
}

// iterate opens healFile and reads every row, returning how many parsed and
// the errors yielded.
func iterate(t *testing.T) (rows int, errs []error) {
	t.Helper()

	seq, err := mpcorb.Open(context.Background(), healFile)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Every error is kept and counted by the caller, which fails on any it
	// does not expect.
	for _, err := range seq {
		if err == nil {
			rows++
		} else {
			errs = append(errs, err)
		}
	}

	return rows, errs
}

// A download that holds no orbit row is refused: it is never cached, and the
// next Open is refused without downloading it again (#1014). Before, the error
// page was cached and read as an empty file, with no error at all.
func TestOpenDownloadsAnErrorPageOnce(t *testing.T) {
	cache, key, src := mpcorbCacheAndSource(t)
	put(t, src, healFile, []byte(errorPage))

	if _, err := mpcorb.Open(context.Background(), healFile); err == nil || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first Open: err %v, want the validation's refusal", err)
	}

	if isCached(t, cache, key) {
		t.Error("the error page was cached")
	}

	if _, err := mpcorb.Open(context.Background(), healFile); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Open, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}

// A download Read cannot finish is refused too, though the rows before the cut
// parse: a truncated stream is a failed transfer of the whole file.
func TestOpenRefusesATruncatedDownload(t *testing.T) {
	cache, key, src := mpcorbCacheAndSource(t)
	put(t, src, healFile, truncatedGzip(t))

	if _, err := mpcorb.Open(context.Background(), healFile); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Open: err %v, want io.ErrUnexpectedEOF from the gzip stream", err)
	}

	if isCached(t, cache, key) {
		t.Error("the truncated download was cached")
	}
}

// A cached copy damaged where it sits, while the source still serves the one
// it was fetched from, is found only while iterating, since Open reads nothing
// of a cache hit. Its error is yielded and it is removed, so the next Open
// fetches the source's copy again.
func TestOpenRemovesACachedCopyItCannotRead(t *testing.T) {
	for _, c := range []struct {
		name    string
		damaged []byte
	}{
		{"no rows", []byte(errorPage)},
		{"truncated", truncatedGzip(t)},
	} {
		t.Run(c.name, func(t *testing.T) {
			cache, key, src := mpcorbCacheAndSource(t)
			put(t, src, healFile, []byte(cometFixture()))

			want, errs := iterate(t)
			if want == 0 || len(errs) > 0 {
				t.Fatalf("the source's copy read %d rows and errors %v", want, errs)
			}

			put(t, cache, key, c.damaged)

			if _, errs := iterate(t); len(errs) != 1 {
				t.Fatalf("the damaged copy yielded errors %v, want exactly one", errs)
			}

			if isCached(t, cache, key) {
				t.Fatal("the damaged copy is still cached after its error was yielded")
			}

			if rows, errs := iterate(t); rows != want || len(errs) > 0 {
				t.Errorf("after removal: %d rows and errors %v, want the source's %d rows", rows, errs, want)
			}
		})
	}
}

// A malformed row is the catalog's, not the copy's: it is yielded, as Read
// documents, and the copy stays.
func TestOpenKeepsACopyWithAMalformedRow(t *testing.T) {
	cache, key, src := mpcorbCacheAndSource(t)

	// Hale-Bopp's row with its perihelion distance replaced by letters, which
	// keeps the row's comet shape but cannot parse.
	bad := []byte(cometFixture() + "    CJ95O010  1997 03 29.0319  x.xxxxxx  0.994897  130.7250  281.8047   89.7360  20260922  -2.0  4.0  C/1995 O1 (Hale-Bopp)                                    MPC194091\n")
	put(t, src, healFile, bad)

	if _, errs := iterate(t); len(errs) != 1 {
		t.Fatalf("errors %v, want the malformed row's one", errs)
	}

	if !isCached(t, cache, key) {
		t.Error("a copy with one malformed row was removed")
	}
}
