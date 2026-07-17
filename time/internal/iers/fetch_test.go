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

func TestFetch(t *testing.T) {
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

	if err := Fetch(context.Background()); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after Fetch")
	}
}

func TestFetchSkipsBodyWhenETagUnchanged(t *testing.T) {
	t.Cleanup(func() {
		RegisterModel(ZeroModel{})
		remote.Reset()
	})

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

	if err := Fetch(context.Background()); err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Fatalf("expected 1 GET after first Fetch, got %d", got)
	}

	if err := Fetch(context.Background()); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Errorf("expected still 1 GET after second Fetch (unchanged ETag should skip the download), got %d", got)
	}
}

func TestFetchHTTPError(t *testing.T) {
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

	err := Fetch(context.Background())
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

func TestFetchIfStale(t *testing.T) {
	// Fetch-calling tests earlier in this package's run share the
	// package-level cooldown state (lastAttempt/errLastFetch) with
	// FetchIfStale — reset it so this test's "cold" scenario isn't
	// silently short-circuited by another test's recent attempt.
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

	if err := FetchIfStale(context.Background(), 41684); err != nil {
		t.Fatalf("FetchIfStale (cold): %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Fatalf("expected 1 GET after the first FetchIfStale, got %d", got)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after FetchIfStale")
	}

	// A second call for an MJD the freshly-registered Table does NOT cover
	// must still skip the network: the retry cooldown holds regardless of
	// coverage, since the last attempt (a moment ago) succeeded.
	if err := FetchIfStale(context.Background(), 99999); err != nil {
		t.Fatalf("FetchIfStale (cooldown): %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Errorf("expected still 1 GET (cooldown should suppress a retry), got %d", got)
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

func TestFetchDefaultDenyIssuesNoRequest(t *testing.T) {
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

	err := Fetch(context.Background())
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("Fetch without EnableDownloads: expected ErrDownloadDenied, got %v", err)
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("denied fetch must not touch the network; server saw %d hits", got)
	}
}

func TestFetchRejectsCorruptDownload(t *testing.T) {
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

	if err := Fetch(context.Background()); err == nil {
		t.Fatal("expected Fetch to reject a corrupt download, got nil error")
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

func TestFetchIfStaleSeedsCooldownFromCacheMtime(t *testing.T) {
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

	// Pre-seed a fresh cache file so FetchIfStale's cold-process cooldown
	// seed (CacheFile().Info().Modified) throttles the very first call in
	// this process, before any Fetch/FetchIfStale has run here.
	cacheFile, err := CacheFile()
	if err != nil {
		t.Fatal(err)
	}

	if err := cacheFile.WriteAll([]byte(sampleFinals2000A)); err != nil {
		t.Fatal(err)
	}

	RegisterModel(nonTableModel{}) // never covers, so the fast path can't skip the seed check

	if err := FetchIfStale(context.Background(), 41684); err != nil {
		t.Fatalf("FetchIfStale: %v", err)
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("expected the cache-mtime-seeded cooldown to suppress the fetch entirely, got %d GETs", got)
	}
}

func TestFetchIfStaleConcurrent(t *testing.T) {
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
			if err := FetchIfStale(context.Background(), 41684); err != nil {
				t.Errorf("FetchIfStale: %v", err)
			}
		})
	}

	wg.Wait()

	if got := getCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 GET across concurrent FetchIfStale calls (re-check-after-lock), got %d", got)
	}
}

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

	if err := Fetch(ctx); !errors.Is(err, context.Canceled) {
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
		if err := Fetch(context.Background()); err != nil {
			t.Fatalf("Fetch: %v", err)
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
	// the second FetchIfStale's HEAD probe would see the same ETag the first
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

	if err := FetchIfStale(context.Background(), 41684); err != nil {
		t.Fatalf("first FetchIfStale: %v", err)
	}

	RegisterModel(nonTableModel{}) // force a second real attempt (never covers)

	if err := FetchIfStale(context.Background(), 99999); err != nil {
		t.Fatalf("second FetchIfStale: %v", err)
	}

	if got := getCount.Load(); got != 2 {
		t.Errorf("expected 2 GETs with cooldown disabled, got %d", got)
	}
}
