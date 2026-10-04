package plan

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestMoonPhasesAreApparent: MoonPhases measures the elongation between the
// apparent Sun and Moon, as published phases are defined (#430). Two checks.
// FullMoonOppositions solves the same instant from Planet.Position, which is
// apparent, and over 2026 the two must agree to a second; with geometric
// longitudes they disagreed by up to 43 s on every Full Moon. And the Full
// Moon of 2026-09-26 must come within 10 s of Skyfield 1.54's 16:49:02 UTC
// (DE421) on the analytical ephemeris; it is 4 s, where it was 33 s late.
func TestMoonPhasesAreApparent(t *testing.T) {
	prov := eph.Default()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2027, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	phases, err := MoonPhases(start, end, prov)
	if err != nil {
		t.Fatal(err)
	}

	oppositions, err := FullMoonOppositions(start, end, prov)
	if err != nil {
		t.Fatal(err)
	}

	var fulls []time.Time

	for _, p := range phases {
		if p.Phase == PhaseFullMoon {
			fulls = append(fulls, p.Time)
		}
	}

	if len(fulls) != 13 || len(oppositions) != 13 {
		t.Fatalf("%d Full Moons from MoonPhases and %d from FullMoonOppositions in 2026, want 13 each",
			len(fulls), len(oppositions))
	}

	for k, full := range fulls {
		if d := full.Sub(oppositions[k].Time).Abs(); d > unit.Seconds(1) {
			t.Errorf("Full Moon %v from MoonPhases, %v from FullMoonOppositions: %v apart",
				full, oppositions[k].Time, d)
		}
	}

	skyfield := time.Date(2026, 9, 26, 16, 49, 2, 0, time.LocationUTC)

	for _, full := range fulls {
		if full.Sub(skyfield).Abs() > unit.Days(1) {
			continue
		}

		if d := full.Sub(skyfield).Abs(); d > unit.Seconds(10) {
			t.Errorf("Full Moon of September 2026 at %v, Skyfield gives %v (off by %v)", full, skyfield, d)
		}

		return
	}

	t.Errorf("no Full Moon within a day of %v", skyfield)
}
