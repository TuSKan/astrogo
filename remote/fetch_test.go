package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errValidateTest = errors.New("not a valid LSK kernel")

func cleanRemoteState(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	SetDataDirPath(t.TempDir())
}

func TestGetFileImmutableExistenceOnly(t *testing.T) {
	cleanRemoteState(t)

	var hits atomic.Int32

	const payload = "kernel-v1"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)

		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFSPK, 0)

	f, err := GetFile(context.Background(), NAIFSPK, "planets/de440s.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	data, _ := f.ReadAll()
	if string(data) != payload {
		t.Fatalf("unexpected content %q", data)
	}

	if _, err := GetFile(context.Background(), NAIFSPK, "planets/de440s.bsp"); err != nil {
		t.Fatalf("GetFile (cached): %v", err)
	}

	if got := hits.Load(); got != 1 {
		t.Errorf("immutable endpoint must not re-fetch on second GetFile; got %d hits", got)
	}
}

func TestGetFileMutableHeadProbeReuse(t *testing.T) {
	cleanRemoteState(t)

	const payload = "eop-data-v1"

	const etag = `"v1"`

	var getHits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", etag)

		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
			return
		}

		getHits.Add(1)

		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(IERSFinals2000A, 0)

	f, err := GetFile(context.Background(), IERSFinals2000A, "", WithCacheName("finals2000A.data"))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	data, _ := f.ReadAll()
	if string(data) != payload {
		t.Fatalf("unexpected content %q", data)
	}

	if got := getHits.Load(); got != 1 {
		t.Fatalf("expected 1 GET after first fetch, got %d", got)
	}

	// Second call: HEAD probe reports the same ETag, cache reused untouched.
	if _, err := GetFile(context.Background(), IERSFinals2000A, "", WithCacheName("finals2000A.data")); err != nil {
		t.Fatalf("GetFile (reuse): %v", err)
	}

	if got := getHits.Load(); got != 1 {
		t.Errorf("expected no additional GET on unchanged content, got %d hits", got)
	}
}

func TestGetFileMutableHeadProbeChanged(t *testing.T) {
	cleanRemoteState(t)

	var (
		mu      sync.Mutex
		payload = "eop-data-v1"
		etag    = `"v1"`
		getHits atomic.Int32
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		p, e := payload, etag
		mu.Unlock()

		w.Header().Set("ETag", e)

		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", strconv.Itoa(len(p)))
			return
		}

		getHits.Add(1)

		_, _ = w.Write([]byte(p))
	}))
	defer srv.Close()

	if err := SetURL(IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(IERSFinals2000A, 0)

	if _, err := GetFile(context.Background(), IERSFinals2000A, "", WithCacheName("finals2000A.data")); err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	mu.Lock()
	payload = "eop-data-v2"
	etag = `"v2"`
	mu.Unlock()

	f, err := GetFile(context.Background(), IERSFinals2000A, "", WithCacheName("finals2000A.data"))
	if err != nil {
		t.Fatalf("GetFile (changed): %v", err)
	}

	data, _ := f.ReadAll()
	if string(data) != "eop-data-v2" {
		t.Errorf("cache not refreshed after upstream change: got %q", data)
	}

	if got := getHits.Load(); got != 2 {
		t.Errorf("expected 2 GETs (initial + refresh), got %d", got)
	}
}

func TestGetFileWithValidateRejectsCorruptDownload(t *testing.T) {
	cleanRemoteState(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("bad-lsk-content"))
	}))
	defer srv.Close()

	if err := SetURL(NAIFLSK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFLSK, 0)

	_, err := GetFile(context.Background(), NAIFLSK, "naif0012.tls",
		WithValidate(func([]byte) error { return errValidateTest }))
	if !errors.Is(err, errValidateTest) {
		t.Fatalf("expected validate error, got %v", err)
	}

	dir, _ := CacheDir(NAIFLSK)
	if dir.Join("naif0012.tls").Exists() {
		t.Error("a validate failure must not leave a cache file behind")
	}
}

func TestGetFileWithCacheNameDiffersFromPath(t *testing.T) {
	cleanRemoteState(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("finals-data"))
	}))
	defer srv.Close()

	if err := SetURL(IERSFinals2000A, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(IERSFinals2000A, 0)

	f, err := GetFile(context.Background(), IERSFinals2000A, "", WithCacheName("finals2000A.data"))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if f.Name() != "finals2000A.data" {
		t.Errorf("cache file name = %q, want %q", f.Name(), "finals2000A.data")
	}
}

