package jpl_test

import (
	"context"
	"errors"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/time"
)

func TestBodyMapping(t *testing.T) {
	tests := []struct {
		id   core.ID
		want int
	}{
		{core.Sun, 10},
		{core.Moon, 301},
		{core.Earth, 399},
		{core.Mars, 4},
	}

	for _, tt := range tests {
		got, ok := jpl.BodyIDToNAIF[tt.id]
		if !ok {
			t.Errorf("BodyIDToNAIF[%v] not found", tt.id)
			continue
		}

		if got != tt.want {
			t.Errorf("BodyIDToNAIF[%v] = %v, want %v", tt.id, got, tt.want)
		}
	}

	_, ok := jpl.BodyIDToNAIF[core.ID(255)]
	if ok {
		t.Error("Expected error for unknown body ID")
	}
}

func TestTimeConv(t *testing.T) {
	l := &lsk.Reader{
		DeltaAt: []lsk.LeapData{
			{JD: 2441317.5, N: 10}, // 1972-JAN-1
			{JD: 2441499.5, N: 11}, // 1972-JUL-1
		},
	}

	// 2023-JAN-1
	tm := time.FromJD(2459945.5, time.UTC)
	// ET seconds past J2000, the argument SPK segments are indexed by. The
	// Julian-date pair this used to go through is gone: summing it quantised
	// the result to about 40 microseconds (#150).
	et := lsk.UTCToET(tm, l)
	if et < 0 {
		t.Errorf("ET %f for 2023 should be > 0", et)
	}

	// TDB runs ahead of UTC by the leap seconds plus 32.184 s, so ET must
	// exceed the same instant counted in plain UTC seconds.
	d1, d2 := tm.UTC().JDParts()
	secUTC := ((d1 - 2451545.0) + d2) * 86400.0

	// 37 leap seconds at 2023 plus 32.184 s, so 69.184. The comment this
	// replaces said "UTC + 11s + 32.184s = 43.184", which has not been right
	// since 1979 — the assertion beside it only checked TDB > UTC, so a
	// 26-second error in the stated expectation went unnoticed.
	if offset := et - secUTC; offset < 69.0 || offset > 69.5 {
		t.Errorf("TDB-UTC at 2023-01-01 is %.4f s, expected about 69.184 "+
			"(37 leap seconds + 32.184)", offset)
	}
}

func TestCheby(t *testing.T) {
	// Simple constant polynomial
	coeffs := []float64{10.0}

	p, v := spk.EvalChebyshev(coeffs, 0.5, 100.0, true)
	if p != 10.0 || v != 0.0 {
		t.Errorf("Constant Cheby: p=%f v=%f, want 10.0, 0.0", p, v)
	}

	// Line p = tau
	coeffs = []float64{0.0, 1.0}

	p, v = spk.EvalChebyshev(coeffs, 0.5, 100.0, true)
	if math.Abs(p-0.5) > 1e-12 || math.Abs(v-0.01) > 1e-12 {
		t.Errorf("Linear Cheby: p=%f v=%f, want 0.5, 0.01", p, v)
	}
}

