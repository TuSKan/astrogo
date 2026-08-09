package s3

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/TuSKan/astrogo/remote"
)

// testEndpointID/testBucket reuse the one real KindS3 endpoint the
// registry ships (remote.CopernicusEODATA, Bucket "eodata") — there is no
// public API to register a scratch KindS3 endpoint from outside package
// remote (endpoints are fixed at compile time in remote's own registry),
// so every test below points the real endpoint's URL at a local
// httptest.Server via remote.SetURL and restores it via remote.Capture.
const (
	testEndpointID = remote.CopernicusEODATA
	testBucket     = "eodata"
)

// registerTestEndpoint points testEndpointID at url for the duration of
// the calling test and restores the registry (URL, download consent,
// enabled state) on cleanup.
func registerTestEndpoint(t *testing.T, url string) {
	t.Helper()
	t.Cleanup(remote.Capture(testEndpointID).Restore)

	if err := remote.SetURL(testEndpointID, url); err != nil {
		t.Fatalf("SetURL: %v", err)
	}
}

type fakeObject struct {
	data []byte
	etag string
}

// errTestValidateFailed is the static sentinel TestFetchIntoValidateFailureLeavesNoCacheFile's
// injected WithValidate hook returns.
var errTestValidateFailed = errors.New("not a valid NetCDF file")

