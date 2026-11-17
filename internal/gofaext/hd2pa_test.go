package gofaext_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestHd2paAgainstSOFAsOwnVector checks the wrapper against the vector SOFA
// publishes for iauHd2pa in t_sofa_c.c: hour angle 1.1, declination 1.2 and
// latitude 0.3 radians give 1.906227428001995580.
func TestHd2paAgainstSOFAsOwnVector(t *testing.T) {
	t.Parallel()

	testutil.AssertNear(t, "Hd2pa", gofaext.Hd2pa(1.1, 1.2, 0.3), 1.906227428001995580, 1e-13)
}
