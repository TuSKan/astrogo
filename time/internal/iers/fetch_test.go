package iers

import (
	"context"
	"errors"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// sampleFinals2000A mimics finals2000A.all format for two consecutive days
// (same fixture shape as reader_test.go's TestParseFinals2000A), covering
// MJD 41684-41685.
const sampleFinals2000A = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

// fakeIERSSource opens a fresh temp directory as a *file.Bucket, points
// remote.IERSFinals2000A's URL at it (SetURL), and writes content at
// "finals2000A.all" — the real source object name production code reads
// (see fetch.go's GetFile call) — a local stand-in for an HTTP source now
// that GetFile can't reach an http:// URL at all (no httpblob driver
// registered yet; see remote/file's package doc).
func fakeIERSSource(t *testing.T, content string) *file.Bucket {
	t.Helper()

	dir := t.TempDir()

	url := testutil.FileURL(t, dir)

	if err := remote.SetURL(remote.IERSFinals2000A, url); err != nil {
		t.Fatal(err)
	}

	bucket, err := file.Open(context.Background(), url)
	if err != nil {
		t.Fatalf("Open fake source: %v", err)
	}

	if err := bucket.WriteAll(context.Background(), "finals2000A.all", []byte(content), nil); err != nil {
		t.Fatalf("seed fake source: %v", err)
	}

	return bucket
}

func TestEnsureLoadedFetchesWhenUncovered(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)

	// Point the on-disk cache at a scratch dir so this test doesn't read a
	// stale cache file left by another test/run.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after EnsureLoaded")
	}
}

func TestEnsureLoadedSkipsBodyWhenETagUnchanged(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	srcBucket := fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })
	SetRetryCooldown(0)

	// Query an MJD the fixture never covers (41684-41685) so covered()
	// never short-circuits EnsureLoaded before reaching fetch() — what's
	// under test here is remote.GetFile's own ETag-based body-skip, not
	// EnsureLoaded's coverage fast path.
	const uncoveredMJD = 99999

	if err := EnsureLoaded(uncoveredMJD); err != nil {
		t.Fatalf("first EnsureLoaded: %v", err)
	}

	cacheBucket, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	attrsBefore, err := cacheBucket.Attributes(context.Background(), prefix+"finals2000A.data")
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if err := EnsureLoaded(uncoveredMJD); err != nil {
		t.Fatalf("second EnsureLoaded: %v", err)
	}

	// Second call: the source is untouched (same mtime/size, so the same
	// fileblob-derived ETag), so the cache must be reused untouched —
	// proved by the cache object's own ModTime staying identical, which
	// only happens if the body was never re-fetched.
	attrsAfter, err := cacheBucket.Attributes(context.Background(), prefix+"finals2000A.data")
	if err != nil {
		t.Fatalf("Attributes (after): %v", err)
	}

	if !attrsAfter.ModTime.Equal(attrsBefore.ModTime) {
		t.Errorf("cache object was rewritten on an unchanged-source EnsureLoaded: ModTime %v -> %v", attrsBefore.ModTime, attrsAfter.ModTime)
	}

	_ = srcBucket // kept alive for clarity; not mutated in this test
}

func TestEnsureLoadedHTTPError(t *testing.T) {
	t.Skip("untestable against a local file:// fake source — a missing " +
		"file has no HTTP-status concept at all (see remote/file's " +
		"package doc on the current http/https scheme gap); this needs " +
		"a real HTTP fixture once TuSKan/go-cloud's HTTP driver lands")
}

// nonTableModel is a Model that isn't *Table, letting tests directly
// exercise covered()'s type-assertion branch (which only trusts *Table
// for a coverage check) without needing a real parsed Table.
type nonTableModel struct{}

func (nonTableModel) EOP(_ float64) (EOP, error) {
	return EOP{}, nil
}

// TestEnsureLoadedFastPathSkipsLockWhenAlreadyCovered covers the very
// first, unlocked covered(mjd) check — the lock-free fast path taken when
// the registered model already covers the query, before EnsureLoaded ever
// touches fetchMu.
func TestEnsureLoadedFastPathSkipsLockWhenAlreadyCovered(t *testing.T) {
	t.Cleanup(resetForTest)

	table, err := ParseFinals2000A(strings.NewReader(sampleFinals2000A))
	if err != nil {
		t.Fatal(err)
	}

	registerModelInternal(table, SourceZero) // prime a starting model without pinning it explicit

	if err := EnsureLoaded(41684); err != nil {
		t.Errorf("expected nil for an already-covered MJD, got %v", err)
	}
}

