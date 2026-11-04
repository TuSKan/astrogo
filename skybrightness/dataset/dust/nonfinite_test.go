package dust

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// nonFinite are the inputs cellOf would convert to an int key.
var nonFinite = []float64{math.NaN(), math.Inf(1), math.Inf(-1)}

// A non-finite direction has no cell. cellOf used to make it one anyway, a key
// that depends on the platform: NaN converts to MinInt64 on amd64 and to 0 on
// arm64, where (NaN, NaN) is {0, 0}, the galactic center's own key (#982).
//
// Each case seeds the very key the lookup would read, computed by cellOf on the
// platform running the test, so without the guard the lookup finds a value and
// the test fails wherever it runs. Seeding fixed keys passed on amd64 without
// the guard, because (NaN, 0) maps to {MinInt64, 0}, a key nobody seeded.
func TestIntensityAtRefusesANonFiniteDirection(t *testing.T) {
	t.Parallel()

	for _, x := range nonFinite {
		for _, d := range []Direction{
			{L: angle.Deg(x), B: angle.Zero()},
			{L: angle.Zero(), B: angle.Deg(x)},
			{L: angle.Deg(x), B: angle.Deg(x)},
		} {
			m := NewMap()
			m.values[cellOf(d.L, d.B)] = 1

			v, err := m.IntensityAt(d.L, d.B)
			if !errors.Is(err, ErrNoCoverage) {
				t.Errorf("IntensityAt(l=%v, b=%v) = %v, %v; want ErrNoCoverage",
					d.L.Degrees(), d.B.Degrees(), v, err)
			}
		}
	}

	// The galactic center itself still answers.
	m := NewMap()
	m.values[cell{l: 0, b: 0}] = 2

	if v, err := m.IntensityAt(angle.Zero(), angle.Zero()); err != nil || v != 2 {
		t.Errorf("IntensityAt(0, 0) = %v, %v; want 2 and no error", v, err)
	}
}

// Fetch refuses a non-finite direction before it reads the cache or asks the
// service. Offline, with an empty cache, a direction that got as far as a
// query would fail as offline instead, so only the refusal passes.
func TestFetchRefusesANonFiniteDirection(t *testing.T) {
	t.Cleanup(remote.Capture(remote.IRSADust).Restore)
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	remote.SetOffline(true)

	for _, x := range nonFinite {
		m, err := Fetch(context.Background(), nil,
			Direction{L: angle.Deg(10), B: angle.Deg(20)},
			Direction{L: angle.Deg(x), B: angle.Deg(20)})
		if !errors.Is(err, ErrNoCoverage) || m != nil {
			t.Errorf("Fetch with l=%v = %v, %v; want no map and ErrNoCoverage", x, m, err)
		}
	}
}
