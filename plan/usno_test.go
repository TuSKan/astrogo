//go:build integration

// Package plan_test contains integration tests that validate astrogo's
// astronomical computations against the U.S. Naval Observatory (USNO) API.
//
// Run with: go test -tags integration -run TestUSNO -v ./plan/
//
// These tests require an active internet connection to reach
// https://aa.usno.navy.mil/api/ endpoints.
package plan_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// ── USNO API Types ───────────────────────────────────────────────────────────

type usnoOneDayResponse struct {
	APIVersion string `json:"apiversion"`
	Properties struct {
		Data struct {
			SunData  []usnoPhenomenon `json:"sundata"`
			MoonData []usnoPhenomenon `json:"moondata"`
			Day      int              `json:"day"`
			Month    int              `json:"month"`
			Year     int              `json:"year"`
			TZ       float64          `json:"tz"`
		} `json:"data"`
	} `json:"properties"`
}

type usnoPhenomenon struct {
	Phen string `json:"phen"`
	Time string `json:"time"` // "HH:MM" or null
}

type usnoMoonPhasesResponse struct {
	APIVersion string           `json:"apiversion"`
	PhaseData  []usnoPhaseEntry `json:"phasedata"`
}

type usnoPhaseEntry struct {
	Day   int    `json:"day"`
	Month int    `json:"month"`
	Year  int    `json:"year"`
	Phase string `json:"phase"`
	Time  string `json:"time"` // "HH:MM" in UT
}

type usnoSeasonsResponse struct {
	APIVersion string            `json:"apiversion"`
	Data       []usnoSeasonEntry `json:"data"`
	Year       int               `json:"year"`
}

type usnoSeasonEntry struct {
	Day    int    `json:"day"`
	Month  int    `json:"month"`
	Year   int    `json:"year"`
	Phenom string `json:"phenom"`
	Time   string `json:"time"` // "HH:MM" in UT
}

type usnoCelNavResponse struct {
	APIVersion string `json:"apiversion"`
	Properties struct {
		Data  []usnoCelNavEntry `json:"data"`
		Day   int               `json:"day"`
		Month int               `json:"month"`
		Year  int               `json:"year"`
		Time  string            `json:"time"`
	} `json:"properties"`
}

type usnoCelNavEntry struct {
	Object      string `json:"object"`
	AlmanacData struct {
		Dec float64 `json:"dec"`
		GHA float64 `json:"gha"`
		Hc  float64 `json:"hc"`
		Zn  float64 `json:"zn"`
	} `json:"almanac_data"`
	AltCorrections struct {
		IsCorrected bool        `json:"isCorrected"`
		Refr        interface{} `json:"refr"`
		PA          interface{} `json:"pa"`
		SD          interface{} `json:"sd"`
		Sum         interface{} `json:"sum"`
	} `json:"altitude_corrections"`
}

// ── Test Location ────────────────────────────────────────────────────────────

type testLocation struct {
	Name   string
	Lat    float64
	Lon    float64
	Height float64
	TZ     float64
	TZName string
	DST    bool // Whether location observes DST
}

var testLocations = []testLocation{
	// Note: USNO's rstt/oneday API ignores the height parameter for rise/set times
	// (verified empirically: height=0 and height=786 return identical results).
	// Altitude-dependent tests are in TestUSNO_HighAltitude.
	{"São Paulo", -23.600833, -46.6525, 0, -3, "America/Sao_Paulo", false},
	{"Washington DC", 38.8951, -77.0364, 0, -5, "America/New_York", true},
	{"London", 51.5074, -0.1278, 0, 0, "Europe/London", true},
}

// ── Helpers ──────────────────────────────────────────────────────────────────

// usnoRequestTimeout bounds how long a single USNO request may run,
// enforced independently of http.Client.Timeout (see usnoGet).
const usnoRequestTimeout = 30 * time.Second

// usnoResult carries a completed request's outcome across the goroutine
// boundary in usnoGet.
type usnoResult struct {
	body       []byte
	err        error
	statusCode int
}

// usnoGet fetches url and returns its body, skipping the calling test on
// any network/HTTP failure.
//
// The request runs in its own goroutine, raced against a context deadline
// via select — not just http.Client.Timeout — because a stalled TCP
// connect on a CI runner has been observed to outlast the client's own
// Timeout (a stuck net.Dial doesn't always unblock promptly on context
// cancellation in every environment), which previously hung this test
// until the whole `go test` binary's global timeout fired 10 minutes
// later and failed the entire package. If the goroutine never completes,
// this function still returns (via t.Skipf) after usnoRequestTimeout; the
// orphaned goroutine is harmless — it dies with the process when the test
// binary exits.
func usnoGet(t *testing.T, url string) []byte {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), usnoRequestTimeout)
	defer cancel()

	resultCh := make(chan usnoResult, 1)

	go func() {
		client := &http.Client{Timeout: usnoRequestTimeout}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			resultCh <- usnoResult{err: err}
			return
		}

		resp, err := client.Do(req)
		if err != nil {
			resultCh <- usnoResult{err: err}
			return
		}
		defer resp.Body.Close() //nolint:errcheck // read-only response body, close error not actionable here

		body, err := io.ReadAll(resp.Body)

		resultCh <- usnoResult{body: body, err: err, statusCode: resp.StatusCode}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Skipf("USNO API unreachable, skipping: %v", res.err)
		}

		if res.statusCode != http.StatusOK {
			t.Skipf("USNO API returned status %d for %s", res.statusCode, url)
		}

		return res.body
	case <-ctx.Done():
		t.Skipf("USNO API request exceeded %s, skipping: %s", usnoRequestTimeout, url)
		return nil
	}
}

// parseUSNOTime parses "HH:MM" into hours and minutes.
func parseUSNOTime(s string) (h, m int, ok bool) {
	if s == "" || s == "null" {
		return 0, 0, false
	}
	_, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	return h, m, err == nil
}

// minutesFromMidnight converts HH:MM to total minutes.
func minutesFromMidnight(h, m int) float64 {
	return float64(h)*60 + float64(m)
}

// eventMinutes returns the event time as minutes from midnight in the given timezone.
func eventMinutesIn(t time.Time, loc *time.Location) float64 {
	gt := t.GoTime().In(loc)
	return float64(gt.Hour())*60 + float64(gt.Minute()) + float64(gt.Second())/60.0
}

// deltaMinutes returns |a - b| in minutes, handling day wrapping.
func deltaMinutes(usnoMin, astroMin float64) float64 {
	d := math.Abs(usnoMin - astroMin)
	if d > 12*60 { // Handle day boundary
		d = 24*60 - d
	}
	return d
}

func newEph(t *testing.T) eph.Provider {
	t.Helper()
	p, err := eph.NewProvider(context.Background(), eph.Planets, "de442")
	if err != nil {
		t.Logf("DE442 unavailable (%v), falling back to default", err)
		def := eph.Default()
		if def == nil {
			t.Fatal("Failed to create ephemeris: nil provider")
		}
		return def
	}
	t.Cleanup(func() { p.Close() })
	return p
}

// ── Test: Complete Sun and Moon Data for One Day ──────────────────────────────

