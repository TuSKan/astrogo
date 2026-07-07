package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// R30 regression: plan/phases.go had no dedicated unit test file — it was
// only exercised via plan/{usno,nasa_eclipse,astropixels}_test.go, all
// tagged `//go:build integration`, so the default `go test ./...` job never
// ran a single line of it. These are fast, offline, deterministic unit
// tests using the local SOFA-backed ephemeris (no network/integration tag).

func TestMoonPhases_KnownYear(t *testing.T) {
	prov := eph.Default()

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.AddDays(365)

	events, err := MoonPhases(start, end, prov)
	if err != nil {
		t.Fatalf("MoonPhases: %v", err)
	}

	// ~12.37 synodic months/year × 4 phases ≈ 49-50 events.
	if len(events) < 45 || len(events) > 55 {
		t.Errorf("expected ~49 phase events in a year, got %d", len(events))
	}

	// Phases must occur in chronological order and cycle
	// New→First→Full→Last, starting wherever the first detected event falls.
	cycle := []MoonPhase{PhaseNewMoon, PhaseFirstQuarter, PhaseFullMoon, PhaseLastQuarter}

	startIdx := 0

	for i, p := range cycle {
		if events[0].Phase == p {
			startIdx = i
			break
		}
	}

	for i, e := range events {
		if i > 0 && e.Time.Sub(events[i-1].Time) <= 0 {
			t.Errorf("event %d out of chronological order: %v <= %v", i, e.Time, events[i-1].Time)
		}

		want := cycle[(startIdx+i)%4]
		if e.Phase != want {
			t.Errorf("event %d: phase = %v, want %v (cycle position %d)", i, e.Phase, want, (startIdx+i)%4)
		}

		if e.Phase.String() == "" {
			t.Errorf("event %d: empty phase name", i)
		}
	}
}

func TestSeasons_KnownYear(t *testing.T) {
	prov := eph.Default()

	events, err := Seasons(2026, prov)
	if err != nil {
		t.Fatalf("Seasons: %v", err)
	}

	if len(events) != 4 {
		t.Fatalf("expected 4 season events, got %d", len(events))
	}

	wantOrder := []Season{SeasonVernalEquinox, SeasonSummerSolstice, SeasonAutumnalEquinox, SeasonWinterSolstice}
	for i, e := range events {
		if e.Season != wantOrder[i] {
			t.Errorf("event %d: season = %v, want %v", i, e.Season, wantOrder[i])
		}

		if e.Season.String() == "" {
			t.Errorf("event %d: empty season name", i)
		}

		if i > 0 && e.Time.Sub(events[i-1].Time) <= 0 {
			t.Errorf("event %d out of chronological order", i)
		}
	}

	// Vernal equinox 2026 falls March 20; sanity-check month/day, not exact time.
	y, m, d, _ := events[0].Time.Calendar()
	if y != 2026 || m != 3 || d < 19 || d > 21 {
		t.Errorf("vernal equinox = %d-%02d-%02d, want ~2026-03-20", y, m, d)
	}
}

func TestApsides_KnownYear(t *testing.T) {
	prov := eph.Default()

	events, err := Apsides(2026, prov)
	if err != nil {
		t.Fatalf("Apsides: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 apsis events (perihelion, aphelion), got %d", len(events))
	}

	peri, aph := events[0], events[1]

	if peri.Apsis != ApsisPerihelion {
		t.Errorf("events[0].Apsis = %v, want ApsisPerihelion", peri.Apsis)
	}

	if aph.Apsis != ApsisAphelion {
		t.Errorf("events[1].Apsis = %v, want ApsisAphelion", aph.Apsis)
	}

	if peri.Apsis.String() == "" || aph.Apsis.String() == "" {
		t.Error("expected non-empty Apsis.String()")
	}

	// Earth's orbit: perihelion ~0.983 AU (early Jan), aphelion ~1.017 AU (early Jul).
	if peri.Distance < 0.98 || peri.Distance > 0.99 {
		t.Errorf("perihelion distance = %.4f AU, want ~0.983", peri.Distance)
	}

	if aph.Distance < 1.01 || aph.Distance > 1.02 {
		t.Errorf("aphelion distance = %.4f AU, want ~1.017", aph.Distance)
	}

	if aph.Distance <= peri.Distance {
		t.Errorf("aphelion (%.4f) should exceed perihelion (%.4f)", aph.Distance, peri.Distance)
	}
}

