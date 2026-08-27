package airglow_test

import (
	"bytes"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/file"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
)

// A cached skytable is served without a network.
//
// # Why this is the test that says something
//
// Because the claim being made is about traffic, and traffic is invisible
// from inside a passing test. Cutting the network is the only assertion that
// distinguishes "the cache was written" from "the cache is actually read":
// before this cache existed, every call to Fetch went to Garching — three
// requests, about 3.7 seconds — however many times a caller assembled the
// same inputs, and dataset.Open's own documentation claimed a warm cache
// needed no services at all. It was the one dataset in the tier not going
// through remote.GetFile or a cache of its own, and nothing said so.
//
// SkyCalc is also a shared research service, so this is a courtesy as much as
// a speed-up: four models over one night meant twelve requests for four
// identical answers.
func TestFetchServesACachedSkytableOffline(t *testing.T) {
	// Not parallel: SetDataDir and SetOffline are both process-global, and a
	// sibling running beside this one would be pointed at its cache or cut
	// off from a network it needed.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	spec := airglow.Spec{
		Observatory:  airglow.Paranal,
		SolarFluxSFU: 100,
		MinNM:        499,
		MaxNM:        504,
	}

	bucket, key, err := airglow.CacheLocation(t.Context(), spec)
	if err != nil {
		t.Fatalf("CacheLocation: %v", err)
	}

	lam := []float64{500, 501, 502, 503}
	ael := []float64{1e3, 2e3, 3e3, 4e3}
	arc := []float64{5e2, 5e2, 5e2, 5e2}

	seeded := bintableFITS(
		[]string{"lam", "flux_ael", "flux_arc", "trans"},
		[][]float64{lam, ael, arc, unity(len(lam))},
	)

	if err := file.Save(t.Context(), bucket, key, bytes.NewReader(seeded)); err != nil {
		t.Fatalf("seeding the cache: %v", err)
	}

	remote.SetOffline(true)
	t.Cleanup(func() { remote.SetOffline(false) })

	got, err := airglow.Fetch(t.Context(), spec)
	if err != nil {
		t.Fatalf("Fetch with a warm cache and no network: %v", err)
	}

	if len(got.LambdaNM) != len(lam) {
		t.Fatalf("got %d samples, want %d — this is not the seeded table",
			len(got.LambdaNM), len(lam))
	}

	for i, nm := range lam {
		if got.LambdaNM[i] != nm {
			t.Errorf("sample %d is at %g nm, want %g", i, got.LambdaNM[i], nm)
		}
	}

	// And it came back as a spectrum rather than as the raw photon counts,
	// so the cached path runs the same conversion the fetched one does.
	for i, r := range got.Radiance {
		if r <= 0 {
			t.Errorf("sample %d has radiance %g; every seeded row carries flux", i, r)
		}
	}
}

// A different request does not collide with a cached one.
//
// The key is a hash of the whole outgoing request, so two Specs that differ
// anywhere must land on different objects. Returning one caller's spectrum to
// another would be worse than not caching: a solar flux of 100 and one of 250
// are different skies, and nothing downstream could tell which it had.
func TestCacheKeyDistinguishesRequests(t *testing.T) {
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

	base := airglow.Spec{Observatory: airglow.Paranal, SolarFluxSFU: 100, MinNM: 400, MaxNM: 900}

	_, first, err := airglow.CacheLocation(t.Context(), base)
	if err != nil {
		t.Fatalf("CacheLocation: %v", err)
	}

	for _, c := range []struct {
		name string
		spec airglow.Spec
	}{
		{"solar flux", airglow.Spec{
			Observatory: airglow.Paranal, SolarFluxSFU: 250, MinNM: 400, MaxNM: 900,
		}},
		{"observatory", airglow.Spec{
			Observatory: airglow.Altitude3060, SolarFluxSFU: 100, MinNM: 400, MaxNM: 900,
		}},
		{"lower bound", airglow.Spec{
			Observatory: airglow.Paranal, SolarFluxSFU: 100, MinNM: 401, MaxNM: 900,
		}},
		{"upper bound", airglow.Spec{
			Observatory: airglow.Paranal, SolarFluxSFU: 100, MinNM: 400, MaxNM: 901,
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, key, err := airglow.CacheLocation(t.Context(), c.spec)
			if err != nil {
				t.Fatalf("CacheLocation: %v", err)
			}

			if key == first {
				t.Errorf("a spec differing in %s hashes to the same key %q, so one "+
					"caller's sky would be served to another", c.name, key)
			}
		})
	}
}
