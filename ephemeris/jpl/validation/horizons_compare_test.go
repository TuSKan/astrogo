//go:build network

package jpl_test

import (
	"context"
	"errors"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// horizonsCase pairs a Horizons target with the astrogo body it is compared
// against, so the NAIF id, the display name and the eph.ID travel together
// instead of being re-derived from the name by an if-chain further down.
type horizonsCase struct {
	naifID int
	name   string
	id     eph.ID
}

func horizonsCases() []horizonsCase {
	return []horizonsCase{
		{10, "Sun", eph.Sun},
		{301, "Moon", eph.Moon},
		{4, "Mars", eph.Mars},
	}
}

// loadCases fetches every comparison point in one pass.
//
// It used to be called once per top-level test, each of which fetched all
// three bodies and then discarded the two it did not want — nine Horizons
// queries to use three. Since the point of the network tag is that these
// tests talk to somebody else's server, asking for the same three vectors
// three times is not a style question but a courtesy one.
func loadCases(t *testing.T) []*StateVector {
	t.Helper()

	const (
		start = "2000-01-01 12:00 TDB"
		stop  = "2000-01-01 12:01"
	)

	out := make([]*StateVector, 0, len(horizonsCases()))

	for _, c := range horizonsCases() {
		sv, err := fetchVector(c.naifID, c.name, start, stop)
		if errors.Is(err, errHorizonsUnavailable) {
			t.Skipf("JPL Horizons is not answering with API data, skipping live comparison: %v", err)
		}

		if err != nil {
			t.Fatalf("failed to fetch %s vector: %v", c.name, err)
		}

		out = append(out, sv)
	}

	return out
}

// TestJPLStateAgainstHorizons compares astrogo's DE440 evaluation against
// Horizons' own state vectors for the same instant.
//
// Both sides are JPL ephemerides — Horizons serves DE441 — so this is a
// same-source consistency check on astrogo's SPK evaluation and time-scale
// handling, not an independent validation of the ephemeris itself. The
// tolerances are set accordingly: two evaluations of what is substantially
// the same integration should agree far more closely than either agrees with
// an analytical series.
func TestJPLStateAgainstHorizons(t *testing.T) {
	requireHorizons(t)

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	defer func() { _ = p.Close() }()

	cases := loadCases(t)
	byName := make(map[string]horizonsCase, len(cases))

	for _, c := range horizonsCases() {
		byName[c.name] = c
	}

	const (
		posTol = 1e-7
		velTol = 1e-8
	)

	for _, sv := range cases {
		hc, ok := byName[sv.Body]
		if !ok {
			t.Errorf("Horizons returned an unrequested body %q", sv.Body)

			continue
		}

		t.Run(sv.Body, func(t *testing.T) {
			tm := time.FromJD(2451545.0+sv.ET/86400.0, time.TDB)

			state, err := p.State(hc.id, tm)
			if err != nil {
				t.Fatalf("State() failed: %v", err)
			}

			diffPos := state.Pos.Sub(vector.Vec3{X: sv.Pos[0], Y: sv.Pos[1], Z: sv.Pos[2]}).Norm()
			diffVel := state.Vel.Sub(vector.Vec3{X: sv.Vel[0], Y: sv.Vel[1], Z: sv.Vel[2]}).Norm()

			// Reported whether or not it passes: a bound that never prints
			// what it measured cannot tell anyone how much room is left
			// under it.
			t.Logf("%s: |dPos| = %.4e AU (%.3f km), |dVel| = %.4e AU/day",
				sv.Body, diffPos, diffPos*kmPerAU, diffVel)

			if diffPos > posTol {
				t.Errorf("Position mismatch: diff=%e AU, want <%e", diffPos, posTol)
				t.Logf("  Got:  %v", state.Pos)
				t.Logf("  Want: %v", sv.Pos)
			}

			if diffVel > velTol {
				t.Errorf("Velocity mismatch: diff=%e AU/day, want <%e", diffVel, velTol)
				t.Logf("  Got:  %v", state.Vel)
				t.Logf("  Want: %v", sv.Vel)
			}
		})
	}
}
