package jpl

import (
	"errors"
	"strings"
	"testing"
)

// TestLoadSmallBodyKernelsRefusesAnEmptyMatch covers the defect this rule
// exists for: spk.CacheAPI returns an empty slice rather than an error when
// Horizons matches nothing, so the loop below it did nothing and the caller
// got a provider back with a nil error and only its planetary base.
func TestLoadSmallBodyKernelsRefusesAnEmptyMatch(t *testing.T) {
	p := &Provider{kernel: "Ceres"}

	err := p.loadSmallBodyKernels(nil)
	if !errors.Is(err, ErrNoSmallBodyKernel) {
		t.Fatalf("loadSmallBodyKernels(nil) = %v, want ErrNoSmallBodyKernel", err)
	}
}

// TestLoadSmallBodyKernelsNamesTheDesignation guards the part of the message
// that does the work. "Ceres" fails while "1;" succeeds, and a reader who is
// not told which designation was tried has no way to find that out.
func TestLoadSmallBodyKernelsNamesTheDesignation(t *testing.T) {
	p := &Provider{kernel: "totally-not-a-body-xyz"}

	err := p.loadSmallBodyKernels(nil)
	if err == nil {
		t.Fatal("expected an error")
	}

	if !strings.Contains(err.Error(), "totally-not-a-body-xyz") {
		t.Errorf("error does not name the designation tried: %v", err)
	}
}
