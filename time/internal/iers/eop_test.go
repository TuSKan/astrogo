package iers

import (
	"strings"
	"sync"
	"testing"
)

// resetForTest is Reset, named for this file's own cleanup call sites.
// Deliberately not RegisterModel(ZeroModel{}): that call is now
// authoritative (see RegisterModel's doc comment), so using it as generic
// test cleanup would leave explicitModel set and silently no-op every
// later test's own EnsureLoaded calls.
func resetForTest() { Reset() }

func TestGetModelDefaultsToZeroModel(t *testing.T) {
	t.Cleanup(resetForTest)

	RegisterModel(ZeroModel{})

	if _, ok := GetModel().(ZeroModel); !ok {
		t.Errorf("expected ZeroModel by default, got %T", GetModel())
	}

	if _, _, ok := Coverage(); ok {
		t.Error("expected Coverage ok=false for ZeroModel")
	}
}

func TestRegisterModelConcurrent(t *testing.T) {
	t.Cleanup(resetForTest)

	var wg sync.WaitGroup

	for range 20 {
		wg.Go(func() { RegisterModel(ZeroModel{}) })
		wg.Go(func() { _ = GetModel() })
	}

	wg.Wait()
}

// TestRegisterModelMarksExplicit verifies RegisterModel's new contract
// directly: a caller's explicit registration is observable via
// modelIsExplicit/EOPSource, and resetForTest — NOT another RegisterModel
// call — is what clears it back out for the next test.
func TestRegisterModelMarksExplicit(t *testing.T) {
	t.Cleanup(resetForTest)

	if modelIsExplicit() {
		t.Fatal("modelIsExplicit() = true before any RegisterModel call")
	}

	if got := EOPSource(); got != SourceZero {
		t.Errorf("EOPSource() = %q before any registration, want %q", got, SourceZero)
	}

	RegisterModel(ZeroModel{})

	if !modelIsExplicit() {
		t.Error("modelIsExplicit() = false after RegisterModel")
	}

	if got := EOPSource(); got != SourceExplicit {
		t.Errorf("EOPSource() = %q after RegisterModel, want %q", got, SourceExplicit)
	}
}

// TestRegisterModelInternalNeverOverridesExplicit verifies the other half
// of the same contract: once a caller has registered a model explicitly,
// registerModelInternal — the lazy loader's own opportunistic path — must
// not replace it, even with genuinely fresher data.
func TestRegisterModelInternalNeverOverridesExplicit(t *testing.T) {
	t.Cleanup(resetForTest)

	RegisterModel(ZeroModel{})

	table, err := ParseFinals2000A(strings.NewReader(sampleFinals2000A))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	registerModelInternal(table, SourceNetwork)

	if _, ok := GetModel().(ZeroModel); !ok {
		t.Errorf("registerModelInternal replaced an explicit model; GetModel() = %T, want ZeroModel", GetModel())
	}

	if got := EOPSource(); got != SourceExplicit {
		t.Errorf("EOPSource() = %q after a blocked internal registration, want %q (unchanged)", got, SourceExplicit)
	}
}
