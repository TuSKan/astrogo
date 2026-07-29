package plan

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// TestPlanetConstructorsIncludesPluto is a regression test: Pluto has its
// own NewPluto constructor (plan/planet.go) and a supported magnitude
// model, but was originally missing from VisibleTonight's planetConstructors
// list — silently excluding it from every result regardless of real
// brightness, unlike every other planet.
func TestPlanetConstructorsIncludesPluto(t *testing.T) {
	for _, ctor := range planetConstructors {
		p := ctor(nil)
		if p.Name() == "Pluto" {
			return
		}
	}

	t.Error("expected planetConstructors to include a Pluto constructor")
}

// TestGatherPlanetaryMoonsSkipsWithoutDownloadConsent verifies
// gatherPlanetaryMoons degrades gracefully — an empty candidate/provider
// result, not an error or panic — when none of its kernels can actually be
// fetched (no download consent granted, no pre-seeded cache), matching
// this package's established "skip an unavailable candidate rather than
// fail the whole query" convention (candidateFromTarget, gatherCandidates).
//
// This captures and restores only remote.NAIFSPK's own consent state (via
// remote.Capture/Restore), rather than the blanket remote.Reset() the
// sibling network tests in this package use — under the "integration"
// build tag, plan/integration_main_test.go's TestMain grants unlimited
// NAIFSPK consent for this package's entire test binary run (real
// de441/de442 fetches need it); a plain remote.Reset() here would revoke
// that for every test running afterward in the same binary, not just this
// one (confirmed: it did, breaking three otherwise unrelated eclipse/
// moon-phase tests when this test ran under `-tags=integration` before
// this fix).
func TestGatherPlanetaryMoonsSkipsWithoutDownloadConsent(t *testing.T) {
	t.Cleanup(remote.Capture(remote.NAIFSPK).Restore)
	remote.DisableDownloads(remote.NAIFSPK)
	remote.SetDataDirPath(t.TempDir())

	candidates, providers := gatherPlanetaryMoons(context.Background(), time.Date(2026, time.August, 1, 0, 0, 0, 0, time.LocationUTC), 30)

	if len(candidates) != 0 {
		t.Errorf("expected no candidates without download consent, got %d", len(candidates))
	}

	if len(providers) != 0 {
		t.Errorf("expected no opened providers without download consent, got %d", len(providers))
	}
}

// TestNeedsSmallBodyEphemeris is a regression test for a real bug found
// during exploration: Ceres/Eris/Haumea/Makemake (resolve.KindDwarfPlanet)
// never reached VisibleTonight's Stage-2 real-ephemeris/magnitude fetch,
// because the Kind gate at candidateFromTarget/gatherCandidates only
// checked KindAsteroid/KindComet — a KindDwarfPlanet target silently fell
// through to a zero-coordinate DeepSkyObject with no magnitude and was
// dropped. KindInterstellar needs the same treatment (SBDB resolves it by
// designation with no coordinate of its own, exactly like an asteroid).
func TestNeedsSmallBodyEphemeris(t *testing.T) {
	cases := []struct {
		kind resolve.Kind
		want bool
	}{
		{resolve.KindAsteroid, true},
		{resolve.KindComet, true},
		{resolve.KindDwarfPlanet, true},
		{resolve.KindInterstellar, true},
		{resolve.KindStar, false},
		{resolve.KindPlanet, false},
		{resolve.KindMoon, false},
		{resolve.KindPlanetaryMoon, false},
		{resolve.KindSatellite, false},
		{resolve.KindGalaxy, false},
		{resolve.KindOther, false},
	}

	for _, c := range cases {
		if got := needsSmallBodyEphemeris(c.kind); got != c.want {
			t.Errorf("needsSmallBodyEphemeris(%v) = %v, want %v", c.kind, got, c.want)
		}
	}
}