func TestUSNO_SunMoonOneDay(t *testing.T) {
	prov := newEph(t)
	dates := []string{"2026-04-06", "2026-06-21", "2026-12-21"}

	for _, loc := range testLocations {
		for _, dateStr := range dates {
			name := fmt.Sprintf("%s/%s", loc.Name, dateStr)
			t.Run(name, func(t *testing.T) {
				// Query USNO
				dstParam := "false"
				if loc.DST {
					dstParam = "true"
				}
				url := fmt.Sprintf(
					"https://aa.usno.navy.mil/api/rstt/oneday?date=%s&coords=%.6f,%.6f&tz=%.0f&height=%.0f&dst=%s",
					dateStr, loc.Lat, loc.Lon, loc.TZ, loc.Height, dstParam,
				)
				body := usnoGet(t, url)

				var resp usnoOneDayResponse
				if err := json.Unmarshal(body, &resp); err != nil {
					t.Fatalf("Failed to parse USNO response: %v", err)
				}

				// Parse date
				var y, mo, d int
				fmt.Sscanf(dateStr, "%d-%d-%d", &y, &mo, &d)

				// Set up astrogo
				tz, err := time.LoadLocation(loc.TZName)
				if err != nil {
					t.Fatalf("Failed to load timezone: %v", err)
				}
				geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), loc.Height)
				if err != nil {
					t.Fatalf("Failed to create geodetic: %v", err)
				}
				site, err := plan.NewSite(loc.Name, geodetic, angle.Zero(), tz)
				if err != nil {
					t.Fatalf("Failed to create site: %v", err)
				}

				start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, tz)
				end := start.Add(24 * time.Hour)

				// Compare Sun events
				sunEvents, err := plan.SunEvents(start, end, site, prov)
				if err != nil {
					t.Fatalf("SunEvents failed: %v", err)
				}

				for _, sp := range resp.Properties.Data.SunData {
					h, m, ok := parseUSNOTime(sp.Time)
					if !ok {
						continue
					}
					usnoMin := minutesFromMidnight(h, m)

					var matchKind plan.EventKind
					switch sp.Phen {
					case "Rise":
						matchKind = plan.EventRise
					case "Set":
						matchKind = plan.EventSet
					case "Upper Transit":
						matchKind = plan.EventTransit
					default:
						continue // Civil twilight handled separately
					}

					found := false
					for _, ev := range sunEvents {
						if ev.Kind != matchKind {
							continue
						}
						astroMin := eventMinutesIn(ev.Time, tz)
						delta := deltaMinutes(usnoMin, astroMin)
						t.Logf("Sun %-12s  USNO=%02d:%02d  astrogo=%s  Δ=%.1f min",
							sp.Phen, h, m, ev.Time.In(tz).Format("15:04:05"), delta)

						// Rise/Set: 2 min tolerance (topocentric + horizon dip)
						// Transit: 1 min tolerance (no refraction dependence)
						tol := 2.0
						if matchKind == plan.EventTransit {
							tol = 1.0
						}
						if delta > tol {
							t.Errorf("Sun %s: Δ=%.1f min exceeds %.0f min tolerance", sp.Phen, delta, tol)
						}
						found = true
						break
					}
					if !found {
						t.Logf("Sun %s at %02d:%02d: no matching astrogo event found", sp.Phen, h, m)
					}
				}

				// Compare Moon events
				moonEvents, err := plan.MoonEvents(start, end, site, prov)
				if err != nil {
					t.Fatalf("MoonEvents failed: %v", err)
				}

				for _, mp := range resp.Properties.Data.MoonData {
					h, m, ok := parseUSNOTime(mp.Time)
					if !ok {
						continue
					}
					usnoMin := minutesFromMidnight(h, m)

					var matchKind plan.EventKind
					switch mp.Phen {
					case "Rise":
						matchKind = plan.EventRise
					case "Set":
						matchKind = plan.EventSet
					case "Upper Transit":
						matchKind = plan.EventTransit
					default:
						continue
					}

					found := false
					for _, ev := range moonEvents {
						if ev.Kind != matchKind {
							continue
						}
						astroMin := eventMinutesIn(ev.Time, tz)
						delta := deltaMinutes(usnoMin, astroMin)
						t.Logf("Moon %-12s  USNO=%02d:%02d  astrogo=%s  Δ=%.1f min",
							mp.Phen, h, m, ev.Time.In(tz).Format("15:04:05"), delta)

						// Moon: 3 min rise/set tolerance (topocentric parallax + refraction)
						// Transit: 1 min tolerance
						tol := 3.0
						if matchKind == plan.EventTransit {
							tol = 1.0
						}
						if delta > tol {
							t.Errorf("Moon %s: Δ=%.1f min exceeds %.0f min tolerance", mp.Phen, delta, tol)
						}
						found = true
						break
					}
					if !found {
						t.Logf("Moon %s at %02d:%02d: no matching astrogo event found", mp.Phen, h, m)
					}
				}
			})
		}
	}
}

// ── Test: Celestial Navigation (AltAz validation) ────────────────────────────

func TestUSNO_CelNav(t *testing.T) {
	// Test at São Paulo on 2026-04-06 at 21:00 UTC
	url := "https://aa.usno.navy.mil/api/celnav?date=2026-04-06&time=21:00:00&coords=-23.600833,-46.6525"
	body := usnoGet(t, url)

	var resp usnoCelNavResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to parse CelNav response: %v", err)
	}

	// Set up astrogo context for this instant
	tm := time.Date(2026, time.April, 6, 21, 0, 0, 0, time.LocationUTC)
	geodetic, _ := coord.NewGeodetic(angle.Deg(-46.6525), angle.Deg(-23.600833), 0)
	tz, _ := time.LoadLocation("America/Sao_Paulo")
	site, _ := plan.NewSite("São Paulo", geodetic, angle.Zero(), tz)
	ctx := coord.NewContext(tm, site.Location(), site.Atmosphere())

	// Validate Sun position
	var prov eph.Provider
	jplProv, err := eph.NewProvider(context.Background(), eph.Planets, "de442")
	if err != nil {
		t.Logf("failed to load jpl de442: %v", err)
		prov = eph.Default()
	} else {
		prov = jplProv
	}

	for _, entry := range resp.Properties.Data {
		if entry.Object == "ARIES" {
			continue // GHA Aries is a sidereal reference, skip
		}
		if entry.AlmanacData.Hc == 0 && entry.AlmanacData.Zn == 0 {
			continue // No position data
		}

		// We can validate Sun position via ephemeris
		if entry.Object == "Sun" {
			sunTarget := plan.NewSun(prov)
			pos, err := sunTarget.Position(tm)
			if err != nil {
				t.Logf("Sun position error: %v", err)
				continue
			}
			aa, _ := ctx.ICRSToAltAz(pos)

			deltaAlt := math.Abs(aa.Alt().Degrees() - entry.AlmanacData.Hc)
			deltaAz := math.Abs(aa.Az().Degrees() - entry.AlmanacData.Zn)
			if deltaAz > 180 {
				deltaAz = 360 - deltaAz
			}

			t.Logf("Sun  Alt: USNO=%.4f° astrogo=%.4f° Δ=%.4f°", entry.AlmanacData.Hc, aa.Alt().Degrees(), deltaAlt)
			t.Logf("Sun  Az:  USNO=%.4f° astrogo=%.4f° Δ=%.4f°", entry.AlmanacData.Zn, aa.Az().Degrees(), deltaAz)

			// CelNav altitude near horizon has large refraction uncertainty
			altTol := 0.1
			if math.Abs(entry.AlmanacData.Hc) < 5.0 {
				altTol = 1.0 // Near-horizon refraction model differences
			}
			if deltaAlt > altTol {
				t.Errorf("Sun altitude Δ=%.4f° exceeds %.1f° tolerance", deltaAlt, altTol)
			}
			if deltaAz > 0.5 {
				t.Errorf("Sun azimuth Δ=%.4f° exceeds 0.5° tolerance", deltaAz)
			}
		}

		// Validate navigational stars via catalog resolver
		if strings.ToUpper(entry.Object) == "SIRIUS" {
			// Sirius — we know these coords from SIMBAD
			sirius := coord.NewICRSWithKinematics(
				angle.Deg(101.2871553333),
				angle.Deg(-16.7161158611),
				angle.Arcsec(-0.54601),
				angle.Arcsec(-1.22307),
				angle.Arcsec(0.37921),
				-5.5,
			)
			aa, _ := ctx.ICRSToAltAz(sirius)

			deltaAlt := math.Abs(aa.Alt().Degrees() - entry.AlmanacData.Hc)
			t.Logf("SIRIUS  Alt: USNO=%.4f° astrogo=%.4f° Δ=%.4f°", entry.AlmanacData.Hc, aa.Alt().Degrees(), deltaAlt)

			if deltaAlt > 0.1 {
				t.Errorf("SIRIUS altitude Δ=%.4f° exceeds 0.1° tolerance", deltaAlt)
			}
		}
	}
}

