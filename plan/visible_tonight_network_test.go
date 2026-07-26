//go:build network

package plan_test

import (
	"context"
	"net"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/catalog/sbdb"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// requireJPL skips the test when JPL's SBDB/Horizons services are
// unreachable — per this project's network test policy, a reachability
// failure must never fail CI outright.
func requireJPL(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "ssd-api.jpl.nasa.gov:443", 5*time.Second)
	if err != nil {
		t.Skipf("JPL unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

// TestVisibleTonight_MinorBodiesRespectMagLimit is the live end-to-end
// test of the asteroid/comet path (catalog/sbdb.SearchBright Stage 1 +
// VisibleTonight's per-candidate Stage-2 real ephemeris fetch): a tight
// magLimit=2 must exclude every asteroid/comet (no known one has ever been
// observed brighter than ~mag 5.1 — a physical fact, not a coincidence of
// tonight's geometry), while a loose magLimit=5 exercises the real
// Horizons small-body SPK fetch for whatever candidates Stage 1 surfaces.
// Real bright-enough candidates existing on any given test-run night
// depends on live astronomical circumstances outside this test's control,
// so magLimit=5 doesn't assert a minimum count — only that the pipeline
// completes without error and that anything it does find is self-consistent.
func TestVisibleTonight_MinorBodiesRespectMagLimit(t *testing.T) {
	requireJPL(t)

	t.Cleanup(remote.Reset)
	remote.SetDataDirPath(t.TempDir())
	remote.EnableDownloads(remote.NAIFSPK, 0)
	remote.EnableDownloads(remote.NAIFLSK, 0)
	remote.EnableDownloads(remote.JPLHorizons, 0)

	site := saoPauloSite(t)
	sources := []resolve.BrightObjectSearcher{sbdb.New()}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	tight, err := plan.VisibleTonight(ctx, site, testNight, 2, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight (magLimit=2): %v", err)
	}

	for _, r := range tight {
		if r.Target.Kind == resolve.KindAsteroid || r.Target.Kind == resolve.KindComet {
			t.Errorf("expected no asteroid/comet at magLimit=2, got %+v", r.Target)
		}
	}

	loose, err := plan.VisibleTonight(ctx, site, testNight, 5, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight (magLimit=5): %v", err)
	}

	for _, r := range loose {
		if r.Target.Kind != resolve.KindAsteroid && r.Target.Kind != resolve.KindComet {
			continue
		}

		if r.ApparentMag >= 5 {
			t.Errorf("minor body %q ApparentMag = %v, want < 5 (magLimit)", r.Target.Name, r.ApparentMag)
		}

		if r.Target.Name == "" || r.Target.SPKID == "" {
			t.Errorf("expected populated Name/SPKID for a real minor body, got %+v", r.Target)
		}

		t.Logf("found minor body: %s (%s), mag=%.2f, constellation=%s", r.Target.Name, r.Target.Kind, r.ApparentMag, r.Constellation)
	}
}

// TestVisibleTonight_PlanetaryMoons is the live end-to-end test of
// plan.WithPlanetaryMoons(): a real SPK fetch for at least one planetary
// satellite kernel, feeding into the same downstream pipeline (windows,
// rise/transit/peak/set, extinction) every other category goes through.
//
// The NAIFSPK download cap is intentionally bounded to ~110 MB — just
// above Mars's short-span kernel (mar099s.bsp, ~64 MB) and Neptune's
// (nep097.bsp, ~100 MB) — rather than left unlimited like the sibling test
// above: unlike a per-body Horizons SPK (KB-MB scale) or even de440s
// (~32 MB), the full planetaryMoons set spans six kernels totaling ~2.4 GB
// (Jupiter's alone is ~1.1 GB), which would make this an unreasonably
// expensive test to run routinely. Denied-by-cap kernels are skipped by
// gatherPlanetaryMoons exactly like a denied-by-policy one — not a test
// failure — so this still exercises the real fetch-and-compute path for
// Phobos/Deimos/Triton without paying for Jupiter/Saturn/Uranus/Pluto's
// far larger kernels every run.
//
// magLimit=16 is loose enough for Triton (~mag 13.5) and Phobos/Deimos
// (~mag 11-13 from Earth, effectively never actually visible at Mars's
// real distance, but included for completeness) to have a chance; whether
// any specific moon clears the horizon and its own brightness bound on any
// given test-run night depends on real astronomical circumstances outside
// this test's control, so — like the sibling test above — this doesn't
// assert a minimum count, only that the pipeline completes without error
// and that anything found is self-consistent.
func TestVisibleTonight_PlanetaryMoons(t *testing.T) {
	requireJPL(t)

	t.Cleanup(remote.Reset)
	remote.SetDataDirPath(t.TempDir())
	remote.EnableDownloads(remote.NAIFSPK, 110<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	site := saoPauloSite(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	results, err := plan.VisibleTonight(ctx, site, testNight, 16, nil, ephemeris.Default(), plan.WithPlanetaryMoons())
	if err != nil {
		t.Fatalf("VisibleTonight with WithPlanetaryMoons: %v", err)
	}

	for _, r := range results {
		if r.Target.Kind != resolve.KindPlanetaryMoon {
			continue
		}

		if r.Target.Name == "" {
			t.Errorf("expected a populated Name for a planetary moon, got %+v", r.Target)
		}

		if r.ApparentMag >= 16 {
			t.Errorf("planetary moon %q ApparentMag = %v, want < 16 (magLimit)", r.Target.Name, r.ApparentMag)
		}

		t.Logf("found planetary moon: %s, mag=%.2f, constellation=%s", r.Target.Name, r.ApparentMag, r.Constellation)
	}
}
