package jpl_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

type ValidationCase struct {
	Body    string     `json:"body"`
	NAIF    int        `json:"naif_id"`
	Epoch   string     `json:"epoch"`
	ET      float64    `json:"et"`
	Pos     [3]float64 `json:"pos"`
	Vel     [3]float64 `json:"vel"`
	UnitPos string     `json:"unit_pos"`
	UnitVel string     `json:"unit_vel"`
}

func loadCases(t *testing.T) []ValidationCase {
	path := filepath.Join("..", "validation", "horizons_reference.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read reference data: %v", err)
	}
	var cases []ValidationCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("failed to unmarshal reference data: %v", err)
	}
	return cases
}

func runHorizonsTest(t *testing.T, bodyName string) {
	p, err := jpl.New("de440s", "../data")
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

			bid := body.Sun
			if c.Body == "Moon" {
				bid = body.Moon
			}
			if c.Body == "Mars" {
				bid = body.Mars
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