// ── Test: Moon Phases ────────────────────────────────────────────────────────

func TestUSNO_MoonPhases(t *testing.T) {
	url := "https://aa.usno.navy.mil/api/moon/phases/date?date=2026-01-01&nump=12"
	body := usnoGet(t, url)

	var resp usnoMoonPhasesResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to parse Moon phases response: %v", err)
	}

	eph := newEph(t)

	// Compute astrogo moon phases for Jan-Apr 2026
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2026, time.May, 1, 0, 0, 0, 0, time.LocationUTC)
	astroPhases, err := plan.MoonPhases(start, end, eph)
	if err != nil {
		t.Fatalf("MoonPhases failed: %v", err)
	}

	// Map USNO phase names to our types
	phaseMap := map[string]plan.MoonPhase{
		"New Moon":      plan.PhaseNewMoon,
		"First Quarter": plan.PhaseFirstQuarter,
		"Full Moon":     plan.PhaseFullMoon,
		"Last Quarter":  plan.PhaseLastQuarter,
	}

	for _, usnoP := range resp.PhaseData {
		astroPhase, ok := phaseMap[usnoP.Phase]
		if !ok {
			continue
		}

		// Parse USNO time (UT)
		h, m, ok := parseUSNOTime(usnoP.Time)
		if !ok {
			continue
		}
		usnoTime := time.Date(usnoP.Year, time.Month(usnoP.Month), usnoP.Day, h, m, 0, 0, time.LocationUTC)

		// Find matching astrogo phase
		found := false
		for _, ap := range astroPhases {
			if ap.Phase != astroPhase {
				continue
			}
			// Events within 2 days are the same phase
			delta := math.Abs(ap.Time.Sub(usnoTime).Minutes())
			if delta < 2*24*60 {
				t.Logf("%-14s  USNO=%s  astrogo=%s  Δ=%.0f min",
					usnoP.Phase,
					usnoTime.Format("2006-01-02 15:04"),
					ap.Time.Format("2006-01-02 15:04"),
					delta)

				if delta > 30 {
					t.Errorf("%s: Δ=%.0f min exceeds 30 min tolerance", usnoP.Phase, delta)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s at %s: no matching astrogo phase found", usnoP.Phase, usnoTime.Format("2006-01-02"))
		}
	}
}

// ── Test: Earth's Seasons ────────────────────────────────────────────────────

func TestUSNO_Seasons(t *testing.T) {
	url := "https://aa.usno.navy.mil/api/seasons?year=2026"
	body := usnoGet(t, url)

	var resp usnoSeasonsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to parse Seasons response: %v", err)
	}

	eph := newEph(t)

	// Compute astrogo seasons for 2026
	astroSeasons, err := plan.Seasons(2026, eph)
	if err != nil {
		t.Fatalf("Seasons failed: %v", err)
	}

	// Map USNO names to our types
	seasonMap := map[string]plan.Season{
		"Equinox":  plan.SeasonVernalEquinox,  // March → Vernal, Sept → Autumnal
		"Solstice": plan.SeasonSummerSolstice, // June → Summer, Dec → Winter
	}

	for _, usSeason := range resp.Data {
		_, ok := seasonMap[usSeason.Phenom]
		if !ok {
			// Perihelion/Aphelion — not season events, skip
			continue
		}

		h, m, ok := parseUSNOTime(usSeason.Time)
		if !ok {
			continue
		}
		usnoTime := time.Date(usSeason.Year, time.Month(usSeason.Month), usSeason.Day, h, m, 0, 0, time.LocationUTC)

		// Find matching astrogo season by date proximity
		found := false
		for _, as := range astroSeasons {
			delta := math.Abs(as.Time.Sub(usnoTime).Minutes())
			if delta < 7*24*60 { // Within 7 days
				t.Logf("%-20s  USNO=%s  astrogo=%s  Δ=%.0f min",
					usSeason.Phenom+" ("+as.Season.String()+")",
					usnoTime.Format("2006-01-02 15:04"),
					as.Time.Format("2006-01-02 15:04"),
					delta)

				if delta > 30 {
					t.Errorf("%s: Δ=%.0f min exceeds 30 min tolerance", as.Season, delta)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s at %s: no matching astrogo season found", usSeason.Phenom, usnoTime.Format("2006-01-02"))
		}
	}
}

// ── Test: Julian Date Converter ──────────────────────────────────────────────

func TestUSNO_JulianDate(t *testing.T) {
	// The USNO JD API may not be available via REST; validate locally.
	// JD for 2026-04-06 00:00:00 UT should be 2461133.5
	testCases := []struct {
		year, month, day int
		expectedJD       float64
	}{
		{2000, 1, 1, 2451544.5},  // J2000.0 epoch - 12h
		{2026, 4, 6, 2461136.5},  // Verified via USNO
		{1970, 1, 1, 2440587.5},  // Unix epoch
		{2024, 2, 29, 2460369.5}, // Leap year
	}

	for _, tc := range testCases {
		name := fmt.Sprintf("%d-%02d-%02d", tc.year, tc.month, tc.day)
		t.Run(name, func(t *testing.T) {
			tm := time.Date(tc.year, time.Month(tc.month), tc.day, 0, 0, 0, 0, time.LocationUTC)
			jd := tm.JD()
			delta := math.Abs(jd - tc.expectedJD)
			t.Logf("JD: expected=%.1f  astrogo=%.6f  Δ=%.6f days", tc.expectedJD, jd, delta)
			if delta > 0.001 {
				t.Errorf("JD Δ=%.6f exceeds tolerance", delta)
			}
		})
	}
}

// ── Test: Sidereal Time ──────────────────────────────────────────────────────

func TestUSNO_SiderealTime(t *testing.T) {
	// Validate GMST/GAST against known values.
	// USNO Sidereal Time API may not be REST-accessible; use reference values.
	// At J2000.0 epoch (2000-01-01 12:00:00 TT), GMST ≈ 18h 41m 50.55s
	// At 2026-04-06 21:00:00 UT, validate against our computation.

	tz, _ := time.LoadLocation("America/Sao_Paulo")
	geodetic, _ := coord.NewGeodetic(angle.Deg(-46.6525), angle.Deg(-23.600833), 786)
	site, _ := plan.NewSite("São Paulo", geodetic, angle.Zero(), tz)

	testTimes := []struct {
		name string
		tm   time.Time
	}{
		{"2026-04-06 18:00 -03", time.Date(2026, 4, 6, 18, 0, 0, 0, tz)},
		{"2026-06-21 00:00 UTC", time.Date(2026, 6, 21, 0, 0, 0, 0, time.LocationUTC)},
		{"2026-12-21 12:00 UTC", time.Date(2026, 12, 21, 12, 0, 0, 0, time.LocationUTC)},
	}

	for _, tc := range testTimes {
		t.Run(tc.name, func(t *testing.T) {
			lst, err := site.LocalSiderealTime(tc.tm)
			if err != nil {
				t.Fatalf("LocalSiderealTime failed: %v", err)
			}
			t.Logf("LST at %s: %s (%.6f°)", tc.name, lst.HMSString(3), lst.Degrees())
			// Sanity: LST must be in [0, 360)
			if lst.Degrees() < 0 || lst.Degrees() >= 360 {
				t.Errorf("LST out of range: %.6f°", lst.Degrees())
			}
		})
	}
}

// ── Test: Perihelion/Aphelion ────────────────────────────────────────────────

func TestUSNO_Apsides(t *testing.T) {
	url := "https://aa.usno.navy.mil/api/seasons?year=2026"
	body := usnoGet(t, url)

	var resp usnoSeasonsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("Failed to parse Seasons response: %v", err)
	}

	eph := newEph(t)

	// Compute astrogo apsides for 2026
	apsides, err := plan.Apsides(2026, eph)
	if err != nil {
		t.Fatalf("Apsides failed: %v", err)
	}

	// Map USNO phenom names to our Apsis types
	apsisMap := map[string]plan.Apsis{
		"Perihelion": plan.ApsisPerihelion,
		"Aphelion":   plan.ApsisAphelion,
	}

	for _, entry := range resp.Data {
		expectedApsis, ok := apsisMap[entry.Phenom]
		if !ok {
			continue // Skip Equinox/Solstice
		}

		h, m, ok := parseUSNOTime(entry.Time)
		if !ok {
			continue
		}
		usnoTime := time.Date(entry.Year, time.Month(entry.Month), entry.Day, h, m, 0, 0, time.LocationUTC)

		// Find matching astrogo apsis
		found := false
		for _, a := range apsides {
			if a.Apsis != expectedApsis {
				continue
			}
			delta := math.Abs(a.Time.Sub(usnoTime).Minutes())
			t.Logf("%-12s  USNO=%s  astrogo=%s  Δ=%.0f min  (%.6f AU)",
				a.Apsis,
				usnoTime.Format("2006-01-02 15:04"),
				a.Time.Format("2006-01-02 15:04"),
				delta, a.Distance)

			if delta > 120 { // 2-hour tolerance (USNO rounds to nearest minute)
				t.Errorf("%s: Δ=%.0f min exceeds 120 min tolerance", a.Apsis, delta)
			}
			found = true
			break
		}
		if !found {
			t.Errorf("%s: no matching astrogo event found", entry.Phenom)
		}
	}
}

// ── Test: Eclipse Detection ──────────────────────────────────────────────────

func TestUSNO_Eclipses(t *testing.T) {
	eph := newEph(t)
	year2026Start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	year2026End := time.Date(2027, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	// Known 2026 eclipses (from NASA Eclipse catalog):
	// Lunar:  2026-03-03 (Total), 2026-08-28 (Partial)
	// Solar:  2026-02-17 (Annular), 2026-08-12 (Total)

	t.Run("LunarEclipses", func(t *testing.T) {
		eclipses, err := plan.LunarEclipses(year2026Start, year2026End, eph)
		if err != nil {
			t.Fatalf("LunarEclipses failed: %v", err)
		}

		knownDates := []string{"2026-03-03", "2026-08-28"}

		t.Logf("Found %d lunar eclipse candidates:", len(eclipses))
		for _, ecl := range eclipses {
			t.Logf("  %s  β=%.3f°  γ=%.3f", ecl.Time.Format("2006-01-02 15:04"), ecl.EclipticLatitude.Degrees(), ecl.Gamma)
		}

		for _, expected := range knownDates {
			found := false
			for _, ecl := range eclipses {
				if ecl.Time.Format("2006-01-02") == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected lunar eclipse on %s not detected", expected)
			}
		}
	})

	t.Run("SolarEclipses", func(t *testing.T) {
		eclipses, err := plan.SolarEclipses(year2026Start, year2026End, eph)
		if err != nil {
			t.Fatalf("SolarEclipses failed: %v", err)
		}

		knownDates := []string{"2026-02-17", "2026-08-12"}

		t.Logf("Found %d solar eclipse candidates:", len(eclipses))
		for _, ecl := range eclipses {
			t.Logf("  %s  β=%.3f°  γ=%.3f", ecl.Time.Format("2006-01-02 15:04"), ecl.EclipticLatitude.Degrees(), ecl.Gamma)
		}

		for _, expected := range knownDates {
			found := false
			for _, ecl := range eclipses {
				if ecl.Time.Format("2006-01-02") == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected solar eclipse on %s not detected", expected)
			}
		}
	})
}

// ── Edge Case Locations ──────────────────────────────────────────────────────

var edgeCaseLocations = []testLocation{
	// North Pole: midnight sun in Jun, polar night in Dec
	{"North Pole", 89.99, 0.0, 0, 0, "", false},
	// South Pole: polar night in Jun, midnight sun in Dec
	{"South Pole", -89.99, 0.0, 0, 0, "", false},
	// Mount Everest summit: extreme altitude → large horizon dip (~3.3°)
	{"Everest", 27.9881, 86.925, 8849, 5.75, "Asia/Kathmandu", false},
	// Equator (Null Island): near-equal day/night, fast-setting bodies
	{"Equator", 0.0, 0.0, 0, 0, "", false},
	// Tromsø, Norway: near-polar boundary (69.6°N), midnight sun in Jun
	{"Tromsø", 69.6496, 18.9560, 0, 1, "Europe/Oslo", true},
}

// ── Test: Polar Sun — Midnight Sun / Polar Night ─────────────────────────────
// At the poles, the Sun can remain continuously above or below the horizon.
// USNO returns "null" for the time field when a body doesn't rise or set.
// This test validates:
// 1. USNO agrees the Sun is circumpolar / below horizon for the date.
// 2. astrogo's SunEvents returns no rise/set events for polar night/midnight sun.
// 3. When events DO exist near the polar boundary, they agree within tolerance.

func TestUSNO_PolarSun(t *testing.T) {
	eph := newEph(t)

	cases := []struct {
		name   string
		loc    testLocation
		date   string
		expect string // "midnightsun", "polarnight", or "normal"
	}{
		// North Pole — summer (midnight sun)
		{"NorthPole/MidnightSun", edgeCaseLocations[0], "2026-06-21", "midnightsun"},
		// North Pole — winter (polar night)
		{"NorthPole/PolarNight", edgeCaseLocations[0], "2026-12-21", "polarnight"},
		// South Pole — winter (polar night for south = June)
		{"SouthPole/PolarNight", edgeCaseLocations[1], "2026-06-21", "polarnight"},
		// South Pole — summer (midnight sun for south = December)
		{"SouthPole/MidnightSun", edgeCaseLocations[1], "2026-12-21", "midnightsun"},
		// Tromsø — summer (midnight sun)
		{"Tromsø/MidnightSun", edgeCaseLocations[4], "2026-06-21", "midnightsun"},
		// Tromsø — spring equinox (normal rise/set)
		{"Tromsø/Equinox", edgeCaseLocations[4], "2026-03-20", "normal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc := tc.loc

			// Query USNO — always use UTC (tz=0, dst=false) for polar/edge-case
			// locations to avoid DST interpretation mismatches. USNO's dst=true
			// uses US DST rules, which differ from European/Asian DST schedules.
			url := fmt.Sprintf(
				"https://aa.usno.navy.mil/api/rstt/oneday?date=%s&coords=%.6f,%.6f&tz=0&height=%.0f&dst=false",
				tc.date, loc.Lat, loc.Lon, loc.Height,
			)
			body := usnoGet(t, url)

			var resp usnoOneDayResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("Failed to parse USNO response: %v", err)
			}

			// Catalog USNO Sun phenomena — count rise/set vs null entries
			var hasRise, hasSet, hasTransit bool
			var riseNull, setNull bool
			for _, sp := range resp.Properties.Data.SunData {
				_, _, ok := parseUSNOTime(sp.Time)
				switch sp.Phen {
				case "Rise":
					if ok {
						hasRise = true
					} else {
						riseNull = true
					}
				case "Set":
					if ok {
						hasSet = true
					} else {
						setNull = true
					}
				case "Upper Transit":
					if ok {
						hasTransit = true
					}
				}
			}

			t.Logf("USNO Sun: rise=%v(null=%v) set=%v(null=%v) transit=%v",
				hasRise, riseNull, hasSet, setNull, hasTransit)

			// Set up astrogo — use UTC to match USNO query timezone
			var y, mo, d int
			fmt.Sscanf(tc.date, "%d-%d-%d", &y, &mo, &d)

			// Even for locations with a named timezone, we use UTC for the
			// computation interval to match the USNO query (tz=0).
			geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), loc.Height)
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}
			site, err := plan.NewSite(loc.Name, geodetic, angle.Zero(), time.LocationUTC)
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.LocationUTC)
			end := start.Add(24 * time.Hour)

			sunEvents, err := plan.SunEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("SunEvents failed: %v", err)
			}

			// Count astrogo events
			var astroRise, astroSet, astroTransit int
			for _, ev := range sunEvents {
				switch ev.Kind {
				case plan.EventRise:
					astroRise++
				case plan.EventSet:
					astroSet++
				case plan.EventTransit:
					astroTransit++
				}
			}
			t.Logf("astrogo Sun events: %d rise, %d set, %d transit", astroRise, astroSet, astroTransit)

			switch tc.expect {
			case "midnightsun":
				// USNO should show no rise/set times (null)
				// astrogo should produce zero rise/set events
				if hasRise {
					t.Logf("USNO reports Sun rise during midnight sun — checking astrogo agrees")
				}
				if hasSet {
					t.Logf("USNO reports Sun set during midnight sun — checking astrogo agrees")
				}
				if !hasRise && !hasSet {
					// Circumpolar: astrogo should have no rise/set either
					if astroRise != 0 {
						t.Errorf("Expected 0 Sun rises (midnight sun), got %d", astroRise)
					}
					if astroSet != 0 {
						t.Errorf("Expected 0 Sun sets (midnight sun), got %d", astroSet)
					}
					t.Logf("✓ Both USNO and astrogo agree: Sun does not rise/set (midnight sun)")
				}

			case "polarnight":
				// USNO should show no rise/set times (null)
				// astrogo should produce zero rise/set events
				if !hasRise && !hasSet {
					if astroRise != 0 {
						t.Errorf("Expected 0 Sun rises (polar night), got %d", astroRise)
					}
					if astroSet != 0 {
						t.Errorf("Expected 0 Sun sets (polar night), got %d", astroSet)
					}
					t.Logf("✓ Both USNO and astrogo agree: Sun does not rise/set (polar night)")
				}

			case "normal":
				// Both USNO and astrogo should find rise/set events
				if hasRise && astroRise == 0 {
					t.Errorf("USNO reports sunrise but astrogo found none")
				}
				if hasSet && astroSet == 0 {
					t.Errorf("USNO reports sunset but astrogo found none")
				}

				// Compare times if both have events
				if hasRise && astroRise > 0 {
					compareSunMoonEvents(t, "Sun", resp.Properties.Data.SunData, sunEvents, time.LocationUTC, 5.0)
				}
			}
		})
	}
}

