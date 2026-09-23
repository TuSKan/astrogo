package eop

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/time"
)

// sampleFinals2000A mimics finals2000A.all format for two consecutive
// days, covering MJD 41684-41685.
const sampleFinals2000A = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

// These tests moved here from time/internal/iers when the EOP dependency
// was inverted. They were always testing this package's behavior —
// consent, ETag revalidation, cache layout — through a package that had
// no business knowing about any of it. They now sit next to the code they
// exercise, and iers keeps only the tests about its own logic.

// fakeIERSSource opens a fresh temp directory as a fsys, points
// remote.IERSFinals2000A's URL at it, and writes content at the source object
// name the loader reads. A local stand-in for HTTP, since remote/file has
// no https driver registered in this build.
func fakeIERSSource(t *testing.T, content string) {
	t.Helper()

	url := testutil.FileURL(t, t.TempDir())

	if err := remote.SetURL(remote.IERSFinals2000A, url); err != nil {
		t.Fatal(err)
	}

	fsys, err := file.OpenFS(url)
	if err != nil {
		t.Fatalf("open fake source: %v", err)
	}

	if err := remote.WriteFile(context.Background(), fsys, "finals2000A.all", strings.NewReader(content)); err != nil {
		t.Fatalf("seed fake source: %v", err)
	}
}

// scratchCache points the data directory at a fresh temp dir so a test
// never reads a cache file left behind by another run.
func scratchCache(t *testing.T) {
	t.Helper()

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	t.Cleanup(func() {
		remote.SetDataDir("")
		remote.Reset()
		time.ResetEOP()
	})
}

func TestEOPLoaderFetchesAndParses(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	data, err := eopLoader{}.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if _, err := time.ParseFinals2000A(bytes.NewReader(data.Raw)); err != nil {
		t.Fatalf("fetched bytes do not parse as finals2000A: %v", err)
	}
}

// TestEOPLoaderDefaultDenyWritesNoCache is the consent contract: without
// remote.EnableDownloads the fetch is refused, and nothing is written.
func TestEOPLoaderDefaultDenyWritesNoCache(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)

	_, err := eopLoader{}.Fetch(context.Background())
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("Fetch without remote.EnableDownloads = %v, want remote.ErrDownloadDenied", err)
	}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if _, err := fs.Stat(fsys, prefix+eopCacheName); err == nil {
		t.Error("a denied fetch must not write a cache file")
	}
}

