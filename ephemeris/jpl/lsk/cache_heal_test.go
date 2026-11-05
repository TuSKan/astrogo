package lsk_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

const (
	healKernel  = "naif0012.tls"
	corruptLSK  = "KPL/LSK\nnot a leap-second table\n"
	corruptDesc = "a kernel with no DELTA_AT table"
)

// lskCacheAndSource points remote.NAIFLSK's cache at a fresh directory and its
// source at another, and returns both filesystems with the cache's key for
// healKernel. Downloads are consented to by this package's TestMain.
func lskCacheAndSource(t *testing.T) (cache remote.FS, key string, source remote.FS) {
	t.Helper()

	scope := remote.Capture(remote.NAIFLSK)
	t.Cleanup(scope.Restore)
	t.Cleanup(func() { remote.SetDataDir("") })

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.NAIFLSK, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("OpenFS source: %v", err)
	}

	bucket, prefix, err := remote.CacheDir(ctx, remote.NAIFLSK)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	return bucket, prefix + healKernel, src
}

func write(t *testing.T, fsys remote.FS, key, content string) {
	t.Helper()

	if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader([]byte(content))); err != nil {
		t.Fatalf("WriteFile %s: %v", key, err)
	}
}

func cached(t *testing.T, fsys remote.FS, key string) bool {
	t.Helper()

	_, err := fs.Stat(fsys, key)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat %s: %v", key, err)
	}

	return err == nil
}

// A cached kernel that does not parse is removed when Cache reports it. Kept,
// it failed every later call the same way, though the doc promised
// "auto-healing" (#1010). Here the source has no kernel to replace it with.
func TestCacheRemovesACorruptCachedKernel(t *testing.T) {
	cache, key, _ := lskCacheAndSource(t)
	write(t, cache, key, corruptLSK)

	if _, err := lsk.Cache(context.Background(), healKernel); !errors.Is(err, lsk.ErrNoLeapseconds) {
		t.Fatalf("Cache over %s: err %v, want ErrNoLeapseconds", corruptDesc, err)
	}

	if cached(t, cache, key) {
		t.Error("the corrupt kernel is still cached after Cache reported it")
	}
}

// Where the source has a good kernel, the same call replaces the corrupt copy
// with it and succeeds (#1014).
func TestCacheReplacesACorruptCachedKernel(t *testing.T) {
	cache, key, src := lskCacheAndSource(t)
	write(t, cache, key, corruptLSK)
	write(t, src, healKernel, minimalLSK)

	r, err := lsk.Cache(context.Background(), healKernel)
	if err != nil {
		t.Fatalf("Cache over %s, with a valid source: %v", corruptDesc, err)
	}

	t.Cleanup(func() { _ = r.Close() })

	if len(r.DeltaAt) == 0 {
		t.Error("the fetched kernel has no DELTA_AT entries")
	}
}

// A download that does not parse never lands in the cache to fail later calls,
// and while the source still serves it, it is not downloaded again (#1014).
func TestCacheRefusesACorruptDownload(t *testing.T) {
	cache, key, src := lskCacheAndSource(t)
	write(t, src, healKernel, corruptLSK)

	if _, err := lsk.Cache(context.Background(), healKernel); !errors.Is(err, lsk.ErrNoLeapseconds) {
		t.Fatalf("Cache with a source serving %s: err %v, want ErrNoLeapseconds", corruptDesc, err)
	}

	if cached(t, cache, key) {
		t.Error("a corrupt download was cached")
	}

	if _, err := lsk.Cache(context.Background(), healKernel); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Cache, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}