// fakeS3Server serves just enough of the S3 HTTP API (path-style HEAD/GET
// against /{testBucket}/{key}, a NoSuchKey 404 body) for the AWS SDK v2
// client to exercise real request signing/parsing end to end, without a
// real S3-compatible backend. lastPath records the most recently
// observed request path, for the leading-slash regression check.
func fakeS3Server(t *testing.T, objects map[string]*fakeObject) (*httptest.Server, *atomic.Pointer[string]) {
	t.Helper()

	var lastPath atomic.Pointer[string]

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		lastPath.Store(&p)

		prefix := "/" + testBucket + "/"
		if !strings.HasPrefix(r.URL.Path, prefix) {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		key := strings.TrimPrefix(r.URL.Path, prefix)

		obj, ok := objects[key]
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code>`+
				`<Message>The specified key does not exist.</Message></Error>`)

			return
		}

		w.Header().Set("ETag", obj.etag)
		w.Header().Set("Content-Length", strconv.Itoa(len(obj.data)))

		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(obj.data)
	}))
	t.Cleanup(srv.Close)

	return srv, &lastPath
}

func testClient(url string) *awss3.Client {
	return awss3.New(awss3.Options{
		Region:       "test-region",
		Credentials:  credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""),
		BaseEndpoint: aws.String(url),
		UsePathStyle: true,
	})
}

func TestRegisterRejectsNonS3Endpoint(t *testing.T) {
	if err := Register(context.Background(), remote.SIMBAD); !errors.Is(err, ErrNotS3Endpoint) {
		t.Errorf("Register(non-KindS3): err = %v, want ErrNotS3Endpoint", err)
	}
}

func TestRegisterRejectsUnknownEndpoint(t *testing.T) {
	if err := Register(context.Background(), "no.such.endpoint"); !errors.Is(err, remote.ErrUnknownEndpoint) {
		t.Errorf("Register(unknown): err = %v, want ErrUnknownEndpoint", err)
	}
}

// Register's ep.Bucket == "" check (ErrNoBucket) has no dedicated test:
// remote.CopernicusEODATA is the only KindS3 endpoint the registry ships,
// it always carries Bucket "eodata", and there is no public API to
// register a bucket-less KindS3 endpoint from outside package remote to
// exercise that branch against. The check itself is a single-line guard
// (see s3.go's Register) reviewed alongside ErrNotS3Endpoint's sibling
// check just above it.

func TestRegisterIdempotent(t *testing.T) {
	srv, _ := fakeS3Server(t, nil)
	registerTestEndpoint(t, srv.URL)

	client := testClient(srv.URL)

	if err := Register(context.Background(), testEndpointID, WithClient(client)); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	if err := Register(context.Background(), testEndpointID, WithClient(client)); err != nil {
		t.Fatalf("second Register: %v", err)
	}
}

func TestRegisterHonorsOfflineAndDisable(t *testing.T) {
	srv, _ := fakeS3Server(t, nil)
	registerTestEndpoint(t, srv.URL)

	remote.SetOffline(true)

	err := Register(context.Background(), testEndpointID)

	remote.SetOffline(false)

	if !errors.Is(err, remote.ErrOffline) {
		t.Errorf("Register while offline: err = %v, want ErrOffline", err)
	}

	remote.Disable(testEndpointID)

	err = Register(context.Background(), testEndpointID)

	remote.Enable(testEndpointID)

	if !errors.Is(err, remote.ErrEndpointDisabled) {
		t.Errorf("Register on a disabled endpoint: err = %v, want ErrEndpointDisabled", err)
	}
}

func mustRegister(t *testing.T, srv *httptest.Server) {
	t.Helper()

	registerTestEndpoint(t, srv.URL)
	remote.EnableDownloads(testEndpointID, 0)

	if err := Register(context.Background(), testEndpointID, WithClient(testClient(srv.URL))); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestProbeReturnsSignature(t *testing.T) {
	srv, _ := fakeS3Server(t, map[string]*fakeObject{
		"CAMS/GLOBAL/2023/01/01/lnsp.nc": {data: []byte("payload"), etag: `"abc123"`},
	})
	mustRegister(t, srv)

	sig, err := probeFor(t, "CAMS/GLOBAL/2023/01/01/lnsp.nc")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	if sig.ETag != `"abc123"` || sig.ContentLength != int64(len("payload")) {
		t.Errorf("Probe signature = %+v, want ETag=%q ContentLength=%d", sig, `"abc123"`, len("payload"))
	}
}

func TestProbeMissingKeyReturns404(t *testing.T) {
	srv, _ := fakeS3Server(t, nil)
	mustRegister(t, srv)

	_, err := probeFor(t, "does/not/exist.nc")

	var httpErr *remote.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("Probe(missing) err = %v, want *remote.HTTPError{StatusCode: 404}", err)
	}

	// remote.Exists must map this the same way it does for HTTP endpoints.
	ok, existsErr := remote.Exists(context.Background(), testEndpointID, "does/not/exist.nc")
	if existsErr != nil || ok {
		t.Errorf("remote.Exists(missing) = (%v, %v), want (false, nil)", ok, existsErr)
	}
}

// probeFor is a small test helper reaching transport{}.Probe directly,
// since Probe itself is unexported and this file is in the same package.
func probeFor(t *testing.T, key string) (remote.Signature, error) {
	t.Helper()

	return transport{}.Probe(context.Background(), testEndpointID, key)
}

func TestGetFileEndToEndAndLeadingSlashRegression(t *testing.T) {
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	const key = "CAMS/GLOBAL/2023/01/01/lnsp.nc"

	payload := []byte("real-cams-bytes")

	objects := map[string]*fakeObject{key: {data: payload, etag: `"v1"`}}

	srv, lastPath := fakeS3Server(t, objects)
	mustRegister(t, srv)

	f, err := remote.GetFile(context.Background(), testEndpointID, key)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, _ := f.ReadAll()
	if !bytes.Equal(got, payload) {
		t.Fatalf("GetFile content = %q, want %q", got, payload)
	}

	// Regression test for the leading-slash finding documented in
	// doc.go: FetchInto/Probe must build the raw, unprefixed S3 key
	// directly, never round-trip through a gofs.File("s3://...") URI
	// (whose CleanPathFromURI would prepend a leading slash and address
	// the wrong object against a real bucket).
	if p := lastPath.Load(); p == nil || *p != "/"+testBucket+"/"+key {
		t.Errorf("request path = %v, want %q (no leading-slash key, path-style addressing)", p, "/"+testBucket+"/"+key)
	}
}

func TestGetFileCacheReuseAndRefetchOnETagChange(t *testing.T) {
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	const key = "CAMS/GLOBAL/2023/01/01/lnsp.nc"

	var hits atomic.Int32

	var mu sync.Mutex

	current := &fakeObject{data: []byte("v1-bytes"), etag: `"v1"`}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)

		mu.Lock()
		obj := *current
		mu.Unlock()

		w.Header().Set("ETag", obj.etag)
		w.Header().Set("Content-Length", strconv.Itoa(len(obj.data)))

		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(obj.data)
	}))
	t.Cleanup(srv.Close)

	mustRegister(t, srv)

	if _, err := remote.GetFile(context.Background(), testEndpointID, key); err != nil {
		t.Fatalf("GetFile #1: %v", err)
	}

	if _, err := remote.GetFile(context.Background(), testEndpointID, key); err != nil {
		t.Fatalf("GetFile #2 (should be cache-reuse via HEAD): %v", err)
	}

	afterUnchanged := hits.Load()
	if afterUnchanged < 2 {
		t.Fatalf("expected at least 2 requests (1 GET + 1 HEAD) before the ETag change, got %d", afterUnchanged)
	}

	mu.Lock()
	current = &fakeObject{data: []byte("v2-bytes-different"), etag: `"v2"`}
	mu.Unlock()

	f, err := remote.GetFile(context.Background(), testEndpointID, key)
	if err != nil {
		t.Fatalf("GetFile #3 (after ETag change): %v", err)
	}

	got, _ := f.ReadAll()
	if string(got) != "v2-bytes-different" {
		t.Errorf("GetFile after ETag change returned stale content: %q", got)
	}

	if hits.Load() <= afterUnchanged {
		t.Error("changed ETag should have triggered at least one more request")
	}
}

func TestFetchIntoValidateFailureLeavesNoCacheFile(t *testing.T) {
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	const key = "CAMS/GLOBAL/2023/01/01/bad.nc"

	srv, _ := fakeS3Server(t, map[string]*fakeObject{
		key: {data: []byte("not-a-real-netcdf-file"), etag: `"v1"`},
	})
	mustRegister(t, srv)

	_, err := remote.GetFile(context.Background(), testEndpointID, key,
		remote.WithValidate(func([]byte) error { return errTestValidateFailed }))
	if err == nil {
		t.Fatal("GetFile with a failing validator should return an error")
	}

	dir, dirErr := remote.CacheDir(testEndpointID)
	if dirErr != nil {
		t.Fatalf("CacheDir: %v", dirErr)
	}

	if dir.Join(key).Exists() {
		t.Error("a failed validator must not leave a cache file behind")
	}
}

func TestFetchIntoProgressReportsIncrementally(t *testing.T) {
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	const key = "CAMS/GLOBAL/2023/01/01/aermr01.nc"

	// Large enough that a single Read call cannot plausibly deliver the
	// whole body in one shot through net/http's default buffering, so an
	// incremental progress callback is a meaningful assertion, not a
	// tautology.
	payload := make([]byte, 8<<20)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	srv, _ := fakeS3Server(t, map[string]*fakeObject{key: {data: payload, etag: `"v1"`}})
	mustRegister(t, srv)

	var (
		mu         sync.Mutex
		calls      int
		sawPartial bool
		lastDone   int64
	)

	_, err := remote.GetFile(context.Background(), testEndpointID, key,
		remote.WithProgress(func(downloaded, total int64) {
			mu.Lock()
			defer mu.Unlock()

			calls++
			lastDone = downloaded

			if downloaded > 0 && downloaded < total {
				sawPartial = true
			}
		}))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if calls < 2 {
		t.Errorf("progress callback fired %d times, want at least 2 (streaming, not one shot)", calls)
	}

	if !sawPartial {
		t.Error("expected at least one progress call with 0 < downloaded < total")
	}

	if lastDone != int64(len(payload)) {
		t.Errorf("final progress downloaded = %d, want %d", lastDone, len(payload))
	}
}

// TestFetchIntoStreamsWithoutBufferingWholeObject is a memory-bound
// regression test for the redesign this package's doc comment documents:
// FetchInto must stream GetObjectOutput.Body straight to remote.Save, not
// buffer the whole object first (the behavior s3fs.ReadAll/OpenReader
// have, and the reason this package doesn't route bytes through them).
// This can't prove zero extra allocation, but a clear, generous bound
// well below the object size is enough to catch a regression back to
// io.ReadAll-the-whole-thing.
func TestFetchIntoStreamsWithoutBufferingWholeObject(t *testing.T) {
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	const key = "CAMS/GLOBAL/2023/01/01/aermr01.nc"

	const objectSize = 32 << 20 // 32 MiB — large enough to make a whole-object buffer visible

	payload := make([]byte, objectSize)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	srv, _ := fakeS3Server(t, map[string]*fakeObject{key: {data: payload, etag: `"v1"`}})
	mustRegister(t, srv)

	runtime.GC()

	var before runtime.MemStats

	runtime.ReadMemStats(&before)

	if _, err := remote.GetFile(context.Background(), testEndpointID, key); err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	runtime.GC()

	var after runtime.MemStats

	runtime.ReadMemStats(&after)

	// A generous bound: a whole-object buffer would show growth on the
	// order of objectSize (32 MiB); streaming through io.Copy's default
	// buffer should show growth on the order of tens of KB. Bound at
	// objectSize/4 so this is robust to normal GC/allocator noise while
	// still failing hard on a real regression to buffered reads.
	const bound = objectSize / 4

	if grew := int64(after.HeapAlloc) - int64(before.HeapAlloc); grew > bound {
		t.Errorf("heap grew by %d bytes fetching a %d-byte object, want well under %d (streaming, not buffered)",
			grew, objectSize, bound)
	}
}
