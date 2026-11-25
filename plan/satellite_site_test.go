package plan

import (
	"errors"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// siteAt is a Site at the location g, for the tests that place satellite
// passes at a geodetic location.
func siteAt(tb testing.TB, g *coord.Geodetic) *Site {
	tb.Helper()

	site, err := NewSite("observer", g.Lat(), g.Lon(), g.Height())
	if err != nil {
		tb.Fatalf("NewSite at %v: %v", g, err)
	}

	return site
}

// TestSatellitePassesTakesItsSiteAndMinimumElevation: passes are searched
// above the geometric horizon unless WithMinElevation says otherwise, so a
// day of ISS passes above 10° is a strict subset of those above 0°, each
// inside one of them. A nil satellite or site is refused rather than
// dereferenced.
func TestSatellitePassesTakesItsSiteAndMinimumElevation(t *testing.T) {
	t.Parallel()

	prov, g := testISS(t)
	sat, site := NewSatellite("ISS", prov.ID(), prov), siteAt(t, g)
	start := epoch.Date(2026, time.April, 17, 0, 0, 0, 0, time.UTC)
	w := Window{Start: start, End: start.Add(unit.Days(1))}

	horizon, err := SatellitePasses(sat, site, w)
	if err != nil {
		t.Fatalf("SatellitePasses: %v", err)
	}

	high, err := SatellitePasses(sat, site, w, WithMinElevation(angle.Deg(10)))
	if err != nil {
		t.Fatalf("SatellitePasses above 10°: %v", err)
	}

	if len(high) == 0 || len(high) >= len(horizon) {
		t.Fatalf("%d passes above 10° and %d above 0°; want some, and fewer than above 0°", len(high), len(horizon))
	}

	for _, p := range horizon {
		if p.Name != "ISS" || p.Culmination.Elevation.Degrees() < 0 {
			t.Errorf("a pass above 0° is named %q and peaks at %.2f°", p.Name, p.Culmination.Elevation.Degrees())
		}
	}

	for _, p := range high {
		if p.Culmination.Elevation.Degrees() < 10 {
			t.Errorf("a pass above 10° peaks at %.2f°", p.Culmination.Elevation.Degrees())
		}

		inside := false

		for _, q := range horizon {
			if !p.Rise.Time.Before(q.Rise.Time) && !p.Set.Time.After(q.Set.Time) {
				inside = true
			}
		}

		if !inside {
			t.Errorf("the pass above 10° from %v to %v lies inside no pass above 0°", p.Rise.Time, p.Set.Time)
		}
	}

	if _, err := SatellitePasses(nil, site, w); !errors.Is(err, ErrNilTarget) {
		t.Errorf("a nil satellite: got %v, want %v", err, ErrNilTarget)
	}

	if _, err := SatellitePasses(sat, nil, w); !errors.Is(err, ErrNilSite) {
		t.Errorf("a nil site: got %v, want %v", err, ErrNilSite)
	}
}
