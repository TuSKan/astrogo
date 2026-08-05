//go:build integration

// This file's one test performs the real, full ~653 MB World Atlas 2015
// download and extraction end to end. It is gated behind BOTH the
// integration build tag AND the ASTROGO_WORLDATLAS_FULL=1 environment
// variable, so it never runs as a side effect of `go test -tags=integration
// ./...` — this is a manual, opt-in verification step, never a CI gate (too
// large, too slow, and it leaves a ~2.8 GB extracted file on disk).
//
// Run with: ASTROGO_WORLDATLAS_FULL=1 go test -tags=integration -timeout 90m
// -run TestEnsureWorldAtlas_FullDownload ./skybrightness/atlas/
package atlas_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/atlas"
)

// TestEnsureWorldAtlas_FullDownload downloads and extracts the real World
// Atlas 2015 archive, then samples a known-bright site (central London) as
// a sanity check — the "does the whole pipeline actually work against the
// real upstream archive" check that download_test.go's synthetic fixture
// can't provide, and download_network_test.go's central-directory-only
// read doesn't exercise (extraction/validation of the real 2.8 GB payload).
func TestEnsureWorldAtlas_FullDownload(t *testing.T) {
	if os.Getenv("ASTROGO_WORLDATLAS_FULL") != "1" {
		t.Skip("set ASTROGO_WORLDATLAS_FULL=1 to run the real ~653 MB World Atlas download (manual, never CI)")
	}

	remote.EnableDownloads(remote.WorldAtlas, 0)

	start := time.Now()

	path, err := atlas.EnsureWorldAtlas(context.Background())
	if err != nil {
		t.Fatalf("EnsureWorldAtlas: %v", err)
	}

	t.Logf("extracted to %s in %s", path, time.Since(start))

	provider, closer, err := atlas.OpenWorldAtlas(context.Background())
	if err != nil {
		t.Fatalf("OpenWorldAtlas: %v", err)
	}
	defer func() { _ = closer.Close() }()

	// Central London — bright, densely populated, always carries real
	// (non no-data) signal in the atlas.
	sqm, err := provider.ZenithBrightness(51.5074, -0.1278)
	if err != nil {
		t.Fatalf("ZenithBrightness: %v", err)
	}

	t.Logf("central London artificial zenith SB = %.2f V mag/arcsec²", float64(sqm))

	// Ordinal sanity, not a pinned absolute value (see A.6's plan note on
	// golden constants needing a merge-time cross-check against
	// lightpollutionmap.info's own readout): central London must be
	// substantially brighter (smaller mag) than the natural-only 22.0
	// zenith background.
	if float64(sqm) >= 22.0 {
		t.Errorf("central London artificial SB %.2f should be well below the 22.0 natural-background baseline", float64(sqm))
	}

	// Re-run: must be a fast, network-free cache hit (exact-size match).
	start = time.Now()

	if _, err := atlas.EnsureWorldAtlas(context.Background()); err != nil {
		t.Fatalf("second EnsureWorldAtlas call: %v", err)
	}

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("second EnsureWorldAtlas call took %s — expected a fast cache hit, not a re-download/re-extraction", elapsed)
	}
}
