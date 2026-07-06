package skybrightness_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// TestFloorScalarConstant verifies a scalar floor is uniform over the sky and
// equals the radiance of its SQM value.
func TestFloorScalarConstant(t *testing.T) {
	t.Parallel()

	const sqm = skybrightness.SurfaceBrightnessV(21.0)

	f := skybrightness.NewFloorSQM(sqm)

	want := sqm.Nanolamberts()

	for _, aa := range []coord.AltAz{
		coord.NewAltAz(angle.Deg(20), angle.Deg(0)),
		coord.NewAltAz(angle.Deg(45), angle.Deg(180)),
		coord.NewAltAz(angle.Deg(89), angle.Deg(270)),
	} {
		got, err := f.Radiance(aa, nil)
		if err != nil {
			t.Fatalf("Radiance: %v", err)
		}

		// Tolerance-based, not exact equality: math.Exp inside Nanolamberts()
		// can be FMA-contracted differently depending on the calling
		// context (direct call here vs. through the GridFunc closure in
		// Radiance), producing a few ULPs of difference — most visible on
		// architectures with native FMA (e.g. arm64/macOS runners).
		testutil.AssertNear(t, "scalar floor radiance", float64(got), float64(want), 1e-9)
	}
}

// TestFloorGridDirectional verifies a grid-backed floor varies with direction.
func TestFloorGridDirectional(t *testing.T) {
	t.Parallel()

	// Brighter (smaller mag) toward the north (light dome), darker toward south.
	grid := skybrightness.GridFunc(func(aa coord.AltAz) skybrightness.SurfaceBrightnessV {
		if aa.Az().Degrees() < 90 || aa.Az().Degrees() > 270 {
			return 18.0 // light dome
		}

		return 21.5 // dark direction
	})
	f := skybrightness.NewFloorGrid(grid)

	north, _ := f.Radiance(coord.NewAltAz(angle.Deg(30), angle.Deg(0)), nil)
	south, _ := f.Radiance(coord.NewAltAz(angle.Deg(30), angle.Deg(180)), nil)

	if !(north > south) {
		t.Errorf("expected brighter radiance toward the light dome (north): north=%g south=%g", north, south)
	}
}

// TestFloorFromBortleRange verifies the valid-class bounds.
func TestFloorFromBortleRange(t *testing.T) {
	t.Parallel()

	for _, bad := range []int{0, -1, 10, 100} {
		if _, err := skybrightness.FloorFromBortle(bad); !errors.Is(err, skybrightness.ErrBortleClass) {
			t.Errorf("FloorFromBortle(%d): expected ErrBortleClass, got %v", bad, err)
		}
	}

	for c := 1; c <= 9; c++ {
		if _, err := skybrightness.FloorFromBortle(c); err != nil {
			t.Errorf("FloorFromBortle(%d): unexpected error %v", c, err)
		}
	}
}

// TestFloorFromBortleMonotonic verifies higher Bortle class ⇒ brighter sky
// (larger radiance), as expected for worsening light pollution.
func TestFloorFromBortleMonotonic(t *testing.T) {
	t.Parallel()

	aa := coord.NewAltAz(angle.Deg(45), angle.Deg(0))

	prev := skybrightness.Nanolambert(-1)

	for c := 1; c <= 9; c++ {
		f, _ := skybrightness.FloorFromBortle(c)
		r, _ := f.Radiance(aa, nil)

		if r < prev {
			t.Errorf("Bortle %d radiance %g < previous %g (non-monotonic)", c, r, prev)
		}

		prev = r
	}
}

// TestBortleClass verifies the brightness → Bortle classification at the class
// midpoints and at the bright/dark extremes, and that darker skies map to lower
// class numbers.
func TestBortleClass(t *testing.T) {
	t.Parallel()

	cases := []struct {
		sb   skybrightness.SurfaceBrightnessV
		want int
	}{
		{22.5, 1}, // darker than class 1 ⇒ clamps to 1
		{21.99, 1},
		{21.6, 3},
		{18.0, 8},
		{17.0, 9}, // brighter than class 9 ⇒ clamps to 9
	}

	for _, c := range cases {
		got, name := skybrightness.BortleClass(c.sb)
		if got != c.want {
			t.Errorf("BortleClass(%.2f) = %d (%q), want %d", float64(c.sb), got, name, c.want)
		}

		if name == "" {
			t.Errorf("BortleClass(%.2f): empty name", float64(c.sb))
		}
	}

	// Darker sky ⇒ lower (better) class number.
	dark, _ := skybrightness.BortleClass(21.9)
	bright, _ := skybrightness.BortleClass(18.5)

	if dark >= bright {
		t.Errorf("expected darker sky to map to a lower Bortle class: %d vs %d", dark, bright)
	}
}

// TestFloorUninitialized verifies the zero-value Floor reports an error.
func TestFloorUninitialized(t *testing.T) {
	t.Parallel()

	var f skybrightness.Floor
	if _, err := f.Radiance(coord.NewAltAz(angle.Deg(45), angle.Deg(0)), nil); !errors.Is(err, skybrightness.ErrUninitializedFloor) {
		t.Errorf("expected ErrUninitializedFloor, got %v", err)
	}
}
