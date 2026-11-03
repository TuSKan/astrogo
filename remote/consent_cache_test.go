package remote

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// A caller without download consent can be given only what is already cached,
// so GetFile asks no source on its behalf (#964). It used to ask a mutable
// source whether the cached copy was current first: a request the caller never
// agreed to, and against a source that did not answer, the whole of its
// timeout.

// countingSource serves body under any name at id's URL and counts every
// request it is sent, a HEAD probe as much as a download.
func countingSource(t *testing.T, id EndpointID, body []byte) *atomic.Int32 {
	t.Helper()

	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.ServeContent(w, r, "object", time.Unix(1_700_000_000, 0), bytes.NewReader(body))
	}))
	t.Cleanup(srv.Close)

	if err := SetURL(id, srv.URL+"/"); err != nil {
		t.Fatal(err)
	}

	return &requests
}

// TestGetFileWithoutConsentServesTheCacheUnasked: a mutable endpoint's object,
// cached while consent was given, is served after consent is withdrawn without
// a request to the source, as offline mode serves it.
func TestGetFileWithoutConsentServesTheCacheUnasked(t *testing.T) {
	cleanRemoteState(t)

	body := []byte("v1 rows")
	requests := countingSource(t, OpenNGC, body)

	EnableDownloads(0, OpenNGC)

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile with consent: %v", err)
	}

	if requests.Load() == 0 {
		t.Fatal("the source was never asked: the cache was not seeded from it")
	}

	DisableDownloads(OpenNGC)

	before := requests.Load()

	fsys, key, err := GetFile(context.Background(), OpenNGC, "NGC.csv")
	if err != nil {
		t.Fatalf("GetFile without consent, with a cached copy: %v", err)
	}

	got, err := fs.ReadFile(fsys, key)
	if err != nil || !bytes.Equal(got, body) {
		t.Errorf("served %q (%v), want the cached %q", got, err, body)
	}

	if n := requests.Load() - before; n != 0 {
		t.Errorf("the source was sent %d requests without consent, want 0", n)
	}
}

// TestGetFileWithoutConsentAndNoCacheAsksNothing: with nothing cached there is
// nothing to serve, and the answer is consent's refusal, again unasked.
func TestGetFileWithoutConsentAndNoCacheAsksNothing(t *testing.T) {
	cleanRemoteState(t)

	requests := countingSource(t, OpenNGC, []byte("v1 rows"))

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); !errors.Is(err, ErrDownloadDenied) {
		t.Errorf("GetFile without consent or a cache = %v, want ErrDownloadDenied", err)
	}

	if n := requests.Load(); n != 0 {
		t.Errorf("the source was sent %d requests without consent, want 0", n)
	}
}

// TestGetFileWithConsentStillAsksAMutableSource: with consent a mutable
// endpoint's cache is still revalidated, which is the request a caller who
// can download is asked for and one who cannot is now spared.
func TestGetFileWithConsentStillAsksAMutableSource(t *testing.T) {
	cleanRemoteState(t)

	requests := countingSource(t, OpenNGC, []byte("v1 rows"))

	EnableDownloads(0, OpenNGC)

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile with consent: %v", err)
	}

	before := requests.Load()

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile again with consent: %v", err)
	}

	if requests.Load() == before {
		t.Error("with consent, a mutable endpoint's cache was served without asking the source whether it is current")
	}
}
