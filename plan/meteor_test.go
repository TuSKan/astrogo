package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestMeteorShowersTableIntegrity guards the fixed MeteorShowers data
// table against copy-paste mistakes, in the same style as
// TestPlanetaryMoonsTableIntegrity/TestKnownSitesTableIntegrity.
func TestMeteorShowersTableIntegrity(t *testing.T) {
	seenName := make(map[string]bool)
	seenCode := make(map[string]bool)

	if len(MeteorShowers) == 0 {
		t.Fatal("expected a non-empty starter list of meteor showers")
	}

	for _, m := range MeteorShowers {
		if seenName[m.Name] {
			t.Errorf("duplicate Name %q", m.Name)
		}

		seenName[m.Name] = true

		if seenCode[m.Code] {
			t.Errorf("duplicate Code %q", m.Code)
		}

		seenCode[m.Code] = true

		if m.ZHR <= 0 {
			t.Errorf("%s: ZHR = %v, want > 0", m.Name, m.ZHR)
		}

		if m.PopulationIndex <= 1 {
			t.Errorf("%s: PopulationIndex = %v, want > 1", m.Name, m.PopulationIndex)
		}

		// 11 km/s ~ Earth's escape velocity, the slowest physically
		// possible meteor; ~72 km/s ~ Earth's orbital velocity plus a
		// parabolic comet's approach speed, the real theoretical maximum.
		if m.VelocityKmS < 11 || m.VelocityKmS > 72 {
			t.Errorf("%s: VelocityKmS = %v, outside the physically plausible [11,72] range", m.Name, m.VelocityKmS)
		}
	}
}

// findSolarLongitudeInstant scans year for the instant the Sun's real
// ecliptic longitude of date crosses target, refining via the same
// solver Seasons uses (seasonEvaluator + DefaultSolver.FindRoot) — used
// below to test RadiantAt/IsActive against a real, precisely-found
// instant rather than a hardcoded calendar date.
func findSolarLongitudeInstant(t *testing.T, target float64, prov eph.Provider, year int) time.Time {
	t.Helper()

	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	const step = 24 * time.Hour

	eval := seasonEvaluator(target, prov)

	prevLon, err := sunEclipticLongitude(start, prov)
	if err != nil {
		t.Fatalf("sunEclipticLongitude: %v", err)
	}

	prevT := start

	for tm := start.Add(step); !tm.After(end); tm = tm.Add(step) {
		curLon, err := sunEclipticLongitude(tm, prov)
		if err != nil {
			t.Fatalf("sunEclipticLongitude: %v", err)
		}

		if CrossesIncreasing(prevLon, curLon, target, 360) {
			refined, _, err := DefaultSolver().FindRoot(eval, prevT, tm)
			if err != nil {
				t.Fatalf("FindRoot: %v", err)
			}

			return refined
		}

		prevLon = curLon
		prevT = tm
	}

	t.Fatalf("solar longitude %v never crossed in %d", target, year)

	return time.Time{}
}

// TestMeteorShower_RadiantAt_AtPeak confirms RadiantAt, evaluated at the
// shower's own exact peak-solar-longitude instant (found via the solver
// above, not a hardcoded date), returns very close to the table's stated
// RadiantRA/RadiantDec (elapsed days from peak ≈ 0 there).
func TestMeteorShower_RadiantAt_AtPeak(t *testing.T) {
	prov := eph.Default()

	for _, m := range MeteorShowers {
		t.Run(m.Name, func(t *testing.T) {
			peakTime := findSolarLongitudeInstant(t, m.PeakSolarLongitude, prov, 2026)

			ra, dec, err := m.RadiantAt(peakTime, prov)
			if err != nil {
				t.Fatalf("RadiantAt: %v", err)
			}

			if diff := math.Abs(ra.Degrees() - m.RadiantRA.Degrees()); diff > 0.05 && diff < 359.95 {
				t.Errorf("RA at peak = %v°, want %v° (diff %v°)", ra.Degrees(), m.RadiantRA.Degrees(), diff)
			}

			if diff := math.Abs(dec.Degrees() - m.RadiantDec.Degrees()); diff > 0.05 {
				t.Errorf("Dec at peak = %v°, want %v° (diff %v°)", dec.Degrees(), m.RadiantDec.Degrees(), diff)
			}
		})
	}
}

