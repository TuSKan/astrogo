package dust

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// errorPage is what a server answering with an error page under a 200 puts
// where a hemisphere should be.
const errorPage = "<html>503 Service Unavailable</html>"

// sfdCacheAndSource points remote.SFDDustMap's cache and source at fresh
// directories and consents to its downloads, for the rest of t. It returns the
// cache with the key the north hemisphere caches under, and the source.
func sfdCacheAndSource(t *testing.T) (cache remote.FS, northKey string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.SFDDustMap).Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.SFDDustMap, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.SFDDustMap)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.SFDDustMap)

	return cache, prefix + northFile, src
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

// A cached hemisphere that does not decode is replaced in the call that finds
// it, rather than failing every later call (#1014).
func TestSFDOpenReplacesACorruptCachedHemisphere(t *testing.T) {
	cache, northKey, src := sfdCacheAndSource(t)
	put(t, cache, northKey, []byte(errorPage))
	put(t, src, northFile, hemisphereFITS(t, "1", [4]float64{1, 2, 3, 4}))
	put(t, src, southFile, hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}))

	sfd, err := Open(context.Background())
	if err != nil {
		t.Fatalf("Open over a corrupt cached hemisphere: %v", err)
	}

	if sfd.north.nsgp != 1 || len(sfd.north.values) != 4 {
		t.Errorf("north came back as nsgp %g with %d pixels, want +1 with 4", sfd.north.nsgp, len(sfd.north.values))
	}
}

// A source serving a hemisphere that does not decode is downloaded once and
// never cached, and the next call is refused without downloading it again.
func TestSFDOpenDownloadsACorruptHemisphereOnce(t *testing.T) {
	cache, northKey, src := sfdCacheAndSource(t)
	put(t, src, northFile, []byte(errorPage))
	put(t, src, southFile, hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}))

	if _, err := Open(context.Background()); !errors.Is(err, ErrSFD) || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first Open: err %v, want ErrSFD from the decode", err)
	}

	if isCached(t, cache, northKey) {
		t.Error("the corrupt hemisphere was cached")
	}

	if _, err := Open(context.Background()); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Open, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}

// Two hemispheres that are each a good map but not opposite ones are refused
// by Open, which reads both. Neither file is wrong on its own, so neither is
// removed: which one is mislabeled cannot be told from here.
func TestSFDOpenKeepsHemispheresItCannotBlame(t *testing.T) {
	cache, northKey, src := sfdCacheAndSource(t)
	put(t, src, northFile, hemisphereFITS(t, "-1", [4]float64{1, 2, 3, 4}))
	put(t, src, southFile, hemisphereFITS(t, "-1", [4]float64{5, 6, 7, 8}))

	if _, err := Open(context.Background()); !errors.Is(err, ErrSFD) {
		t.Fatalf("Open over two south maps: err %v, want ErrSFD", err)
	}

	if !isCached(t, cache, northKey) {
		t.Error("a hemisphere that decodes was removed over a check between the two")
	}
}