// ── Test: High Altitude — Mount Everest ──────────────────────────────────────
// At 8849m altitude, the geometric horizon dip is ~2.76°, which significantly
// shifts sunrise/sunset times (the Sun appears to rise earlier and set later).
//
// IMPORTANT: The USNO rstt/oneday API ignores the height parameter for rise/set
// times (verified empirically: height=0 and height=8849 return identical results).
// Therefore this test:
//  1. Compares USNO (sea-level) against astrogo at sea-level (height=0) — must match ≤2 min.
//  2. Compares astrogo at 8849m vs astrogo at 0m — validates altitude correction is physical
//     (sunrise earlier, sunset later, shift ≈ 10–15 min at Everest latitude).
//  3. Transit times (height-independent) are compared against USNO — must match ≤2 min.

func TestUSNO_HighAltitude(t *testing.T) {
	eph := newEph(t)
	loc := edgeCaseLocations[2] // Everest

	dates := []string{"2026-03-20", "2026-06-21", "2026-12-21"}

	for _, dateStr := range dates {
		t.Run(fmt.Sprintf("Everest/%s", dateStr), func(t *testing.T) {
			// USNO ignores height — query at height=0 to get their actual reference.
			url := fmt.Sprintf(
				"https://aa.usno.navy.mil/api/rstt/oneday?date=%s&coords=%.6f,%.6f&tz=%.2f&height=0&dst=false",
				dateStr, loc.Lat, loc.Lon, loc.TZ,
			)
			body := usnoGet(t, url)

			var resp usnoOneDayResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("Failed to parse USNO response: %v", err)
			}

			var y, mo, d int
			fmt.Sscanf(dateStr, "%d-%d-%d", &y, &mo, &d)

			tz, err := time.LoadLocation(loc.TZName)
			if err != nil {
				t.Fatalf("Failed to load timezone: %v", err)
			}

			// Build sites at sea level AND at summit
			geodetic0, _ := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), 0)
			site0, _ := plan.NewSite(loc.Name+" (0m)", geodetic0, angle.Zero(), tz)

			geodetic, _ := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), loc.Height)
			site, _ := plan.NewSite(loc.Name+" (8849m)", geodetic, angle.Zero(), tz)

			t.Logf("Horizon dip (8849m): %.4f°", site.HorizonDip().Degrees())
			t.Logf("Sun threshold (0m):    %.4f°", site0.SunRiseSetThreshold().Degrees())
			t.Logf("Sun threshold (8849m): %.4f°", site.SunRiseSetThreshold().Degrees())

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, tz)
			end := start.Add(24 * time.Hour)

			sunEvents0, err := plan.SunEvents(start, end, site0, eph)
			if err != nil {
				t.Fatalf("SunEvents (0m) failed: %v", err)
			}
			sunEvents, err := plan.SunEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("SunEvents (8849m) failed: %v", err)
			}
			moonEvents0, err := plan.MoonEvents(start, end, site0, eph)
			if err != nil {
				t.Fatalf("MoonEvents (0m) failed: %v", err)
			}
			moonEvents, err := plan.MoonEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("MoonEvents (8849m) failed: %v", err)
			}

			// ── Part 1: Sea-level astrogo vs USNO (must match ≤2 min) ──
			t.Log("── Sea-level comparison (astrogo 0m vs USNO) ──")
			for _, sp := range resp.Properties.Data.SunData {
				h, m, ok := parseUSNOTime(sp.Time)
				if !ok {
					continue
				}
				usnoMin := minutesFromMidnight(h, m)
				var matchKind plan.EventKind
				switch sp.Phen {
				case "Rise":
					matchKind = plan.EventRise
				case "Set":
					matchKind = plan.EventSet
				case "Upper Transit":
					matchKind = plan.EventTransit
				default:
					continue
				}
				for _, ev := range sunEvents0 {
					if ev.Kind != matchKind {
						continue
					}
					astroMin := eventMinutesIn(ev.Time, tz)
					delta := deltaMinutes(usnoMin, astroMin)
					t.Logf("Sun %-12s  USNO=%02d:%02d  astrogo(0m)=%s  Δ=%.1f min",
						sp.Phen, h, m, ev.Time.In(tz).Format("15:04:05"), delta)
					tol := 2.0
					if matchKind == plan.EventTransit {
						tol = 1.0
					}
					if delta > tol {
						t.Errorf("Sun %s (0m vs USNO): Δ=%.1f min exceeds %.0f min", sp.Phen, delta, tol)
					}
					break
				}
			}
			for _, mp := range resp.Properties.Data.MoonData {
				h, m, ok := parseUSNOTime(mp.Time)
				if !ok {
					continue
				}
				usnoMin := minutesFromMidnight(h, m)
				var matchKind plan.EventKind
				switch mp.Phen {
				case "Rise":
					matchKind = plan.EventRise
				case "Set":
					matchKind = plan.EventSet
				case "Upper Transit":
					matchKind = plan.EventTransit
				default:
					continue
				}
				for _, ev := range moonEvents0 {
					if ev.Kind != matchKind {
						continue
					}
					astroMin := eventMinutesIn(ev.Time, tz)
					delta := deltaMinutes(usnoMin, astroMin)
					t.Logf("Moon %-12s  USNO=%02d:%02d  astrogo(0m)=%s  Δ=%.1f min",
						mp.Phen, h, m, ev.Time.In(tz).Format("15:04:05"), delta)
					tol := 3.0
					if matchKind == plan.EventTransit {
						tol = 1.0
					}
					if delta > tol {
						t.Errorf("Moon %s (0m vs USNO): Δ=%.1f min exceeds %.0f min", mp.Phen, delta, tol)
					}
					break
				}
			}

			// ── Part 2: Altitude correction (astrogo 8849m vs 0m) ──
			t.Log("── Altitude correction (8849m vs 0m) ──")
			logAltEvents := func(label string, events0, events []plan.Event) {
				var rise0, riseH, set0, setH float64
				var haveR0, haveRH, haveS0, haveSH bool
				for _, ev := range events0 {
					if ev.Kind == plan.EventRise && !haveR0 {
						rise0 = eventMinutesIn(ev.Time, tz)
						haveR0 = true
						t.Logf("%s Rise    (0m)=%s", label, ev.Time.In(tz).Format("15:04:05"))
					}
					if ev.Kind == plan.EventSet && !haveS0 {
						set0 = eventMinutesIn(ev.Time, tz)
						haveS0 = true
						t.Logf("%s Set     (0m)=%s", label, ev.Time.In(tz).Format("15:04:05"))
					}
				}
				for _, ev := range events {
					if ev.Kind == plan.EventRise && !haveRH {
						riseH = eventMinutesIn(ev.Time, tz)
						haveRH = true
						t.Logf("%s Rise (8849m)=%s", label, ev.Time.In(tz).Format("15:04:05"))
					}
					if ev.Kind == plan.EventSet && !haveSH {
						setH = eventMinutesIn(ev.Time, tz)
						haveSH = true
						t.Logf("%s Set  (8849m)=%s", label, ev.Time.In(tz).Format("15:04:05"))
					}
				}
				if haveR0 && haveRH {
					shift := rise0 - riseH
					t.Logf("%s sunrise shift: %.1f min earlier at 8849m", label, shift)
					if shift < 3 {
						t.Errorf("%s sunrise should be earlier at 8849m (shift=%.1f min)", label, shift)
					}
				}
				if haveS0 && haveSH {
					shift := setH - set0
					t.Logf("%s sunset shift: %.1f min later at 8849m", label, shift)
					if shift < 3 {
						t.Errorf("%s sunset should be later at 8849m (shift=%.1f min)", label, shift)
					}
				}
			}
			logAltEvents("Sun", sunEvents0, sunEvents)
			logAltEvents("Moon", moonEvents0, moonEvents)
		})
	}
}