// TestEnsureLoadedSkipsEntirelyWhenModelExplicit covers EnsureLoaded's own
// fast path added alongside RegisterModel's authoritative contract: once a
// caller has explicitly registered a model -- even ZeroModel, which covers
// nothing -- EnsureLoaded must return immediately without touching disk or
// network at all, for ANY mjd. Proven by granting no download consent and
// pointing at an empty cache dir: if the fast path didn't fire,
// EnsureLoaded would fall through to a real fetch attempt and fail with
// remote.ErrDownloadDenied, not return nil.
func TestEnsureLoadedSkipsEntirelyWhenModelExplicit(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	RegisterModel(ZeroModel{})

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded with an explicit model should short-circuit and return nil, got: %v", err)
	}

	if _, _, ok := Coverage(); ok {
		t.Error("expected the explicitly-registered ZeroModel to remain untouched, not silently replaced")
	}
}

func TestEnsureLoadedRespectsCooldownAcrossMJDs(t *testing.T) {
	// EnsureLoaded-calling tests earlier in this package's run share the
	// package-level cooldown state (lastAttempt/errLastFetch) — reset it
	// so this test's "cold" scenario isn't silently short-circuited by
	// another test's recent attempt.
	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	srcBucket := fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	// nonTableModel never reports coverage, so the fast-path check can't
	// short-circuit the fetch regardless of the requested MJD.
	registerModelInternal(nonTableModel{}, SourceZero) // prime, not pin — see registerModelInternal

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded (cold): %v", err)
	}

	cacheBucket, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	attrsAfterFirst, err := cacheBucket.Attributes(context.Background(), prefix+"finals2000A.data")
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after EnsureLoaded")
	}

	// Change the source content, so a re-fetch (if the cooldown didn't
	// suppress it) would be observable via a changed cache ModTime.
	if err := srcBucket.WriteAll(context.Background(), "finals2000A.all", []byte(sampleFinals2000A+"\n"), nil); err != nil {
		t.Fatalf("mutate fake source: %v", err)
	}

	// A second call for an MJD the freshly-registered Table does NOT cover
	// must still skip the network: the retry cooldown holds regardless of
	// coverage, since the last attempt (a moment ago) succeeded.
	if err := EnsureLoaded(99999); err != nil {
		t.Fatalf("EnsureLoaded (cooldown): %v", err)
	}

	attrsAfterSecond, err := cacheBucket.Attributes(context.Background(), prefix+"finals2000A.data")
	if err != nil {
		t.Fatalf("Attributes (after second EnsureLoaded): %v", err)
	}

	if !attrsAfterSecond.ModTime.Equal(attrsAfterFirst.ModTime) {
		t.Errorf("cache was refetched despite the retry cooldown: ModTime %v -> %v", attrsAfterFirst.ModTime, attrsAfterSecond.ModTime)
	}
}

// TestEnsureLoadedFallsThroughOnCorruptPreSeededCache covers the disk-read
// step's failure path: a pre-seeded cache file that fails to parse (e.g.
// truncated, hand-edited badly) must not crash or get stuck — EnsureLoaded
// falls through to the consent-gated fetch step exactly as if no cache
// file existed at all.
func TestEnsureLoadedFallsThroughOnCorruptPreSeededCache(t *testing.T) {
	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	// Disable the retry cooldown so the corrupt file's mtime-seeded
	// throttle (proven separately by TestEnsureLoadedSeedsCooldownFromCacheMtime)
	// doesn't mask what this test checks: that a failed disk-read/parse
	// genuinely falls through to the consent-gated fetch step, not just to
	// a suppressed no-op.
	SetRetryCooldown(0)

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	bucket, key, err := CacheFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// A single line with no newline, past bufio.Scanner's default token
	// limit, makes ParseFinals2000A's scan fail.
	if err := bucket.WriteAll(context.Background(), key, []byte(strings.Repeat("x", 70*1024)), nil); err != nil {
		t.Fatal(err)
	}

	// No consent granted: the fetch step, reached after the corrupt cache
	// read fails, must deny rather than hang or panic.
	if err := EnsureLoaded(41684); !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied after a corrupt pre-seeded cache, got %v", err)
	}

	if _, ok := GetModel().(ZeroModel); !ok {
		t.Errorf("model must stay ZeroModel after a corrupt cache read, got %T", GetModel())
	}
}

func TestCacheFile(t *testing.T) {
	t.Cleanup(remote.Reset)

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	_, key, err := CacheFile(context.Background())
	if err != nil {
		t.Fatalf("CacheFile: %v", err)
	}

	if path.Base(key) != "finals2000A.data" {
		t.Errorf("CacheFile key = %q, want base %q", key, "finals2000A.data")
	}
}

