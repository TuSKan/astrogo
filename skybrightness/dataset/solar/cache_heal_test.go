package solar_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
)

// errorPage is what a server answering with an error page under a 200 puts
// where the spectrum should be.
const errorPage = "<html>503 Service Unavailable</html>"

// calspecCacheAndSource points remote.CALSPEC's cache and source at fresh
// directories and consents to its downloads, for the rest of t. It returns the
// cache with the key the reference spectrum caches under, and the source.
func calspecCacheAndSource(t *testing.T) (cache remote.FS, key string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.CALSPEC).Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.CALSPEC, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.CALSPEC)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.CALSPEC)

	return cache, prefix + solar.CALSPECSolarReference, src
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

// The case #1014 measured: an error page cached as the reference spectrum
// failed every call, twice in a row with the page still there. It is now
// replaced in the call that finds it.
func TestOpenReplacesACorruptCachedSpectrum(t *testing.T) {
	cache, key, src := calspecCacheAndSource(t)
	put(t, cache, key, []byte(errorPage))
	put(t, src, solar.CALSPECSolarReference, calspecLike(t, []float64{4000, 5000, 6000}, []float64{2.5, 4.0, 1.0}))

	s, err := solar.Open(context.Background())
	if err != nil {
		t.Fatalf("Open over a corrupt cached copy: %v", err)
	}

	if len(s.Irradiance) != 3 {
		t.Errorf("%d samples, want the source's 3", len(s.Irradiance))
	}
}

// A source serving a file that does not parse is downloaded once and never
// cached, and the next call is refused without downloading it again.
func TestOpenDownloadsACorruptSpectrumOnce(t *testing.T) {
	cache, key, src := calspecCacheAndSource(t)
	put(t, src, solar.CALSPECSolarReference, []byte(errorPage))

	if _, err := solar.Open(context.Background()); err == nil || errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first Open: err %v, want the parse error", err)
	}

	if isCached(t, cache, key) {
		t.Error("the corrupt download was cached")
	}

	if _, err := solar.Open(context.Background()); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Open, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}
