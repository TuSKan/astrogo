package dust

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// response renders a service document carrying one 100 micron intensity,
// wrapped in the two blocks that share its element names — the shape the real
// service answers with, and the reason parse looks for the block by
// description rather than taking the first refPixelValue.
func response(mjySr float64) string {
	return fmt.Sprintf(`<?xml version="1.0"?>
<results status="ok">
  <result><desc>E(B-V) Reddening</desc>
    <statistics><refPixelValue>0.0231 (mag)</refPixelValue></statistics></result>
  <result><desc>100 Micron Emission</desc>
    <statistics><refPixelValue>%.4f (MJy/sr)</refPixelValue></statistics></result>
  <result><desc>Dust Temperature</desc>
    <statistics><refPixelValue>21.1993 (K)</refPixelValue></statistics></result>
</results>`, mjySr)
}

// fakeIRSA stands the service up locally and points remote.IRSADust at it,
// with a fresh cache directory per test. Returns the request counter, which is
// what most of these tests are actually about: this package exists because it
// once spent twenty-five minutes of a shared facility's time re-asking
// questions it had already answered.
//
// handler is called per request and returns the intensity to serve, or an
// error status to send instead.
func fakeIRSA(t *testing.T, handler func(n int) (mjySr float64, status int)) *atomic.Int64 {
	t.Helper()

	t.Cleanup(remote.Capture().Restore)

	var calls atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(calls.Add(1))

		v, status := handler(n)
		if status != 0 {
			w.WriteHeader(status)

			return
		}

		_, _ = w.Write([]byte(response(v)))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.IRSADust, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	return &calls
}

// serve is the ordinary handler: every request succeeds, with a distinct value
// per call so a test can tell which answer landed where.
func serve(n int) (float64, int) { return 1000 * float64(n), 0 }

// TestFetchServesASecondSessionFromDisk is the property the cache exists for.
//
// A sightline's 100 micron intensity does not change, so a value fetched once
// is a value fetched for good. The assertion is not that the second call is
// fast but that it issues no request at all — the client is built lazily, on
// the first sightline that actually needs asking, so a fully cached call must
// open no connection.
func TestFetchServesASecondSessionFromDisk(t *testing.T) {
	calls := fakeIRSA(t, serve)

	dirs := []Direction{
		{L: angle.Deg(10), B: angle.Deg(20)},
		{L: angle.Deg(30), B: angle.Deg(-40)},
	}

	first, err := Fetch(context.Background(), nil, dirs...)
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Fatalf("first Fetch made %d requests, want one per direction", got)
	}

	// A new session: nothing in memory, everything on disk.
	second, err := Fetch(context.Background(), nil, dirs...)
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("a fully cached Fetch asked the service %d more times, want 0", got-2)
	}

	for _, d := range dirs {
		want, err := first.IntensityAt(d.L, d.B)
		if err != nil {
			t.Fatalf("first map is missing l=%v b=%v: %v", d.L, d.B, err)
		}

		got, err := second.IntensityAt(d.L, d.B)
		if err != nil {
			t.Errorf("second map is missing l=%v b=%v: %v", d.L, d.B, err)
			continue
		}

		if math.Abs(got-want) > 1e-6 {
			t.Errorf("l=%v b=%v: cached %v, fetched %v", d.L, d.B, got, want)
		}
	}
}

// TestFetchQueriesACellOnce: the cache is keyed by cell, not by direction, so
// directions that round to the same cell are one question.
func TestFetchQueriesACellOnce(t *testing.T) {
	calls := fakeIRSA(t, serve)

	// Three directions inside one 0.1 degree cell.
	_, err := Fetch(context.Background(), nil,
		Direction{L: angle.Deg(10.00), B: angle.Deg(20.00)},
		Direction{L: angle.Deg(10.01), B: angle.Deg(20.01)},
		Direction{L: angle.Deg(9.99), B: angle.Deg(19.99)},
	)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("three directions in one cell cost %d requests, want 1", got)
	}
}

// TestFetchKeepsWhatItLearnedWhenCutOff: being cut off part way through a long
// list should cost the remaining sightlines, not the ones already paid for.
// Without this the next run re-asks for everything, which is the behaviour the
// cache was added to stop.
func TestFetchKeepsWhatItLearnedWhenCutOff(t *testing.T) {
	calls := fakeIRSA(t, func(n int) (float64, int) {
		if n == 1 {
			return 4242, 0
		}

		return 0, http.StatusInternalServerError
	})

	good := Direction{L: angle.Deg(10), B: angle.Deg(20)}

	_, err := Fetch(context.Background(), nil,
		good,
		Direction{L: angle.Deg(30), B: angle.Deg(-40)},
	)
	if err == nil {
		t.Fatal("Fetch returned no error although the service failed")
	}

	if calls.Load() < 2 {
		t.Fatalf("the service was asked %d times; the failure did not come from the second sightline", calls.Load())
	}

	held, err := CachedDirections(context.Background())
	if err != nil {
		t.Fatalf("CachedDirections: %v", err)
	}

	if len(held) != 1 {
		t.Fatalf("cache holds %d directions after a partial run, want the 1 that succeeded: %v", len(held), held)
	}

	if math.Abs(held[0].Intensity-4242) > 1e-6 {
		t.Errorf("cached intensity = %v, want 4242", held[0].Intensity)
	}

	if math.Abs(held[0].L.Degrees()-good.L.Degrees()) > cellSizeDeg ||
		math.Abs(held[0].B.Degrees()-good.B.Degrees()) > cellSizeDeg {
		t.Errorf("cached direction = l %v b %v, want l %v b %v",
			held[0].L, held[0].B, good.L, good.B)
	}
}

