package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestHorizonRefractionDefaultsToTheAlmanac: a site that names no air keeps
// the almanacs' 34′, so every rise and set agrees with USNO's and Skyfield's
// as before (#588).
func TestHorizonRefractionDefaultsToTheAlmanac(t *testing.T) {
	t.Parallel()

	site, err := NewSiteEarthLocation("sea level", 45, 0, 0)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	if got := site.horizonRefraction(); got != standardRefraction {
		t.Errorf("horizon refraction %.6f°, want the almanacs' %.6f°", got, standardRefraction)
	}

	if got := site.RiseSetThreshold().Degrees(); got != -standardRefraction {
		t.Errorf("RiseSetThreshold %.6f°, want %.6f°", got, -standardRefraction)
	}
}

// TestHorizonRefractionFollowsTheAir: with WithHorizonRefraction the horizon
// refracts as the named air does, at 0° of apparent altitude.
//
// At the almanac's own conditions, 10 °C and 1010 hPa, that is Bennett-NA's
// 33.8′, the Nautical Almanac's figure for 0°00′. Denser air refracts more,
// thinner less, as pressure over temperature: 38.6′ at −20 °C and 1030 hPa,
// and about 22′ in the standard atmosphere's air at 4,205 m. A zero pressure
// is no refraction, and the horizon is the dip alone.
func TestHorizonRefractionFollowsTheAir(t *testing.T) {
	t.Parallel()

	const heightM = 100.0

	for _, c := range []struct {
		name   string
		air    atmosphere.Refraction
		lo, hi float64 // arcminutes
	}{
		{"the almanac's air", atmosphere.Refraction{Pressure: 1010, Temperature: 10}, 33.75, 33.85},
		{"cold, dense air", atmosphere.Refraction{Pressure: 1030, Temperature: -20}, 38.4, 38.8},
		{"air at 4,205 m", atmosphere.AtAltitude(unit.Meters(4205)), 21, 23},
		{"no air", atmosphere.Refraction{}, 0, 0},
	} {
		site, err := NewSiteEarthLocation(c.name, 45, 0, heightM, WithHorizonRefraction(c.air))
		if err != nil {
			t.Fatalf("NewSiteEarthLocation: %v", err)
		}

		r := site.horizonRefraction() * 60
		if r < c.lo || r > c.hi {
			t.Errorf("%s: horizon refraction %.3f′, want %.2f′–%.2f′", c.name, r, c.lo, c.hi)
		}

		dip := site.HorizonDip().Degrees()

		for _, th := range []struct {
			name      string
			got, want float64
		}{
			{"RiseSetThreshold", site.RiseSetThreshold().Degrees(), -r/60 - dip},
			{"SunRiseSetThreshold", site.SunRiseSetThreshold().Degrees(), -0.2667 - r/60 - dip},
			{"MoonRiseSetThreshold", site.MoonRiseSetThreshold().Degrees(), -0.2583 - r/60 - dip},
		} {
			if math.Abs(th.got-th.want) > 1e-12 {
				t.Errorf("%s: %s %.9f°, want %.9f°", c.name, th.name, th.got, th.want)
			}
		}
	}
}

