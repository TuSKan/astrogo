package iers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote"
)

func TestFetchNow(t *testing.T) {
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

	if err := FetchNow(context.Background()); err != nil {
		t.Fatalf("FetchNow: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after FetchNow")
	}
}

func TestFetchNowSkipsBodyWhenETagUnchanged(t *testing.T) {
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

	if err := FetchNow(context.Background()); err != nil {
		t.Fatalf("first FetchNow: %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Fatalf("expected 1 GET after first FetchNow, got %d", got)
	}

	if err := FetchNow(context.Background()); err != nil {
		t.Fatalf("second FetchNow: %v", err)
	}

	if got := getCount.Load(); got != 1 {
		t.Errorf("expected still 1 GET after second FetchNow (unchanged ETag should skip the download), got %d", got)
	}
}

func TestFetchNowHTTPError(t *testing.T) {
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

	err := FetchNow(context.Background())
	if !errors.Is(err, ErrEOPHTTPStatus) {
		t.Fatalf("expected ErrEOPHTTPStatus, got %v", err)
	}

	if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
		t.Errorf("expected status code in error, got: %v", err)
	}
}

// neverCoveredModel is a Model that reports no coverage for any epoch,
// distinct from ZeroModel: RegisterModel(ZeroModel{}) is indistinguishable
// from "nothing registered yet" (registerIfDefault's own sentinel check),
// so it doesn't actually block the package's one-time lazy embedded-data
// load from overwriting it on the next GetModel() call. A distinct type
// sidesteps that entirely.
type neverCoveredModel struct{}

func (neverCoveredModel) EOP(_ float64) (EOP, error) {
	return EOP{}, nil
}

func TestFetchIfStale(t *testing.T) {
	// FetchNow-calling tests earlier in this package's run share the
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

	// neverCoveredModel never reports coverage, so the fast-path check
	// can't short-circuit the fetch regardless of the requested MJD.
	RegisterModel(neverCoveredModel{})

	if err := FetchIfStale(41684); err != nil {
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
	if err := FetchIfStale(99999); err != nil {
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

func TestFetchNowDefaultDenyIssuesNoRequest(t *testing.T) {
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

	err := FetchNow(context.Background())
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("FetchNow without EnableDownloads: expected ErrDownloadDenied, got %v", err)
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("denied fetch must not touch the network; server saw %d hits", got)
	}
}
