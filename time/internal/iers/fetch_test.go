package iers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote"
)

// sampleFinals2000A mimics finals2000A.all format for two consecutive days
// (same fixture shape as reader_test.go's TestParseFinals2000A), covering
// MJD 41684-41685.
const sampleFinals2000A = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

func TestEnsureLoadedFetchesWhenUncovered(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)

	// Point the on-disk cache at a scratch dir so this test doesn't read a
	// stale cache file left by another test/run.
	remote.SetDataDirPath(t.TempDir())
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
		RegisterModel(ZeroModel{})
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	var getCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getCount.Add(1)
		}

		w.Header().Set("ETag", `"fixed-test-etag"`)
		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
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

	if got := getCount.Load(); got != 1 {
		t.Fatalf("expected 1 GET after first EnsureLoaded, got %d", got)
	}

	if err := EnsureLoaded(uncoveredMJD); err != nil {
		t.Fatalf("second EnsureLoaded: %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Errorf("expected still 1 GET after second EnsureLoaded (unchanged ETag should skip the download), got %d", got)
	}
}

func TestEnsureLoadedHTTPError(t *testing.T) {
	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	err := EnsureLoaded(41684)
	if !errors.Is(err, ErrEOPHTTPStatus) {
		t.Fatalf("expected ErrEOPHTTPStatus, got %v", err)
	}

	if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expected status code in error, got: %v", err)
	}
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
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	table, err := ParseFinals2000A(strings.NewReader(sampleFinals2000A))
	if err != nil {
		t.Fatal(err)
	}

	RegisterModel(table)

	if err := EnsureLoaded(41684); err != nil {
		t.Errorf("expected nil for an already-covered MJD, got %v", err)
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
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	var getCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getCount.Add(1)
		}

		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	// nonTableModel never reports coverage, so the fast-path check can't
	// short-circuit the fetch regardless of the requested MJD.
	RegisterModel(nonTableModel{})

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded (cold): %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Fatalf("expected 1 GET after the first EnsureLoaded, got %d", got)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after EnsureLoaded")
	}

	// A second call for an MJD the freshly-registered Table does NOT cover
	// must still skip the network: the retry cooldown holds regardless of
	// coverage, since the last attempt (a moment ago) succeeded.
	if err := EnsureLoaded(99999); err != nil {
		t.Fatalf("EnsureLoaded (cooldown): %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Errorf("expected still 1 GET (cooldown should suppress a retry), got %d", got)
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
		RegisterModel(ZeroModel{})
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	// Disable the retry cooldown so the corrupt file's mtime-seeded
	// throttle (proven separately by TestEnsureLoadedSeedsCooldownFromCacheMtime)
	// doesn't mask what this test checks: that a failed disk-read/parse
	// genuinely falls through to the consent-gated fetch step, not just to
	// a suppressed no-op.
	SetRetryCooldown(0)

	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	cacheFile, err := CacheFile()
	if err != nil {
		t.Fatal(err)
	}

	// A single line with no newline, past bufio.Scanner's default token
	// limit, makes ParseFinals2000A's scan fail.
	if err := cacheFile.WriteAll([]byte(strings.Repeat("x", 70*1024))); err != nil {
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

	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	f, err := CacheFile()
	if err != nil {
		t.Fatalf("CacheFile: %v", err)
	}

	if f.Name() != "finals2000A.data" {
		t.Errorf("CacheFile name = %q, want %q", f.Name(), "finals2000A.data")
	}
}

func TestEnsureLoadedDefaultDenyIssuesNoRequest(t *testing.T) {
	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	t.Cleanup(remote.Reset)

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)

		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	err := EnsureLoaded(41684)
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("EnsureLoaded without EnableDownloads: expected ErrDownloadDenied, got %v", err)
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("denied fetch must not touch the network; server saw %d hits", got)
	}
}

func TestEnsureLoadedRejectsCorruptDownload(t *testing.T) {
	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	// A single line with no newline, past bufio.Scanner's default token
	// limit, makes ParseFinals2000A's scan fail — a realistic stand-in
	// for a truncated/garbled response.
	corrupt := strings.Repeat("x", 70*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(corrupt))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	if err := EnsureLoaded(41684); err == nil {
		t.Fatal("expected EnsureLoaded to reject a corrupt download, got nil error")
	}

	if _, ok := GetModel().(ZeroModel); !ok {
		t.Errorf("model must be unchanged after a rejected download, got %T", GetModel())
	}

	cacheFile, err := CacheFile()
	if err != nil {
		t.Fatalf("CacheFile: %v", err)
	}

	if cacheFile.Exists() {
		t.Error("a rejected download must not be written to the cache")
	}
}