// TestEOPLoaderSkipsBodyWhenETagUnchanged proves the revalidation is a
// content check rather than a wall-clock window: an untouched source has
// the same ETag, so the cache object must not be rewritten.
func TestEOPLoaderSkipsBodyWhenETagUnchanged(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	ctx := context.Background()

	if _, err := (eopLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	before, err := fs.Stat(fsys, prefix+eopCacheName)
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if _, err := (eopLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	after, err := fs.Stat(fsys, prefix+eopCacheName)
	if err != nil {
		t.Fatalf("Attributes (after): %v", err)
	}

	if !after.ModTime().Equal(before.ModTime()) {
		t.Errorf("cache rewritten against an unchanged source: ModTime %v -> %v", before.ModTime(), after.ModTime())
	}
}

// TestEOPLoaderRejectsCorruptDownload keeps remote.WithValidate honest: a
// response that does not parse must never be trusted as the new cache.
func TestEOPLoaderRejectsCorruptDownload(t *testing.T) {
	scratchCache(t)

	// A single line with no newline, past bufio.Scanner's default token
	// limit, makes ParseFinals2000A's scan fail — a realistic stand-in for
	// a truncated or garbled response. Short garbage will not do: it parses
	// cleanly into an empty table.
	fakeIERSSource(t, strings.Repeat("x", 70*1024))
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	if _, err := (eopLoader{}).Fetch(context.Background()); err == nil {
		t.Fatal("Fetch accepted a corrupt download")
	}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if _, err := fs.Stat(fsys, prefix+eopCacheName); err == nil {
		t.Error("a corrupt download must not be cached")
	}

	if _, ok := time.GetModel().(time.ZeroModel); !ok {
		t.Errorf("model must be unchanged after a rejected download, got %T", time.GetModel())
	}
}

// TestEOPLoaderCachedReadsAPreSeededFile covers the offline deployment:
// a file copied in by hand has no recorded ETag, so remote.GetFile's cache-hit
// path cannot find it and Cached must read the object directly.
func TestEOPLoaderCachedReadsAPreSeededFile(t *testing.T) {
	scratchCache(t)

	ctx := context.Background()

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if err := remote.WriteFile(ctx, fsys, prefix+eopCacheName, strings.NewReader(sampleFinals2000A)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	data, err := eopLoader{}.Cached(ctx)
	if err != nil {
		t.Fatalf("Cached: %v", err)
	}

	if string(data.Raw) != sampleFinals2000A {
		t.Error("Cached returned different bytes than were pre-seeded")
	}

	if data.ModTime.IsZero() {
		t.Error("Cached returned a zero ModTime; the retry cooldown is seeded from it")
	}
}

func TestEOPLoaderCachedReportsNoDataWhenEmpty(t *testing.T) {
	scratchCache(t)

	_, err := eopLoader{}.Cached(context.Background())
	if !errors.Is(err, time.ErrNoEOPData) {
		t.Fatalf("Cached with an empty cache = %v, want ErrNoEOPData", err)
	}
}

// TestEOPLoaderDoesNotAccumulateCacheFiles guards against a cache that
// grows one object per fetch.
func TestEOPLoaderDoesNotAccumulateCacheFiles(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	for range 3 {
		if _, err := (eopLoader{}).Fetch(context.Background()); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
	}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	// The bulletin plus its ETag sidecar. IERS is Mutable, so the ETag the
	// bulletin was fetched under is kept beside it — that is what lets the next
	// process reuse the cache rather than re-download. Two objects, and it stays
	// two however many times Fetch runs, which is the accumulation this guards.
	want := []string{eopCacheName, eopCacheName + ".etag"}

	got := testutil.BucketKeys(t, fsys, prefix)
	if !slices.Equal(got, want) {
		t.Errorf("cache holds %v, want exactly %v", got, want)
	}
}

// TestInitRegistersTheLoader is the whole point of the inversion: merely
// importing this package must wire astrogo/time up, because that is what
// keeps the change invisible to every existing caller.
func TestInitRegistersTheLoader(t *testing.T) {
	scratchCache(t)

	ctx := context.Background()

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if err := remote.WriteFile(ctx, fsys, prefix+eopCacheName, strings.NewReader(sampleFinals2000A)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	// Nothing here registers a loader: if init did not, this reports
	// ErrNoEOPLoader instead of finding the file above.
	// MJD 41684 as a JD, on the UTC scale.
	time.FromJD(41684+2400000.5, time.UTC).EOP()

	if got := time.EOPSource(); got != "cache" {
		t.Errorf("EOPSource = %q, want %q", got, "cache")
	}
}

// TestCachedDegradesWhenTheCacheCannotBeOpened covers the branch an air-gapped
// deployment hits when its cache location is wrong.
//
// Every failure here answers ErrNoEOPData rather than the underlying error, on
// purpose: Time.EOP has no error return, so a caller cannot be told anything
// richer than "no data", and time's own one-time warning is the notice that
// accuracy degraded. Returning a different error would only push a value that
// nothing can read further up.
func TestCachedDegradesWhenTheCacheCannotBeOpened(t *testing.T) {
	scope := remote.Capture(remote.IERSFinals2000A)
	t.Cleanup(scope.Restore)

	// A scheme no driver registers, so opening the cache fsys fails.
	remote.SetDataDir("no-such-scheme://example.invalid/cache")

	if _, err := (eopLoader{}).Cached(t.Context()); !errors.Is(err, time.ErrNoEOPData) {
		t.Errorf("Cached with an unopenable cache = %v, want ErrNoEOPData", err)
	}
}