// TestHorizonRefractionMovesRiseAndSet: the rise and set solver puts a star's
// rise where its geometric altitude reaches the site's threshold, whatever
// air set it, so denser air brings the rise earlier and the set later, and
// no air does the opposite.
func TestHorizonRefractionMovesRiseAndSet(t *testing.T) {
	t.Parallel()

	sirius := NewStar("Sirius", angle.Deg(101.2872), angle.Deg(-16.7161))
	start := time.Date(2026, 3, 20, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2026, 3, 21, 0, 0, 0, 0, time.LocationUTC)

	events := func(opts ...SiteOption) (site *Site, rise, set time.Time) {
		t.Helper()

		site, err := NewSiteEarthLocation("sea level", 45, 0, 0, opts...)
		if err != nil {
			t.Fatalf("NewSiteEarthLocation: %v", err)
		}

		evs, err := VisibilityEvents(start, end, sirius, site)
		if err != nil {
			t.Fatalf("VisibilityEvents: %v", err)
		}

		for _, e := range evs {
			if e.Kind == EventRise {
				rise = e.Time
			}

			if e.Kind == EventSet {
				set = e.Time
			}
		}

		if rise.IsZero() || set.IsZero() {
			t.Fatalf("no rise and set in %v", evs)
		}

		return site, rise, set
	}

	geometric := func(site *Site, at time.Time) float64 {
		t.Helper()

		pos, err := sirius.Position(at)
		if err != nil {
			t.Fatalf("Position: %v", err)
		}

		aa, err := coord.NewContext(at, site.Location(), atmosphere.Refraction{}).ICRSToAltAz(pos)
		if err != nil {
			t.Fatalf("ICRSToAltAz: %v", err)
		}

		return aa.Alt().Degrees()
	}

	_, almanacRise, almanacSet := events()
	dense, denseRise, denseSet := events(WithHorizonRefraction(atmosphere.Refraction{Pressure: 1030, Temperature: -20}))
	vacuum, vacuumRise, vacuumSet := events(WithHorizonRefraction(atmosphere.Refraction{}))

	if !denseRise.Before(almanacRise) || !almanacRise.Before(vacuumRise) {
		t.Errorf("rises: dense air %v, the almanac %v, no air %v; want them in that order", denseRise, almanacRise, vacuumRise)
	}

	if !vacuumSet.Before(almanacSet) || !almanacSet.Before(denseSet) {
		t.Errorf("sets: no air %v, the almanac %v, dense air %v; want them in that order", vacuumSet, almanacSet, denseSet)
	}

	for _, c := range []struct {
		name string
		site *Site
		at   time.Time
	}{
		{"dense air, rise", dense, denseRise},
		{"dense air, set", dense, denseSet},
		{"no air, rise", vacuum, vacuumRise},
		{"no air, set", vacuum, vacuumSet},
	} {
		// The solver refines to well under a second; a star near the
		// horizon at 45° moves about 0.004° a second.
		if got, want := geometric(c.site, c.at), c.site.RiseSetThreshold().Degrees(); math.Abs(got-want) > 0.004 {
			t.Errorf("%s at %v: geometric altitude %.5f°, the threshold %.5f°", c.name, c.at, got, want)
		}
	}
}

// TestHorizonRefractionIsPartOfTheSite: the air is carried by the site's
// copies and told apart by Equal, since it moves every rise and set.
func TestHorizonRefractionIsPartOfTheSite(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	air := atmosphere.Refraction{Pressure: 1030, Temperature: -20}

	plain, _ := NewSite("s", loc)
	cold, _ := NewSite("s", loc, WithHorizonRefraction(air))
	coldAgain, _ := NewSite("s", loc, WithHorizonRefraction(air))
	bennett, _ := NewSite("s", loc, WithHorizonRefraction(atmosphere.Refraction{
		Pressure: 1030, Temperature: -20, Model: atmosphere.RefractionBennett{},
	}))

	if plain.Equal(cold) || cold.Equal(plain) {
		t.Error("a site with horizon air equals one without")
	}

	if !cold.Equal(coldAgain) {
		t.Error("two sites with the same horizon air differ")
	}

	if cold.Equal(bennett) {
		t.Error("sites whose horizon air names different models are equal")
	}

	warm, _ := NewSite("s", loc, WithHorizonRefraction(atmosphere.Refraction{Pressure: 1030, Temperature: 25}))
	if cold.Equal(warm) {
		t.Error("sites whose horizon air differs in temperature are equal")
	}

	withHorizon, err := cold.WithHorizon(angle.Deg(5))
	if err != nil {
		t.Fatalf("WithHorizon: %v", err)
	}

	for name, s := range map[string]*Site{
		"WithHorizon":  withHorizon,
		"WithTimeZone": cold.WithTimeZone(time.LocationUTC),
	} {
		if got, want := s.horizonRefraction(), cold.horizonRefraction(); got != want {
			t.Errorf("%s dropped the horizon air: %.6f°, want %.6f°", name, got, want)
		}
	}
}
