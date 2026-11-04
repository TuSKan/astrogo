package coord_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// nonFinite are the inputs that converting to an int turns into an index:
// MinInt64 on amd64, 0 on arm64.
var nonFinite = []float64{math.NaN(), math.Inf(1), math.Inf(-1)}

// A non-finite azimuth names no direction. CompassDirection used to call it
// "N", since converting NaN to an int lands on the first label on amd64 and on
// arm64 alike (#979), and plan's VisibleTonight reports that label as a
// target's direction.
func TestCompassDirectionOfANonFiniteAzimuthIsEmpty(t *testing.T) {
	t.Parallel()

	for _, r := range nonFinite {
		if got := coord.CompassDirection(angle.Rad(r)); got != "" {
			t.Errorf("CompassDirection(%v) = %q, want \"\"", r, got)
		}

		if got := coord.NewAltAz(angle.Deg(30), angle.Rad(r)).Compass(); got != "" {
			t.Errorf("AltAz with azimuth %v: Compass() = %q, want \"\"", r, got)
		}
	}

	// A finite azimuth still gets its label, so the guard is not refusing
	// everything.
	if got := coord.CompassDirection(angle.Deg(90)); got != "E" {
		t.Errorf("CompassDirection(90°) = %q, want E", got)
	}
}

// SmallCircle refuses a non-finite radius rather than return points that are
// NaN, which NewGeodetic and FromECEF both refuse to build (#979).
func TestSmallCircleRefusesANonFiniteRadius(t *testing.T) {
	t.Parallel()

	center, err := coord.NewGeodetic(angle.Deg(10), angle.Deg(20), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	for _, r := range nonFinite {
		points, err := coord.SmallCircle(center, angle.Rad(r), 8)
		if !errors.Is(err, coord.ErrNotFinite) || points != nil {
			t.Errorf("SmallCircle(radius %v) = %d points, %v; want no points and ErrNotFinite", r, len(points), err)
		}
	}

	// The same center with a finite radius still gives every point.
	points, err := coord.SmallCircle(center, angle.Deg(18), 8)
	if err != nil || len(points) != 8 {
		t.Fatalf("SmallCircle(radius 18°) = %d points, %v; want 8 and no error", len(points), err)
	}
}
