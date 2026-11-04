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
// "auto-healing" (#1010).
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

// And so the next call heals: with the corrupt copy gone, it fetches the
// source's and succeeds.
func TestCacheFetchesAFreshKernelAfterACorruptOne(t *testing.T) {
	cache, key, src := lskCacheAndSource(t)
	write(t, cache, key, corruptLSK)
	write(t, src, healKernel, minimalLSK)

	if _, err := lsk.Cache(context.Background(), healKernel); !errors.Is(err, lsk.ErrNoLeapseconds) {
		t.Fatalf("first Cache, over the corrupt copy: err %v, want ErrNoLeapseconds", err)
	}

	r, err := lsk.Cache(context.Background(), healKernel)
	if err != nil {
		t.Fatalf("second Cache, with a valid source: %v", err)
	}

	t.Cleanup(func() { _ = r.Close() })

	if len(r.DeltaAt) == 0 {
		t.Error("the fetched kernel has no DELTA_AT entries")
	}
}

// A download is parsed before it is cached: a corrupt one is refused and
// never lands in the cache to fail later calls.
func TestCacheRefusesACorruptDownload(t *testing.T) {
	cache, key, src := lskCacheAndSource(t)
	write(t, src, healKernel, corruptLSK)

	if _, err := lsk.Cache(context.Background(), healKernel); err == nil {
		t.Fatalf("Cache with a source serving %s: no error", corruptDesc)
	}

	if cached(t, cache, key) {
		t.Error("a corrupt download was cached")
	}
}
