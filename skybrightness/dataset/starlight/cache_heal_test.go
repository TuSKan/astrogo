package starlight_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// errorPage is what a server answering with an error page under a 200 puts
// where the map should be.
const errorPage = "<html>503 Service Unavailable</html>"

// gaiaMapCacheAndSource points remote.GaiaStarMap's cache and source at fresh
// directories and consents to its downloads, for the rest of t. It returns the
// cache with the key the map caches under, and the source.
func gaiaMapCacheAndSource(t *testing.T) (cache remote.FS, key string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.GaiaStarMap).Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.GaiaStarMap, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.GaiaStarMap)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.GaiaStarMap)

	return cache, prefix + starlight.TotalStarlightMap, src
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

// A cached map that does not load is replaced in the call that finds it,
// rather than failing every later call (#1014).
func TestOpenReplacesACorruptCachedMap(t *testing.T) {
	cache, key, src := gaiaMapCacheAndSource(t)
	put(t, cache, key, []byte(errorPage))
	put(t, src, starlight.TotalStarlightMap, tinyMap(t))

	m, err := starlight.Open(context.Background())
	if err != nil {
		t.Fatalf("Open over a corrupt cached copy: %v", err)
	}

	if m.Source != starlight.TotalStarlightMap {
		t.Errorf("Source %q, want %q", m.Source, starlight.TotalStarlightMap)
	}
}

// A source serving a map that does not load is downloaded once and never
// cached, and the next call is refused without downloading it again.
func TestOpenDownloadsACorruptMapOnce(t *testing.T) {
	cache, key, src := gaiaMapCacheAndSource(t)
	put(t, src, starlight.TotalStarlightMap, []byte(errorPage))

	if _, err := starlight.Open(context.Background()); err == nil || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first Open: err %v, want the decompression error", err)
	}

	if isCached(t, cache, key) {
		t.Error("the corrupt download was cached")
	}

	if _, err := starlight.Open(context.Background()); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Open, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}
