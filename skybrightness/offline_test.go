package skybrightness_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
)

// This is the behavioral form of the spec's no-hidden-network rule: not "the
// package does not import remote", which TestCoreDoesNotImportIOPackages
// checks, but "evaluation does not fetch".
//
// remote.SetOffline(true) makes every registered endpoint refuse to resolve,
// so any acquisition inside Estimate would fail loudly rather than quietly
// dialling out. The result must also be identical to the online one: a model
// that silently degrades when the network is gone is not deterministic, which
// is what §31's reproducibility rests on.
//
// It runs the six real components fullsky_test.go assembles, with the Moon up,
// so moonlight queries the Scene's ephemeris and builds a coord.Context, and
// the natural terms locate the Sun. Offline goes first: a component that
// acquired something on its first call would already hold it if the online
// call ran first. Until #973 the model here was two constant stubs, whose
// output is identical offline by construction.
//
// Not parallel: offline mode is process-wide. This test lives in the external
// test package, so it does not make the core itself depend on remote.
func TestEstimateWorksOffline(t *testing.T) {
	when, _ := nearFullMoonUp(t)
	m, scene, grid := assembleSky(t, when)

	q := skybrightness.Query{
		Scene:     scene,
		Direction: coord.NewAltAz(angle.Deg(70), angle.Deg(120)),
		Grid:      grid,
	}

	t.Cleanup(remote.Capture().Restore)
	remote.SetOffline(true)

	offline, err := m.Estimate(context.Background(), q)
	if err != nil {
		t.Fatalf("Estimate while offline: %v; evaluation must not need the network", err)
	}

	remote.SetOffline(false)

	online, err := m.Estimate(context.Background(), q)
	if err != nil {
		t.Fatalf("Estimate (online): %v", err)
	}

	a, b := offline.SpectralRadiance(), online.SpectralRadiance()
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("offline gave %d samples and online %d", len(a), len(b))
	}

	var lit int

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs offline: %v against %v online", i, a[i], b[i])
		}

		if a[i] > 0 {
			lit++
		}
	}

	// A sky of zeros would be identical offline too, and prove nothing.
	if lit == 0 {
		t.Fatal("every sample is zero; the components evaluated nothing")
	}
}

// A sky map is many evaluations; none of them may reach the network either.
func TestSkyMapWorksOffline(t *testing.T) {
	when, _ := nearFullMoonUp(t)
	m, scene, grid := assembleSky(t, when)

	t.Cleanup(remote.Capture().Restore)
	remote.SetOffline(true)

	points, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: scene, Grid: grid}, 8)
	if err != nil {
		t.Fatalf("SkyMap while offline: %v", err)
	}

	if len(points) == 0 {
		t.Fatal("SkyMap returned no samples")
	}
}