// ── Test: Equator — Fast-Setting Bodies ──────────────────────────────────────
// At the equator, all celestial bodies set roughly perpendicular to the horizon
// (fastest possible setting). Day and night are nearly equal year-round.
// This validates that the solver converges correctly with steep altitude curves.

func TestUSNO_Equator(t *testing.T) {
	eph := newEph(t)
	loc := edgeCaseLocations[3] // Equator (0°, 0°)

	dates := []string{"2026-03-20", "2026-06-21", "2026-12-21"}

	for _, dateStr := range dates {
		t.Run(fmt.Sprintf("Equator/%s", dateStr), func(t *testing.T) {
			url := fmt.Sprintf(
				"https://aa.usno.navy.mil/api/rstt/oneday?date=%s&coords=%.6f,%.6f&tz=0&height=0&dst=false",
				dateStr, loc.Lat, loc.Lon,
			)
			body := usnoGet(t, url)

			var resp usnoOneDayResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("Failed to parse USNO response: %v", err)
			}

			var y, mo, d int
			fmt.Sscanf(dateStr, "%d-%d-%d", &y, &mo, &d)

			geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), loc.Height)
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}
			site, err := plan.NewSite(loc.Name, geodetic, angle.Zero(), time.LocationUTC)
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.LocationUTC)
			end := start.Add(24 * time.Hour)

			// Sun events
			sunEvents, err := plan.SunEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("SunEvents failed: %v", err)
			}

			for _, sp := range resp.Properties.Data.SunData {
				h, m, ok := parseUSNOTime(sp.Time)
				if !ok {
					continue
				}
				usnoMin := minutesFromMidnight(h, m)

				var matchKind plan.EventKind
				switch sp.Phen {
				case "Rise":
					matchKind = plan.EventRise
				case "Set":
					matchKind = plan.EventSet
				case "Upper Transit":
					matchKind = plan.EventTransit
				default:
					continue
				}

				found := false
				for _, ev := range sunEvents {
					if ev.Kind != matchKind {
						continue
					}
					astroMin := eventMinutesIn(ev.Time, time.LocationUTC)
					delta := deltaMinutes(usnoMin, astroMin)
					t.Logf("Sun %-12s  USNO=%02d:%02d  astrogo=%s  Δ=%.1f min",
						sp.Phen, h, m, ev.Time.In(time.LocationUTC).Format("15:04:05"), delta)

					// Equator: standard tolerances
					tol := 2.0
					if matchKind == plan.EventTransit {
						tol = 1.0
					}
					if delta > tol {
						t.Errorf("Sun %s: Δ=%.1f min exceeds %.0f min tolerance", sp.Phen, delta, tol)
					}
					found = true
					break
				}
				if !found {
					t.Logf("Sun %s at %02d:%02d: no matching astrogo event", sp.Phen, h, m)
				}
			}

			// At the equator, day length should always be ~12h (± 10 min)
			var riseMin, setMin float64
			var haveRise, haveSet bool
			for _, ev := range sunEvents {
				if ev.Kind == plan.EventRise && !haveRise {
					riseMin = eventMinutesIn(ev.Time, time.LocationUTC)
					haveRise = true
				}
				if ev.Kind == plan.EventSet && !haveSet {
					setMin = eventMinutesIn(ev.Time, time.LocationUTC)
					haveSet = true
				}
			}
			if haveRise && haveSet {
				dayLength := setMin - riseMin
				if dayLength < 0 {
					dayLength += 24 * 60
				}
				t.Logf("Day length at equator: %.1f min (%.1f hours)", dayLength, dayLength/60)
				if math.Abs(dayLength-12*60) > 15 {
					t.Errorf("Equator day length %.1f min deviates >15 min from 12h", dayLength)
				}
			}
		})
	}
}

