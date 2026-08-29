package iers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// sampleFinals2000A mimics finals2000A.all format for two consecutive days
// (same fixture shape as reader_test.go's TestParseFinals2000A), covering
// MJD 41684-41685.
const sampleFinals2000A = `73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300   .143000   .137000   .8075000   -18.637    -3.667
73 1 3 41685.00 I  0.118980 0.011039  0.135656 0.013616  I 0.8056163 0.0002710  3.5563 0.1916  P    -0.751    0.199    -0.701    0.300   .141000   .134000   .8044000   -18.636    -3.571  `

// fakeLoader stands in for astrogo/remote's loader.
//
// These tests used to drive the real thing: a temp-directory bucket, a
// rewritten endpoint URL and download consent, all to exercise a retry
// cooldown. That was only possible because this package imported remote,
// which is the dependency the Loader seam exists to remove — so what
// remains here is the logic this package actually owns, and it no longer
// needs a storage backend to run.
//
// Behaviour that belongs to remote — ETag body-skip, consent default-deny,
// corrupt-download rejection — is tested in remote's own eop_test.go
// against astrogo/time's public API.
type fakeLoader struct {
	mu sync.Mutex

	cached    Data
	cachedErr error

	fetched    Data
	fetchErr   error
	fetchDelay time.Duration

	cachedCalls int
	fetchCalls  int
}

func (f *fakeLoader) Cached(context.Context) (Data, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.cachedCalls++

	return f.cached, f.cachedErr
}

func (f *fakeLoader) Fetch(ctx context.Context) (Data, error) {
	f.mu.Lock()
	f.fetchCalls++
	delay := f.fetchDelay
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return Data{}, fmt.Errorf("fakeLoader: %w", ctx.Err())
		case <-time.After(delay):
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	return f.fetched, f.fetchErr
}

func (f *fakeLoader) counts() (cached, fetched int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.cachedCalls, f.fetchCalls
}

// installLoader registers l and clears the cooldown state, so a test's
// cold scenario is not short-circuited by an earlier test's attempt.
func installLoader(t *testing.T, l Loader) {
	t.Helper()

	clearCooldown()
	RegisterLoader(l)

	t.Cleanup(func() {
		RegisterLoader(nil)
		resetForTest()
		clearCooldown()
	})
}

func clearCooldown() {
	fetchMu.Lock()
	defer fetchMu.Unlock()

	lastAttempt = time.Time{}
	errLastFetch = nil
}

// errUpstream stands in for whatever the loader's own layer failed with.
var errUpstream = errors.New("upstream unavailable")

// noCache is a loader whose cache is empty, so every EnsureLoaded falls
// through to Fetch.
func noCache(fetched Data, fetchErr error) *fakeLoader {
	return &fakeLoader{cachedErr: ErrNoEOPData, fetched: fetched, fetchErr: fetchErr}
}

func TestEnsureLoadedFetchesWhenUncovered(t *testing.T) {
	l := noCache(Data{Raw: []byte(sampleFinals2000A)}, nil)
	installLoader(t, l)

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after EnsureLoaded")
	}

	if _, fetches := l.counts(); fetches != 1 {
		t.Errorf("Fetch called %d times, want 1", fetches)
	}
}

// TestEnsureLoadedWithoutLoaderReportsErrNoLoader is the degradation path
// the whole seam rests on: with no loader registered — a program that
// imports astrogo/time but not astrogo/remote — this must fail cleanly so
// the caller falls back to zero EOP, rather than panicking or blocking.
func TestEnsureLoadedWithoutLoaderReportsErrNoLoader(t *testing.T) {
	installLoader(t, nil)

	err := EnsureLoaded(41684)
	if !errors.Is(err, ErrNoLoader) {
		t.Fatalf("EnsureLoaded with no loader = %v, want ErrNoLoader", err)
	}
}

func TestEnsureLoadedPropagatesFetchError(t *testing.T) {
	installLoader(t, noCache(Data{}, errUpstream))

	err := EnsureLoaded(41684)
	if !errors.Is(err, errUpstream) {
		t.Fatalf("EnsureLoaded = %v, want it to wrap %v", err, errUpstream)
	}
}

func TestEnsureLoadedReadsCacheWithoutFetching(t *testing.T) {
	l := &fakeLoader{cached: Data{Raw: []byte(sampleFinals2000A)}}
	installLoader(t, l)

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if got := EOPSource(); got != SourceCache {
		t.Errorf("EOPSource = %q, want %q", got, SourceCache)
	}

	if _, fetches := l.counts(); fetches != 0 {
		t.Errorf("Fetch called %d times; a covering cache must not hit the network", fetches)
	}
}

// TestEnsureLoadedFallsThroughOnCorruptCache keeps the guarantee that a
// damaged cache file is not the end of the road: the fetch still runs.
func TestEnsureLoadedFallsThroughOnCorruptCache(t *testing.T) {
	l := &fakeLoader{
		cached:  Data{Raw: []byte("not a finals2000A file")},
		fetched: Data{Raw: []byte(sampleFinals2000A)},
	}
	installLoader(t, l)

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if _, fetches := l.counts(); fetches != 1 {
		t.Errorf("Fetch called %d times, want 1 after a corrupt cache", fetches)
	}
}

