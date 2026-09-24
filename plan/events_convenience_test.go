package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// R29 regression: plan/events.go's convenience wrappers (Conjunctions,
// ConjunctionsEcliptic, Appulses, Oppositions, GreatestElongations,
// FullMoonOppositions, VisibilityEvents, NextNewMoon, NextFullMoon) and the
// EventFamilyIllumination dispatch (solveIllumination) had zero coverage
// under default `go test ./...`.

func TestConjunctions_MarsJupiter(t *testing.T) {
	prov := eph.Default()
	mars := NewMars(prov)
	jupiter := NewJupiter(prov)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(730)) // 2 years: conjunctions are infrequent for outer planets

	events, err := Conjunctions(start, end, mars, jupiter)
	if err != nil {
		t.Fatalf("Conjunctions: %v", err)
	}

	for i, e := range events {
		if i > 0 && !e.Time.After(events[i-1].Time) {
			t.Errorf("event %d out of chronological order", i)
		}
	}
}

func TestConjunctionsEcliptic_MarsJupiter(t *testing.T) {
	prov := eph.Default()
	mars := NewMars(prov)
	jupiter := NewJupiter(prov)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(730))

	events, err := ConjunctionsEcliptic(start, end, mars, jupiter)
	if err != nil {
		t.Fatalf("ConjunctionsEcliptic: %v", err)
	}

	for i, e := range events {
		if i > 0 && !e.Time.After(events[i-1].Time) {
			t.Errorf("event %d out of chronological order", i)
		}
	}
}

func TestAppulses_MarsJupiter(t *testing.T) {
	prov := eph.Default()
	mars := NewMars(prov)
	jupiter := NewJupiter(prov)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(730))

	events, err := Appulses(start, end, mars, jupiter)
	if err != nil {
		t.Fatalf("Appulses: %v", err)
	}

	for i, e := range events {
		// Appulse Value is the minimum angular separation in degrees —
		// must be non-negative and physically bounded (<= 180°).
		if e.Value < 0 || e.Value > 180 {
			t.Errorf("event %d: appulse separation = %.2f°, out of [0,180]", i, e.Value)
		}
	}
}

func TestOppositions_MarsSun(t *testing.T) {
	prov := eph.Default()
	mars := NewMars(prov)
	sun := NewSun(prov)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(365 * 3)) // Mars oppositions occur roughly every ~26 months

	events, err := Oppositions(start, end, mars, sun)
	if err != nil {
		t.Fatalf("Oppositions: %v", err)
	}

	if len(events) == 0 {
		t.Error("expected at least one Mars opposition within 3 years")
	}
}

func TestGreatestElongations_Venus(t *testing.T) {
	prov := eph.Default()
	venus := NewVenus(prov)
	sun := NewSun(prov)

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(584)) // one full Venus synodic period

	events, err := GreatestElongations(start, end, venus, sun)
	if err != nil {
		t.Fatalf("GreatestElongations: %v", err)
	}

	// A full synodic period contains one greatest-East and one greatest-West
	// elongation for Venus.
	if len(events) < 1 {
		t.Error("expected at least one greatest elongation within a full Venus synodic period")
	}

	for i, e := range events {
		if i > 0 && !e.Time.After(events[i-1].Time) {
			t.Errorf("event %d out of chronological order (GreatestElongations must sort East+West events)", i)
		}

		// Venus's maximum elongation is ~47°; the reported Value should be
		// well below a right angle from the Sun in either direction.
		if e.Value <= 0 || e.Value > 50 {
			t.Errorf("event %d: elongation = %.2f°, want in (0,50]", i, e.Value)
		}
	}
}

