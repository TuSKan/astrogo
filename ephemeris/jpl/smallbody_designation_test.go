//go:build network

package jpl_test

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
)

// A small-body provider that loaded nothing used to be indistinguishable
// from one that worked.
//
// spk.CacheAPI returns an empty slice rather than an error when Horizons
// matches nothing, so NewProvider returned a provider with a nil error for
// "Ceres", for "101955" and for an invented string alike. SupportedBodies
// then listed the eleven bodies of the planetary base kernel — which is
// exactly what a working provider looks like from the outside, and the only
// way to notice was to ask for a state and get ErrNoSegment much later, in
// code far from the mistake.
//
// Counting readers is not sufficient either: "1;" and "4;" both return a
// non-empty kernel carrying no small-body segment, so the check is on the
// body set rather than on the file.
func TestSmallBodyDesignationFailuresAreLoud(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	start := atime.Date(2026, 1, 1, 0, 0, 0, 0, atime.LocationUTC)

	for _, des := range []string{
		"Ceres",     // a name: Horizons matches small bodies by number
		"bogus-xyz", // nothing at all
		"1;",        // valid syntax, kernel with no small-body segment
	} {
		t.Run(des, func(t *testing.T) {
			_, err := jpl.NewProvider(context.Background(), core.SmallBody, des,
				jpl.WithTimeInterval(start.AddDays(-5), start.AddDays(5)))

			if !errors.Is(err, jpl.ErrNoSmallBodyKernel) {
				t.Fatalf("NewProvider(%q) error = %v, want ErrNoSmallBodyKernel", des, err)
			}
		})
	}
}

// And the designations that do work still do.
func TestSmallBodyDesignationsThatResolve(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	start := atime.Date(2026, 1, 1, 0, 0, 0, 0, atime.LocationUTC)

	for _, tc := range []struct {
		des  string
		want core.ID
	}{
		{"433", 433},    // Eros, by number
		{"433;", 433},   // the same, in Horizons' own designation syntax
		{"3200;", 3200}, // Phaethon
	} {
		t.Run(tc.des, func(t *testing.T) {
			p, err := jpl.NewProvider(context.Background(), core.SmallBody, tc.des,
				jpl.WithTimeInterval(start.AddDays(-5), start.AddDays(5)))
			if err != nil {
				t.Fatalf("NewProvider(%q): %v", tc.des, err)
			}

			t.Cleanup(func() { _ = p.Close() })

			if _, err := p.State(tc.want, start); err != nil {
				t.Errorf("State(%d) after loading %q: %v", tc.want, tc.des, err)
			}
		})
	}
}