// TestEnsureLoadedSeedsCooldownFromCacheModTime stops a fresh process from
// re-downloading immediately after a recent attempt: the cache file's age
// stands in for when we last tried.
func TestEnsureLoadedSeedsCooldownFromCacheModTime(t *testing.T) {
	l := &fakeLoader{
		cached:  Data{Raw: []byte(sampleFinals2000A), ModTime: time.Now()},
		fetched: Data{Raw: []byte(sampleFinals2000A)},
	}
	installLoader(t, l)

	SetRetryCooldown(time.Hour)
	t.Cleanup(func() { SetRetryCooldown(5 * time.Minute) })

	// 99999 is far outside the sample's coverage, so the cache read
	// registers a table but does not satisfy the query.
	_ = EnsureLoaded(99999)

	if _, fetches := l.counts(); fetches != 0 {
		t.Errorf("Fetch called %d times; a cache written inside the cooldown must suppress it", fetches)
	}
}

func TestEnsureLoadedRespectsCooldown(t *testing.T) {
	l := noCache(Data{}, errUpstream)
	installLoader(t, l)

	SetRetryCooldown(time.Hour)
	t.Cleanup(func() { SetRetryCooldown(5 * time.Minute) })

	_ = EnsureLoaded(41684)
	_ = EnsureLoaded(41685)
	_ = EnsureLoaded(50000)

	if _, fetches := l.counts(); fetches != 1 {
		t.Errorf("Fetch called %d times across three MJDs; the cooldown should allow 1", fetches)
	}
}

func TestSetRetryCooldownZeroDisablesThrottling(t *testing.T) {
	l := noCache(Data{}, errUpstream)
	installLoader(t, l)

	SetRetryCooldown(0)
	t.Cleanup(func() { SetRetryCooldown(5 * time.Minute) })

	_ = EnsureLoaded(41684)
	_ = EnsureLoaded(41685)

	if _, fetches := l.counts(); fetches != 2 {
		t.Errorf("Fetch called %d times with throttling off, want 2", fetches)
	}
}

func TestEnsureLoadedSkipsEntirelyWhenModelExplicit(t *testing.T) {
	l := noCache(Data{Raw: []byte(sampleFinals2000A)}, nil)
	installLoader(t, l)

	// An explicit choice is authoritative — including the deliberate
	// zero-EOP one, which the lazy loader must never silently replace.
	RegisterModel(ZeroModel{})

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	cached, fetches := l.counts()
	if cached != 0 || fetches != 0 {
		t.Errorf("loader touched (%d cached, %d fetch) despite an explicit model", cached, fetches)
	}
}

func TestEnsureLoadedFastPathSkipsLoaderWhenAlreadyCovered(t *testing.T) {
	table, err := ParseFinals2000A(strings.NewReader(sampleFinals2000A))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	l := noCache(Data{Raw: []byte(sampleFinals2000A)}, nil)
	installLoader(t, l)

	registerModelInternal(table, SourceCache)

	if err := EnsureLoaded(41684); err != nil {
		t.Fatalf("EnsureLoaded: %v", err)
	}

	if cached, fetches := l.counts(); cached != 0 || fetches != 0 {
		t.Errorf("loader touched (%d cached, %d fetch) when the model already covered the epoch", cached, fetches)
	}
}

func TestCoveredNonTableModel(t *testing.T) {
	t.Cleanup(resetForTest)

	registerModelInternal(nonTableModel{}, SourceZero)

	if covered(41684) {
		t.Error("covered reported true for a model that is not a Table")
	}
}

func TestFetchContextCancellation(t *testing.T) {
	l := noCache(Data{Raw: []byte(sampleFinals2000A)}, nil)
	l.fetchDelay = 2 * time.Second

	installLoader(t, l)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := fetch(ctx, l); err == nil {
		t.Fatal("fetch with a cancelled context returned nil")
	}
}

func TestEnsureLoadedConcurrent(t *testing.T) {
	l := noCache(Data{Raw: []byte(sampleFinals2000A)}, nil)
	installLoader(t, l)

	var wg sync.WaitGroup

	for range 16 {
		wg.Go(func() { _ = EnsureLoaded(41684) })
	}

	wg.Wait()

	// The coverage check is repeated inside the lock, so a successful
	// concurrent load is respected rather than re-fetched by every waiter.
	if _, fetches := l.counts(); fetches != 1 {
		t.Errorf("Fetch called %d times from 16 goroutines, want 1", fetches)
	}

	if _, _, ok := Coverage(); !ok {
		t.Error("expected a coverage-reporting model after concurrent EnsureLoaded")
	}
}

type nonTableModel struct{}

func (nonTableModel) EOP(float64) (EOP, error) { return EOP{}, ErrNoRecords }
