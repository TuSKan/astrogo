package openngc

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// errorPage is what a server answering with an error page under a 200 puts
// where a CSV should be.
const errorPage = "<html>503 Service Unavailable</html>"

// ngcCacheAndSource points remote.OpenNGC's cache and source at fresh
// directories, the source holding the sample CSVs, and consents to its
// downloads, for the rest of t. It returns the cache with the key NGC.csv
// caches under, and the source.
func ngcCacheAndSource(t *testing.T) (cache remote.FS, key string, src remote.FS) {
	t.Helper()

	t.Cleanup(remote.Capture(remote.OpenNGC).Restore)

	src = fakeSources(t)

	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	cache, prefix, err := remote.CacheDir(context.Background(), remote.OpenNGC)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	return cache, prefix + "NGC.csv", src
}

func put(t *testing.T, fsys remote.FS, key, content string) {
	t.Helper()

	if err := remote.WriteFile(context.Background(), fsys, key, bytes.NewReader([]byte(content))); err != nil {
		t.Fatalf("write %s: %v", key, err)
	}
}

// A cached CSV damaged where it sits, while the source still serves the copy
// it was fetched from, is replaced in the call that finds it (#1014). Before,
// the error page parsed as an empty catalog and was kept as one.
func TestFetchReplacesACorruptCachedCSV(t *testing.T) {
	cache, key, _ := ngcCacheAndSource(t)

	loadNow(t)
	put(t, cache, key, errorPage)
	loadNow(t)

	b, err := fs.ReadFile(cache, key)
	if err != nil {
		t.Fatalf("read the cached NGC.csv: %v", err)
	}

	if string(b) != sampleNGCCSV {
		t.Errorf("the cache holds %q, want the source's copy", b)
	}
}

// A source serving an error page in place of the CSV is downloaded once and
// never cached, and the next load is refused without downloading it again.
func TestFetchDownloadsACorruptCSVOnce(t *testing.T) {
	cache, key, src := ngcCacheAndSource(t)
	put(t, src, "NGC.csv", errorPage)

	if _, err := New().Resolve(context.Background(), "M42"); !errors.Is(err, errMissingColumn) {
		t.Fatalf("first load: err %v, want errMissingColumn", err)
	}

	if _, err := fs.Stat(cache, key); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the error page was cached (Stat: %v)", err)
	}

	if _, err := New().Resolve(context.Background(), "M42"); !errors.Is(err, remote.ErrRejectedSource) {
		t.Fatalf("second load, source unchanged: err %v, want remote.ErrRejectedSource", err)
	}
}

// A header without a column the parser reads is refused rather than read as
// index 0, the failure that let an error page pass for an empty catalog.
func TestParseOpenNGCRefusesAHeaderWithoutItsColumns(t *testing.T) {
	for _, header := range []string{
		errorPage,
		"Name;Type;RA;Dec;M;Common names;V-Mag;B-Mag", // no Identifiers
	} {
		if _, err := parseOpenNGC(strings.NewReader(header + "\n")); !errors.Is(err, errMissingColumn) {
			t.Errorf("parseOpenNGC(header %q): err %v, want errMissingColumn", header, err)
		}
	}
}