func TestCoveredNonTableModel(t *testing.T) {
	t.Cleanup(func() { RegisterModel(ZeroModel{}) })

	RegisterModel(nonTableModel{})

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
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	// Pre-seed the cache file directly — bypassing remote.GetFile/consent
	// entirely, exactly like a hand-copied deployment file.
	cacheFile, err := CacheFile()
	if err != nil {
		t.Fatal(err)
	}

	if err := cacheFile.WriteAll([]byte(sampleFinals2000A)); err != nil {
		t.Fatal(err)
	}

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("expected zero network hits when a pre-seeded cache file already covers the query, got %d", got)
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
		RegisterModel(ZeroModel{})
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)

		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	// Pre-seed a fresh cache file that does NOT cover the queried MJD, so
	// the disk-read step registers it but still falls through toward a
	// network fetch — exercising the cooldown-seeded-from-mtime throttle
	// rather than the "already covered" fast path.
	cacheFile, err := CacheFile()
	if err != nil {
		t.Fatal(err)
	}

	if err := cacheFile.WriteAll([]byte(sampleFinals2000A)); err != nil {
		t.Fatal(err)
	}

	const uncoveredMJD = 99999

	if err := EnsureLoaded(uncoveredMJD); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("expected the cache-mtime-seeded cooldown to suppress the fetch entirely, got %d GETs", got)
	}
}

func TestEnsureLoadedConcurrent(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	var getCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getCount.Add(1)
		}

		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	RegisterModel(nonTableModel{})

	var wg sync.WaitGroup

	for range 10 {
		wg.Go(func() {
			if err := EnsureLoaded(41684); err != nil {
				t.Errorf("EnsureLoaded: %v", err)
			}
		})
	}

	wg.Wait()

	if got := getCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 GET across concurrent EnsureLoaded calls (re-check-after-lock), got %d", got)
	}
}

// TestFetchContextCancellation exercises fetch (the unexported, ctx-taking
// core EnsureLoaded serializes on) directly — EnsureLoaded itself has no
// ctx parameter (it uses context.Background() internally, matching
// openngc.New()'s lazy-load precedent), so context cancellation can only
// be observed at this lower layer.
func TestFetchContextCancellation(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := fetch(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	cacheFile, err := CacheFile()
	if err != nil {
		t.Fatal(err)
	}

	if cacheFile.Exists() {
		t.Error("a cancelled fetch must not write a cache file")
	}
}

// TestFetchDoesNotAccumulateCacheFiles exercises fetch directly (see
// TestFetchContextCancellation's doc comment) — calling it 3 times in a
// row bypasses EnsureLoaded's coverage-based short-circuit, which would
// otherwise make repeat calls no-ops after the first success.
func TestFetchDoesNotAccumulateCacheFiles(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	for range 3 {
		if err := fetch(context.Background()); err != nil {
			t.Fatalf("fetch: %v", err)
		}
	}

	dir, err := remote.CacheDir(remote.IERSFinals2000A)
	if err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir.LocalPath())
	if err != nil {
		t.Fatal(err)
	}

	var dataFiles []string

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".signature.json") {
			dataFiles = append(dataFiles, e.Name())
		}
	}

	if len(dataFiles) != 1 || dataFiles[0] != "finals2000A.data" {
		t.Errorf("expected exactly one finals2000A.data cache file, got %v", dataFiles)
	}
}

func TestSetRetryCooldown(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
		SetRetryCooldown(5 * time.Minute)
	})

	fetchMu.Lock()
	lastAttempt = time.Time{}
	errLastFetch = nil
	fetchMu.Unlock()

	var getCount atomic.Int32

	var reqCount atomic.Int32

	// A unique ETag on every response (GET or HEAD) defeats remote.GetFile's
	// own HEAD-probe cache reuse (the IERS endpoint is Mutable) — otherwise
	// the second EnsureLoaded's HEAD probe would see the same ETag the first
	// GET's response carried and reuse the cache without a real GET,
	// confounding what this test actually checks: SetRetryCooldown's
	// throttle, not remote's separate content-unchanged reuse.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			getCount.Add(1)
		}

		w.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, reqCount.Add(1)))
		_, _ = w.Write([]byte(sampleFinals2000A))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.IERSFinals2000A, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	SetRetryCooldown(0)
	RegisterModel(nonTableModel{})

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("first EnsureLoaded: %v", err)
	}

	RegisterModel(nonTableModel{}) // force a second real attempt (never covers)

	if err := EnsureLoaded(99999); err != nil {
		t.Fatalf("second EnsureLoaded: %v", err)
	}

	if got := getCount.Load(); got != 2 {
		t.Errorf("expected 2 GETs with cooldown disabled, got %d", got)
	}
}