func TestJPLUnitsAreAUAndAUPerDay(t *testing.T) {
	p := mustPlanetProvider(t)

	t.Cleanup(func() {
		err := p.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	state, err := p.State(core.Sun, fixedEpoch())
	if err != nil {
		t.Fatalf("failed to evaluate Sun state: %v", err)
	}

	dist := state.Pos.Norm()
	if dist < 0.9 || dist > 1.1 {
		t.Errorf("Sun distance %f AU seems wrong for AU units", dist)
	}
}

func TestJPLUnsupportedBody(t *testing.T) {
	p := mustPlanetProvider(t)

	t.Cleanup(func() {
		err := p.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	_, err := p.State(core.ID(255), fixedEpoch())
	if err == nil {
		t.Error("Expected error for unsupported body")
	}
}

func TestJPLOutOfCoverageEpoch(t *testing.T) {
	p := mustPlanetProvider(t)

	t.Cleanup(func() {
		err := p.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	// Year 5000
	tm := time.FromJD(3545000.0, time.UTC)

	_, err := p.State(core.Sun, tm)
	if err == nil {
		t.Error("Expected error for out-of-coverage epoch")
	}
}

func TestJPLDeterministicRepeatedCalls(t *testing.T) {
	p := mustPlanetProvider(t)

	t.Cleanup(func() {
		err := p.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	tm := fixedEpoch()

	s1, err := p.State(core.Sun, tm)
	if err != nil {
		t.Fatalf("s1 failed: %v", err)
	}

	s2, err := p.State(core.Sun, tm)
	if err != nil {
		t.Fatalf("s2 failed: %v", err)
	}

	if s1.Pos.X != s2.Pos.X || s1.Pos.Y != s2.Pos.Y || s1.Pos.Z != s2.Pos.Z {
		t.Error("Re-evaluating at same epoch produced different results")
	}
}

func TestSourceSelection(t *testing.T) {
	t.Run("Planets", func(t *testing.T) {
		p := mustPlanetProvider(t)

		if p == nil {
			t.Fatal("Planets source returned nil provider")
		}

		if err := p.Close(); err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	t.Run("Unsupported", func(t *testing.T) {
		unsupported := []core.Source{core.Satellites, core.Stations}
		for _, s := range unsupported {
			_, err := jpl.NewProvider(context.Background(), s, "")
			if err == nil {
				t.Errorf("Expected error for unsupported source %v", s)
			}
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		_, err := jpl.NewProvider(context.Background(), core.Source("unknown"), "")
		if err == nil {
			t.Error("Expected error for unknown source")
		}
	})
}

func TestSmallBodyEros(t *testing.T) {
	// Eros (433)
	// We use a specific time where it has coverage
	start := time.FromJD(2460000.5, time.UTC) // 2023-FEB-25
	end := time.FromJD(2460001.5, time.UTC)   // 2023-FEB-26

	p, err := jpl.NewProvider(
		context.Background(),
		core.SmallBody,
		"433",
		jpl.WithTimeInterval(start, end),
	)
	if err != nil {
		if errors.Is(err, spk.ErrHorizonsEmptyKernel) {
			// Live-confirmed this session (raw HTTP response decoded and
			// inspected byte-for-byte, independent of any astrogo code):
			// Horizons' own server can generate a syntactically valid but
			// functionally empty SPK (a DAF file record claiming a summary
			// record exists, with that record and everything after the
			// comment area all zero bytes) for this exact request — a real
			// external anomaly, not an astrogo bug. CacheAPI now detects
			// and rejects this rather than silently caching a broken
			// kernel (see ErrHorizonsEmptyKernel's own doc comment); skip
			// rather than fail this untagged, live-network test on it.
			t.Skipf("Horizons returned an empty/unusable SPK for Eros: %v (known external anomaly, not astrogo)", err)
		}

		t.Fatalf("Failed to create smallbody provider: %v", err)
	}

	t.Cleanup(func() {
		err := p.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	t.Logf("Loaded %d kernels", len(p.SupportedBodies()))

	// Check if Eros is in supported bodies
	bodies := p.SupportedBodies()
	// SupportedBodies reports NAIF's own small-body identifier now, not
	// the bare number, which used to collide with the planets.
	found := slices.Contains(bodies, core.SmallBodyID(433))

	if !found {
		t.Errorf("Eros (433) not found in supported bodies: %v", bodies)
	}

	// Get state
	// A bare core.ID(433) still resolves, so this stays as it was.
	state, err := p.State(core.ID(433), start)
	if err != nil {
		t.Fatalf("Failed to get state for Eros: %v", err)
	}

	t.Logf("Eros State: Pos=%v, Vel=%v", state.Pos, state.Vel)

	// Verify position is reasonable (range for Eros is ~1.1 to 1.8 AU from Sun)
	// Geocentric distance for Eros varies.
	dist := state.Pos.Norm()
	if dist < 0.1 || dist > 5.0 {
		t.Errorf("Suspicious geocentric distance for Eros: %f AU", dist)
	}

	t.Logf("Eros State at %v: Pos=%v Dist=%v AU", start, state.Pos, dist)
}

func TestSmallBodyMultiMatch(t *testing.T) {
	// Querying "Apophis" matches multiple entries in SBDB,
	// but here we are passing a "kernel" command to Horizons.
	// Horizons might return a list if the command is ambiguous.
	start := time.FromJD(2460000.5, time.UTC)
	end := time.FromJD(2460001.5, time.UTC)

	p, err := jpl.NewProvider(
		context.Background(),
		core.SmallBody,
		"Apophis", // "Apophis" is ambiguous in Horizons web, but let's see API
		jpl.WithTimeInterval(start, end),
	)
	if err != nil {
		if errors.Is(err, spk.ErrHorizonsEmptyKernel) {
			// See TestSmallBodyEros's identical skip for the full
			// explanation: a live-confirmed Horizons server anomaly, not
			// an astrogo bug.
			t.Skipf("Horizons returned an empty/unusable SPK for Apophis: %v (known external anomaly, not astrogo)", err)
		}

		// If it's ambiguous, spk.CacheAPI should have handled it or returned error
		t.Fatalf("Failed to create provider for Apophis: %v", err)
	}

	t.Cleanup(func() {
		err := p.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	bodies := p.SupportedBodies()
	if len(bodies) == 0 {
		t.Error("Expected at least one body loaded for Apophis")
	}

	t.Logf("Loaded bodies for 'Apophis': %v", bodies)
}

// TestProvider_ConcurrentAddKernelAndState is a regression test for R26:
// AddKernel mutated Kernels/Index/ByTarget/ByTargetCoverage with no locking,
// so a caller adding a kernel after construction while other goroutines were
// concurrently reading (State/FindSegment/SupportedBodies) would race. This
// can't detect the race directly without cgo's -race detector (unavailable
// in this sandbox), but it does exercise the exact interleaving under real
// CI, and confirms nothing panics or deadlocks under contention.
func TestProvider_ConcurrentAddKernelAndState(t *testing.T) {
	p := mustPlanetProvider(t)

	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	tm := fixedEpoch()

	var wg sync.WaitGroup

	// Readers: hammer State/FindSegment/SupportedBodies concurrently.
	for range 8 {
		wg.Go(func() {
			for range 50 {
				if _, err := p.State(core.Sun, tm); err != nil {
					t.Errorf("State: %v", err)
				}

				p.SupportedBodies()
			}
		})
	}

	// Writer: re-open the already-cached planetary kernel and add it again,
	// concurrently with the readers above.
	wg.Go(func() {
		for range 4 {
			reader, err := spk.CacheDownload(context.Background(), "planets/de440s.bsp")
			if err != nil {
				t.Errorf("CacheDownload: %v", err)
				return
			}

			if err := p.AddKernel(reader); err != nil {
				t.Errorf("AddKernel: %v", err)
			}
		}
	})

	wg.Wait()
}
