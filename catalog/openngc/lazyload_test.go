package openngc

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestNewDoesNoIO is the whole point of the change: construction is pure, so
// it cannot block, cannot fail, and cannot consume a deadline. Offline mode is
// the strongest available statement of "no I/O happened" — under it any fetch
// attempt fails, so a New that reached the network could not return here at all.
func TestNewDoesNoIO(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)
	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	remote.SetOffline(true)

	if New() == nil {
		t.Fatal("New returned nil")
	}
}

// TestFailedLoadIsRetried is the second half of the change. A load failure used
// to be recorded at construction and replayed for the life of the provider, so
// a consent gate granted a moment later — or an endpoint that came back — never
// took effect. The same provider must be able to answer once the reason for the
// failure is gone.
func TestFailedLoadIsRetried(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	// Downloads not enabled yet: the catalog is unreachable.
	p := New()

	if _, err := p.Resolve(context.Background(), "M42"); err == nil {
		t.Fatal("Resolve without download consent: want an error, got none")
	} else if errors.Is(err, resolve.ErrNotFound) {
		t.Errorf("an unreachable catalog reported as a missing object: %v", err)
	}

	// Grant consent on the same provider and ask again.
	remote.EnableDownloads(0, remote.OpenNGC)

	got, err := p.Resolve(context.Background(), "M42")
	if err != nil {
		t.Fatalf("Resolve after consent was granted: %v", err)
	}

	if got.ID != "NGC1976" {
		t.Errorf("Resolve(M42).ID = %q, want NGC1976", got.ID)
	}
}

// TestLoadHappensOnce: the catalog is 7 MB and is kept for the life of the
// provider. Going offline after a successful load and asking again is the
// behavioural way to say so — a provider that re-fetched per query would fail
// the second call.
func TestLoadHappensOnce(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)
	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	p := New()

	if _, err := p.Resolve(context.Background(), "M42"); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}

	remote.SetOffline(true)

	if _, err := p.Resolve(context.Background(), "M31"); err != nil {
		t.Errorf("second Resolve went back to the source: %v", err)
	}
}

// TestLoadHonoursContext: the fetch now runs under the caller's context, which
// is the reason for moving it out of New in the first place.
func TestLoadHonoursContext(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)
	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := New().Resolve(ctx, "M42"); !errors.Is(err, context.Canceled) {
		t.Errorf("Resolve with a cancelled context: err = %v, want context.Canceled", err)
	}
}

// TestSearchBrightReportsLoadFailure guards the one query whose signature has
// no error return. Before the load moved, a failure here was not reported at
// all — SearchBright never consulted the recorded error, so an unreachable
// catalog came back as a sky containing no bright objects.
func TestSearchBrightReportsLoadFailure(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	// Downloads deliberately left disabled.

	var (
		n      int
		gotErr error
	)

	for _, err := range New().SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 6}) {
		n++

		if err != nil {
			gotErr = err
		}
	}

	if gotErr == nil {
		t.Errorf("SearchBright over an unreachable catalog yielded %d targets and no error", n)
	}
}

// TestConcurrentFirstQueriesLoadOnce: mu is held across the fetch, so the
// second arrival waits rather than issuing its own request. Meaningful under
// -race, which is where this package's concurrency is actually checked.
func TestConcurrentFirstQueriesLoadOnce(t *testing.T) {
	t.Cleanup(remote.Capture().Restore)

	fakeSources(t)
	remote.EnableDownloads(0, remote.OpenNGC)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	p := New()

	const goroutines = 8

	var wg sync.WaitGroup

	errs := make([]error, goroutines)

	for i := range goroutines {
		wg.Go(func() {
			_, errs[i] = p.Resolve(context.Background(), "M42")
		})
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: Resolve(M42): %v", i, err)
		}
	}
}
