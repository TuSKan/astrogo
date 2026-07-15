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
