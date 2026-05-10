package satellite_test

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

const (
	issLine1 = "1 25544U 98067A   26109.48995873  .00010082  00000-0  19194-3 0  9990"
	issLine2 = "2 25544  51.6329 230.6068 0006631 325.6576  34.3983 15.48833250562650"
)

func TestNewFromTLE(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	if sat.Name != "ISS (ZARYA)" {
		t.Errorf("Name = %q, want %q", sat.Name, "ISS (ZARYA)")
	}
}

func TestNewFromTLEViaStrings(t *testing.T) {
	// Verify that round-tripping through TLE strings preserves satellite identity.
	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	if sat.Name != "ISS (ZARYA)" {
		t.Errorf("Name = %q, want %q", sat.Name, "ISS (ZARYA)")
	}
}

func TestState(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	epoch := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)

	state, err := sat.State(eph.ID(0), epoch)
	if err != nil {
		t.Fatalf("State failed: %v", err)
	}

	// Position should be very small in AU (LEO ≈ 6800 km / 149597870.7 km ≈ 4.5e-5 AU).
	posAU := state.Distance()
	distKm := state.DistanceKm()
	t.Logf("GCRS position: %.8f AU (%.1f km)", posAU, distKm)

	if posAU < 3e-5 || posAU > 6e-5 {
		t.Errorf("Position in AU = %e, expected ~4.5e-5 for LEO", posAU)
	}

	// Distance in km should be ~6800 km from geocenter.
	if distKm < 6400 || distKm > 7200 {
		t.Errorf("DistanceKm = %.1f, expected ~6800 km for LEO", distKm)
	}

	// Velocity should be non-zero.
	speed := state.Speed()
	if speed == 0 {
		t.Error("Speed is zero, expected non-zero for orbiting satellite")
	}

	t.Logf("GCRS velocity: %.8f AU/day", speed)
}

func TestStateForwardOrbit(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	// Propagate 90 minutes forward (approximately one orbit).
	t0 := time.Date(2026, 4, 19, 11, 45, 32, 0, time.LocationUTC)
	t1 := t0.AddDays(90.0 / 1440.0)

	s0, err := sat.State(0, t0)
	if err != nil {
		t.Fatalf("State at epoch failed: %v", err)
	}

	s1, err := sat.State(0, t1)
	if err != nil {
		t.Fatalf("State at epoch+90m failed: %v", err)
	}

	r0 := s0.DistanceKm()
	r1 := s1.DistanceKm()

	t.Logf("r at epoch:      %.2f km", r0)
	t.Logf("r at epoch+90m:  %.2f km", r1)

	// After one orbit, the radius should be similar (near-circular).
	if math.Abs(r0-r1) > 50 {
		t.Errorf("|r0-r1| = %.2f km, expected < 50 km for near-circular orbit", math.Abs(r0-r1))
	}
}

func TestAltitude(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	epoch := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)

	alt, err := sat.Altitude(epoch)
	if err != nil {
		t.Fatalf("Altitude failed: %v", err)
	}

	t.Logf("Altitude: %.1f km", alt)

	// Altitude should be ~410-420 km.
	if alt < 300 || alt > 500 {
		t.Errorf("Altitude = %.1f km, expected ~410 km for ISS", alt)
	}
}

func TestOrbitalPeriod(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	period := sat.OrbitalPeriod()
	t.Logf("Orbital period: %.2f minutes", period)

	// ISS orbits ~15.5 times/day → ~93 minutes.
	if period < 90 || period > 95 {
		t.Errorf("Period = %.2f min, expected ~93 min for ISS", period)
	}
}
