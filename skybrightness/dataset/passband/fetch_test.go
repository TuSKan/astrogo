package passband_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
)

// Fetch's own path, against a fake service rather than SVO.
//
// It used to be covered only by a network-tagged test, which means it was
// covered only when someone remembered to run that suite with SVO reachable —
// and never in CI. Pointing the endpoint at an httptest server exercises the
// same code offline and deterministically: the request that is issued, the
// cache that is written, and the cache that is read back instead of a second
// request.

// serveProfile stands up a fake SVO, points the endpoint at it, and returns the
// number of requests it has answered.
func serveProfile(t *testing.T, body string) (hits *int, gotID *string) {
	t.Helper()

	var (
		count int
		id    string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		id = r.URL.Query().Get("ID")

		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(srv.Close)

	scope := remote.Capture(remote.SVOFilterProfile)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.SVOFilterProfile, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	// A cache of this test's own, so a profile left by another run cannot
	// answer the first request and make the fetch look like it never happened.
	remote.SetDataDir(tempBucketURL(t))

	return &count, &id
}

func TestFetchRequestsTheFilterAndCachesIt(t *testing.T) {
	hits, gotID := serveProfile(t, voTable(goodParams(), goodRows(), false))

	band, err := passband.Fetch(t.Context(), "Generic/Bessell.V")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(band.WavelengthNM) != len(goodRows()) {
		t.Errorf("%d samples, want %d", len(band.WavelengthNM), len(goodRows()))
	}

	if *gotID != "Generic/Bessell.V" {
		t.Errorf("service saw ID=%q, want the filter that was asked for", *gotID)
	}

	if *hits != 1 {
		t.Fatalf("service saw %d requests for the first fetch, want 1", *hits)
	}

	// The second call is the point of the cache: same answer, no request.
	again, err := passband.Fetch(t.Context(), "Generic/Bessell.V")
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if len(again.WavelengthNM) != len(band.WavelengthNM) {
		t.Errorf("cached profile has %d samples, want %d", len(again.WavelengthNM), len(band.WavelengthNM))
	}

	if *hits != 1 {
		t.Errorf("service saw %d requests in total, want 1 — the second call did not read the cache", *hits)
	}
}

// A service that answers with something unusable must fail rather than cache
// it, or the bad answer is served from disk for the life of the cache.
func TestFetchDoesNotCacheAnUnusableAnswer(t *testing.T) {
	hits, _ := serveProfile(t, `<VOTABLE><RESOURCE>`)

	if _, err := passband.Fetch(t.Context(), "Generic/Bessell.V"); err == nil {
		t.Fatal("a truncated profile was accepted")
	}

	if _, err := passband.Fetch(t.Context(), "Generic/Bessell.V"); err == nil {
		t.Fatal("a truncated profile was accepted on the second call")
	}

	if *hits != 2 {
		t.Errorf("service saw %d requests, want 2 — an unparseable answer was cached", *hits)
	}
}

// tempBucketURL is a file:// URL for a directory belonging to this test.
func tempBucketURL(t *testing.T) string {
	t.Helper()

	return testutil.FileURL(t, t.TempDir())
}

// TestFetchStillWorksWhenTheCacheCannotBeOpened pins the documented promise
// that a cache is an optimisation and never a dependency.
//
// A deployment with a misconfigured cache location must still get its filter
// curves — slowly, one request per call, but correctly. Treating a bad cache as
// a fetch failure would turn a configuration mistake into a dead library.
func TestFetchStillWorksWhenTheCacheCannotBeOpened(t *testing.T) {
	hits, _ := serveProfile(t, voTable(goodParams(), goodRows(), false))

	// A scheme no driver registers, so the cache bucket cannot be opened.
	remote.SetDataDir("no-such-scheme://example.invalid/cache")

	band, err := passband.Fetch(t.Context(), "Generic/Bessell.V")
	if err != nil {
		t.Fatalf("Fetch with an unopenable cache: %v", err)
	}

	if len(band.WavelengthNM) != len(goodRows()) {
		t.Errorf("%d samples, want %d", len(band.WavelengthNM), len(goodRows()))
	}

	// Nothing was cached, so the second call asks again rather than failing.
	if _, err := passband.Fetch(t.Context(), "Generic/Bessell.V"); err != nil {
		t.Fatalf("second Fetch with an unopenable cache: %v", err)
	}

	if *hits != 2 {
		t.Errorf("service saw %d requests, want 2 — without a cache every call is a request", *hits)
	}
}
