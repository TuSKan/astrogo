package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestEclipseGammaIsTheCanons holds EclipseEvent.Gamma to the gamma NASA's Five
// Millennium Canons publish for every eclipse of 2024 to 2028
// (SEcat5/SE2001-2100.html, LEcat5/LE2001-2100.html): signed, in Earth
// equatorial radii, positive north. It was the distance as a fraction of the
// grazing one, with no sign, and read 0.2239 for the 2024 April 8 eclipse
// whose gamma is 0.3431; the October one, at -0.3509, read 0.2236.
//
// The set runs from central eclipses to the 2027 July 18 penumbral eclipse,
// whose penumbral magnitude is 0.0014, so both ends of the scale and both
// signs are held. Measured on eph.Default, the analytical ephemeris: 0.0010
// Earth radii at worst (the 2027 February 20 lunar eclipse) and 0.0007 for
// the Sun (2025 March 29), every sign agreeing. That is the ephemeris, not the
// geometry: on DE441 the same geometry agrees with all 2885 eclipses of
// nasa_eclipse_test.go's six centuries to 0.0002.
func TestEclipseGammaIsTheCanons(t *testing.T) {
	t.Parallel()

	const tolerance = 0.0015

	prov := eph.Default()

	cases := []struct {
		find  func(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error)
		date  string
		y, m  int
		d     int
		gamma float64
	}{
		{SolarEclipses, "2024-04-08", 2024, 4, 8, 0.3431},
		{SolarEclipses, "2024-10-02", 2024, 10, 2, -0.3509},
		{SolarEclipses, "2025-03-29", 2025, 3, 29, 1.0405},
		{SolarEclipses, "2025-09-21", 2025, 9, 21, -1.0651},
		{SolarEclipses, "2026-02-17", 2026, 2, 17, -0.9743},
		{SolarEclipses, "2026-08-12", 2026, 8, 12, 0.8977},
		{SolarEclipses, "2027-02-06", 2027, 2, 6, -0.2952},
		{SolarEclipses, "2027-08-02", 2027, 8, 2, 0.1421},
		{SolarEclipses, "2028-01-26", 2028, 1, 26, 0.3901},
		{SolarEclipses, "2028-07-22", 2028, 7, 22, -0.6056},
		{LunarEclipses, "2024-03-25", 2024, 3, 25, 1.0609},
		{LunarEclipses, "2024-09-18", 2024, 9, 18, -0.9792},
		{LunarEclipses, "2025-03-14", 2025, 3, 14, 0.3484},
		{LunarEclipses, "2025-09-07", 2025, 9, 7, -0.2752},
		{LunarEclipses, "2026-03-03", 2026, 3, 3, -0.3765},
		{LunarEclipses, "2026-08-28", 2026, 8, 28, 0.4964},
		{LunarEclipses, "2027-02-20", 2027, 2, 20, -1.0480},
		{LunarEclipses, "2027-07-18", 2027, 7, 18, -1.5758},
		{LunarEclipses, "2027-08-17", 2027, 8, 17, 1.2797},
		{LunarEclipses, "2028-01-12", 2028, 1, 12, 0.9817},
		{LunarEclipses, "2028-07-06", 2028, 7, 6, -0.7903},
		{LunarEclipses, "2028-12-31", 2028, 12, 31, 0.3258},
	}

	for _, c := range cases {
		day := time.Date(c.y, time.Month(c.m), c.d, 0, 0, 0, 0, time.LocationUTC)

		events, err := c.find(day.Add(unit.Days(-1)), day.Add(unit.Days(2)), prov)
		if err != nil || len(events) != 1 {
			t.Errorf("%s: %d eclipses, err %v; want 1", c.date, len(events), err)

			continue
		}

		if got := events[0].Gamma; math.Abs(got-c.gamma) > tolerance {
			t.Errorf("%s %s: Gamma %+.4f, NASA gives %+.4f", c.date, events[0].Type, got, c.gamma)
		}
	}
}
