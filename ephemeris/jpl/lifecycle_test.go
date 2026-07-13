package jpl_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// TestNewProvider_ColdCacheDownloadsDisabled confirms astrogo never
// auto-downloads a kernel: a fresh, empty DataDir with NAIFSPK downloads
// explicitly disabled (overriding this package's TestMain-granted consent)
// must fail with an actionable ErrDownloadDenied, not a silent download.
func TestNewProvider_ColdCacheDownloadsDisabled(t *testing.T) {
	remote.DisableDownloads(remote.NAIFSPK)

	t.Cleanup(func() { remote.EnableDownloads(remote.NAIFSPK, 0) })

	_, err := jpl.NewProvider(core.Planets, "de440s", jpl.WithDataDir(t.TempDir()))
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

// TestKernelLifecycle exercises Open/AddKernelFile/RemoveKernel/UnloadAll/
// LoadedKernels against real local kernel files — obtained via NewProvider
// (this package's TestMain already grants NAIFSPK/NAIFLSK download
// consent, so this reuses/populates the shared cache like every other test
// in this file) and then reopened purely from disk with zero network
// involvement, proving the offline path works independently of NewProvider.
func TestKernelLifecycle(t *testing.T) {
	seed, err := jpl.NewProvider(core.Planets, "de440s")
	if err != nil {
		t.Fatalf("seed provider: %v", err)
	}

	kernels := seed.LoadedKernels()
	if len(kernels) != 1 || kernels[0].Path == "" {
		t.Fatalf("expected 1 loaded kernel with a recorded path, got %+v", kernels)
	}

	spkPath := kernels[0].Path
	// The LSK isn't tracked in LoadedKernels (it's a separate field, not a
	// Kernel) — reconstruct its path from the same "lsk/naif0012.tls"
	// join NewProvider itself uses (provider.go's lsk.Cache call).
	lskPath := filepath.Join(seed.DataDir, "lsk", "naif0012.tls")

	if err := seed.Close(); err != nil {
		t.Fatalf("close seed provider: %v", err)
	}

	// Open: pure local construction, zero network.
	p, err := jpl.Open(lskPath, spkPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = p.Close() })

	got := p.LoadedKernels()
	if len(got) != 1 {
		t.Fatalf("expected 1 kernel after Open, got %d", len(got))
	}

	if got[0].Path != spkPath {
		t.Errorf("Path = %q, want %q", got[0].Path, spkPath)
	}

	if got[0].Segments == 0 {
		t.Error("expected at least one segment")
	}

	// AddKernelFile: load the same file again as a second kernel.
	if err := p.AddKernelFile(spkPath); err != nil {
		t.Fatalf("AddKernelFile: %v", err)
	}

	if got := p.LoadedKernels(); len(got) != 2 {
		t.Fatalf("expected 2 kernels after AddKernelFile, got %d", len(got))
	}

	// State should still resolve Mars (index rebuilt correctly after the add).
	if _, err := p.State(core.Mars, seedEpoch(t)); err != nil {
		t.Errorf("State after AddKernelFile: %v", err)
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

	if err := p.AddKernelFile(spkPath); err != nil {
		t.Fatalf("AddKernelFile after UnloadAll: %v", err)
	}

	if _, err := p.State(core.Mars, seedEpoch(t)); err != nil {
		t.Errorf("State after UnloadAll+AddKernelFile: %v", err)
	}
}

func seedEpoch(t *testing.T) (epoch time.Time) {
	t.Helper()

	return time.FromJD(2460000.5, time.UTC)
}
