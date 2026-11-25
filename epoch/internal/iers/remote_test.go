package iers

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/remotetest"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// These are the remote-backed loader's tests. They lived in remote/eop with
// the loader while epoch could not import remote, and came back with it.

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

	fsys, err := remote.OpenFS(context.Background(), url)
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

	t.Cleanup(remote.Capture().Restore)
	t.Cleanup(Reset)

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
}

func TestEOPLoaderFetchesAndParses(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)

	data, err := remoteLoader{}.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if _, err := ParseFinals2000A(bytes.NewReader(data.Raw)); err != nil {
		t.Fatalf("fetched bytes do not parse as finals2000A: %v", err)
	}
}

// TestEOPLoaderUnderARefusingPolicyWritesNoCache is the download policy's
// contract: a fetch it refuses is refused, and nothing is written.
func TestEOPLoaderUnderARefusingPolicyWritesNoCache(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	remotetest.RefuseDownloads(t, remote.IERSFinals2000A)

	_, err := remoteLoader{}.Fetch(context.Background())
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("Fetch under a refusing policy = %v, want remote.ErrDownloadDenied", err)
	}

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if _, err := fs.Stat(fsys, prefix+cacheName); err == nil {
		t.Error("a denied fetch must not write a cache file")
	}
}

// TestEOPLoaderSkipsBodyWhenETagUnchanged proves the revalidation is a
// content check rather than a wall-clock window: an untouched source has
// the same ETag, so the cache object must not be rewritten.
func TestEOPLoaderSkipsBodyWhenETagUnchanged(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)

	ctx := context.Background()

	if _, err := (remoteLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	before, err := fs.Stat(fsys, prefix+cacheName)
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if _, err := (remoteLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	after, err := fs.Stat(fsys, prefix+cacheName)
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

	for _, c := range []struct{ name, body string }{
		// A single line with no newline, past bufio.Scanner's default token
		// limit, makes ParseFinals2000A's scan fail: a garbled response.
		{"a line past the scanner's limit", strings.Repeat("x", 70*1024)},
		// What a server under maintenance sends with a 200. Every line is
		// shorter than a bulletin row, and it parsed into an empty table, so
		// this test used the line above instead and the page was accepted
		// (#1042).
		{"an error page", errorPage},
	} {
		t.Run(c.name, func(t *testing.T) {
			scratchCache(t)
			fakeIERSSource(t, c.body)

			if _, err := (remoteLoader{}).Fetch(context.Background()); err == nil {
				t.Fatal("Fetch accepted a corrupt download")
			}

			fsys, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
			if err != nil {
				t.Fatalf("remote.CacheDir: %v", err)
			}

			if _, err := fs.Stat(fsys, prefix+cacheName); err == nil {
				t.Error("a corrupt download must not be cached")
			}

			if _, ok := GetModel().(ZeroModel); !ok {
				t.Errorf("model must be unchanged after a rejected download, got %T", GetModel())
			}
		})
	}
}

// errorPage is what a server under maintenance answers with a 200.
const errorPage = "<html><body><h1>503 Service Unavailable</h1></body></html>\n"

// A good bulletin in the cache survives the source starting to serve an error
// page. The page parsed as an empty bulletin, passed the validator and
// replaced the good copy on disk (#1042).
func TestEOPLoaderKeepsAGoodBulletinOverAnErrorPage(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)

	ctx := context.Background()

	if _, err := (remoteLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	fakeIERSSource(t, errorPage)

	if data, err := (remoteLoader{}).Fetch(ctx); err == nil {
		t.Errorf("Fetch of an error page: %d bytes and no error", len(data.Raw))
	}

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	cached, err := fs.ReadFile(fsys, prefix+cacheName)
	if err != nil {
		t.Fatalf("read the cached bulletin: %v", err)
	}

	if string(cached) != sampleFinals2000A {
		t.Errorf("the cached bulletin is now %q, want the good one kept", cached[:min(len(cached), 60)])
	}
}

// A cached bulletin that no longer parses is replaced from the source in the
// same Fetch, though the source's ETag has not changed. The parser was passed
// as WithValidate, which sees only a download, so GetFile returned such a copy
// for as long as the ETag held, and time failed to parse it each time (#1043).
//
// The damage here is a NaN UT1-UTC: a copy cached before #1037, when the
// parser accepted one, is exactly a copy that went bad in place.
func TestEOPLoaderReplacesADamagedCachedBulletin(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)

	ctx := context.Background()

	if _, err := (remoteLoader{}).Fetch(ctx); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	damaged := strings.Replace(sampleFinals2000A, " 0.8084178", "       NaN", 1)
	if damaged == sampleFinals2000A {
		t.Fatal("the damage did not apply")
	}

	if err := remote.WriteFile(ctx, fsys, prefix+cacheName, strings.NewReader(damaged)); err != nil {
		t.Fatalf("damage the cached copy: %v", err)
	}

	data, err := (remoteLoader{}).Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch over a damaged cached copy: %v", err)
	}

	if string(data.Raw) != sampleFinals2000A {
		t.Errorf("Fetch returned %q, want the source's bulletin", data.Raw[:min(len(data.Raw), 60)])
	}

	cached, err := fs.ReadFile(fsys, prefix+cacheName)
	if err != nil || string(cached) != sampleFinals2000A {
		t.Errorf("the cached copy is not the source's after the heal (err %v)", err)
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

	if err := remote.WriteFile(ctx, fsys, prefix+cacheName, strings.NewReader(sampleFinals2000A)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	data, err := remoteLoader{}.Cached(ctx)
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

	_, err := remoteLoader{}.Cached(context.Background())
	if !errors.Is(err, ErrNoEOPData) {
		t.Fatalf("Cached with an empty cache = %v, want ErrNoEOPData", err)
	}
}

// TestEOPLoaderDoesNotAccumulateCacheFiles guards against a cache that
// grows one object per fetch.
func TestEOPLoaderDoesNotAccumulateCacheFiles(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)

	for range 3 {
		if _, err := (remoteLoader{}).Fetch(context.Background()); err != nil {
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
	want := []string{cacheName, cacheName + ".etag"}

	got := testutil.BucketKeys(t, fsys, prefix)
	if !slices.Equal(got, want) {
		t.Errorf("cache holds %v, want exactly %v", got, want)
	}
}

// TestTheRemoteLoaderIsTheDefault: with no loader registered, the lazy load
// reads the bulletin through remote, so a file pre-seeded in remote's cache is
// what an EOP lookup finds.
func TestTheRemoteLoaderIsTheDefault(t *testing.T) {
	installLoader(t, nil)
	scratchCache(t)

	ctx := context.Background()

	fsys, prefix, err := remote.CacheDir(ctx, remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("remote.CacheDir: %v", err)
	}

	if err := remote.WriteFile(ctx, fsys, prefix+cacheName, strings.NewReader(sampleFinals2000A)); err != nil {
		t.Fatalf("pre-seed: %v", err)
	}

	// MJD 41684 as a JD, on the UTC scale.
	_ = EnsureLoaded(41684)

	if got := EOPSource(); got != "cache" {
		t.Errorf("EOPSource = %q, want %q", got, "cache")
	}
}

// TestCachedDegradesWhenTheCacheCannotBeOpened covers the branch an air-gapped
// deployment hits when its cache location is wrong.
//
// Every failure here answers ErrNoEOPData rather than the underlying error, on
// purpose: Time.EOP has no error return, so a caller cannot be told anything
// richer than "no data", and epoch's own one-time warning is the notice that
// accuracy degraded. Returning a different error would only push a value that
// nothing can read further up.
func TestCachedDegradesWhenTheCacheCannotBeOpened(t *testing.T) {
	scope := remote.Capture(remote.IERSFinals2000A)
	t.Cleanup(scope.Restore)

	// A scheme no driver registers, so opening the cache fsys fails.
	remote.SetDataDir("no-such-scheme://example.invalid/cache")

	if _, err := (remoteLoader{}).Cached(t.Context()); !errors.Is(err, ErrNoEOPData) {
		t.Errorf("Cached with an unopenable cache = %v, want ErrNoEOPData", err)
	}
}

// TestEOPFetchUnderARefusingPolicyIgnoresAForeignLock is #424's case at the
// loader: another process holds the lock on the EOP cache entry, and this one's
// policy refuses the download. The fetch must be refused at once rather than
// wait for the lock, which it used to do for up to half an hour, holding the
// EOP mutex.
func TestEOPFetchUnderARefusingPolicyIgnoresAForeignLock(t *testing.T) {
	scratchCache(t)
	fakeIERSSource(t, sampleFinals2000A)
	remotetest.RefuseDownloads(t, remote.IERSFinals2000A)

	fsys, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	if err := remote.WriteFile(context.Background(), fsys, prefix+cacheName+".lock", strings.NewReader("")); err != nil {
		t.Fatalf("plant lock: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if _, err := (remoteLoader{}).Fetch(ctx); !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("Fetch with a foreign lock under a refusing policy: %v, want ErrDownloadDenied at once", err)
	}
}
