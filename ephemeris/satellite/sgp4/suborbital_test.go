package sgp4_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/vector"
)

// An element set whose orbit is under the surface at its own epoch builds, and
// At reports ErrDecayed with the state it computed (#1054).
//
// This is Vallado's error 5, "sub-orbital", which the reference no longer
// raises: sgp4init carries the check commented out ("sgp4fix remove this check
// as it is unnecessary"), since the decay check in sgp4 handles it. astrogo
// follows the reference, so there is no sentinel for it, and docs/sgp4.md
// §6.2 named one, ErrSubOrbital, that never existed. This pins the behavior
// the corrected table describes.
func TestASubOrbitalElementSetBuildsAndReportsDecay(t *testing.T) {
	t.Parallel()

	el := leo()
	// 17.5 revolutions a day is a semi-major axis of about 6266 km, inside the
	// Earth's 6378 km equatorial radius, so the orbit is underground at epoch.
	el.MeanMotion = 17.5

	p, err := sgp4.New(el)
	if err != nil {
		t.Fatalf("New refused a sub-orbital element set (%v); the reference builds it", err)
	}

	pos, vel, err := p.At(0)
	if !errors.Is(err, sgp4.ErrDecayed) {
		t.Fatalf("At(0) returned %v, want ErrDecayed", err)
	}

	if pos == (vector.Vec3{}) || vel == (vector.Vec3{}) {
		t.Error("ErrDecayed came back with a zero state; it is documented as carrying the state computed")
	}
}