func TestGetFileWithProgressReportsBytesDirectSavePath(t *testing.T) {
	cleanRemoteState(t)

	const payload = "kernel-progress-payload"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFSPK, 0)

	var mu sync.Mutex

	var last int64

	var calls int

	_, err := GetFile(context.Background(), NAIFSPK, "planets/progress.bsp",
		WithProgress(func(downloaded, _ int64) {
			mu.Lock()
			defer mu.Unlock()

			calls++
			last = downloaded
		}))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if calls == 0 {
		t.Fatal("WithProgress callback was never invoked")
	}

	if last != int64(len(payload)) {
		t.Errorf("final downloaded = %d, want %d", last, len(payload))
	}

	calls = 0

	if _, err := GetFile(context.Background(), NAIFSPK, "planets/progress.bsp",
		WithProgress(func(int64, int64) { calls++ })); err != nil {
		t.Fatalf("GetFile (cached): %v", err)
	}

	if calls != 0 {
		t.Errorf("WithProgress must not fire on a cache hit; got %d calls", calls)
	}
}

func TestGetFileWithProgressReportsBytesValidatedPath(t *testing.T) {
	cleanRemoteState(t)

	const payload = "leap-second-progress-payload"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(NAIFLSK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFLSK, 0)

	var last int64

	_, err := GetFile(context.Background(), NAIFLSK, "naif0012.tls",
		WithValidate(func([]byte) error { return nil }),
		WithProgress(func(downloaded, _ int64) { last = downloaded }))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if last != int64(len(payload)) {
		t.Errorf("final downloaded = %d, want %d", last, len(payload))
	}
}

func TestGetFileDownloadDeniedWithoutConsent(t *testing.T) {
	cleanRemoteState(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("kernel-bytes"))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	_, err := GetFile(context.Background(), NAIFSPK, "planets/de442.bsp")
	if !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied, got %v", err)
	}

	dir, _ := CacheDir(NAIFSPK)
	if dir.Join("de442.bsp").Exists() {
		t.Error("denied download must not create a cache file")
	}
}

func TestGetFileRespectsOfflineAndDisable(t *testing.T) {
	cleanRemoteState(t)

	EnableDownloads(NAIFSPK, 0)

	Disable(NAIFSPK)

	if _, err := GetFile(context.Background(), NAIFSPK, "planets/de442.bsp"); !errors.Is(err, ErrEndpointDisabled) {
		t.Errorf("expected ErrEndpointDisabled, got %v", err)
	}

	Enable(NAIFSPK)
	SetOffline(true)

	if _, err := GetFile(context.Background(), NAIFSPK, "planets/de442.bsp"); !errors.Is(err, ErrOffline) {
		t.Errorf("expected ErrOffline, got %v", err)
	}
}

func TestGetFileWithDownloadTimeoutOverridesEndpointDefault(t *testing.T) {
	cleanRemoteState(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)

		_, _ = w.Write([]byte("too-slow"))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFSPK, 0)

	_, err := GetFile(context.Background(), NAIFSPK, "planets/slow.bsp", WithDownloadTimeout(10*time.Millisecond))
	if err == nil {
		t.Fatal("expected the request to time out")
	}
}

func TestGetFileReturnsUsableFileForAllOpenModes(t *testing.T) {
	cleanRemoteState(t)

	const payload = "random-access-content"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFSPK, 0)

	f, err := GetFile(context.Background(), NAIFSPK, "planets/de440s.bsp")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if all, err := f.ReadAll(); err != nil || string(all) != payload {
		t.Errorf("ReadAll = %q, %v; want %q, nil", all, err, payload)
	}

	r, err := f.OpenReader()
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}

	seqData, _ := io.ReadAll(r)
	_ = r.Close()

	if string(seqData) != payload {
		t.Errorf("OpenReader content = %q, want %q", seqData, payload)
	}

	rs, err := f.OpenReadSeeker()
	if err != nil {
		t.Fatalf("OpenReadSeeker: %v", err)
	}
	defer rs.Close() //nolint:errcheck // test

	buf := make([]byte, len(payload))
	if _, err := io.ReadFull(rs, buf); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}

	if string(buf) != payload {
		t.Errorf("OpenReadSeeker content = %q, want %q", buf, payload)
	}
}
