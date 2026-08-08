package constants_test

import (
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestPhotometric_ABZeroPoint(t *testing.T) {
	zp := constants.Photometric.ABZeroPoint

	// 3631 Jy in SI base (W/(m2 Hz)): 3631e-26.
	testutil.AssertRelNear(t, "ABZeroPoint", zp.Value, 3631e-26, 1e-15)

	if !zp.Exact {
		t.Error("ABZeroPoint.Exact = false, want true (defining value of the AB system)")
	}

	if zp.Uncertainty != 0 {
		t.Errorf("ABZeroPoint.Uncertainty = %v, want 0", zp.Uncertainty)
	}
}

func TestPhotometric_Name(t *testing.T) {
	if got := constants.Photometric.Name(); got != "photometric" {
		t.Errorf("Photometric.Name() = %q, want %q", got, "photometric")
	}
}