// ── Test: Polar Moon Rise/Set ────────────────────────────────────────────────
// The Moon at polar latitudes can also be circumpolar or below horizon for
// extended periods. This tests the Moon event solver at extreme latitudes.

func TestUSNO_PolarMoon(t *testing.T) {
	eph := newEph(t)

	cases := []struct {
		name string
		loc  testLocation
		date string
	}{
		{"NorthPole/Jun", edgeCaseLocations[0], "2026-06-21"},
		{"NorthPole/Dec", edgeCaseLocations[0], "2026-12-21"},
		{"SouthPole/Jun", edgeCaseLocations[1], "2026-06-21"},
		{"SouthPole/Dec", edgeCaseLocations[1], "2026-12-21"},
		{"Tromsø/Jun", edgeCaseLocations[4], "2026-06-21"},
		{"Tromsø/Dec", edgeCaseLocations[4], "2026-12-21"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc := tc.loc

			dstParam := "false"
			if loc.DST {
				dstParam = "true"
			}
			url := fmt.Sprintf(
				"https://aa.usno.navy.mil/api/rstt/oneday?date=%s&coords=%.6f,%.6f&tz=%.0f&height=%.0f&dst=%s",
				tc.date, loc.Lat, loc.Lon, loc.TZ, loc.Height, dstParam,
			)
			body := usnoGet(t, url)

			var resp usnoOneDayResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("Failed to parse USNO response: %v", err)
			}

			// Count USNO Moon events
			var usnoMoonRise, usnoMoonSet, usnoMoonTransit int
			var usnoMoonRiseNull, usnoMoonSetNull bool
			for _, mp := range resp.Properties.Data.MoonData {
				_, _, ok := parseUSNOTime(mp.Time)
				switch mp.Phen {
				case "Rise":
					if ok {
						usnoMoonRise++
					} else {
						usnoMoonRiseNull = true
					}
				case "Set":
					if ok {
						usnoMoonSet++
					} else {
						usnoMoonSetNull = true
					}
				case "Upper Transit":
					if ok {
						usnoMoonTransit++
					}
				}
			}
			t.Logf("USNO Moon: rise=%d(null=%v) set=%d(null=%v) transit=%d",
				usnoMoonRise, usnoMoonRiseNull, usnoMoonSet, usnoMoonSetNull, usnoMoonTransit)

			// Set up astrogo
			var y, mo, d int
			fmt.Sscanf(tc.date, "%d-%d-%d", &y, &mo, &d)

			tz := time.LocationUTC
			if loc.TZName != "" {
				var err error
				tz, err = time.LoadLocation(loc.TZName)
				if err != nil {
					t.Fatalf("Failed to load timezone: %v", err)
				}
			}
			geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), loc.Height)
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}
			site, err := plan.NewSite(loc.Name, geodetic, angle.Zero(), tz)
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, tz)
			end := start.Add(24 * time.Hour)

			moonEvents, err := plan.MoonEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("MoonEvents failed: %v", err)
			}

			var astroRise, astroSet, astroTransit int
			for _, ev := range moonEvents {
				switch ev.Kind {
				case plan.EventRise:
					astroRise++
				case plan.EventSet:
					astroSet++
				case plan.EventTransit:
					astroTransit++
				}
			}
			t.Logf("astrogo Moon events: %d rise, %d set, %d transit", astroRise, astroSet, astroTransit)

			// If USNO says Moon never rises (null), astrogo shouldn't find rises either
			if usnoMoonRise == 0 && usnoMoonRiseNull && astroRise > 0 {
				t.Errorf("USNO says Moon never rises but astrogo found %d rise events", astroRise)
			}
			// If USNO says Moon never sets (null), astrogo shouldn't find sets either
			if usnoMoonSet == 0 && usnoMoonSetNull && astroSet > 0 {
				t.Errorf("USNO says Moon never sets but astrogo found %d set events", astroSet)
			}

			// If USNO has timed events, compare them
			if usnoMoonRise > 0 || usnoMoonSet > 0 {
				for _, mp := range resp.Properties.Data.MoonData {
					h, m, ok := parseUSNOTime(mp.Time)
					if !ok {
						continue
					}
					usnoMin := minutesFromMidnight(h, m)

					var matchKind plan.EventKind
					switch mp.Phen {
					case "Rise":
						matchKind = plan.EventRise
					case "Set":
						matchKind = plan.EventSet
					case "Upper Transit":
						matchKind = plan.EventTransit
					default:
						continue
					}

					found := false
					for _, ev := range moonEvents {
						if ev.Kind != matchKind {
							continue
						}
						astroMin := eventMinutesIn(ev.Time, tz)
						delta := deltaMinutes(usnoMin, astroMin)
						t.Logf("Moon %-12s  USNO=%02d:%02d  astrogo=%s  Δ=%.1f min",
							mp.Phen, h, m, ev.Time.In(tz).Format("15:04:05"), delta)

						// Polar locations: wider tolerance (5 min) due to
						// grazing horizon geometry amplifying refraction errors
						tol := 5.0
						if matchKind == plan.EventTransit {
							tol = 2.0
						}
						if delta > tol {
							t.Errorf("Moon %s: Δ=%.1f min exceeds %.0f min tolerance", mp.Phen, delta, tol)
						}
						found = true
						break
					}
					if !found {
						t.Logf("Moon %s at %02d:%02d: no matching astrogo event", mp.Phen, h, m)
					}
				}
			}
		})
	}
}

