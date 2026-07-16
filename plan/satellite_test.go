package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// R29 regression: plan/satellite.go had zero coverage under default
// `go test ./...` — NewSatellite, Position, GeocentricVec, GetDetails,
// ApparentMagnitudeCtx, LookAngle, SatellitePasses, and findCulmination were
// all unexercised. Uses the same real ISS TLE already validated offline in
// ephemeris/satellite/satellite_test.go (no network access).
const (
	issLine1 = "1 25544U 98067A   26109.48995873  .00010082  00000-0  19194-3 0  9990"
	issLine2 = "2 25544  51.6329 230.6068 0006631 325.6576  34.3983 15.48833250562650"
)

func newISSProvider(t *testing.T) *satellite.Satellite {
	t.Helper()

	sat, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	return sat
}

func TestPlanSatellite_PositionAndDetails(t *testing.T) {
	prov := newISSProvider(t)
	sat := NewSatellite("ISS", eph.ID(0), prov, WithStdMag(-1.3, magnitude.ConventionMcCants))

	tm := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)

	icrs, err := sat.Position(tm)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	if icrs.RA().Degrees() == 0 && icrs.Dec().Degrees() == 0 {
		t.Error("Position returned the zero value; expected a real ISS sky position")
	}

	vec, err := sat.GeocentricVec(tm)
	if err != nil {
		t.Fatalf("GeocentricVec: %v", err)
	}

	// ISS orbits at ~400km altitude ≈ 6771 km from geocenter ≈ 4.5e-5 AU.
	const kmPerAU = 149597870.7

	distKm := vec.Norm() * kmPerAU
	if distKm < 6000 || distKm > 7500 {
		t.Errorf("geocentric distance = %.0f km, want ~6771 km (LEO)", distKm)
	}

	if sat.Name() != "ISS" {
		t.Errorf("Name() = %q, want ISS", sat.Name())
	}

	if sat.EphID() != eph.ID(0) {
		t.Errorf("EphID() = %v, want 0", sat.EphID())
	}

	if sat.Provider() != prov {
		t.Error("Provider() did not return the constructor-supplied provider")
	}

	loc, err := coord.NewGeodetic(angle.Deg(-46.63), angle.Deg(-23.55), 760)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("São Paulo", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	d, err := sat.GetDetails(ctx)
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Name != "ISS" {
		t.Errorf("GetDetails Name = %q, want ISS", d.Name)
	}
}

func TestPlanSatellite_ApparentMagnitudeRequiresContext(t *testing.T) {
	prov := newISSProvider(t)
	sat := NewSatellite("ISS", eph.ID(0), prov, WithStdMag(-1.3, magnitude.ConventionMcCants))

	tm := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)

	// Without a context, ApparentMagnitude must fail with a clear error
	// (use ApparentMagnitudeCtx instead) rather than silently returning 0.
	if _, err := sat.ApparentMagnitude(tm); err == nil {
		t.Error("expected ApparentMagnitude to fail without observer context")
	}

	loc, err := coord.NewGeodetic(angle.Deg(-46.63), angle.Deg(-23.55), 760)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("São Paulo", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	m, err := sat.ApparentMagnitudeCtx(tm, ctx)
	if err != nil {
		t.Fatalf("ApparentMagnitudeCtx: %v", err)
	}

	// A LEO satellite's apparent magnitude is bounded (roughly -8 to +15
	// across all illumination/range geometries); anything outside that is a
	// clear sign of a broken phase-angle or range computation.
	if m < -8 || m > 15 {
		t.Errorf("ApparentMagnitudeCtx = %.2f, out of plausible range", m)
	}
}

func TestLookAngle_ISS(t *testing.T) {
	prov := newISSProvider(t)

	loc, err := coord.NewGeodetic(angle.Deg(-46.63), angle.Deg(-23.55), 760)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	tm := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(tm, loc, defaultAtm)

	altaz, err := LookAngle(prov, 0, ctx)
	if err != nil {
		t.Fatalf("LookAngle: %v", err)
	}

	// LookAngle doesn't gate on horizon, so the slant range can be anywhere
	// from ~400 km (directly overhead) to ~13000 km (satellite on the far
	// side of Earth). The bound here just needs to rule out a unit mismatch
	// (e.g. AU instead of km, ~1e8 km) rather than pin an exact geometry.
	if altaz.Dist() < 300 || altaz.Dist() > 14000 {
		t.Errorf("LookAngle range = %.0f km, want a plausible geocentric-scale slant range (300-14000 km)",
			altaz.Dist())
	}
}

func TestSatellitePasses_ISS(t *testing.T) {
	prov := newISSProvider(t)

	loc, err := coord.NewGeodetic(angle.Deg(-46.63), angle.Deg(-23.55), 760)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	start := time.Date(2026, 4, 19, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(24 * time.Hour)

	passes, err := SatellitePasses(prov, "ISS", start, end, loc, angle.Deg(10))
	if err != nil {
		t.Fatalf("SatellitePasses: %v", err)
	}

	// ISS orbital period ~92 min ⇒ several passes over 24h, though not all
	// clear the 10° elevation gate; assert internal consistency rather than
	// an exact count.
	for i, p := range passes {
		if p.Name != "ISS" {
			t.Errorf("pass %d: Name = %q, want ISS", i, p.Name)
		}

		if !p.Rise.Time.Before(p.Culmination.Time) {
			t.Errorf("pass %d: rise (%v) not before culmination (%v)", i, p.Rise.Time, p.Culmination.Time)
		}

		if !p.Culmination.Time.Before(p.Set.Time) {
			t.Errorf("pass %d: culmination (%v) not before set (%v)", i, p.Culmination.Time, p.Set.Time)
		}

		if p.Culmination.Elevation.Degrees() < p.Rise.Elevation.Degrees() {
			t.Errorf("pass %d: culmination elevation (%.1f) below rise elevation (%.1f)",
				i, p.Culmination.Elevation.Degrees(), p.Rise.Elevation.Degrees())
		}

		if p.Rise.Elevation.Degrees() < minElevationTolerance {
			t.Errorf("pass %d: rise elevation %.2f° below the 10° gate (tolerance %.2f°)",
				i, p.Rise.Elevation.Degrees(), minElevationTolerance)
		}

		if p.Duration <= 0 {
			t.Errorf("pass %d: non-positive duration %v", i, p.Duration)
		}
	}
}

// minElevationTolerance allows a small margin below the nominal 10° gate for
// the Chandrupatla root-finder's refinement tolerance.
const minElevationTolerance = 9.9

func TestPlanSatellite_StaticMagnitudeAndPhaseModel(t *testing.T) {
	prov := newISSProvider(t)

	noStdMag := NewSatellite("ISS", eph.ID(0), prov)
	if _, ok := noStdMag.StaticMagnitude(); ok {
		t.Error("expected StaticMagnitude ok=false when WithStdMag was not set")
	}

	withStdMag := NewSatellite("ISS", eph.ID(0), prov,
		WithStdMag(-1.3, magnitude.ConventionMcCants), WithPhaseModel(magnitude.PhaseCylinder))

	m, ok := withStdMag.StaticMagnitude()
	if !ok {
		t.Fatal("expected StaticMagnitude ok=true when WithStdMag was set")
	}

	if m != -1.3 {
		t.Errorf("StaticMagnitude = %v, want -1.3", m)
	}
}
