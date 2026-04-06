package jpl_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/time"
)

func TestBodyMapping(t *testing.T) {
	tests := []struct {
		id   ephemeris.ID
		want int
	}{
		{ephemeris.Sun, 10},
		{ephemeris.Moon, 301},
		{ephemeris.Earth, 399},
		{ephemeris.Mars, 4},
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

	_, ok := jpl.BodyIDToNAIF[ephemeris.ID(255)]
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
	tdb := lsk.UTCToTDB(tm, l)

	// Pre-calculated approx: UTC + 11s + 32.184s = UTC + 43.184s
	// 2459945.5 + 43.184/86400 = 2459945.5005
	if tdb < 2459945.5 {
		t.Errorf("TDB %f should be > UTC %f", tdb, 2459945.5)
	}

	et := lsk.TDBToET(tdb)
	if et < 0 {
		t.Errorf("ET %f for 2023 should be > 0", et)
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
	p, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de440s"), jpl.WithDataDir("data"))
	if err != nil {
		t.Fatalf("failed to initialize provider: %v", err)
	}
	defer p.Close()

	state, err := p.State(ephemeris.Sun, time.NowUTC())
	if err != nil {
		t.Fatalf("failed to evaluate Sun state: %v", err)
	}
	dist := state.Pos.Norm()
	if dist < 0.9 || dist > 1.1 {
		t.Errorf("Sun distance %f AU seems wrong for AU units", dist)
	}
}

func TestJPLUnsupportedBody(t *testing.T) {
	p, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de440s"), jpl.WithDataDir("data"))
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer p.Close()

	_, err = p.State(ephemeris.ID(255), time.NowUTC())
	if err == nil {
		t.Error("Expected error for unsupported body")
	}
}

func TestJPLOutOfCoverageEpoch(t *testing.T) {
	p, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de440s"), jpl.WithDataDir("data"))
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer p.Close()

	// Year 5000
	tm := time.FromJD(3545000.0, time.UTC)
	_, err = p.State(ephemeris.Sun, tm)
	if err == nil {
		t.Error("Expected error for out-of-coverage epoch")
	}
}

func TestJPLDeterministicRepeatedCalls(t *testing.T) {
	p, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de440s"), jpl.WithDataDir("data"))
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
	defer p.Close()

	tm := time.NowUTC()
	s1, err := p.State(ephemeris.Sun, tm)
	if err != nil {
		t.Fatalf("s1 failed: %v", err)
	}
	s2, err := p.State(ephemeris.Sun, tm)
	if err != nil {
		t.Fatalf("s2 failed: %v", err)
	}

	if s1.Pos.X != s2.Pos.X || s1.Pos.Y != s2.Pos.Y || s1.Pos.Z != s2.Pos.Z {
		t.Error("Re-evaluating at same epoch produced different results")
	}
}

func TestSourceSelection(t *testing.T) {
	t.Run("Planets", func(t *testing.T) {
		p, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de440s"), jpl.WithDataDir("data"))
		if err != nil {
			t.Fatalf("Planets source failed: %v", err)
		}
		if p == nil {
			t.Fatal("Planets source returned nil provider")
		}
		p.Close()
	})

	t.Run("Unsupported", func(t *testing.T) {
		unsupported := []jpl.Source{jpl.Satellites, jpl.Stations}
		for _, s := range unsupported {
			_, err := jpl.NewProvider(jpl.WithSource(s))
			if err == nil {
				t.Errorf("Expected error for unsupported source %v", s)
			}
		}
	})

	t.Run("Unknown", func(t *testing.T) {
		_, err := jpl.NewProvider(jpl.WithSource(jpl.Source("unknown")))
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
		jpl.WithSource(jpl.SmallBody),
		jpl.WithKernel("433"),
		jpl.WithTimeInterval(start, end),
		jpl.WithDataDir("data"),
	)
	if err != nil {
		t.Fatalf("Failed to create smallbody provider: %v", err)
	}
	defer p.Close()

	t.Logf("Loaded %d kernels", len(p.SupportedBodies()))

	// Check if Eros is in supported bodies
	bodies := p.SupportedBodies()
	found := false
	for _, b := range bodies {
		if b == ephemeris.ID(433) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Eros (433) not found in supported bodies: %v", bodies)
	}

	// Get state
	state, err := p.State(ephemeris.ID(433), start)
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
		jpl.WithSource(jpl.SmallBody),
		jpl.WithKernel("Apophis"), // "Apophis" is ambiguous in Horizons web, but let's see API
		jpl.WithTimeInterval(start, end),
		jpl.WithDataDir("data"),
	)
	if err != nil {
		// If it's ambiguous, spk.CacheAPI should have handled it or returned error
		t.Fatalf("Failed to create provider for Apophis: %v", err)
	}
	defer p.Close()

	bodies := p.SupportedBodies()
	if len(bodies) == 0 {
		t.Error("Expected at least one body loaded for Apophis")
	}
	t.Logf("Loaded bodies for 'Apophis': %v", bodies)
}
