package plan

import (
	"context"
	"testing"

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

// TestPlanetaryMoonsTableIntegrity guards planetaryMoons against copy-paste
// mistakes that would silently corrupt gatherPlanetaryMoons' output: a
// duplicate NAIF ID (two moons colliding on the same body), an empty name
// or kernel, or an H value outside any real moon's plausible range (a
// typo — e.g. a transposed digit — would otherwise surface exactly like
// the impossible-magnitude bug this session already found and fixed in
// ephemeris/jpl/spk's Type 21 reader).
func TestPlanetaryMoonsTableIntegrity(t *testing.T) {
	seenID := make(map[int32]string)

	for _, m := range planetaryMoons {
		if m.name == "" {
			t.Errorf("moonSpec with NAIF ID %d has an empty name", m.naifID)
		}

		if m.kernel == "" {
			t.Errorf("moonSpec %q has an empty kernel", m.name)
		}

		if prev, ok := seenID[m.naifID]; ok {
			t.Errorf("NAIF ID %d used by both %q and %q", m.naifID, prev, m.name)
		}

		seenID[m.naifID] = m.name

		// The faintest real absolute magnitude among these moons (Deimos,
		// H≈12.89) and the brightest (Ganymede, H≈-2.09) bound a generous
		// plausible range — anything outside it is almost certainly a typo,
		// not a real published value.
		if m.h < -5 || m.h > 20 {
			t.Errorf("%s: H=%.2f is outside a plausible range for a named Solar System moon", m.name, m.h)
		}
	}

	if len(planetaryMoons) != 21 {
		t.Errorf("expected 21 planetary moons, got %d", len(planetaryMoons))
	}
}

// TestGatherPlanetaryMoonsSkipsWithoutDownloadConsent verifies
// gatherPlanetaryMoons degrades gracefully — an empty candidate/provider
// result, not an error or panic — when none of its kernels can actually be
// fetched (no download consent granted, no pre-seeded cache), matching
// this package's established "skip an unavailable candidate rather than
// fail the whole query" convention (candidateFromTarget, gatherCandidates).
//
// This saves and restores only remote.NAIFSPK's own consent state (via
// remote.Lookup/EnableDownloads/DisableDownloads), rather than the
// blanket remote.Reset() the sibling network tests in this package use —
// under the "integration" build tag, plan/integration_main_test.go's
// TestMain grants unlimited NAIFSPK consent for this package's entire test
// binary run (real de441/de442 fetches need it); a plain remote.Reset()
// here would revoke that for every test running afterward in the same
// binary, not just this one (confirmed: it did, breaking three otherwise
// unrelated eclipse/moon-phase tests when this test ran under
// `-tags=integration` before this fix).
func TestGatherPlanetaryMoonsSkipsWithoutDownloadConsent(t *testing.T) {
	prevEP, _ := remote.Lookup(remote.NAIFSPK)

	t.Cleanup(func() {
		if prevEP.DownloadsOK {
			remote.EnableDownloads(remote.NAIFSPK, prevEP.MaxDownloadSize)
		} else {
			remote.DisableDownloads(remote.NAIFSPK)
		}
	})
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
