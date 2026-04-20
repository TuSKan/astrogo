package satellite_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/norad"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

// ISS TLE for 2026-04-19T11:45:32.833440 UTC.
const (
	issLine1 = "1 25544U 98067A   26109.48996335  .00010082  00000+0  19194-3 0  9992"
	issLine2 = "2 25544  51.6329 230.6068 0006631 325.6576  34.3983 15.48833250562656"
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

func TestNewFromGP(t *testing.T) {
	gp := norad.GP{
		ObjectName:      "ISS (ZARYA)",
		ObjectID:        "1998-067A",
		Epoch:           "2026-04-19T11:45:32.833440",
		MeanMotion:      15.4883325,
		Eccentricity:    0.00066312,
		Inclination:     51.6329,
		RAOfAscNode:     230.6068,
		ArgOfPericenter: 325.6576,
		MeanAnomaly:     34.3983,
		EphemerisType:   0,
		Classification:  "U",
		NoradCatID:      25544,
		ElementSetNo:    999,
		RevAtEpoch:      56265,
		BStar:           0.00019193879,
		MeanMotionDot:   0.00010082,
		MeanMotionDDot:  0,
	}

	sat, err := satellite.NewFromGP(gp)
	if err != nil {
		t.Fatalf("NewFromGP failed: %v", err)
	}

	if sat.Name != "ISS (ZARYA)" {
		t.Errorf("Name = %q, want %q", sat.Name, "ISS (ZARYA)")
	}
}

func TestPropagateECI(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	// Propagate at the TLE epoch itself.
	epoch := time.Date(2026, 4, 19, 11, 45, 32, 0, time.LocationUTC)
	pos, vel, err := sat.PropagateECI(epoch)
	if err != nil {
		t.Fatalf("PropagateECI at epoch failed: %v", err)
	}

	// ISS should be at ~420 km altitude → position vector ~6800 km from center.
	r := pos.Norm()
	t.Logf("ECI position: (%.2f, %.2f, %.2f) km, |r| = %.2f km", pos.X, pos.Y, pos.Z, r)
	t.Logf("ECI velocity: (%.4f, %.4f, %.4f) km/s", vel.X, vel.Y, vel.Z)

	if r < 6400 || r > 7200 {
		t.Errorf("Position magnitude = %.1f km, expected ~6800 km for LEO", r)
	}

	// Velocity should be ~7.7 km/s for LEO.
	v := vel.Norm()
	if v < 7.0 || v > 8.5 {
		t.Errorf("Velocity magnitude = %.2f km/s, expected ~7.7 km/s for LEO", v)
	}
}

func TestPropagateForward(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	// Propagate 90 minutes forward (approximately one orbit).
	t0 := time.Date(2026, 4, 19, 11, 45, 32, 0, time.LocationUTC)
	t1 := t0.AddDays(90.0 / 1440.0)

	pos0, _, _ := sat.PropagateECI(t0)
	pos1, _, _ := sat.PropagateECI(t1)

	r0 := pos0.Norm()
	r1 := pos1.Norm()

	t.Logf("r at epoch:      %.2f km", r0)
	t.Logf("r at epoch+90m:  %.2f km", r1)

	// After one orbit, the radius should be similar (near-circular).
	if math.Abs(r0-r1) > 50 {
		t.Errorf("|r0-r1| = %.2f km, expected < 50 km for near-circular orbit", math.Abs(r0-r1))
	}
}

func TestState(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	epoch := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)
	state, err := sat.State(ephemeris.ID(0), epoch)
	if err != nil {
		t.Fatalf("State failed: %v", err)
	}

	// Position should be very small in AU (LEO ≈ 6800 km / 149597870.7 km ≈ 4.5e-5 AU).
	posAU := state.Pos.Norm()
	t.Logf("GCRS position: %.8f AU (%.1f km)", posAU, posAU*149597870.7)

	if posAU < 3e-5 || posAU > 6e-5 {
		t.Errorf("Position in AU = %e, expected ~4.5e-5 for LEO", posAU)
	}

	// Velocity should be non-zero.
	velMag := state.Vel.Norm()
	if velMag == 0 {
		t.Error("Velocity is zero, expected non-zero for orbiting satellite")
	}
	t.Logf("GCRS velocity: %.8f AU/day", velMag)
}

func TestSubSatellitePoint(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	epoch := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)
	geo, err := sat.SubSatellitePoint(epoch)
	if err != nil {
		t.Fatalf("SubSatellitePoint failed: %v", err)
	}

	lat := geo.Lat().Degrees()
	lon := geo.Lon().Degrees()
	alt := geo.Height() / 1e3 // metres → km

	t.Logf("Sub-satellite point: lat=%.4f° lon=%.4f° alt=%.1f km", lat, lon, alt)

	// ISS latitude should be within ±51.6° (orbital inclination).
	if math.Abs(lat) > 52.0 {
		t.Errorf("Latitude = %.2f°, exceeds inclination bound (51.6°)", lat)
	}

	// Altitude should be ~410-420 km.
	if alt < 300 || alt > 500 {
		t.Errorf("Altitude = %.1f km, expected ~410 km for ISS", alt)
	}
}

func TestOrbitalPeriod(t *testing.T) {
	sat, err := satellite.NewFromGP(norad.GP{
		ObjectName:      "ISS (ZARYA)",
		ObjectID:        "1998-067A",
		Epoch:           "2026-04-19T11:45:32.833440",
		MeanMotion:      15.4883325,
		Eccentricity:    0.00066312,
		Inclination:     51.6329,
		RAOfAscNode:     230.6068,
		ArgOfPericenter: 325.6576,
		MeanAnomaly:     34.3983,
		NoradCatID:      25544,
		Classification:  "U",
		BStar:           0.00019193879,
		MeanMotionDot:   0.00010082,
	})
	if err != nil {
		t.Fatalf("NewFromGP failed: %v", err)
	}

	period := sat.OrbitalPeriod()
	t.Logf("Orbital period: %.2f minutes", period)

	// ISS orbits ~15.5 times/day → ~93 minutes.
	if period < 90 || period > 95 {
		t.Errorf("Period = %.2f min, expected ~93 min for ISS", period)
	}
}

func TestLookAngle(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE failed: %v", err)
	}

	// Observer at São Paulo.
	observer, err := coord.NewGeodetic(angle.Deg(-46.6333), angle.Deg(-23.5505), 760)
	if err != nil {
		t.Fatalf("NewGeodetic failed: %v", err)
	}

	epoch := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)
	az, el, rng, err := sat.LookAngle(epoch, observer)
	if err != nil {
		t.Fatalf("LookAngle failed: %v", err)
	}

	t.Logf("Look angle: az=%.2f° el=%.2f° range=%.1f km", az.Degrees(), el.Degrees(), rng)

	// Azimuth should be in [0, 360).
	if az.Degrees() < 0 || az.Degrees() >= 360 {
		t.Errorf("Azimuth = %.2f°, expected [0, 360)", az.Degrees())
	}

	// Range should be reasonable (500-12000 km for LEO).
	if rng < 100 || rng > 20000 {
		t.Errorf("Range = %.1f km, outside reasonable bounds", rng)
	}
}
