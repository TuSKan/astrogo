package remote

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"
)

var errRejectEverything = errors.New("rejected by test validator")

// serveBytes stands up a static HTTP file server whose URL can be handed
// to SetURL directly. http:// is a default remote/file scheme, so this
// needs no opt-in import — and this is what the real IERS, NAIF, OpenNGC
// and GFZ endpoints look like.
func serveBytes(t *testing.T, name string, body []byte) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/") != name {
			http.NotFound(w, r)

			return
		}

		http.ServeContent(w, r, name, time.Unix(1_700_000_000, 0), bytes.NewReader(body))
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/"
}

// Every https:// KindFile endpoint in the registry depends on this path
// working end to end: bucket-open by scheme, HEAD for size and ETag,
// ranged GET for the body, then promotion into the cache.
func TestGetFileFromHTTPSource(t *testing.T) {
	cleanRemoteState(t)

	body := []byte(strings.Repeat("naif0012.tls contents; ", 200))

	if err := SetURL(NAIFLSK, serveBytes(t, "naif0012.tls", body)); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, NAIFLSK)

	bucket, key, err := GetFile(context.Background(), NAIFLSK, "naif0012.tls")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := bucket.ReadAll(context.Background(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, body) {
		t.Errorf("cached %d bytes, want %d", len(got), len(body))
	}
}

func TestGetFileFromHTTPSourceDeniedWithoutConsent(t *testing.T) {
	cleanRemoteState(t)

	if err := SetURL(NAIFLSK, serveBytes(t, "naif0012.tls", []byte("x"))); err != nil {
		t.Fatal(err)
	}

	if _, _, err := GetFile(context.Background(), NAIFLSK, "naif0012.tls"); !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("GetFile without consent = %v, want ErrDownloadDenied", err)
	}
}

// A missing object on a reachable source is (false, nil) — distinct from
// "could not tell", and reached without moving a body or asking consent.
func TestExistsAgainstHTTPSource(t *testing.T) {
	cleanRemoteState(t)

	if err := SetURL(NAIFLSK, serveBytes(t, "naif0012.tls", []byte("x"))); err != nil {
		t.Fatal(err)
	}

	got, err := Exists(context.Background(), NAIFLSK, "naif0012.tls")
	if err != nil || !got {
		t.Errorf("Exists(present) = (%v, %v), want (true, nil)", got, err)
	}

	got, err = Exists(context.Background(), NAIFLSK, "naif0099.tls")
	if err != nil || got {
		t.Errorf("Exists(absent) = (%v, %v), want (false, nil)", got, err)
	}
}

// Validation runs against the staged object before promotion, so a
// rejected download leaves nothing at the cache key for a later call to
// reuse — the failure mode that made a corrupt Horizons kernel permanent.
func TestValidateFailureLeavesNothingCached(t *testing.T) {
	cleanRemoteState(t)

	if err := SetURL(NAIFLSK, serveBytes(t, "naif0012.tls", []byte("corrupt"))); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, NAIFLSK)

	_, _, err := GetFile(context.Background(), NAIFLSK, "naif0012.tls",
		WithValidate(func(io.Reader) error { return errRejectEverything }))
	if !errors.Is(err, errRejectEverything) {
		t.Fatalf("GetFile = %v, want the validator's own error", err)
	}

	bucket, prefix, err := CacheDir(context.Background(), NAIFLSK)
	if err != nil {
		t.Fatal(err)
	}

	for _, key := range []string{prefix + "naif0012.tls", partialKey(prefix + "naif0012.tls")} {
		if exists, _ := bucket.Exists(context.Background(), key); exists {
			t.Errorf("%s survived a failed validation", key)
		}
	}
}

// The validator sees the whole object as a stream, never a buffer, and
// what it approved is exactly what lands at the cache key.
func TestValidateSeesFullStagedContent(t *testing.T) {
	cleanRemoteState(t)

	body := []byte(strings.Repeat("finals2000A row\n", 5000))

	if err := SetURL(IERSFinals2000A, serveBytes(t, "finals2000A.all", body)); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, IERSFinals2000A)

	var seen int64

	bucket, key, err := GetFile(context.Background(), IERSFinals2000A, "finals2000A.all",
		WithCacheName("finals2000A.data"),
		WithValidate(func(r io.Reader) error {
			n, err := io.Copy(io.Discard, r)
			seen = n

			if err != nil {
				return fmt.Errorf("drain staged object: %w", err)
			}

			return nil
		}))
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	if seen != int64(len(body)) {
		t.Errorf("validator saw %d bytes, want %d", seen, len(body))
	}

	if !strings.HasSuffix(key, "finals2000A.data") {
		t.Errorf("cache key = %q, want the WithCacheName override", key)
	}

	got, err := bucket.ReadAll(context.Background(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if !bytes.Equal(got, body) {
		t.Error("cached content differs from what the validator approved")
	}
}

// A Mutable endpoint reuses its cache only while the source ETag it
// recorded still matches, so a second call transfers nothing and a
// changed upstream is picked up.
func TestMutableSourceRevalidatesByETag(t *testing.T) {
	cleanRemoteState(t)

	var hits int

	body := []byte("v1 rows")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			hits++
		}

		http.ServeContent(w, r, "NGC.csv", time.Unix(1_700_000_000, 0), bytes.NewReader(body))
	}))
	t.Cleanup(srv.Close)

	if err := SetURL(OpenNGC, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(0, OpenNGC)

	for range 3 {
		if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
			t.Fatalf("GetFile: %v", err)
		}
	}

	if hits != 1 {
		t.Errorf("source body fetched %d times, want 1 — the ETag check must skip the transfer", hits)
	}
}

func TestDataDirCanBeAnyBucketURL(t *testing.T) {
	t.Cleanup(func() {
		SetDataDir("")
		Reset()
	})

	// Nothing about the cache assumes local disk: pointing the data
	// directory at a different bucket URL is the whole configuration step.
	url := testutil.FileURL(t, t.TempDir())
	SetDataDir(url)

	if got := DataDirURL(); got != url {
		t.Fatalf("DataDirURL() = %q, want %q", got, url)
	}

	if _, err := DataDir(context.Background()); err != nil {
		t.Fatalf("DataDir: %v", err)
	}
}