func TestEnsureLoadedDefaultDenyIssuesNoRequest(t *testing.T) {
	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	t.Cleanup(remote.Reset)

	fakeIERSSource(t, sampleFinals2000A)

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	err := EnsureLoaded(41684)
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("EnsureLoaded without EnableDownloads: expected ErrDownloadDenied, got %v", err)
	}

	cacheBucket, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	// A failed existence check is not "exists".
	if exists, _ := cacheBucket.Exists(context.Background(), prefix+"finals2000A.data"); exists {
		t.Error("denied fetch must not write a cache file")
	}
}

func TestEnsureLoadedRejectsCorruptDownload(t *testing.T) {
	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	// A single line with no newline, past bufio.Scanner's default token
	// limit, makes ParseFinals2000A's scan fail — a realistic stand-in
	// for a truncated/garbled response.
	corrupt := strings.Repeat("x", 70*1024)

	fakeIERSSource(t, corrupt)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	if err := EnsureLoaded(41684); err == nil {
		t.Fatal("expected EnsureLoaded to reject a corrupt download, got nil error")
	}

	if _, ok := GetModel().(ZeroModel); !ok {
		t.Errorf("model must be unchanged after a rejected download, got %T", GetModel())
	}

	bucket, key, err := CacheFile(context.Background())
	if err != nil {
		t.Fatalf("CacheFile: %v", err)
	}

	// A failed existence check is not "exists", same as a real miss.
	if exists, _ := bucket.Exists(context.Background(), key); exists {
		t.Error("a rejected download must not be written to the cache")
	}
}

func TestCoveredNonTableModel(t *testing.T) {
	t.Cleanup(resetForTest)

	registerModelInternal(nonTableModel{}, SourceZero) // prime, not pin — see registerModelInternal

	if covered(41684) {
		t.Error("covered() must return false for a non-*Table Model")
	}
}

// TestEnsureLoadedReadsPreSeededCacheWithoutNetwork proves the core of the
// lazy-load contract: a finals2000A file already sitting on disk (as if
// hand-copied there, never fetched via remote.GetFile — so it has no
// signature sidecar) is read and registered directly, with zero network
// access and no download consent required.
func TestEnsureLoadedReadsPreSeededCacheWithoutNetwork(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	// No source is configured at all (remote.SetURL is never called, so
	// the endpoint keeps its default https:// URL) — https has no
	// registered blob driver in this build (see remote/file's package
	// doc), so fetch() is guaranteed to fail immediately, without any
	// real network I/O, if it were ever reached. EnsureLoaded returning
	// nil below is therefore proof the disk-read fast path satisfied the
	// query and fetch() was never reached at all, not just that a real
	// fetch attempt would have failed.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	// Pre-seed the cache file directly — bypassing remote.GetFile/consent
	// entirely, exactly like a hand-copied deployment file.
	bucket, key, err := CacheFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := bucket.WriteAll(context.Background(), key, []byte(sampleFinals2000A), nil); err != nil {
		t.Fatal(err)
	}

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after reading the pre-seeded cache")
	}
}

// TestEnsureLoadedSeedsCooldownFromCacheMtime covers the case where a
// pre-seeded cache file exists but doesn't cover the requested MJD: the
// disk-read step still registers it (best available data) and seeds the
// retry cooldown from the file's mtime, so a stale-but-present file
// doesn't cause an immediate network fetch attempt.
func TestEnsureLoadedSeedsCooldownFromCacheMtime(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	// A source WOULD be reachable (real content sitting in it) if fetch()
	// were invoked — so proving the cache's ModTime stays unchanged after
	// EnsureLoaded is genuine evidence the mtime-seeded cooldown suppressed
	// a real fetch, not just that there was nothing to fetch.
	srcBucket := fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	// Pre-seed a fresh cache file that does NOT cover the queried MJD, so
	// the disk-read step registers it but still falls through toward a
	// network fetch — exercising the cooldown-seeded-from-mtime throttle
	// rather than the "already covered" fast path.
	bucket, key, err := CacheFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if err := bucket.WriteAll(context.Background(), key, []byte(sampleFinals2000A), nil); err != nil {
		t.Fatal(err)
	}

	attrsBefore, err := bucket.Attributes(context.Background(), key)
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	const uncoveredMJD = 99999

	if err := EnsureLoaded(uncoveredMJD); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	attrsAfter, err := bucket.Attributes(context.Background(), key)
	if err != nil {
		t.Fatalf("Attributes (after): %v", err)
	}

	if !attrsAfter.ModTime.Equal(attrsBefore.ModTime) {
		t.Errorf("expected the cache-mtime-seeded cooldown to suppress the fetch entirely; cache was rewritten (ModTime %v -> %v)", attrsBefore.ModTime, attrsAfter.ModTime)
	}

	_ = srcBucket // kept alive for clarity; not read directly by this test
}

