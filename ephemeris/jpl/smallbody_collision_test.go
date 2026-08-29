//go:build network

package jpl_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
)

// TestLowNumberedAsteroidsLoad covers the bodies that used to be unreachable.
//
// Horizons returns a perfectly good Type 21 kernel for each of these — 72,704
// bytes, verified — whose segment target is 20000000+number. Folding that
// down to the bare number put 4 Vesta on core.ID(4), which is Mars, so
// SupportedBodies saw a duplicate and dropped it. NewProvider then reported
// ErrNoSmallBodyKernel and blamed the designation, which was never the cause.
//
// Ceres, Pallas, Juno and Vesta are four of the five largest asteroids, and
// none of them could be loaded at all.
func TestLowNumberedAsteroidsLoad(t *testing.T) {
	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")

	start := atime.FromJD(2460305.5, atime.TDB)
	stop := atime.FromJD(2460355.5, atime.TDB)

	cases := []struct {
		number      int
		designation string
		name        string
		collidesAs  core.ID
	}{
		{1, "1;", "Ceres", core.Mercury},
		{2, "2;", "Pallas", core.Venus},
		{4, "4;", "Vesta", core.Mars},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := jpl.NewProvider(context.Background(), core.SmallBody, c.designation,
				jpl.WithTimeInterval(start, stop))
			if err != nil {
				t.Fatalf("%s (%s): %v", c.name, c.designation, err)
			}

			defer func() { _ = p.Close() }()

			want := core.SmallBodyID(c.number)

			var found bool

			for _, id := range p.SupportedBodies() {
				if id == want {
					found = true
				}
			}

			if !found {
				t.Fatalf("%s not in SupportedBodies: %v", want, p.SupportedBodies())
			}

			state, err := p.State(want, start)
			if err != nil {
				t.Fatalf("State(%s): %v", want, err)
			}

			// The asteroid must not be the planet it used to collide with.
			planet, perr := p.State(c.collidesAs, start)
			if perr != nil {
				t.Fatalf("State(%s): %v", c.collidesAs, perr)
			}

			if state.Pos.Sub(planet.Pos).Norm() < 1e-6 {
				t.Errorf("%s and %s returned the same position", want, c.collidesAs)
			}

			r := state.Pos.Norm()
			t.Logf("%s: |r| = %.4f AU (%s is at %.4f AU)", c.name, r, c.collidesAs, planet.Pos.Norm())
		})
	}
}