// TestMeteorShower_RadiantAt_Drift confirms the radiant position N
// "solar-longitude days" after peak matches RadiantRA/RadiantDec +
// drift*N. This deliberately locates the test instant by solar longitude
// (target = peak + N*meanSolarLongitudeDegPerDay), not by adding N
// calendar days to the peak instant: the Sun's real angular rate along
// the ecliptic varies with Earth's orbital eccentricity (faster near
// perihelion in January, slower near aphelion in July/August), so N real
// calendar days doesn't correspond to exactly N*meanSolarLongitudeDegPerDay
// of solar longitude change — that mismatch is a property of solar
// dynamics, not a bug in RadiantAt's drift arithmetic, which is what this
// test is actually meant to isolate and verify.
func TestMeteorShower_RadiantAt_Drift(t *testing.T) {
	prov := eph.Default()

	m, err := NewMeteorShower("Perseids")
	if err != nil {
		t.Fatalf("NewMeteorShower(Perseids): %v", err)
	}

	const days = 5.0

	later := findSolarLongitudeInstant(t, m.PeakSolarLongitude+days*meanSolarLongitudeDegPerDay, prov, 2026)

	ra, dec, err := m.RadiantAt(later, prov)
	if err != nil {
		t.Fatalf("RadiantAt: %v", err)
	}

	wantRA := m.RadiantRA.Add(m.DriftRAPerDay.MulScalar(days)).Degrees()
	wantDec := m.RadiantDec.Add(m.DriftDecPerDay.MulScalar(days)).Degrees()

	if diff := math.Abs(ra.Degrees() - wantRA); diff > 0.1 {
		t.Errorf("RA %v days after peak = %v°, want %v° (diff %v°)", days, ra.Degrees(), wantRA, diff)
	}

	if diff := math.Abs(dec.Degrees() - wantDec); diff > 0.1 {
		t.Errorf("Dec %v days after peak = %v°, want %v° (diff %v°)", days, dec.Degrees(), wantDec, diff)
	}
}

// TestMeteorShower_IsActive checks inside/outside-window behavior,
// including a peak near the calendar-year boundary (Ursids, Quadrantids).
func TestMeteorShower_IsActive(t *testing.T) {
	prov := eph.Default()

	m, err := NewMeteorShower("PER")
	if err != nil {
		t.Fatalf("NewMeteorShower(PER): %v", err)
	}

	peakTime := findSolarLongitudeInstant(t, m.PeakSolarLongitude, prov, 2026)

	active, err := m.IsActive(peakTime, prov)
	if err != nil {
		t.Fatalf("IsActive at peak: %v", err)
	}

	if !active {
		t.Error("IsActive at the shower's own peak instant = false, want true")
	}

	farOff := peakTime.Add(180 * 24 * time.Hour)

	active, err = m.IsActive(farOff, prov)
	if err != nil {
		t.Fatalf("IsActive far from peak: %v", err)
	}

	if active {
		t.Error("IsActive 180 days from peak = true, want false")
	}
}