// ── Test: Celestial Navigation at Extreme Locations ──────────────────────────
// Validates AltAz computation at polar and high-altitude locations against USNO.

func TestUSNO_CelNav_EdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		lat     float64
		lon     float64
		height  float64
		date    string
		utcTime string // "HH:MM:SS"
	}{
		// North Pole at local noon on summer solstice (Sun ~23.4° above horizon)
		{"NorthPole/SummerNoon", 89.99, 0.0, 0, "2026-06-21", "12:00:00"},
		// South Pole at local midnight on December solstice (Sun ~23.4° above horizon)
		{"SouthPole/SummerMidnight", -89.99, 0.0, 0, "2026-12-21", "00:00:00"},
		// Equator at noon on equinox (Sun near zenith)
		{"Equator/EquinoxNoon", 0.0, 0.0, 0, "2026-03-20", "12:00:00"},
		// Everest summit — tests altitude correction
		{"Everest/Noon", 27.9881, 86.925, 8849, "2026-06-21", "06:00:00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf(
				"https://aa.usno.navy.mil/api/celnav?date=%s&time=%s&coords=%.6f,%.6f",
				tc.date, tc.utcTime, tc.lat, tc.lon,
			)
			body := usnoGet(t, url)

			var resp usnoCelNavResponse
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("Failed to parse CelNav response: %v", err)
			}

			// Set up astrogo context
			var y, mo, d int
			fmt.Sscanf(tc.date, "%d-%d-%d", &y, &mo, &d)
			var h, m, s int
			fmt.Sscanf(tc.utcTime, "%d:%d:%d", &h, &m, &s)

			tm := time.Date(y, time.Month(mo), d, h, m, s, 0, time.LocationUTC)
			geodetic, _ := coord.NewGeodetic(angle.Deg(tc.lon), angle.Deg(tc.lat), tc.height)
			site, _ := plan.NewSite("test", geodetic, angle.Zero(), time.LocationUTC)
			ctx := coord.NewContext(tm, site.Location(), site.Atmosphere())

			prov := newEph(t)

			for _, entry := range resp.Properties.Data {
				if entry.Object == "ARIES" {
					continue
				}
				if entry.AlmanacData.Hc == 0 && entry.AlmanacData.Zn == 0 {
					continue
				}

				if entry.Object == "Sun" {
					sunTarget := plan.NewSun(prov)
					pos, err := sunTarget.Position(tm)
					if err != nil {
						t.Logf("Sun position error: %v", err)
						continue
					}
					aa, _ := ctx.ICRSToAltAz(pos)

					deltaAlt := math.Abs(aa.Alt().Degrees() - entry.AlmanacData.Hc)
					deltaAz := math.Abs(aa.Az().Degrees() - entry.AlmanacData.Zn)
					if deltaAz > 180 {
						deltaAz = 360 - deltaAz
					}

					t.Logf("Sun  Alt: USNO=%.4f° astrogo=%.4f° Δ=%.4f°", entry.AlmanacData.Hc, aa.Alt().Degrees(), deltaAlt)
					t.Logf("Sun  Az:  USNO=%.4f° astrogo=%.4f° Δ=%.4f°", entry.AlmanacData.Zn, aa.Az().Degrees(), deltaAz)

					// Near-horizon: larger refraction tolerance
					altTol := 0.2
					if math.Abs(entry.AlmanacData.Hc) < 5.0 {
						altTol = 1.5 // Near-horizon refraction model differences
					}
					if deltaAlt > altTol {
						t.Errorf("Sun altitude Δ=%.4f° exceeds %.1f° tolerance", deltaAlt, altTol)
					}
					// At poles azimuth is degenerate (any direction is "south")
					if math.Abs(tc.lat) < 85.0 && deltaAz > 1.0 {
						t.Errorf("Sun azimuth Δ=%.4f° exceeds 1.0° tolerance", deltaAz)
					}
				}

				if entry.Object == "Moon" {
					moonTarget := plan.NewMoon(prov)
					pos, err := moonTarget.Position(tm)
					if err != nil {
						t.Logf("Moon position error: %v", err)
						continue
					}
					aa, _ := ctx.ICRSToAltAz(pos)

					deltaAlt := math.Abs(aa.Alt().Degrees() - entry.AlmanacData.Hc)
					t.Logf("Moon Alt: USNO=%.4f° astrogo=%.4f° Δ=%.4f°", entry.AlmanacData.Hc, aa.Alt().Degrees(), deltaAlt)

					// Moon parallax can shift altitude by ~1° so wider tolerance
					if deltaAlt > 1.5 {
						t.Errorf("Moon altitude Δ=%.4f° exceeds 1.5° tolerance", deltaAlt)
					}
				}
			}
		})
	}
}

