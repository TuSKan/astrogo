package coord

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestAStarThatCannotBeMovedIsLeftWhereItIs: a star SOFA's Pmsafe cannot
// move, here one with a parallax of 1e300″, is left at its own position and
// epoch when a transform moves it to J2000.0, since Astrometric and the FK
// conversions have no error to report the failure through.
func TestAStarThatCannotBeMovedIsLeftWhereItIs(t *testing.T) {
	t.Parallel()

	j2016 := time.FromJD(2457389.0, time.TT)
	star := NewICRSWithKinematics(angle.Deg(150), angle.Deg(20),
		angle.Arcsec(0.1), angle.Arcsec(0.1), angle.Arcsec(1e300), unit.KmPerSec(0), j2016)

	if _, err := PropagateEpoch(star, time.J2000()); !errors.Is(err, ErrPropagationFailed) {
		t.Fatalf("test setup: PropagateEpoch error %v, want ErrPropagationFailed", err)
	}

	got := star.atJ2000()
	if got.RA() != star.RA() || got.Dec() != star.Dec() || !got.Epoch().Equal(j2016) {
		t.Errorf("atJ2000 moved the star to (%v, %v) at %v; want it left at (%v, %v) at J2016.0",
			got.RA(), got.Dec(), got.Epoch(), star.RA(), star.Dec())
	}
}
