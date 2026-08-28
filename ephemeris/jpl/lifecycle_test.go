package jpl_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestNewProvider_ColdCacheDownloadsDisabled confirms astrogo never
// auto-downloads a kernel: a fresh, empty DataDir with NAIFSPK downloads
// explicitly disabled (overriding this package's TestMain-granted consent)
// must fail with an actionable ErrDownloadDenied, not a silent download.
func TestNewProvider_ColdCacheDownloadsDisabled(t *testing.T) {
	t.Cleanup(remote.Capture(remote.NAIFSPK).Restore)

	remote.DisableDownloads(remote.NAIFSPK)
	// A fresh, empty data dir isolates this cold-cache scenario from the
	// shared cache other tests in this package populate. remote.SetDataDir
	// is the only control for that: a provider has no data-dir option.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	_, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
	if !errors.Is(err, remote.ErrDownloadDenied) {
		t.Fatalf("expected ErrDownloadDenied, got %v", err)
	}

	msg := err.Error()
	for _, want := range []string{"de440s", "EnableDownloads", "NAIFSPK"} {
		if !strings.Contains(msg, want) {
			t.Errorf("denial message missing %q: %s", want, msg)
		}
	}
}

// TestKernelLifecycle exercises Open/AddKernelFrom/RemoveKernel/UnloadAll/
// LoadedKernels against real cached kernels — obtained via NewProvider
// (this package's TestMain grants NAIFSPK/NAIFLSK consent, so this reuses
// the shared cache like every other test here) and then reopened straight
// from the cache bucket with zero network involvement, proving the offline
// path works independently of NewProvider.
func TestKernelLifecycle(t *testing.T) {
	seed, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
	if err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	kernels := seed.LoadedKernels()
	if len(kernels) != 1 || kernels[0].Key == "" {
		t.Fatalf("expected 1 loaded kernel with a recorded key, got %+v", kernels)
	}

	ctx := context.Background()

	spkBucket, spkPrefix, err := remote.CacheDir(ctx, remote.NAIFSPK)
	if err != nil {
		t.Fatalf("CacheDir(NAIFSPK): %v", err)
	}

	spkKey := spkPrefix + kernels[0].Key

	// The LSK is a separate field, not a Kernel, so it is not in
	// LoadedKernels — name it the same way NewProvider's lsk.Cache does.
	_, lskPrefix, err := remote.CacheDir(ctx, remote.NAIFLSK)
	if err != nil {
		t.Fatalf("CacheDir(NAIFLSK): %v", err)
	}

	lskKey := lskPrefix + "lsk/naif0012.tls"

	if err := seed.Close(); err != nil {
		t.Fatalf("close seed provider: %v", err)
	}

	// Open: straight from the cache bucket, zero network.
	p, err := jpl.Open(ctx, spkBucket, lskKey, spkKey)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = p.Close() })

	got := p.LoadedKernels()
	if len(got) != 1 {
		t.Fatalf("expected 1 kernel after Open, got %d", len(got))
	}

	if got[0].Key != spkKey {
		t.Errorf("Key = %q, want %q", got[0].Key, spkKey)
	}

	if got[0].Segments == 0 {
		t.Error("expected at least one segment")
	}

	// AddKernelFrom: load the same object again as a second kernel.
	if err := p.AddKernelFrom(ctx, spkBucket, spkKey); err != nil {
		t.Fatalf("AddKernelFrom: %v", err)
	}

	if got := p.LoadedKernels(); len(got) != 2 {
		t.Fatalf("expected 2 kernels after AddKernelFrom, got %d", len(got))
	}

	// State should still resolve Mars (index rebuilt correctly after the add).
	if _, err := p.State(core.Mars, seedEpoch(t)); err != nil {
		t.Errorf("State after AddKernelFrom: %v", err)
	}

	// RemoveKernel: drop the first kernel; the second (identical) one must
	// still serve queries, proving the index was correctly rebuilt rather
	// than left pointing at stale KernelIndex positions.
	if err := p.RemoveKernel(0); err != nil {
		t.Fatalf("RemoveKernel: %v", err)
	}

	if got := p.LoadedKernels(); len(got) != 1 {
		t.Fatalf("expected 1 kernel after RemoveKernel, got %d", len(got))
	}

	if _, err := p.State(core.Mars, seedEpoch(t)); err != nil {
		t.Errorf("State after RemoveKernel: %v", err)
	}

	// Invalid index.
	if err := p.RemoveKernel(5); !errors.Is(err, jpl.ErrKernelIndexOutOfRange) {
		t.Errorf("expected ErrKernelIndexOutOfRange, got %v", err)
	}

	// UnloadAll: provider becomes empty but reusable.
	if err := p.UnloadAll(); err != nil {
		t.Fatalf("UnloadAll: %v", err)
	}

	if got := p.LoadedKernels(); len(got) != 0 {
		t.Fatalf("expected 0 kernels after UnloadAll, got %d", len(got))
	}

	if err := p.AddKernelFrom(ctx, spkBucket, spkKey); err != nil {
		t.Fatalf("AddKernelFrom after UnloadAll: %v", err)
	}

	if _, err := p.State(core.Mars, seedEpoch(t)); err != nil {
		t.Errorf("State after UnloadAll+AddKernelFrom: %v", err)
	}
}

func seedEpoch(t *testing.T) (epoch time.Time) {
	t.Helper()

	return time.FromJD(2460000.5, time.UTC)
}
