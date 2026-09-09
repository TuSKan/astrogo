package iers

import (
	"testing"
	"time"
)

// uncoveredMJD is far outside sampleFinals2000A's two days, so no amount of
// loading will ever cover it — which is the whole point.
const uncoveredMJD = 200000.0

// bulletinModTime stands in for when the cached file was last written.
//
// Fixed rather than time.Now: nothing here depends on it being recent, and a
// test that reads the wall clock drifts across the IERS measured/predicted
// boundary and starts failing on a future date with no code change.
var bulletinModTime = time.Date(2026, time.March, 20, 0, 0, 0, 0, time.UTC)

// TestEnsureLoadedReadsTheCacheOnceForAnUncoveredEpoch is the regression.
//
// covered(mjd) was the only fast path, so an epoch the bulletin does not
// cover fell through to Loader.Cached on *every* lookup and re-parsed the
// whole file. Measured through Time.EOP: 71 ns for a covered epoch against
// 11 ms allocating 15.6 MB for one just outside — a 150,000x cliff, silent,
// on two ordinary requests (scheduling more than a year out, and history
// before 1973).
//
// It also made this repository's own scheduler benchmarks meaningless: they
// run at time.ZeroTime, and 99.88% of their allocated bytes were this.
func TestEnsureLoadedReadsTheCacheOnceForAnUncoveredEpoch(t *testing.T) {
	l := &fakeLoader{
		cached:   Data{Raw: []byte(sampleFinals2000A), ModTime: bulletinModTime},
		fetchErr: errUpstream,
	}

	installLoader(t, l)

	const lookups = 50

	for range lookups {
		_ = EnsureLoaded(uncoveredMJD)
	}

	cached, _ := l.counts()
	if cached != 1 {
		t.Errorf("Loader.Cached called %d times for %d lookups of one uncovered epoch, want 1; "+
			"re-reading the bulletin cannot change whether it covers an epoch it does not cover",
			cached, lookups)
	}
}

// TestEnsureLoadedStillReadsTheCacheForACoveredEpoch is the half the
// throttle must not break: the first lookup still loads.
func TestEnsureLoadedStillReadsTheCacheForACoveredEpoch(t *testing.T) {
	l := &fakeLoader{
		cached:   Data{Raw: []byte(sampleFinals2000A), ModTime: bulletinModTime},
		fetchErr: errUpstream,
	}

	installLoader(t, l)

	if err := EnsureLoaded(41684.5); err != nil {
		t.Fatalf("EnsureLoaded for a covered epoch: %v", err)
	}

	cached, fetched := l.counts()
	if cached != 1 {
		t.Errorf("Loader.Cached called %d times, want 1", cached)
	}

	if fetched != 0 {
		t.Errorf("Loader.Fetch called %d times for an epoch the cache covers, want 0", fetched)
	}
}

// TestResetAllowsTheCacheToBeReadAgain covers the contract the throttle
// could otherwise have broken quietly.
//
// Reset promises a caller can start over, and it is the cached read that
// keeps that promise — a Reset followed by a lookup must not find the
// throttle still closed and silently keep the zero model.
func TestResetAllowsTheCacheToBeReadAgain(t *testing.T) {
	l := &fakeLoader{
		cached:   Data{Raw: []byte(sampleFinals2000A), ModTime: bulletinModTime},
		fetchErr: errUpstream,
	}

	installLoader(t, l)

	if err := EnsureLoaded(41684.5); err != nil {
		t.Fatalf("first EnsureLoaded: %v", err)
	}

	Reset()

	if err := EnsureLoaded(41684.5); err != nil {
		t.Fatalf("EnsureLoaded after Reset: %v", err)
	}

	if cached, _ := l.counts(); cached != 2 {
		t.Errorf("Loader.Cached called %d times across a Reset, want 2; Reset says a "+
			"subsequent EnsureLoaded is free to load again", cached)
	}
}

// TestZeroCooldownDisablesTheCacheThrottle keeps SetRetryCooldown meaning
// what it says for both halves it now governs.
func TestZeroCooldownDisablesTheCacheThrottle(t *testing.T) {
	l := &fakeLoader{
		cached:   Data{Raw: []byte(sampleFinals2000A), ModTime: bulletinModTime},
		fetchErr: errUpstream,
	}

	installLoader(t, l)

	previous := currentRetryCooldown()

	SetRetryCooldown(0)

	t.Cleanup(func() { SetRetryCooldown(previous) })

	const lookups = 3

	for range lookups {
		_ = EnsureLoaded(uncoveredMJD)
	}

	if cached, _ := l.counts(); cached != lookups {
		t.Errorf("Loader.Cached called %d times with the cooldown disabled, want %d",
			cached, lookups)
	}
}

// currentRetryCooldown reads the cooldown under the mutex that guards it,
// so a test can put back whatever it found.
func currentRetryCooldown() time.Duration {
	fetchMu.Lock()
	defer fetchMu.Unlock()

	return retryCooldown
}
