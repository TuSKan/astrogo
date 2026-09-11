package remote

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/time"
)

// The re-exports are one line each, which is exactly why they need tests.
//
// A one-line delegation has one interesting failure: it forwards to the wrong
// thing, or drops an argument on the way. Neither shows up in a build, and a
// caller who can no longer reach a subpackage to check has nothing to compare
// against. So each test below asserts the name arrived somewhere observable —
// a header on the wire, a request count, a real object in a real bucket —
// rather than that it returned a non-nil value.

// TestBucketRoundTripThroughTheFrontDoor covers OpenBucket, Save and
// IsNotFound together, because separately none of them proves much.
func TestBucketRoundTripThroughTheFrontDoor(t *testing.T) {
	bucket, err := OpenBucket(t.Context(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("OpenBucket: %v", err)
	}

	// A key nothing has written is a miss, not a broken store. Every
	// cache-before-fetch path in the module turns on telling those apart.
	if _, err := bucket.ReadAll(t.Context(), "absent.dat"); !IsNotFound(err) {
		t.Errorf("IsNotFound(%v) = false, want true for a key never written", err)
	}

	const payload = "kernel bytes"

	if err := Save(t.Context(), bucket, "planets/de440s.bsp", strings.NewReader(payload)); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := bucket.ReadAll(t.Context(), "planets/de440s.bsp")
	if err != nil {
		t.Fatalf("ReadAll after Save: %v", err)
	}

	if string(got) != payload {
		t.Errorf("read back %q, want %q", got, payload)
	}

	if IsNotFound(err) {
		t.Error("IsNotFound reported true for a key that was just written")
	}
}

// TestReaderAtOptionsReachTheReader proves WithChunkSize and WithCachedChunks
// are forwarded rather than accepted and dropped.
//
// The observable is request count: a ReaderAt fetches aligned chunks, so
// reading a 64-byte object one chunk at a time costs one request per chunk.
// An option that never arrived would leave the 64 KiB default in place and the
// whole object would come back in a single request — which is why the
// assertion is on the count and not on the bytes, since the bytes are correct
// either way.
func TestReaderAtOptionsReachTheReader(t *testing.T) {
	const (
		size      = 64
		chunkSize = 16
	)

	body := strings.Repeat("ab", size/2)

	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		http.ServeContent(w, r, "object.dat", time.Unix(1_700_000_000, 0), strings.NewReader(body))
	}))
	defer srv.Close()

	bucket, err := OpenBucket(t.Context(), srv.URL)
	if err != nil {
		t.Fatalf("OpenBucket: %v", err)
	}

	ra, err := NewReaderAt(t.Context(), bucket, "object.dat",
		WithChunkSize(chunkSize), WithCachedChunks(1))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}

	before := requests.Load()

	buf := make([]byte, size)
	if _, err := ra.ReadAt(buf, 0); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("ReadAt: %v", err)
	}

	if string(buf) != body {
		t.Errorf("ReadAt returned %q, want the object's own bytes", buf)
	}

	if fetches := requests.Load() - before; fetches < size/chunkSize {
		t.Errorf("reading %d bytes at a %d-byte chunk size cost %d requests, want at least %d.\n"+
			"  Fewer means WithChunkSize never reached the reader and the 64 KiB default "+
			"served the whole object in one go.", size, chunkSize, fetches, size/chunkSize)
	}

	if err := ra.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := ra.ReadAt(buf, 0); !errors.Is(err, ErrReaderAtClosed) {
		t.Errorf("ReadAt after Close = %v, want ErrReaderAtClosed", err)
	}
}

// TestAPIOptionsReachTheRequest walks every option the front door re-exports
// to something the server, the clock or the attempt count can see.
//
// One test rather than six because they share the setup and each assertion is
// one line; splitting them would say the same thing at four times the length.
func TestAPIOptionsReachTheRequest(t *testing.T) {
	var (
		gotUserAgent string
		gotAuth      string
		attempts     atomic.Int32
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)

		gotUserAgent = r.Header.Get("User-Agent")
		gotAuth = r.Header.Get("Authorization")

		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	scope := Capture(SIMBAD)
	t.Cleanup(scope.Restore)

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	client, err := NewAPIClient(SIMBAD,
		WithUserAgent("astrogo-test/1.0"),
		WithAuthToken("Token", "s3cret"),
		WithRetries(2),
		WithMinInterval(time.Millisecond),
		WithTimeout(10*time.Second),
	)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Get(t.Context(), SIMBAD, "", nil)
	if err == nil {
		t.Fatal("expected the 503 to surface as an error")
	}

	if gotUserAgent != "astrogo-test/1.0" {
		t.Errorf("User-Agent = %q, want WithUserAgent's value", gotUserAgent)
	}

	if gotAuth != "Token s3cret" {
		t.Errorf("Authorization = %q, want WithAuthToken's scheme and token", gotAuth)
	}

	// A 503 the default policy retries, twice, after the first attempt.
	if got := attempts.Load(); got != 3 {
		t.Errorf("server saw %d attempts, want 3 — WithRetries(2) did not reach the client", got)
	}

	// The status has to survive the wrapping, through the re-exported type.
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err = %v, want a *remote.HTTPError", err)
	}

	if httpErr.HTTPStatus() != http.StatusServiceUnavailable {
		t.Errorf("HTTPStatus() = %d, want 503", httpErr.HTTPStatus())
	}

	if !errors.Is(err, ErrRetriable) {
		t.Error("an exhausted 503 must wrap ErrRetriable, or a caller cannot tell it from a 404")
	}
}

