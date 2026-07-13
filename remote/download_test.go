package remote

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	gofs "github.com/ungerik/go-fs"
)

func TestDownloadDefaultDenyIssuesNoRequest(t *testing.T) {
	t.Cleanup(Reset)

	var hits int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		_, _ = w.Write([]byte("kernel-bytes"))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	dest := gofs.File(filepath.Join(t.TempDir(), "de442.bsp"))

	err := Download(context.Background(), NAIFSPK, "planets/de442.bsp", dest)
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied, got %v", err)
	}

	if hits != 0 {
		t.Errorf("denied download must not touch the network; server saw %d hits", hits)
	}

	if dest.Exists() {
		t.Error("denied download must not create the destination file")
	}
}

func TestDownloadEnabledSucceeds(t *testing.T) {
	t.Cleanup(Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("kernel-bytes"))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFSPK, 0)

	dest := gofs.File(filepath.Join(t.TempDir(), "de440s.bsp"))

	if err := Download(context.Background(), NAIFSPK, "planets/de440s.bsp", dest); err != nil {
		t.Fatalf("Download: %v", err)
	}

	data, err := os.ReadFile(dest.LocalPath())
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}

	if string(data) != "kernel-bytes" {
		t.Errorf("unexpected content %q", data)
	}
}

func TestDownloadContentLengthOverLimitDenied(t *testing.T) {
	t.Cleanup(Reset)

	payload := strings.Repeat("x", 4096)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	// NAIFSPK's ApproxSize is SizeVaries (-1), so the pre-request check
	// passes and the denial must come from the Content-Length re-check.
	EnableDownloads(NAIFSPK, 1024)

	dest := gofs.File(filepath.Join(t.TempDir(), "big.bsp"))

	err := Download(context.Background(), NAIFSPK, "planets/big.bsp", dest)
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected Content-Length denial, got %v", err)
	}

	if dest.Exists() {
		t.Error("denied download must not leave a destination file")
	}
}

func TestDownloadNoPartialFileOnMidBodyFailure(t *testing.T) {
	t.Cleanup(Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "1000000")

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		_, _ = w.Write([]byte("partial"))

		flusher.Flush()

		// Abort the connection mid-body.
		hj, ok := w.(http.Hijacker)
		if !ok {
			return
		}

		conn, _, _ := hj.Hijack()
		_ = conn.Close()
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFSPK, 0)

	dir := t.TempDir()
	dest := gofs.File(filepath.Join(dir, "broken.bsp"))

	err := Download(context.Background(), NAIFSPK, "planets/broken.bsp", dest)
	if err == nil {
		t.Fatal("expected mid-body failure")
	}

	if dest.Exists() {
		t.Error("failed download must not leave a partial destination file")
	}

	// No leaked temp files either.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "download-") {
			t.Errorf("leaked temp file %s", e.Name())
		}
	}
}

func TestDownloadProgressCallback(t *testing.T) {
	t.Cleanup(Reset)

	payload := strings.Repeat("y", 8192)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(NAIFLSK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFLSK, 0)

	var lastWritten, lastTotal int64

	dest := gofs.File(filepath.Join(t.TempDir(), "naif0012.tls"))

	err := Download(context.Background(), NAIFLSK, "lsk/naif0012.tls", dest,
		WithProgress(func(written, total int64) {
			lastWritten, lastTotal = written, total
		}))
	if err != nil {
		t.Fatalf("Download: %v", err)
	}

	if lastWritten != int64(len(payload)) {
		t.Errorf("progress written = %d, want %d", lastWritten, len(payload))
	}

	if lastTotal != int64(len(payload)) {
		t.Errorf("progress total = %d, want %d", lastTotal, len(payload))
	}
}

func TestDownloadURLBypassesConsentButNotOffline(t *testing.T) {
	t.Cleanup(Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("generated-data"))
	}))
	defer srv.Close()

	dest := gofs.File(filepath.Join(t.TempDir(), "gen.csv"))

	// Explicit tool invocation = consent: no EnableDownloads needed.
	if err := DownloadURL(context.Background(), srv.URL, dest); err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}

	if data, _ := os.ReadFile(dest.LocalPath()); string(data) != "generated-data" {
		t.Errorf("unexpected content %q", data)
	}

	// Global offline still applies.
	SetOffline(true)

	dest2 := gofs.File(filepath.Join(t.TempDir(), "gen2.csv"))
	if err := DownloadURL(context.Background(), srv.URL, dest2); !errors.Is(err, ErrOffline) {
		t.Errorf("expected ErrOffline, got %v", err)
	}
}

func TestSubsystemDirAndDataDirOverride(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	base := t.TempDir()
	SetDataDirPath(base)

	if got := DataDir().LocalPath(); got != base {
		t.Errorf("DataDir = %s, want %s", got, base)
	}

	dir, err := SubsystemDir("jpl")
	if err != nil {
		t.Fatalf("SubsystemDir: %v", err)
	}

	if !dir.IsDir() {
		t.Errorf("SubsystemDir should create %s", dir)
	}

	if filepath.Base(dir.LocalPath()) != "jpl" {
		t.Errorf("unexpected subsystem dir %s", dir)
	}

	// Default (unset) resolves under the user cache dir.
	SetDataDir("")

	cacheBase, _ := os.UserCacheDir()
	if want := filepath.Join(cacheBase, "astrogo"); DataDir().LocalPath() != want {
		t.Errorf("default DataDir = %s, want %s", DataDir().LocalPath(), want)
	}
}
