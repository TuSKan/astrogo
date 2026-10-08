package plan

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// paranalMoonNight is Paranal at 2026-09-26 00:00 UTC, the Moon at 27°
// altitude, where the site sees it 0.85° from where the Earth's center does.
func paranalMoonNight(t *testing.T) (*Site, time.Time) {
	t.Helper()

	site, err := NewSiteEarthLocation("Paranal", -24.6272, -70.4045, 2635)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	return site, time.Date(2026, 9, 26, 0, 0, 0, 0, time.LocationUTC)
}

// TestMoonSepIsTheSeparationTheSiteSees holds MoonSep to Skyfield 1.55 on
// DE440s, from the site rather than the Earth's center (#634):
//
//	obs = earth + wgs84.latlon(-24.6272, -70.4045, elevation_m=2635)
//	obs.at(t).observe(star).apparent().separation_from(obs.at(t).observe(moon).apparent())
//
// Both stars are 30° from the Moon as the site sees it. From the Earth's
// center Skyfield has them 30.4075° and 30.7618° away, which is what MoonSep
// used to report.
func TestMoonSepIsTheSeparationTheSiteSees(t *testing.T) {
	t.Parallel()

	site, at := paranalMoonNight(t)

	for _, c := range []struct {
		name          string
		ra, dec, want float64
	}{
		{"north of the Moon", 354.229381, 29.937454, 30.0023},
		{"east of the Moon", 24.229381, -0.062546, 30.0049},
	} {
		got, err := MoonSep{Threshold: angle.Deg(30.5)}.Check(NewStar(c.name, angle.Deg(c.ra), angle.Deg(c.dec)), at, site)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		t.Logf("%s: %.4f°, Skyfield %.4f°", c.name, got.Value, c.want)

		// The star is a catalog place and the Moon an apparent one, so
		// annual aberration, which moves them differently, is left in;
		// measured at 0.005°.
		if d := math.Abs(got.Value - c.want); d > 0.01 {
			t.Errorf("%s: %.4f°, Skyfield sees %.4f° from the site", c.name, got.Value, c.want)
		}

		if got.Pass {
			t.Errorf("%s: passes a 30.5° threshold at %.4f°", c.name, got.Value)
		}
	}
}

// TestMoonNoteMeasuresFromTheSite: VisibleTonight's advisory names a target
// within 30° of a bright Moon. A star 29.8° east of the Moon as the site
// sees it is about 30.6° from where the Earth's center has the Moon, and got
// no advisory until #634.
func TestMoonNoteMeasuresFromTheSite(t *testing.T) {
	t.Parallel()

	site, at := paranalMoonNight(t)
	ctx := coord.NewContext(at, site.Location(), site.Refraction())

	note := moonNote(eph.Default(), NewStar("east of the Moon", angle.Deg(354.229381+29.8), angle.Deg(-0.062546)), at, ctx)
	if !strings.Contains(note, "30° away") {
		t.Errorf("moonNote = %q, want an advisory at 30° away", note)
	}
}
