package jpl_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
)

func TestBodyMapping(t *testing.T) {
	tests := []struct {
		id   body.ID
		want int
	}{
		{body.Sun, 10},
		{body.Moon, 301},
		{body.Earth, 399},
		{body.Mars, 4},
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

	_, ok := jpl.BodyIDToNAIF[body.ID(255)]
	if ok {
		t.Error("Expected error for unknown body ID")
	}
}

func TestTimeConv(t *testing.T) {
	l := &jpl.LSK{
		DeltaAt: []jpl.LeapData{
			{JD: 2441317.5, N: 10}, // 1972-JAN-1
			{JD: 2441499.5, N: 11}, // 1972-JUL-1
		},
	}

	// 2023-JAN-1
	tm := time.FromJD(2459945.5, time.UTC)
	tdb := jpl.UTCToTDB(tm, l)

	// Pre-calculated approx: UTC + 11s + 32.184s = UTC + 43.184s
	// 2459945.5 + 43.184/86400 = 2459945.5005
	if tdb < 2459945.5 {
		t.Errorf("TDB %f should be > UTC %f", tdb, 2459945.5)
	}

	et := jpl.TDBToET(tdb)
	if et < 0 {
		t.Errorf("ET %f for 2023 should be > 0", et)
	}
}

func TestCheby(t *testing.T) {
	// Simple constant polynomial
	coeffs := []float64{10.0}
	p, v := jpl.EvalChebyshev(coeffs, 0.5, 100.0, true)
	if p != 10.0 || v != 0.0 {
		t.Errorf("Constant Cheby: p=%f v=%f, want 10.0, 0.0", p, v)
	}

	// Line p = tau
	coeffs = []float64{0.0, 1.0}
	p, v = jpl.EvalChebyshev(coeffs, 0.5, 100.0, true)
	if math.Abs(p-0.5) > 1e-12 || math.Abs(v-0.01) > 1e-12 {
		t.Errorf("Linear Cheby: p=%f v=%f, want 0.5, 0.01", p, v)
	}
}

func TestJPLUnitsAreAUAndAUPerDay(t *testing.T) {
	p, _ := jpl.New("de440s", "data")
	defer p.Close()

	state, _ := p.State(body.Sun, time.Now())
	dist := state.Pos.Norm()
	if dist < 0.9 || dist > 1.1 {
		t.Errorf("Sun distance %f AU seems wrong for AU units", dist)
	}
}

func TestJPLUnsupportedBody(t *testing.T) {
	p, _ := jpl.New("de440s", "data")
	defer p.Close()

	_, err := p.State(body.ID(255), time.Now())
	if err == nil {
		t.Error("Expected error for unsupported body")
	}
}

func TestJPLOutOfCoverageEpoch(t *testing.T) {
	p, _ := jpl.New("de440s", "data")
	defer p.Close()

	// Year 5000
	tm := time.FromJD(3545000.0, time.UTC)
	_, err := p.State(body.Sun, tm)
	if err == nil {
		t.Error("Expected error for out-of-coverage epoch")
	}
}

func TestJPLDeterministicRepeatedCalls(t *testing.T) {
	p, _ := jpl.New("de440s", "data")
	defer p.Close()

	tm := time.Now()
	s1, _ := p.State(body.Sun, tm)
	s2, _ := p.State(body.Sun, tm)

	if s1.Pos.X != s2.Pos.X || s1.Pos.Y != s2.Pos.Y || s1.Pos.Z != s2.Pos.Z {
		t.Error("Re-evaluating at same epoch produced different results")
	}
}