// TestCachedDirectionsIsOrdered: the ordering is what lets the all-sky
// validation report the same worst case twice running. Go randomises map
// iteration, so without the sort the same cache reads back differently on
// consecutive runs and no comparison against it is reproducible.
//
// Seeded through writeCache rather than through Fetch: the subject is the
// round trip, and going via Fetch would spend six seconds of the default test
// run on the two-second pacing the service is owed.
func TestCachedDirectionsIsOrdered(t *testing.T) {
	fakeIRSA(t, serve)

	ctx := context.Background()

	bucket, prefix, err := remote.CacheDir(ctx, remote.IRSADust)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	// Deliberately not in sorted order.
	err = writeCache(ctx, bucket, path.Join(prefix, cacheFile), map[cell]float64{
		{l: 300, b: 100}: 3,
		{l: 100, b: 100}: 1,
		{l: 200, b: -50}: 2,
	})
	if err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	held, err := CachedDirections(ctx)
	if err != nil {
		t.Fatalf("CachedDirections: %v", err)
	}

	if len(held) != 3 {
		t.Fatalf("cache holds %d directions, want 3", len(held))
	}

	for i := 1; i < len(held); i++ {
		prev, cur := held[i-1], held[i]
		if cur.B < prev.B || (cur.B == prev.B && cur.L < prev.L) {
			t.Errorf("entry %d (l %v b %v) sorts before entry %d (l %v b %v)",
				i, cur.L, cur.B, i-1, prev.L, prev.B)
		}
	}
}

// TestCachedDirectionsOnAColdCache: a cache that has never been written is a
// normal state, so an empty result rather than an error. Reporting a failure
// here would make a first run look broken.
func TestCachedDirectionsOnAColdCache(t *testing.T) {
	fakeIRSA(t, serve)

	held, err := CachedDirections(context.Background())
	if err != nil {
		t.Fatalf("a cold cache reported an error: %v", err)
	}

	if len(held) != 0 {
		t.Errorf("a cold cache produced %d directions", len(held))
	}
}

// TestReadCacheSkipsUnusableLines: a truncated or corrupt line costs the
// sightline on it, not the whole file — the worst case is asking IRSA again
// for that one direction, and discarding every other answer to avoid it would
// be the more expensive mistake.
func TestReadCacheSkipsUnusableLines(t *testing.T) {
	fakeIRSA(t, serve)

	ctx := context.Background()

	bucket, prefix, err := remote.CacheDir(ctx, remote.IRSADust)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	key := path.Join(prefix, cacheFile)

	body := strings.Join([]string{
		"100 200 1.5e+03", // good
		"100 200",         // truncated
		"x 200 1.0",       // l not a number
		"100 y 1.0",       // b not a number
		"100 201 abc",     // value not a number
		"100 202 -1.0",    // negative intensity
		"100 203 NaN",     // not a number
		"100 204 +Inf",    // unbounded
		"",                // blank
		"101 200 2.5e+03", // good
	}, "\n") + "\n"

	if err := remote.Save(ctx, bucket, key, strings.NewReader(body)); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	got := readCache(ctx, bucket, key)

	want := map[cell]float64{
		{l: 100, b: 200}: 1500,
		{l: 101, b: 200}: 2500,
	}

	if len(got) != len(want) {
		t.Fatalf("readCache kept %d entries, want %d: %v", len(got), len(want), got)
	}

	for c, v := range want {
		if math.Abs(got[c]-v) > 1e-6 {
			t.Errorf("cell %+v = %v, want %v", c, got[c], v)
		}
	}
}

// TestReadCacheOnAMissingFile: no file yet is a cold cache, which readCache
// reports as an empty map rather than by failing — Fetch has no way to act on
// the difference and would only be able to give up.
func TestReadCacheOnAMissingFile(t *testing.T) {
	fakeIRSA(t, serve)

	ctx := context.Background()

	bucket, prefix, err := remote.CacheDir(ctx, remote.IRSADust)
	if err != nil {
		t.Fatalf("CacheDir: %v", err)
	}

	if got := readCache(ctx, bucket, path.Join(prefix, "nothing-here.txt")); len(got) != 0 {
		t.Errorf("a missing cache file produced %d entries", len(got))
	}
}

// TestFetchWithNoDirectionsAsksNothing: the early return, which is what keeps a
// caller with an empty target list from opening a cache or a connection.
func TestFetchWithNoDirectionsAsksNothing(t *testing.T) {
	calls := fakeIRSA(t, serve)

	m, err := Fetch(context.Background(), nil)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if m == nil {
		t.Fatal("Fetch returned a nil map for an empty direction list")
	}

	if got := calls.Load(); got != 0 {
		t.Errorf("an empty direction list made %d requests", got)
	}
}

// TestFetchAddsToAnExistingMap: passing a map in accumulates rather than
// starting over, and a direction already held in memory is not re-asked.
func TestFetchAddsToAnExistingMap(t *testing.T) {
	calls := fakeIRSA(t, serve)

	first := Direction{L: angle.Deg(10), B: angle.Deg(20)}
	second := Direction{L: angle.Deg(30), B: angle.Deg(-40)}

	m, err := Fetch(context.Background(), nil, first)
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	if _, err := Fetch(context.Background(), m, first, second); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("%d requests in total, want 2: the direction already in the map was re-asked", got)
	}

	if m.Len() != 2 {
		t.Errorf("map holds %d directions, want 2 — the second Fetch started over instead of adding", m.Len())
	}

	if _, err := m.IntensityAt(first.L, first.B); err != nil {
		t.Errorf("the first direction was lost from the map: %v", err)
	}
}