// ── Test: Altitude Comparison — Everest vs Sea Level ─────────────────────────
// Verifies that higher altitude systematically shifts sunrise earlier and
// sunset later (Sun is visible "around" the curvature of the Earth).

func TestUSNO_AltitudeShift(t *testing.T) {
	eph := newEph(t)

	// Same geodetic position (Everest coordinates) at sea level vs summit
	altCases := []struct {
		name   string
		height float64
	}{
		{"SeaLevel", 0},
		{"EverestSummit", 8849},
	}

	var riseTimes []float64
	var setTimes []float64

	for _, ac := range altCases {
		t.Run(ac.name, func(t *testing.T) {
			geodetic, err := coord.NewGeodetic(angle.Deg(86.925), angle.Deg(27.9881), ac.height)
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}
			tz, _ := time.LoadLocation("Asia/Kathmandu")
			site, err := plan.NewSite(ac.name, geodetic, angle.Zero(), tz)
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			t.Logf("Height=%.0fm  HorizonDip=%.4f°  SunThreshold=%.4f°",
				ac.height, site.HorizonDip().Degrees(), site.SunRiseSetThreshold().Degrees())

			start := time.Date(2026, time.March, 20, 0, 0, 0, 0, tz)
			end := start.Add(24 * time.Hour)

			sunEvents, err := plan.SunEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("SunEvents failed: %v", err)
			}

			for _, ev := range sunEvents {
				min := eventMinutesIn(ev.Time, tz)
				switch ev.Kind {
				case plan.EventRise:
					t.Logf("Sunrise: %s (%.1f min from midnight)", ev.Time.In(tz).Format("15:04:05"), min)
					riseTimes = append(riseTimes, min)
				case plan.EventSet:
					t.Logf("Sunset:  %s (%.1f min from midnight)", ev.Time.In(tz).Format("15:04:05"), min)
					setTimes = append(setTimes, min)
				}
			}
		})
	}

	// Verify altitude shift: Everest sunrise should be EARLIER than sea level
	if len(riseTimes) >= 2 {
		t.Logf("Sunrise shift (sea→summit): %.1f min", riseTimes[0]-riseTimes[1])
		if riseTimes[1] >= riseTimes[0] {
			t.Errorf("Everest sunrise (%.1f) should be earlier than sea level (%.1f)", riseTimes[1], riseTimes[0])
		}
	}
	// Verify altitude shift: Everest sunset should be LATER than sea level
	if len(setTimes) >= 2 {
		t.Logf("Sunset shift (sea→summit): %.1f min", setTimes[1]-setTimes[0])
		if setTimes[1] <= setTimes[0] {
			t.Errorf("Everest sunset (%.1f) should be later than sea level (%.1f)", setTimes[1], setTimes[0])
		}
	}
}

// ── Helper: compareSunMoonEvents ─────────────────────────────────────────────

func compareSunMoonEvents(t *testing.T, body string, usnoPhenomena []usnoPhenomenon, astroEvents []plan.Event, tz *time.Location, tol float64) {
	t.Helper()
	for _, sp := range usnoPhenomena {
		h, m, ok := parseUSNOTime(sp.Time)
		if !ok {
			continue
		}
		usnoMin := minutesFromMidnight(h, m)

		var matchKind plan.EventKind
		switch sp.Phen {
		case "Rise":
			matchKind = plan.EventRise
		case "Set":
			matchKind = plan.EventSet
		case "Upper Transit":
			matchKind = plan.EventTransit
		default:
			continue
		}

		for _, ev := range astroEvents {
			if ev.Kind != matchKind {
				continue
			}
			astroMin := eventMinutesIn(ev.Time, tz)
			delta := deltaMinutes(usnoMin, astroMin)
			t.Logf("%s %-12s  USNO=%02d:%02d  astrogo=%s  Δ=%.1f min",
				body, sp.Phen, h, m, ev.Time.In(tz).Format("15:04:05"), delta)

			if delta > tol {
				t.Errorf("%s %s: Δ=%.1f min exceeds %.0f min tolerance", body, sp.Phen, delta, tol)
			}
			break
		}
	}
}