func TestEnsureLoadedConcurrent(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	registerModelInternal(nonTableModel{}, SourceZero) // prime, not pin — see registerModelInternal

	var wg sync.WaitGroup

	for range 10 {
		wg.Go(func() {
			if err := EnsureLoaded(41684); err != nil {
				t.Errorf("EnsureLoaded: %v", err)
			}
		})
	}

	wg.Wait()

	// No per-request hit counter is available against a local file:// fake
	// source (unlike httptest's handler) — the meaningful property under
	// concurrency is that the cache ends up with exactly one, uncorrupted
	// copy of the fetched content, which the underlying lock/re-check
	// mechanism (see remote/lock_test.go) is responsible for guaranteeing.
	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after concurrent EnsureLoaded calls")
	}

	bucket, key, err := CacheFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	got, err := bucket.ReadAll(context.Background(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != sampleFinals2000A {
		t.Error("cache content was corrupted by concurrent EnsureLoaded calls")
	}
}

// TestFetchContextCancellation exercises fetch (the unexported, ctx-taking
// core EnsureLoaded serializes on) directly — EnsureLoaded itself has no
// ctx parameter (it uses context.Background() internally, matching
// openngc.New()'s lazy-load precedent), so context cancellation can only
// be observed at this lower layer.
func TestFetchContextCancellation(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	bucket, key, err := CacheFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// A failed existence check is not "exists", same as a real miss.
	if exists, _ := bucket.Exists(context.Background(), key); exists {
		t.Error("a cancelled fetch must not write a cache file")
	}
}

// TestFetchDoesNotAccumulateCacheFiles exercises fetch directly (see
// TestFetchContextCancellation's doc comment) — calling it 3 times in a
// row bypasses EnsureLoaded's coverage-based short-circuit, which would
// otherwise make repeat calls no-ops after the first success.
func TestFetchDoesNotAccumulateCacheFiles(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
	})

	fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	for range 3 {
		if err := fetch(context.Background()); err != nil {
			t.Fatalf("fetch: %v", err)
		}
	}

	bucket, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	got := testutil.BucketKeys(t, bucket, prefix)
	if len(got) != 1 || got[0] != "finals2000A.data" {
		t.Errorf("expected exactly one finals2000A.data cache object, got %v", got)
	}
}

func TestSetRetryCooldown(t *testing.T) {
	t.Cleanup(func() {
		resetForTest()
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	srcBucket := fakeIERSSource(t, sampleFinals2000A)

	remote.EnableDownloads(0, remote.IERSFinals2000A)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	SetRetryCooldown(0)
	registerModelInternal(nonTableModel{}, SourceZero) // prime, not pin — see registerModelInternal

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("first EnsureLoaded: %v", err)
	}

	cacheBucket, prefix, err := remote.CacheDir(context.Background(), remote.IERSFinals2000A)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	attrsAfterFirst, err := cacheBucket.Attributes(context.Background(), prefix+"finals2000A.data")
	if err != nil {
		t.Fatalf("Attributes: %v", err)
	}

	// Mutate the source content so the second real GET (if the cooldown
	// doesn't suppress it) is observable via a changed cache ModTime —
	// defeats fetchInto's own content-unchanged reuse, which would
	// otherwise confound what this test actually checks: SetRetryCooldown's
	// throttle, not remote's separate unchanged-content fast path.
	if err := srcBucket.WriteAll(context.Background(), "finals2000A.all", []byte(sampleFinals2000A+"\n"), nil); err != nil {
		t.Fatalf("mutate fake source: %v", err)
	}

	registerModelInternal(nonTableModel{}, SourceZero) // prime, not pin; forces a second real attempt (never covers)

	if err := EnsureLoaded(99999); err != nil {
		t.Fatalf("second EnsureLoaded: %v", err)
	}

	attrsAfterSecond, err := cacheBucket.Attributes(context.Background(), prefix+"finals2000A.data")
	if err != nil {
		t.Fatalf("Attributes (after second EnsureLoaded): %v", err)
	}

	if attrsAfterSecond.ModTime.Equal(attrsAfterFirst.ModTime) {
		t.Error("expected a second real fetch with cooldown disabled; cache was not refetched")
	}
}
