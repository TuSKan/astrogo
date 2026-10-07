package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// jenniskensFit is one shower's activity fit as Jenniskens (1994, A&A 287,
// 990) prints it, transcribed here independently of meteorShowers so that a
// slip in either shows: Table 3c's main peak (ZHRᵖ, Bᵖ) and background
// (ZHRᵇ, Bᵇ⁺, Bᵇ⁻) where he fits two curves, and Table 3b's single curve,
// with zhrB zero, otherwise.
type jenniskensFit struct {
	zhrP, peakRise, peakFall float64
	zhrB, bgRise, bgFall     float64
}

var jenniskensFits = map[string]jenniskensFit{
	"quadrantids":              {110, 2.5, 2.5, 20, 0.37, 0.45},    // Table 3c, Boo
	"lyrids":                   {12.8, 0.22, 0.22, 0, 0, 0},        // Table 3b, Lyr
	"eta_aquariids":            {36.7, 0.080, 0.080, 0, 0, 0},      // Table 3b, eAq
	"southern_delta_aquariids": {11.4, 0.091, 0.091, 0, 0, 0},      // Table 3b, dAZ
	"perseids":                 {70, 0.35, 0.35, 23, 0.050, 0.092}, // Table 3c, Per
	"orionids":                 {25, 0.12, 0.12, 0, 0, 0},          // Table 3b, Ori
	"leonids":                  {23, 0.39, 0.39, 0, 0, 0},          // Table 3b, Leo
	"geminids":                 {74, 0.59, 0.81, 18, 0.09, 0.31},   // Table 3c, Gem
	"ursids":                   {10, 0.9, 0.9, 2.0, 0.08, 0.2},     // Table 3c, Urs
}

// activity is the fit's ZHR at delta degrees of solar longitude from the
// maximum, as a fraction of its ZHR there: his Eq. 8, summed over the
// curves he fits.
func (f jenniskensFit) activity(delta float64) float64 {
	bp, bb := f.peakFall, f.bgFall
	if delta < 0 {
		bp, bb = f.peakRise, f.bgRise
	}

	d := math.Abs(delta)

	return (f.zhrP*math.Pow(10, -bp*d) + f.zhrB*math.Pow(10, -bb*d)) / (f.zhrP + f.zhrB)
}

// TestActivityProfilesAreJenniskens holds every built-in shower's profile to
// Jenniskens' fit, either side of the maximum (#572).
func TestActivityProfilesAreJenniskens(t *testing.T) {
	t.Parallel()

	for key, fit := range jenniskensFits {
		m, ok := meteorShowers[key]
		if !ok {
			t.Errorf("%s: not in the table", key)

			continue
		}

		for _, delta := range []float64{-8, -3, -0.5, 0, 0.5, 3, 8} {
			got, want := m.Activity.at(delta), fit.activity(delta)
			if math.Abs(got-want) > 1e-12*math.Max(1, want) {
				t.Errorf("%s at %+g°: activity %.6g, Jenniskens' fit gives %.6g", key, delta, got, want)
			}
		}
	}

	if len(jenniskensFits) != len(meteorShowers) {
		t.Errorf("%d fits for %d showers", len(jenniskensFits), len(meteorShowers))
	}
}

// TestZHRAtFollowsTheSun checks ZHRAt against the profile at Skyfield's
// solar longitudes (DE440s, ecliptic_J2000_frame), on both branches of two
// showers, one with a symmetric main peak and one without.
func TestZHRAtFollowsTheSun(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, c := range []struct {
		key    string
		at     time.Time
		lambda float64 // Skyfield's λ☉ (J2000) at that instant
	}{
		{"perseids", time.Date(2026, 8, 4, 2, 0, 0, 0, time.LocationUTC), 131.3705},
		{"perseids", time.Date(2026, 8, 13, 2, 0, 33, 0, time.LocationUTC), 140.0000},
		{"perseids", time.Date(2026, 8, 20, 2, 0, 0, 0, time.LocationUTC), 146.7295},
		{"geminids", time.Date(2026, 12, 12, 0, 0, 0, 0, time.LocationUTC), 259.5861},
		{"geminids", time.Date(2026, 12, 15, 12, 0, 0, 0, time.LocationUTC), 263.1458},
	} {
		m := meteorShowers[c.key]

		got, err := m.ZHRAt(c.at, prov)
		if err != nil {
			t.Fatalf("%s: %v", c.key, err)
		}

		// astrogo's solar longitude is within 0.002° of Skyfield's, which
		// moves the steepest of these by 0.3 per cent.
		want := m.ZHR * jenniskensFits[c.key].activity(c.lambda-m.PeakSolarLongitude)
		if math.Abs(got-want) > 0.01*want {
			t.Errorf("%s at %v: ZHR %.3f, want %.3f", c.key, c.at, got, want)
		}
	}
}

// TestObservedRateFollowsTheDate is #572's reproduction: the Perseids from
// 45°N, at the naked-eye limit of 6.5. ObservedRate gave the maximum rate
// on every night of the year, 97.4 an hour on 1 March, when the shower is
// not active at all, and more nine days before the maximum (82.8) than at
// it (81.5).
func TestObservedRateFollowsTheDate(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	per := meteorShowers["perseids"]

	site, err := NewSiteEarthLocation("45N", 45, 0, 0)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	rate := func(month, day, hour int) float64 {
		t.Helper()

		r, err := per.ObservedRate(time.Date(2026, time.Month(month), day, hour, 0, 0, 0, time.LocationUTC), site, prov, 6.5)
		if err != nil {
			t.Fatalf("ObservedRate: %v", err)
		}

		return r
	}

	for _, c := range []struct{ month, day, hour int }{{1, 15, 23}, {3, 1, 3}} {
		if r := rate(c.month, c.day, c.hour); r != 0 {
			t.Errorf("2026-%02d-%02d %02dh, outside the activity window: %.1f an hour, want 0",
				c.month, c.day, c.hour, r)
		}
	}

	before, peak := rate(8, 4, 2), rate(8, 13, 2)
	if before <= 0 || before >= peak/5 {
		t.Errorf("nine days before the maximum %.1f an hour, at it %.1f: want a small fraction of it", before, peak)
	}
}

// TestAFullTurnWindowContainsEveryLongitude: [0°, 360°] wrapped to the single
// point 0°, so a shower declared active all year was active only there.
func TestAFullTurnWindowContainsEveryLongitude(t *testing.T) {
	t.Parallel()

	for _, lambda := range []float64{0, 0.5, 90, 180, 359.9} {
		if !solarLongitudeInRange(lambda, 0, 360) {
			t.Errorf("λ☉ %g° outside [0°, 360°]", lambda)
		}
	}

	// An ordinary window, and one across 0°, are unchanged.
	if solarLongitudeInRange(200, 100, 150) || !solarLongitudeInRange(120, 100, 150) {
		t.Error("[100°, 150°] misplaced 200° or 120°")
	}

	if !solarLongitudeInRange(5, 350, 10) || solarLongitudeInRange(20, 350, 10) {
		t.Error("[350°, 10°] misplaced 5° or 20°")
	}
}