// TestWithRetryPolicyReachesTheClient is separate because it has to contradict
// the default to prove anything: this policy refuses a 503, which
// DefaultRetryPolicy retries, so a single attempt cannot come from the default
// still being in force.
func TestWithRetryPolicyReachesTheClient(t *testing.T) {
	var attempts atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)

		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	scope := Capture(SIMBAD)
	t.Cleanup(scope.Restore)

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	client, err := NewAPIClient(SIMBAD,
		WithRetries(3),
		WithRetryPolicy(func(Attempt) bool { return false }),
	)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	if _, err := client.Get(t.Context(), SIMBAD, "", nil); err == nil {
		t.Fatal("expected the 503 to surface as an error")
	}

	if got := attempts.Load(); got != 1 {
		t.Errorf("server saw %d attempts, want 1 — the custom policy refuses to retry", got)
	}
}

// TestDefaultRetryPolicyIsTheSameRule pins the re-export to the rule it names.
//
// A table here would duplicate remote/api's own; what is worth checking at this
// layer is that the exported name is that function and not a second opinion
// that could drift from it.
func TestDefaultRetryPolicyIsTheSameRule(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		want   bool
	}{
		{"429 rate limit", http.StatusTooManyRequests, true},
		{"503 unavailable", http.StatusServiceUnavailable, true},
		{"501 not implemented", http.StatusNotImplemented, false},
		{"404 not found", http.StatusNotFound, false},
		{"no response at all", 0, true},
	} {
		a := Attempt{StatusCode: tc.status}
		if got := DefaultRetryPolicy(a); got != tc.want {
			t.Errorf("%s: DefaultRetryPolicy = %v, want %v", tc.name, got, tc.want)
		}

		if got, want := DefaultRetryPolicy(a), api.DefaultRetryPolicy(a); got != want {
			t.Errorf("%s: the re-export disagrees with the rule it names (%v vs %v)", tc.name, got, want)
		}
	}
}

// TestDefaultAPITimeoutIsTheSameValue keeps the documented fallback from
// becoming a second, drifting copy of the real one.
func TestDefaultAPITimeoutIsTheSameValue(t *testing.T) {
	t.Parallel()

	if DefaultAPITimeout != api.DefaultTimeout {
		t.Errorf("DefaultAPITimeout = %v, want api.DefaultTimeout (%v)", DefaultAPITimeout, api.DefaultTimeout)
	}
}

// TestPostVerbsReachTheServerThroughTheFrontDoor covers the two methods whose
// success path nothing else here exercises.
//
// PostForm carries TAP-ADQL queries and PostJSON carries Horizons kernel
// requests, so between them they are most of astrogo's write traffic. The
// offline test above only ever reaches their refusal branch, which leaves the
// forwarding itself — the body, the content type, the resolved URL — unchecked.
func TestPostVerbsReachTheServerThroughTheFrontDoor(t *testing.T) {
	var (
		gotContentType string
		gotBody        string
		gotPath        string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path

		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)

		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	scope := Capture(VizieR)
	t.Cleanup(scope.Restore)

	if err := SetURL(VizieR, srv.URL+"/tap"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	client, err := NewAPIClient(VizieR)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	form, err := client.PostForm(t.Context(), VizieR, "sync", url.Values{"QUERY": {"SELECT 1"}})
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}

	_ = form.Close()

	if !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("PostForm Content-Type = %q", gotContentType)
	}

	if gotBody != "QUERY=SELECT+1" {
		t.Errorf("PostForm body = %q, want the form it was given", gotBody)
	}

	if gotPath != "/tap/sync" {
		t.Errorf("PostForm path = %q, want the endpoint URL joined with the request path", gotPath)
	}

	resp, err := client.PostJSON(t.Context(), VizieR, "sync", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	_ = resp.Close()

	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Errorf("PostJSON Content-Type = %q", gotContentType)
	}

	if gotBody != `{"k":"v"}` {
		t.Errorf("PostJSON body = %q, want the payload it was given", gotBody)
	}
}
