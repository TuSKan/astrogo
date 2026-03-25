package catalog

import (
	"math"
	"testing"
)

// fuzzyFloatEq safely evaluates equality between geometric components.
func fuzzyFloatEq(a, b float64) bool {
	if math.IsNaN(a) && math.IsNaN(b) {
		return true
	}
	return math.Abs(a-b) < 1e-9
}

func TestCoordinateParsing(t *testing.T) {
	t.Run("Parse Declination cleanly handles exact signs", func(t *testing.T) {
		// Testing identical degree string formatting with zero offsets ensuring
		// negative polarity is preserved explicitly.
		negDec := parseDec("-00:15:30")
		expectedDegrees := -(0.0 + 15.0/60.0 + 30.0/3600.0) // correctly -0.25833333333 degrees
		expectedRadians := expectedDegrees * (math.Pi / 180.0)
		
		if !fuzzyFloatEq(negDec, expectedRadians) {
			t.Errorf("expected negative polarity radian %f, got %f", expectedRadians, negDec)
		}
	})

	t.Run("Parse Right Ascension calculates continuous conversion reliably", func(t *testing.T) {
		// Testing RA component into Radians directly.
		ra := parseRA("02:30:00") // 2.5 hours
		expectedHours := 2.5
		expectedRadians := expectedHours * (math.Pi / 12.0)
		
		if !fuzzyFloatEq(ra, expectedRadians) {
			t.Errorf("expected right ascension radian %f, got %f", expectedRadians, ra)
		}
	})
}

func TestLookup(t *testing.T) {
	// Need to invoke Lookup and ensure structural coherence
	target, err := Lookup("IC0001")
	if err != nil {
		t.Fatalf("unexpected failure tracking embedded element: %v", err)
	}
	if target.ID != "IC0001" {
		t.Fatalf("expected ID IC0001, got %s", target.ID)
	}
	
	// RA: 00:08:27.05
	// Dec: +27:43:03.6
	expectedRA := (0.0 + 8.0/60.0 + 27.05/3600.0) * (math.Pi / 12.0)
	if !fuzzyFloatEq(target.RA, expectedRA) {
		t.Errorf("expected target RA %f, got %f", expectedRA, target.RA)
	}
	
	expectedDec := (27.0 + 43.0/60.0 + 3.6/3600.0) * (math.Pi / 180.0)
	if !fuzzyFloatEq(target.Dec, expectedDec) {
		t.Errorf("expected target Dec %f, got %f", expectedDec, target.Dec)
	}
}
