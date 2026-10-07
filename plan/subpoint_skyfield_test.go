//go:build integration

package plan_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// TestSubpointsAgreeWithSkyfield holds the subsolar and sublunar points to
// Skyfield 1.55's wgs84.subpoint_of the apparent place, on DE440s (#579):
//
//	p = earth.at(t).observe(eph[body]).apparent()
//	g = wgs84.subpoint_of(p)
//
// subpoint_of takes the foot of the ellipsoid normal through the body. Until
// #579 SublunarPoint took the point whose normal is parallel to the Moon's
// direction instead, 4.3" south of Skyfield's on 2026-03-20.
//
// Latitude is held to 0.05". Neither side applies polar motion here:
// Skyfield loads none by default, and these tests run without remote/eop.
// Longitude is held to 1.5", because Skyfield applies DUT1 and this test
// does not: 0.05 to 0.07 s this year, which is the 0.6 to 0.8" measured.
func TestSubpointsAgreeWithSkyfield(t *testing.T) {
	prov, err := eph.NewProvider(kernelContext(t), eph.Planets, "de440s")
	if err != nil {
		t.Fatalf("DE440s provider: %v", err)
	}

	t.Cleanup(func() { _ = prov.Close() })

	march := time.Date(2026, 3, 20, 12, 0, 0, 0, time.LocationUTC)
	june := time.Date(2026, 6, 21, 6, 30, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		name     string
		point    func(eph.Provider, time.Time) (*coord.Geodetic, error)
		at       time.Time
		lat, lon float64 // Skyfield's, degrees
	}{
		{"Sun", plan.SubsolarPoint, march, -0.045488, 1.858895},
		{"Sun", plan.SubsolarPoint, june, 23.437920, 82.941626},
		{"Moon", plan.SublunarPoint, march, 10.505128, 18.069419},
		{"Moon", plan.SublunarPoint, june, 1.451051, 165.199363},
	} {
		g, err := c.point(prov, c.at)
		if err != nil {
			t.Fatalf("%s at %v: %v", c.name, c.at, err)
		}

		dLat := (g.Lat().Degrees() - c.lat) * 3600
		dLon := (g.Lon().Degrees() - c.lon) * 3600
		t.Logf("%s at %v: %+.3f\" in latitude, %+.3f\" in longitude from Skyfield", c.name, c.at, dLat, dLon)

		if math.Abs(dLat) > 0.05 {
			t.Errorf("%s at %v: latitude %.6f°, Skyfield %.6f°, %+.3f\" apart", c.name, c.at, g.Lat().Degrees(), c.lat, dLat)
		}

		if math.Abs(dLon) > 1.5 {
			t.Errorf("%s at %v: longitude %.6f°, Skyfield %.6f°, %+.3f\" apart", c.name, c.at, g.Lon().Degrees(), c.lon, dLon)
		}
	}
}
