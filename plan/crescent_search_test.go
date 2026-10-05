package plan

import (
	"math"
	"slices"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// moonsetWholeWindow is moonsetFor as one MoonEvents call over the two days
// around sunset, the search the chunked one replaced.
func moonsetWholeWindow(t *testing.T, sunset time.Time, site *Site, prov eph.Provider) (time.Time, bool) {
	t.Helper()

	evs, err := MoonEvents(sunset.Add(unit.Days(-1)), sunset.Add(unit.Days(1)), site, prov)
	if err != nil {
		t.Fatalf("MoonEvents: %v", err)
	}

	var lastBefore *Event

	for i := range evs {
		e := &evs[i]
		if !isRiseOrSet(*e) {
			continue
		}

		if e.Time.Before(sunset) {
			lastBefore = e

			continue
		}

		if e.Kind == EventSet && (lastBefore == nil || lastBefore.Kind == EventRise) {
			return e.Time, true
		}
	}

	if lastBefore != nil && lastBefore.Kind == EventSet {
		return lastBefore.Time, true
	}

	return time.Time{}, false
}

// The chunked rise and set searches and the bracketed new-moon search are
// optimizations, and must find what one search over the whole window finds,
// across a lunation and at a latitude where the Moon's lag runs to hours.
// Both sample on the same 15-minute grid, so they refine the same brackets;
// the tolerances allow only for that.
func TestCrescentSearchesMatchAWholeWindow(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, s := range []struct {
		name     string
		lat, lon float64
	}{
		{"Port of Spain", 10.65, -61.52},
		{"Reykjavik", 64.15, -21.94},
	} {
		site := crescentSite(t, s.lat, s.lon)

		for day := 0; day < 30; day += 4 {
			// Local noon, every fourth day of a lunation from the new moon
			// of 2025-03-29.
			evening := time.Date(2025, 3, 29+day, 12, 0, 0, 0, time.LocationUTC).Add(unit.Hours(-s.lon / 15))

			sunEvent, found, err := firstEvent(SunEvents, evening, 1, site, prov, isSet)
			if err != nil || !found {
				t.Fatalf("%s day %d: sunset found %v, err %v", s.name, day, found, err)
			}

			sunset := sunEvent.Time

			evs, err := SunEvents(evening, evening.Add(unit.Days(1)), site, prov)
			if err != nil {
				t.Fatalf("SunEvents: %v", err)
			}

			var want time.Time

			for _, e := range evs {
				if isSet(e) {
					want = e.Time

					break
				}
			}

			if d := math.Abs(sunset.Sub(want).Seconds()); d > 1e-3 {
				t.Errorf("%s day %d: chunked sunset %v, whole day %v", s.name, day, sunset, want)
			}

			moonset, err := moonsetFor(sunset, site, prov)
			wantSet, ok := moonsetWholeWindow(t, sunset, site, prov)

			switch {
			case !ok && err == nil:
				t.Errorf("%s day %d: chunked moonset %v, whole window none", s.name, day, moonset)
			case ok && err != nil:
				t.Errorf("%s day %d: chunked moonset: %v, whole window %v", s.name, day, err, wantSet)
			case ok && math.Abs(moonset.Sub(wantSet).Seconds()) > 1e-3:
				t.Errorf("%s day %d: chunked moonset %v, whole window %v", s.name, day, moonset, wantSet)
			}

			age, err := moonAgeHours(sunset, prov)
			if err != nil {
				t.Fatalf("%s day %d: moonAgeHours: %v", s.name, day, err)
			}

			if wantAge := ageFromALunation(t, sunset, prov); math.Abs(age-wantAge) > 1.0/3600 {
				t.Errorf("%s day %d: bracketed age %.6f h, a lunation's search %.6f h", s.name, day, age, wantAge)
			}
		}
	}
}

// ageFromALunation is the Moon's age at t from a search over the month
// before it, the search moonAgeHours brackets.
func ageFromALunation(t *testing.T, at time.Time, prov eph.Provider) float64 {
	t.Helper()

	phases, err := MoonPhases(at.Add(unit.Days(-31)), at, prov)
	if err != nil {
		t.Fatalf("MoonPhases: %v", err)
	}

	for _, p := range slices.Backward(phases) {
		if p.Phase == PhaseNewMoon {
			return at.Sub(p.Time).Hours()
		}
	}

	t.Fatalf("no new moon in the month before %v", at)

	return 0
}

// An instant just before a new moon, with the elongation near 360°, is the
// widest bracket moonAgeHours draws, reaching back past a whole lunation.
func TestMoonAgeJustBeforeANewMoon(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	// 2025-03-29 10:58 UT, minus an hour.
	at := time.Date(2025, 3, 29, 9, 58, 0, 0, time.LocationUTC)

	age, err := moonAgeHours(at, prov)
	if err != nil {
		t.Fatalf("moonAgeHours: %v", err)
	}

	if want := ageFromALunation(t, at, prov); math.Abs(age-want) > 1.0/3600 {
		t.Errorf("age %.6f h, a lunation's search %.6f h", age, want)
	}

	if age < 24*29 {
		t.Errorf("age %.3f h an hour before a new moon, want most of a lunation", age)
	}
}
