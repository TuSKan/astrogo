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
	"time"
)

// A caller whose download policy refuses can be given only what is already
// cached, so GetFile asks no source on its behalf (#964). It used to ask a
// mutable source whether the cached copy was current first: a request the
// caller had refused, and against a source that did not answer, the whole of
// its timeout.

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

// TestGetFileUnderARefusingPolicyServesTheCacheUnasked: a mutable endpoint's
// object, cached while downloads were allowed, is served under a policy that
// refuses them without a request to the source, as offline mode serves it.
func TestGetFileUnderARefusingPolicyServesTheCacheUnasked(t *testing.T) {
	cleanRemoteState(t)

	body := []byte("v1 rows")
	requests := countingSource(t, OpenNGC, body)

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile with downloads allowed: %v", err)
	}

	if requests.Load() == 0 {
		t.Fatal("the source was never asked: the cache was not seeded from it")
	}

	SetPolicy(func(Endpoint, int64) error { return errCustomPolicyForTest })

	before := requests.Load()

	fsys, key, err := GetFile(context.Background(), OpenNGC, "NGC.csv")
	if err != nil {
		t.Fatalf("GetFile under a refusing policy, with a cached copy: %v", err)
	}

	got, err := fs.ReadFile(fsys, key)
	if err != nil || !bytes.Equal(got, body) {
		t.Errorf("served %q (%v), want the cached %q", got, err, body)
	}

	if n := requests.Load() - before; n != 0 {
		t.Errorf("the source was sent %d requests under a refusing policy, want 0", n)
	}
}

// TestGetFileUnderARefusingPolicyWithNoCacheAsksNothing: with nothing cached
// there is nothing to serve, and the answer is the policy's refusal, again
// unasked.
func TestGetFileUnderARefusingPolicyWithNoCacheAsksNothing(t *testing.T) {
	cleanRemoteState(t)

	requests := countingSource(t, OpenNGC, []byte("v1 rows"))

	SetPolicy(func(Endpoint, int64) error { return errCustomPolicyForTest })

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); !errors.Is(err, ErrDownloadDenied) {
		t.Errorf("GetFile under a refusing policy with no cache = %v, want ErrDownloadDenied", err)
	}

	if n := requests.Load(); n != 0 {
		t.Errorf("the source was sent %d requests under a refusing policy, want 0", n)
	}
}

// TestGetFileWithDownloadsAllowedStillAsksAMutableSource: a caller who may
// download still has a mutable endpoint's cache revalidated, which is the
// request one whose policy refuses is spared.
func TestGetFileWithDownloadsAllowedStillAsksAMutableSource(t *testing.T) {
	cleanRemoteState(t)

	requests := countingSource(t, OpenNGC, []byte("v1 rows"))

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile with downloads allowed: %v", err)
	}

	before := requests.Load()

	if _, _, err := GetFile(context.Background(), OpenNGC, "NGC.csv"); err != nil {
		t.Fatalf("GetFile again with downloads allowed: %v", err)
	}

	if requests.Load() == before {
		t.Error("with downloads allowed, a mutable endpoint's cache was served without asking the source whether it is current")
	}
}
