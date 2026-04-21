//go:build network

package jpl_test

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

func loadCases(t *testing.T) []*StateVector {
	sun, err := fetchVector(10, "Sun", "2000-01-01 12:00 TDB", "2000-01-01 12:01")
	if err != nil {
		t.Fatalf("failed to fetch sun vector: %v", err)
	}
	moon, err := fetchVector(301, "Moon", "2000-01-01 12:00 TDB", "2000-01-01 12:01")
	if err != nil {
		t.Fatalf("failed to fetch moon vector: %v", err)
	}
	mars, err := fetchVector(4, "Mars", "2000-01-01 12:00 TDB", "2000-01-01 12:01")
	if err != nil {
		t.Fatalf("failed to fetch mars vector: %v", err)
	}
	return []*StateVector{sun, moon, mars}
}

func runHorizonsTest(t *testing.T, bodyName string) {
	p, err := jpl.NewProvider(core.Planets, "de440", jpl.WithDataDir("../data"))
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}
	defer p.Close()

	cases := loadCases(t)
	const posTol = 1e-7
	const velTol = 1e-8

	found := false
	for _, c := range cases {
		if c.Body != bodyName && bodyName != "Planetary" {
			continue
		}
		if bodyName == "Planetary" && (c.Body == "Sun" || c.Body == "Moon") {
			continue
		}
		found = true

		t.Run(c.Body, func(t *testing.T) {
			tm := time.FromJD(2451545.0+c.ET/86400.0, time.TDB)

			bid := eph.Sun
			if c.Body == "Moon" {
				bid = eph.Moon
			}
			if c.Body == "Mars" {
				bid = eph.Mars
			}

			state, err := p.State(bid, tm)
			if err != nil {
				t.Fatalf("State() failed: %v", err)
			}

			diffPos := state.Pos.Sub(vector.Vec3{X: c.Pos[0], Y: c.Pos[1], Z: c.Pos[2]}).Norm()
			if diffPos > posTol {
				t.Errorf("Position mismatch: diff=%e AU, want <%e", diffPos, posTol)
				t.Logf("  Got:  %v", state.Pos)
				t.Logf("  Want: %v", c.Pos)
			}

			diffVel := state.Vel.Sub(vector.Vec3{X: c.Vel[0], Y: c.Vel[1], Z: c.Vel[2]}).Norm()
			if diffVel > velTol {
				t.Errorf("Velocity mismatch: diff=%e AU/day, want <%e", diffVel, velTol)
				t.Logf("  Got:  %v", state.Vel)
				t.Logf("  Want: %v", c.Vel)
			}
		})
	}
	if !found {
		t.Errorf("No cases found for %s", bodyName)
	}
}

func TestJPLStateAgainstHorizonsSun(t *testing.T) {
	runHorizonsTest(t, "Sun")
}

func TestJPLStateAgainstHorizonsMoon(t *testing.T) {
	runHorizonsTest(t, "Moon")
}

func TestJPLStateAgainstHorizonsPlanetaryBodies(t *testing.T) {
	runHorizonsTest(t, "Planetary")
}
