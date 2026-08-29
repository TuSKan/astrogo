package remote

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
	atime "github.com/TuSKan/astrogo/time"
)

// sampleFinals2000A mimics finals2000A.all format for two consecutive
// days, covering MJD 41684-41685.
const sampleFinals2000A = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

// These tests moved here from time/internal/iers when the EOP dependency
// was inverted. They were always testing this package's behaviour —
// consent, ETag revalidation, cache layout — through a package that had
// no business knowing about any of it. They now sit next to the code they
// exercise, and iers keeps only the tests about its own logic.

// fakeIERSSource opens a fresh temp directory as a bucket, points
// IERSFinals2000A's URL at it, and writes content at the source object
// name the loader reads. A local stand-in for HTTP, since remote/file has
// no https driver registered in this build.
func fakeIERSSource(t *testing.T, content string) {
	t.Helper()

	url := testutil.FileURL(t, t.TempDir())

	if err := SetURL(IERSFinals2000A, url); err != nil {
		t.Fatal(err)
	}

	bucket, err := file.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("open fake source: %v", err)
	}

	if err := bucket.WriteAll(context.Background(), "finals2000A.all", []byte(content), nil); err != nil {
		t.Fatalf("seed fake source: %v", err)
	}
}

// scratchCache points the data directory at a fresh temp dir so a test
// never reads a cache file left behind by another run.
func scratchCache(t *testing.T) {
	t.Helper()

	SetDataDir(testutil.FileURL(t, t.TempDir()))

	t.Cleanup(func() {
		SetDataDir("")
		Reset()
		atime.ResetEOP()
	})
}

func TestEOPLoaderFetchesAndParses(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	EnableDownloads(0, IERSFinals2000A)

	data, err := eopLoader{}.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if _, err := atime.ParseFinals2000A(bytes.NewReader(data.Raw)); err != nil {
		t.Fatalf("fetched bytes do not parse as finals2000A: %v", err)
	}
}

// TestEOPLoaderDefaultDenyWritesNoCache is the consent contract: without
// EnableDownloads the fetch is refused, and nothing is written.
func TestEOPLoaderDefaultDenyWritesNoCache(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)

	_, err := eopLoader{}.Fetch(context.Background())
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("Fetch without EnableDownloads = %v, want ErrDownloadDenied", err)
	}

	bucket, prefix, err := CacheDir(context.Background(), IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if exists, _ := bucket.Exists(context.Background(), prefix+eopCacheName); exists {
		t.Error("a denied fetch must not write a cache file")
	}
}

// TestEOPLoaderSkipsBodyWhenETagUnchanged proves the revalidation is a
// content check rather than a wall-clock window: an untouched source has
// the same ETag, so the cache object must not be rewritten.
func TestEOPLoaderSkipsBodyWhenETagUnchanged(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	EnableDownloads(0, IERSFinals2000A)

	ctx := context.Background()

	if _, err := (eopLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	bucket, prefix, err := CacheDir(ctx, IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	before, err := bucket.Attributes(ctx, prefix+eopCacheName)
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if _, err := (eopLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	after, err := bucket.Attributes(ctx, prefix+eopCacheName)
	if err != nil {
		t.Fatalf("Attributes (after): %v", err)
	}

	if !after.ModTime.Equal(before.ModTime) {
		t.Errorf("cache rewritten against an unchanged source: ModTime %v -> %v", before.ModTime, after.ModTime)
	}
}

// TestEOPLoaderRejectsCorruptDownload keeps WithValidate honest: a
// response that does not parse must never be trusted as the new cache.
func TestEOPLoaderRejectsCorruptDownload(t *testing.T) {
	scratchCache(t)

	// A single line with no newline, past bufio.Scanner's default token
	// limit, makes ParseFinals2000A's scan fail — a realistic stand-in for
	// a truncated or garbled response. Short garbage will not do: it parses
	// cleanly into an empty table.
	fakeIERSSource(t, strings.Repeat("x", 70*1024))
	EnableDownloads(0, IERSFinals2000A)

	if _, err := (eopLoader{}).Fetch(context.Background()); err == nil {
		t.Fatal("Fetch accepted a corrupt download")
	}

	bucket, prefix, err := CacheDir(context.Background(), IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if exists, _ := bucket.Exists(context.Background(), prefix+eopCacheName); exists {
		t.Error("a corrupt download must not be cached")
	}

	if _, ok := atime.GetModel().(atime.ZeroModel); !ok {
		t.Errorf("model must be unchanged after a rejected download, got %T", atime.GetModel())
	}
}

// TestEOPLoaderCachedReadsAPreSeededFile covers the offline deployment:
// a file copied in by hand has no recorded ETag, so GetFile's cache-hit
// path cannot find it and Cached must read the object directly.
func TestEOPLoaderCachedReadsAPreSeededFile(t *testing.T) {
	scratchCache(t)

	ctx := context.Background()

	bucket, prefix, err := CacheDir(ctx, IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if err := bucket.WriteAll(ctx, prefix+eopCacheName, []byte(sampleFinals2000A), nil); err != nil {
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
	if !errors.Is(err, atime.ErrNoEOPData) {
		t.Fatalf("Cached with an empty cache = %v, want ErrNoEOPData", err)
	}
}

// TestEOPLoaderDoesNotAccumulateCacheFiles guards against a cache that
// grows one object per fetch.
func TestEOPLoaderDoesNotAccumulateCacheFiles(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	EnableDownloads(0, IERSFinals2000A)

	for range 3 {
		if _, err := (eopLoader{}).Fetch(context.Background()); err != nil {
			t.Fatalf("Fetch: %v", err)
		}
	}

	bucket, prefix, err := CacheDir(context.Background(), IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	got := testutil.BucketKeys(t, bucket, prefix)
	if len(got) != 1 || got[0] != eopCacheName {
		t.Errorf("expected exactly one %s cache object, got %v", eopCacheName, got)
	}
}

// TestInitRegistersTheLoader is the whole point of the inversion: merely
// importing this package must wire astrogo/time up, because that is what
// keeps the change invisible to every existing caller.
func TestInitRegistersTheLoader(t *testing.T) {
	scratchCache(t)

	ctx := context.Background()

	bucket, prefix, err := CacheDir(ctx, IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if err := bucket.WriteAll(ctx, prefix+eopCacheName, []byte(sampleFinals2000A), nil); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	// Nothing here registers a loader: if init did not, this reports
	// ErrNoEOPLoader instead of finding the file above.
	// MJD 41684 as a JD, on the UTC scale.
	atime.FromJD(41684+2400000.5, atime.UTC).EOP()

	if got := atime.EOPSource(); got != "cache" {
		t.Errorf("EOPSource = %q, want %q", got, "cache")
	}
}