func TestFullMoonOppositions_MatchesMoonPhases(t *testing.T) {
	prov := eph.Default()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(90))

	events, err := FullMoonOppositions(start, end, prov)
	if err != nil {
		t.Fatalf("FullMoonOppositions: %v", err)
	}

	phases, err := MoonPhases(start, end, prov)
	if err != nil {
		t.Fatalf("MoonPhases: %v", err)
	}

	var fullMoons []MoonPhaseEvent

	for _, p := range phases {
		if p.Phase == PhaseFullMoon {
			fullMoons = append(fullMoons, p)
		}
	}

	// FullMoonOppositions (the Moon–Sun opposition in ecliptic longitude) and
	// MoonPhases' PhaseFullMoon (elongation = 180°) are the same physical
	// event, found via two different solver paths — they should agree on the
	// count and on the time. This compared counts only until #413, which let
	// FullMoonOppositions' right-ascension solution sit up to 3.25 hours off.
	//
	// The minute allows for MoonPhases' geometric positions, which run about
	// 40 s behind the apparent ones FullMoonOppositions uses (#430).
	if len(events) != len(fullMoons) {
		t.Fatalf("FullMoonOppositions found %d events, MoonPhases found %d PhaseFullMoon events; want equal",
			len(events), len(fullMoons))
	}

	for i := range events {
		if off := events[i].Time.Sub(fullMoons[i].Time).Minutes(); math.Abs(off) > 1 {
			t.Errorf("Full Moon %d: FullMoonOppositions %v, MoonPhases %v (%+.1f minutes)",
				i, events[i].Time, fullMoons[i].Time, off)
		}
	}
}

func TestVisibilityEvents_Star(t *testing.T) {
	site, err := NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	star := NewStar("Test Star", angle.Deg(100), angle.Deg(-10))

	start := time.Date(2026, 6, 15, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(1))

	events, err := VisibilityEvents(start, end, star, site)
	if err != nil {
		t.Fatalf("VisibilityEvents: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one rise/transit/set event in a 24h window")
	}

	for i, e := range events {
		if i > 0 && !e.Time.After(events[i-1].Time) {
			t.Errorf("event %d out of chronological order", i)
		}
	}
}

func TestNextNewMoonAndNextFullMoon(t *testing.T) {
	prov := eph.Default()

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	newMoon, err := NextNewMoon(start, prov)
	if err != nil {
		t.Fatalf("NextNewMoon: %v", err)
	}

	if newMoon.Time.Before(start) || newMoon.Time.Sub(start) > unit.Days(30) {
		t.Errorf("NextNewMoon = %v, want within 30 days after %v", newMoon.Time, start)
	}

	fullMoon, err := NextFullMoon(start, prov)
	if err != nil {
		t.Fatalf("NextFullMoon: %v", err)
	}

	if fullMoon.Time.Before(start) || fullMoon.Time.Sub(start) > unit.Days(30) {
		t.Errorf("NextFullMoon = %v, want within 30 days after %v", fullMoon.Time, start)
	}

	// Cross-check against MoonPhases over a matching window.
	phases, err := MoonPhases(start, start.Add(unit.Days(35)), prov)
	if err != nil {
		t.Fatalf("MoonPhases: %v", err)
	}

	var wantNew, wantFull time.Time

	for _, p := range phases {
		switch p.Phase { //nolint:exhaustive // only New/Full are relevant here
		case PhaseNewMoon:
			if wantNew.IsZero() {
				wantNew = p.Time
			}
		case PhaseFullMoon:
			if wantFull.IsZero() {
				wantFull = p.Time
			}
		}
	}

	tolerance := unit.Minutes(1)

	if d := newMoon.Time.Sub(wantNew); d > tolerance || d < -tolerance {
		t.Errorf("NextNewMoon = %v, MoonPhases' first New Moon = %v (diff %v > %v)",
			newMoon.Time, wantNew, d, tolerance)
	}

	if d := fullMoon.Time.Sub(wantFull); d > tolerance || d < -tolerance {
		t.Errorf("NextFullMoon = %v, MoonPhases' first Full Moon = %v (diff %v > %v)",
			fullMoon.Time, wantFull, d, tolerance)
	}
}
