package skybrightness_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
)

// This is the real form of the spec's no-hidden-network rule: not "the
// package does not import remote", but "evaluation does not fetch".
//
// remote.SetOffline(true) makes every registered endpoint refuse to
// resolve, so any hidden acquisition inside Estimate would fail loudly
// rather than quietly dialling out. The result must also be identical to
// the online one — a model that silently degrades when the network is gone
// is not deterministic, which is what §31's reproducibility rests on.
//
// This test lives in the external test package, so it does not make the
// core itself depend on remote.
func TestEstimateWorksOffline(t *testing.T) {
	m, err := skybrightness.NewModel("test",
		constantComponent{id: skybrightness.Starlight, value: 1e-9},
		constantComponent{id: skybrightness.Zodiacal, value: 2e-9},
	)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	q := skybrightness.Query{Scene: testScene(t), Direction: zenith()}

	online, err := m.Estimate(context.Background(), q)
	if err != nil {
		t.Fatalf("Estimate (online): %v", err)
	}

	remote.SetOffline(true)
	t.Cleanup(func() { remote.SetOffline(false) })

	offline, err := m.Estimate(context.Background(), q)
	if err != nil {
		t.Fatalf("Estimate while offline: %v — evaluation must not need the network", err)
	}

	a, b := online.SpectralRadiance(), offline.SpectralRadiance()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs offline: %v vs %v", i, a[i], b[i])
		}
	}
}

// A sky map is many evaluations; none of them may reach the network either.
func TestSkyMapWorksOffline(t *testing.T) {
	m, err := skybrightness.NewModel("test", constantComponent{id: skybrightness.Starlight, value: 1e-9})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	remote.SetOffline(true)
	t.Cleanup(func() { remote.SetOffline(false) })

	points, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: testScene(t)}, 8)
	if err != nil {
		t.Fatalf("SkyMap while offline: %v", err)
	}

	if len(points) == 0 {
		t.Fatal("SkyMap returned no samples")
	}
}
