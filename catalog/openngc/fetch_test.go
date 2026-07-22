package openngc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	astrotime "github.com/TuSKan/astrogo/time"
)

const (
	sampleNGCCSV = `Name;Type;RA;Dec;M;Common names;Identifiers;V-Mag;B-Mag
NGC1976;Nb;05:35:17.3;-05:23:28;42;Orion Nebula;;4.0;5.5
`
	sampleAddendumCSV = `Name;Type;RA;Dec;M;Common names;Identifiers;V-Mag;B-Mag
NGC0224;G;00:42:44.3;+41:16:09;31;Andromeda Galaxy;;3.4;4.4
`
)

// serveSources returns an httptest.Server that responds to the two OpenNGC
// source paths (NGC.csv/addendum.csv, as joined onto remote.OpenNGC's base
// URL) with fixed fixture content, tracking how many GET requests each path
// received.
func serveSources(t *testing.T, getCounts map[string]*int32) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body string

		switch {
		case strings.HasSuffix(r.URL.Path, "NGC.csv"):
			body = sampleNGCCSV
		case strings.HasSuffix(r.URL.Path, "addendum.csv"):
			body = sampleAddendumCSV
		default:
			http.NotFound(w, r)
			return
		}

		if r.Method == http.MethodGet {
			if c, ok := getCounts[r.URL.Path]; ok {
				atomic.AddInt32(c, 1)
			}
		}

		w.Header().Set("ETag", `"`+body[:8]+`"`) // stable per-fixture ETag
		_, _ = w.Write([]byte(body))
	}))
}

func TestNewFetchesFromNetworkWhenDownloadsEnabled(t *testing.T) {
	t.Cleanup(remote.Reset)

	getCounts := map[string]*int32{
		"/NGC.csv":      new(int32),
		"/addendum.csv": new(int32),
	}

	srv := serveSources(t, getCounts)
	defer srv.Close()

	if err := remote.SetURL(remote.OpenNGC, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.OpenNGC, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	p := New()

	got, ok := p.Resolve(context.Background(), "M42")
	if !ok || got.ID != "NGC1976" {
		t.Errorf("Resolve(M42) = %+v, %v, want NGC1976, true", got, ok)
	}

	// Regression: Epoch used to never be set despite OpenNGC's RA/Dec being
	// implicitly J2000 by the catalog's own convention.
	if !got.Epoch.Equal(astrotime.J2000) {
		t.Errorf("Epoch = %v, want time.J2000", got.Epoch)
	}

	if got, ok := p.Resolve(context.Background(), "M31"); !ok || got.ID != "NGC224" {
		t.Errorf("Resolve(M31) = %+v, %v, want NGC224, true", got, ok)
	}
}

func TestNewSkipsBodyWhenUnchanged(t *testing.T) {
	t.Cleanup(remote.Reset)

	getCounts := map[string]*int32{
		"/NGC.csv":      new(int32),
		"/addendum.csv": new(int32),
	}

	srv := serveSources(t, getCounts)
	defer srv.Close()

	if err := remote.SetURL(remote.OpenNGC, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.OpenNGC, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	_ = New()

	for path, c := range getCounts {
		if got := atomic.LoadInt32(c); got != 1 {
			t.Fatalf("expected 1 GET for %s after first New(), got %d", path, got)
		}
	}

	_ = New()

	for path, c := range getCounts {
		if got := atomic.LoadInt32(c); got != 1 {
			t.Errorf("expected still 1 GET for %s after second New() (unchanged content should skip download), got %d", path, got)
		}
	}
}

func TestNewDefaultDenyIssuesNoRequest(t *testing.T) {
	t.Cleanup(remote.Reset)

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)

		_, _ = w.Write([]byte(sampleNGCCSV))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.OpenNGC, srv.URL); err != nil {
		t.Fatal(err)
	}

	// Downloads intentionally left disabled (the default).
	p := New()

	if got := hits.Load(); got != 0 {
		t.Errorf("New() must not touch the network when downloads aren't enabled; server saw %d hits", got)
	}

	if _, ok := p.Resolve(context.Background(), "M42"); ok {
		t.Error("expected an empty provider when downloads are disabled")
	}
}

// TestNewDoesNotAccumulateCacheFiles is a regression test: repeated New()
// calls must reuse a single cache file per source name, never leave stale
// versions behind (the concern that originally motivated fetchSource).
func TestNewDoesNotAccumulateCacheFiles(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := serveSources(t, map[string]*int32{})
	defer srv.Close()

	if err := remote.SetURL(remote.OpenNGC, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(remote.OpenNGC, 0)
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	for range 3 {
		_ = New()
	}

	dir, err := remote.CacheDir(remote.OpenNGC)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	entries, err := os.ReadDir(dir.LocalPath())
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}

	wantNames := map[string]bool{"NGC.csv": true, "addendum.csv": true}
	gotFiles := 0

	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), ".signature.json") {
			continue
		}

		gotFiles++

		if !wantNames[e.Name()] {
			t.Errorf("unexpected cache file %q (possible version accumulation)", e.Name())
		}
	}

	if gotFiles != len(wantNames) {
		t.Errorf("expected exactly %d cache files after 3 New() calls, got %d", len(wantNames), gotFiles)
	}
}
