package skybrightness

import (
	"math"
	"testing"
)

// A NaN wavelength fails both range checks and used to index one past the
// last pivot; a non-finite wavelength is not from the table.
func TestInterpolatePivotsRefusesNonFiniteWavelength(t *testing.T) {
	t.Parallel()

	e := NewExtragalacticBackground()

	for _, lambda := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		value, extrapolated := interpolatePivots(e.spectral, lambda)
		if !extrapolated {
			t.Errorf("λ=%v not from the table must be flagged", lambda)
		}

		if !math.IsNaN(value) {
			t.Errorf("λ=%v = %v, want NaN", lambda, value)
		}
	}
}
