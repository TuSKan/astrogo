//go:build network

package dust_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness/dataset/dust"
	"github.com/TuSKan/astrogo/time"
)

// The real service, checked against two numbers that are properties of the
// Galaxy rather than of this repository.
//
// Interstellar dust is concentrated in the disc, so 100 micron emission toward
// the Galactic centre is enormous and toward the pole is nearly nothing. Four
// orders of magnitude separate them. A parser that returned the reddening
// column instead — the trap this response invites, since every result block
// uses the same element names — would give values of plausible size in the
// wrong quantity, and only the ratio between two very different sightlines
// makes that visible.
func TestFetchAgainstTheRealService(t *testing.T) {
	testutil.RequireReachable(t, "irsa.ipac.caltech.edu:443")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	m, err := dust.Fetch(ctx, nil,
		dust.Direction{L: angle.Deg(0), B: angle.Deg(0)},  // Galactic centre
		dust.Direction{L: angle.Deg(0), B: angle.Deg(90)}, // north Galactic pole
	)
	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	centre, err := m.IntensityAt(angle.Deg(0), angle.Deg(0))
	if err != nil {
		t.Fatalf("centre: %v", err)
	}

	pole, err := m.IntensityAt(angle.Deg(0), angle.Deg(90))
	if err != nil {
		t.Fatalf("pole: %v", err)
	}

	t.Logf("100 micron: centre %.1f MJy/sr, pole %.3f MJy/sr, ratio %.0f",
		centre, pole, centre/pole)

	// The pole is the cleanest sky there is: under a few MJy/sr.
	if pole <= 0 || pole > 5 {
		t.Errorf("pole is %.3f MJy/sr, want under 5", pole)
	}

	// The centre is thousands.
	if centre < 1000 {
		t.Errorf("centre is %.1f MJy/sr, want over 1000", centre)
	}

	// And the contrast is what proves the right column was read: reddening in
	// magnitudes would not span four orders of magnitude between these two.
	if centre/pole < 1000 {
		t.Errorf("centre/pole is %.0f, want over 1000 — the wrong column may have been read",
			centre/pole)
	}

	// A repeat costs nothing: the cell is already held.
	before := m.Len()
	if _, err := dust.Fetch(ctx, m,
		dust.Direction{L: angle.Deg(0), B: angle.Deg(90)}); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if m.Len() != before {
		t.Errorf("a repeated direction added a cell: %d -> %d", before, m.Len())
	}
}
