package luminosity_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/luminosity"
)

const (
	// photopicFile is the file luminosity.Photopic fetches.
	photopicFile = "CIE_sle_photopic.csv"

	// The first three rows of the real file, as open_test.go serves them.
	photopicRows = "360,0.0000039170000\n361,0.0000043935810\n362,0.0000049296040\n"

	// errorPage is what a server answering with an error page under a 200
	// puts where the curve should be.
	errorPage = "<html>503 Service Unavailable</html>"
)

// cieCacheAndSource points remote.CIELuminosity's cache and source at fresh
// directories and consents to its downloads, for the rest of t. It returns the
// cache with the key the photopic curve caches under, and the source.
func cieCacheAndSource(t *testing.T) (cache remote.FS, key string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.CIELuminosity).Restore)

	ctx := context.Background()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	srcURL := testutil.FileURL(t, t.TempDir())
	if err := remote.SetURL(remote.CIELuminosity, srcURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	src, err := remote.OpenFS(ctx, srcURL)
	if err != nil {
		t.Fatalf("open source: %v", err)
	}

	cache, prefix, err := remote.CacheDir(ctx, remote.CIELuminosity)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	remote.EnableDownloads(0, remote.CIELuminosity)

	return cache, prefix + photopicFile, src
}

func put(t *testing.T, fsys remote.FS, key, content string) {
	t.Helper()

	if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader([]byte(content))); err != nil {
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

// A cached curve that does not parse is replaced in the call that finds it,
// rather than failing every later call (#1014).
func TestOpenReplacesACorruptCachedCurve(t *testing.T) {
	cache, key, src := cieCacheAndSource(t)
	put(t, cache, key, errorPage)
	put(t, src, photopicFile, photopicRows)

	band, err := luminosity.Open(context.Background(), luminosity.Photopic)
	if err != nil {
		t.Fatalf("Open over a corrupt cached copy: %v", err)
	}

	if len(band.WavelengthNM) != 3 {
		t.Errorf("%d samples, want the source's 3", len(band.WavelengthNM))
	}
}

// A source serving a curve that does not parse is downloaded once and never
// cached, and the next call is refused without downloading it again.
func TestOpenDownloadsACorruptCurveOnce(t *testing.T) {
	cache, key, src := cieCacheAndSource(t)
	put(t, src, photopicFile, errorPage)

	if _, err := luminosity.Open(context.Background(), luminosity.Photopic); !errors.Is(err, luminosity.ErrCurve) ||
		errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("first Open: err %v, want the parse error", err)
	}

	if isCached(t, cache, key) {
		t.Error("the corrupt download was cached")
	}

	if _, err := luminosity.Open(context.Background(), luminosity.Photopic); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second Open, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}