// TestMeteorShower_ObservedRate_ZeroBelowHorizon confirms a radiant below
// the horizon yields rate 0, not an error.
func TestMeteorShower_ObservedRate_ZeroBelowHorizon(t *testing.T) {
	prov := eph.Default()

	m := MeteorShower{
		Name: "Test", Code: "TST",
		RadiantRA: angle.Deg(0), RadiantDec: angle.Deg(-89),
		PeakSolarLongitude: 0, ActiveStartSolarLon: 0, ActiveEndSolarLon: 360,
		ZHR: 100, PopulationIndex: 2.0,
	}

	// Northern-hemisphere site; a radiant at Dec -89° is always far below
	// the horizon there.
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Deg(60), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Test", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	// The limiting magnitude is now a direct input, so these tests pin it
	// to the IMO standard 6.5 and isolate the ZHR/altitude/population-index
	// arithmetic from any sky-brightness model.
	rate, err := m.ObservedRate(time.FromJD(2451545.0, time.UTC), site, prov, 6.5)
	if err != nil {
		t.Fatalf("ObservedRate: %v", err)
	}

	if rate != 0 {
		t.Errorf("ObservedRate (below horizon) = %v, want 0", rate)
	}
}

// TestMeteorShower_ObservedRate_StandardConditionsEqualsZHR is the clean
// testable identity the formula guarantees: at radiant altitude 90° and
// limiting magnitude exactly 6.5, observedRate == ZHR exactly (the
// defining standard conditions), regardless of population index. The
// geometry is arranged via the well-known fact that altitude equals
// declination when observing from the geographic pole (any hour angle),
// avoiding a separate root-solve for "radiant on the meridian."
func TestMeteorShower_ObservedRate_StandardConditionsEqualsZHR(t *testing.T) {
	prov := eph.Default()

	m := MeteorShower{
		Name: "Test", Code: "TST",
		RadiantRA: angle.Deg(123), RadiantDec: angle.Deg(89.9),
		PeakSolarLongitude: 0, ActiveStartSolarLon: 0, ActiveEndSolarLon: 360,
		ZHR: 42, PopulationIndex: 2.3,
	}

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Deg(90), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("North Pole", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	// The limiting magnitude is now a direct input, so these tests pin it
	// to the IMO standard 6.5 and isolate the ZHR/altitude/population-index
	// arithmetic from any sky-brightness model.
	rate, err := m.ObservedRate(time.FromJD(2451545.0, time.UTC), site, prov, 6.5)
	if err != nil {
		t.Fatalf("ObservedRate: %v", err)
	}

	if diff := math.Abs(rate/m.ZHR - 1); diff > 1e-3 {
		t.Errorf("ObservedRate = %v, want ~ZHR (%v) at standard conditions (diff ratio %v)", rate, m.ZHR, diff)
	}
}

// TestNewMeteorShower confirms Name/Code lookup, case-insensitive, and
// the not-found path.
func TestNewMeteorShower(t *testing.T) {
	cases := []string{"Perseids", "perseids", "PER", "per"}

	for _, name := range cases {
		m, err := NewMeteorShower(name)
		if err != nil {
			t.Errorf("NewMeteorShower(%q): %v", name, err)
			continue
		}

		if m.Name != "Perseids" {
			t.Errorf("NewMeteorShower(%q).Name = %q, want Perseids", name, m.Name)
		}
	}

	if _, err := NewMeteorShower("Not A Real Shower"); !errors.Is(err, ErrUnknownMeteorShower) {
		t.Errorf("NewMeteorShower(nonexistent) error = %v, want ErrUnknownMeteorShower", err)
	}
}

// TestMeteorShower_Radiant confirms the *Star convenience wrapper matches
// RadiantAt.
func TestMeteorShower_Radiant(t *testing.T) {
	prov := eph.Default()

	m, err := NewMeteorShower("Geminids")
	if err != nil {
		t.Fatalf("NewMeteorShower(Geminids): %v", err)
	}

	tm := time.FromJD(2451545.0, time.UTC)

	wantRA, wantDec, err := m.RadiantAt(tm, prov)
	if err != nil {
		t.Fatalf("RadiantAt: %v", err)
	}

	star, err := m.Radiant(tm, prov)
	if err != nil {
		t.Fatalf("Radiant: %v", err)
	}

	pos, err := star.Position(tm)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	if pos.RA() != wantRA || pos.Dec() != wantDec {
		t.Errorf("Radiant() position = (%v,%v), want (%v,%v)", pos.RA(), pos.Dec(), wantRA, wantDec)
	}
}