func TestMoonIllumination_NewAndFull(t *testing.T) {
	prov := eph.Default()

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.AddDays(60)

	phases, err := MoonPhases(start, end, prov)
	if err != nil {
		t.Fatalf("MoonPhases: %v", err)
	}

	var sawNew, sawFull bool

	for _, p := range phases {
		frac, _, err := MoonIllumination(p.Time, prov)
		if err != nil {
			t.Fatalf("MoonIllumination at %v: %v", p.Time, err)
		}

		switch p.Phase {
		case PhaseNewMoon:
			sawNew = true

			if frac > 0.05 {
				t.Errorf("New Moon illumination = %.3f, want ~0", frac)
			}
		case PhaseFullMoon:
			sawFull = true

			if frac < 0.95 {
				t.Errorf("Full Moon illumination = %.3f, want ~1", frac)
			}
		case PhaseFirstQuarter, PhaseLastQuarter:
			if frac < 0.3 || frac > 0.7 {
				t.Errorf("%v illumination = %.3f, want ~0.5", p.Phase, frac)
			}
		}
	}

	if !sawNew || !sawFull {
		t.Fatalf("expected both New and Full moon phases in a 60-day window (sawNew=%v sawFull=%v)", sawNew, sawFull)
	}
}

// TestEclipses_KnownYear covers LunarEclipses and SolarEclipses together
// (table-driven, per .agents/rules/rules.md's style preference) since both
// share the same shape: docs/VALIDATION.md documents 2/2 detected for 2026
// against the NASA Eclipse Catalog, each within its own Gamma/latitude limit.
func TestEclipses_KnownYear(t *testing.T) {
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.AddDays(365)

	cases := []struct {
		name       string
		find       func(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error)
		wantType   EclipseType
		wantString string
		limitDeg   float64
	}{
		{"Lunar", LunarEclipses, EclipseLunar, "Lunar Eclipse", 1.58},
		{"Solar", SolarEclipses, EclipseSolar, "Solar Eclipse", 1.58},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prov := eph.Default()

			eclipses, err := c.find(start, end, prov)
			if err != nil {
				t.Fatalf("%s: %v", c.name, err)
			}

			if len(eclipses) != 2 {
				t.Fatalf("expected 2 %s eclipses in 2026, got %d", c.name, len(eclipses))
			}

			for i, e := range eclipses {
				if e.Type != c.wantType {
					t.Errorf("eclipse %d: Type = %v, want %v", i, e.Type, c.wantType)
				}

				if e.Type.String() != c.wantString {
					t.Errorf("eclipse %d: Type.String() = %q, want %q", i, e.Type.String(), c.wantString)
				}

				if e.Gamma < 0 || e.Gamma > 1 {
					t.Errorf("eclipse %d: Gamma = %.3f, want in [0,1]", i, e.Gamma)
				}

				if math.Abs(e.EclipticLatitude.Degrees()) > c.limitDeg {
					t.Errorf("eclipse %d: |ecliptic latitude| = %.3f°, want <= %.2f°",
						i, math.Abs(e.EclipticLatitude.Degrees()), c.limitDeg)
				}
			}
		})
	}
}

func TestEclipseType_StringUnknown(t *testing.T) {
	var e EclipseType = 99
	if got := e.String(); got == "Lunar Eclipse" || got == "Solar Eclipse" {
		t.Errorf("expected an unknown-value string for out-of-range EclipseType, got %q", got)
	}
}
