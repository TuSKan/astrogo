//go:build integration

package plan_test

// Package plan_test contains integration tests that validate astrogo's
// astronomical computations against the U.S. Naval Observatory (USNO) API.
//
// Run with: go test -tags integration -run TestUSNO -v ./plan/
//
// These tests require an active internet connection to reach
// https://aa.usno.navy.mil/api/ endpoints.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
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
		IsCorrected bool `json:"isCorrected"`
		Refr        any  `json:"refr"`
		PA          any  `json:"pa"`
		SD          any  `json:"sd"`
		Sum         any  `json:"sum"`
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

// usnoHost is the address the pre-check probes and the API this file queries.
const usnoHost = "aa.usno.navy.mil:443"

var (
	// usnoReachableMu guards usnoReachableOK. Only a *success* is recorded:
	// see requireUSNO for why a cached failure is the dangerous direction.
	usnoReachableMu sync.Mutex
	usnoReachableOK bool
)

// requireUSNO skips the calling test if the USNO API is unreachable. Call it
// as the first line of every top-level USNO test function — t.Skip halts that
// function immediately, so no subtest beneath it runs either.
//
// The pre-check exists because a full USNO outage otherwise costs each of this
// file's ~30 usnoGet call sites its own usnoRequestTimeout (30s) run
// sequentially, summing past the CI job's own 10-minute binary timeout before
// the last few ever get a chance to skip gracefully — killing the package's
// test run instead of skipping fast.
//
// # Only a success is remembered
//
// This used to hold both answers in a sync.Once, and one unlucky probe
// therefore disabled all eleven TestUSNO_* functions for the rest of the
// binary — reported as SKIP, which reads as a pass in every summary that
// exists. Measured on a developer machine while the host was answering curl in
// 2.2s: net.DialTimeout with a 5s budget overran to 17.3s and failed, and the
// immediately following dial connected in 140ms. aa.usno.navy.mil resolves to
// a single address and drops or delays SYNs intermittently, so a single-shot
// probe is a coin toss that decides the whole suite.
//
// USNO is one of the four references the README's headline accuracy claim
// names, the integration job is continue-on-error, and a skipped suite is
// invisible. Caching only the positive costs one extra probe per top-level
// test during a genuine outage — eleven times testutil.ReachableTimeout,
// under a minute — and removes the failure mode where a moment of packet loss
// silently retires the reference.
//
// testutil.Reachable rather than a dial of our own, because it already carries
// the IPv4 retry that was added for exactly this class of bug: a probe that
// declares a service absent because of the resolver's opinion of its address
// family, on a service that is there.
func requireUSNO(t *testing.T) {
	t.Helper()

	if usnoReachable(func() bool { return testutil.Reachable(usnoHost) }) {
		return
	}

	t.Skipf("%s did not answer within %v", usnoHost, testutil.ReachableTimeout*time.Nanosecond)
}

// usnoReachable is requireUSNO's memo, separated from the probe so the one
// thing that went wrong here can be tested without a network: a failure must
// not be remembered.
//
// Returns true once probe has succeeded, and calls probe again every time it
// has not. See requireUSNO for why that asymmetry is the whole fix.
func usnoReachable(probe func() bool) bool {
	usnoReachableMu.Lock()
	defer usnoReachableMu.Unlock()

	if usnoReachableOK {
		return true
	}

	usnoReachableOK = probe()

	return usnoReachableOK
}

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

// checkUSNOCivilTwilight holds one civil twilight event to the USNO time
// for it. USNO's phenomenon is the Sun's center at 6° below the horizon, the
// same definition plan.CivilTwilight uses, so the two answer one question.
//
// A USNO twilight with no astrogo event is an error, not a log line: the
// site, day and threshold are the same, so a missing event means astrogo
// lost a crossing that happened.
func checkUSNOCivilTwilight(t *testing.T, phen string, h, m int, ev *plan.Event, tz *time.Location) {
	t.Helper()

	if ev == nil {
		t.Errorf("Sun %s: USNO reports %02d:%02d, astrogo found no event", phen, h, m)

		return
	}

	delta := deltaMinutes(minutesFromMidnight(h, m), eventMinutesIn(ev.Time, tz))
	t.Logf("Sun %-20s  USNO=%02d:%02d  astrogo=%s  Δ=%.1f min",
		phen, h, m, ev.Time.In(tz).Format("15:04:05"), delta)

	// USNO rounds to the nearest minute, so 0.5 min is rounding alone, and
	// that is the most measured over the nine site-days here. Twilight is a
	// geometric −6°, so 1 min is rounding plus half a minute of margin: the
	// same bound rise and set are held to, see usnoTolerance.
	// Measured, a −5.5° threshold moves these eighteen events by 1.8 to
	// 5.3 min, so half a degree of error cannot pass.
	const tol = 1.0
	if delta > tol {
		t.Errorf("Sun %s: Δ=%.1f min exceeds %.0f min tolerance", phen, delta, tol)
	}
}

// newEph builds the DE442 provider these USNO comparisons need, or says why it
// could not.
//
// # The fallback is kept, but not for an unreachable NAIF
//
// Falling back to the analytic default when a kernel will not load is right for
// a test that needs *an* ephemeris. It is wrong for one that measures astrogo
// against a published almanac to a fraction of an arcsecond, because the
// analytic default is not accurate to a fraction of an arcsecond and was never
// meant to be.
//
// That is not hypothetical. In the run recorded on #348, NAIF was unreachable,
// this helper quietly substituted the analytic provider, and
// TestUSNODecomposesTheTopocentricBias then compared it against USNO and
// reported:
//
//	declination bias -0.456 arcsec exceeds 0.2; declination cannot see Earth
//	rotation, so this is the apparent-place chain -- precession-nutation,
//	aberration or deflection
//
// Every word of which is a correct reading of the numbers and a wrong
// conclusion about the cause. A reader following it would go looking for a
// defect in the apparent-place chain that is not there, because the real
// difference was the ephemeris underneath. A red build is a nuisance; a red
// build that names an innocent subsystem costs somebody an afternoon.
//
// So a network failure skips, and everything else still falls back: a kernel
// that is absent for any reason the network is not -- consent withheld, a bad
// local cache -- leaves the old behavior untouched.
func newEph(t *testing.T) eph.Provider {
	t.Helper()

	p, err := eph.NewProvider(kernelContext(t), eph.Planets, "de442")
	if err != nil {
		// UpstreamFailure rather than Unreachable: it also counts a fetch that
		// ran out of kernelContext's budget, which is NAIF too slow to finish
		// and as external as NAIF being down.
		if reason, ok := testutil.UpstreamFailure(err); ok {
			t.Skipf("NAIF did not deliver DE442 (%s): %v "+
				"(external, not astrogo -- and the analytic fallback is not accurate "+
				"enough to compare against USNO)", reason, err)
		}

		t.Logf("DE442 unavailable (%v), falling back to default", err)

		def := eph.Default()
		if def == nil {
			t.Fatal("Failed to create ephemeris: nil provider")
		}

		return def
	}

	t.Cleanup(func() { _ = p.Close() })

	return p
}

// ── Test: Complete Sun and Moon Data for One Day ──────────────────────────────

func TestUSNO_SunMoonOneDay(t *testing.T) {
	requireUSNO(t)

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
				if _, err := fmt.Sscanf(dateStr, "%d-%d-%d", &y, &mo, &d); err != nil {
					t.Fatalf("parsing the fixture date: %v", err)
				}

				// Set up astrogo
				tz, err := time.LoadLocation(loc.TZName)
				if err != nil {
					t.Fatalf("Failed to load timezone: %v", err)
				}

				geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), unit.Meters(loc.Height))
				if err != nil {
					t.Fatalf("Failed to create geodetic: %v", err)
				}

				site, err := plan.NewSite(loc.Name, geodetic, plan.WithTimeZone(tz))
				if err != nil {
					t.Fatalf("Failed to create site: %v", err)
				}

				start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, tz)
				end := start.Add(unit.Hours(24))

				// Compare Sun events
				sunEvents, err := plan.SunEvents(start, end, site, prov)
				if err != nil {
					t.Fatalf("SunEvents failed: %v", err)
				}

				// Civil twilight comes back in the same USNO response. Until
				// #532 it was skipped as "handled separately" and nothing else
				// compared it, so astrogo's twilight times had no external
				// reference at all.
				civilDawn, civilDusk, err := plan.CivilDawnDusk(start, end, site, prov)
				if err != nil {
					t.Fatalf("CivilDawnDusk failed: %v", err)
				}

				for _, sp := range resp.Properties.Data.SunData {
					h, m, ok := parseUSNOTime(sp.Time)
					if !ok {
						continue
					}

					switch sp.Phen {
					case "Begin Civil Twilight":
						checkUSNOCivilTwilight(t, sp.Phen, h, m, civilDawn, tz)
					case "End Civil Twilight":
						checkUSNOCivilTwilight(t, sp.Phen, h, m, civilDusk, tz)
					}
				}

				compareUSNOEvents(t, "Sun", resp.Properties.Data.SunData, sunEvents, site.SunRiseSetThreshold(), start, tz)

				moonEvents, err := plan.MoonEvents(start, end, site, prov)
				if err != nil {
					t.Fatalf("MoonEvents failed: %v", err)
				}

				compareUSNOEvents(t, "Moon", resp.Properties.Data.MoonData, moonEvents, site.MoonRiseSetThreshold(), start, tz)
			})
		}
	}
}

// Celestial navigation, including the extreme places, is TestUSNO_CelNav in
// usno_celnav_test.go: an offline fixture of USNO's own values, so it runs in
// every build rather than only under this file's integration tag.

// ── Test: Moon Phases ────────────────────────────────────────────────────────

func TestUSNO_MoonPhases(t *testing.T) {
	requireUSNO(t)

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
				t.Logf("%-14s  USNO=%s  astrogo=%s  Δ=%.2f min",
					usnoP.Phase,
					usnoTime.Format("2006-01-02 15:04"),
					ap.Time.Format("2006-01-02 15:04:05"),
					delta)

				// USNO gives the minute, so half a minute is its rounding and
				// the rest is the model's. This was 30 minutes, against the
				// minute docs/VALIDATION.md claims; with geometric longitudes
				// (#430) every phase also ran about 40 s late.
				if delta > 1 {
					t.Errorf("%s: Δ=%.2f min exceeds the 1 min tolerance", usnoP.Phase, delta)
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

// TestUSNO_Seasons compares the equinoxes and solstices with USNO's for years
// spread over the 18.6-year nutation cycle, to a minute: USNO rounds to the
// minute, so its own rounding is half of that.
//
// It used to check 2026 alone, to 30 minutes, and VALIDATION.md read "2–4
// min". Nutation in longitude was missing (#414), and 2026 is a year in which
// it is small: over 1972–2100 the seasons were up to 8.5 minutes off, and 5
// minutes in 2027. Several years, and a bound the model has to earn, are what
// would have shown it.
func TestUSNO_Seasons(t *testing.T) {
	requireUSNO(t)

	const tolMinutes = 1.0

	eph := newEph(t)

	for _, year := range []int{2020, 2024, 2027, 2031, 2035} {
		body := usnoGet(t, fmt.Sprintf("https://aa.usno.navy.mil/api/seasons?year=%d", year))

		var resp usnoSeasonsResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("%d: failed to parse Seasons response: %v", year, err)
		}

		astroSeasons, err := plan.Seasons(year, eph)
		if err != nil {
			t.Fatalf("%d: Seasons failed: %v", year, err)
		}

		compared := 0

		for _, usSeason := range resp.Data {
			if usSeason.Phenom != "Equinox" && usSeason.Phenom != "Solstice" {
				// Perihelion/Aphelion — not season events.
				continue
			}

			h, m, ok := parseUSNOTime(usSeason.Time)
			if !ok {
				t.Errorf("%d: unparseable USNO time %q", year, usSeason.Time)

				continue
			}

			usnoTime := time.Date(usSeason.Year, time.Month(usSeason.Month), usSeason.Day, h, m, 0, 0, time.LocationUTC)

			found := false

			for _, as := range astroSeasons {
				delta := as.Time.Sub(usnoTime).Minutes()
				if math.Abs(delta) >= 7*24*60 {
					continue
				}

				t.Logf("%-20s  USNO=%s  astrogo=%s  Δ=%+.2f min",
					usSeason.Phenom+" ("+as.Season.String()+")",
					usnoTime.Format("2006-01-02 15:04"), as.Time.Format("2006-01-02 15:04:05"), delta)

				if math.Abs(delta) > tolMinutes {
					t.Errorf("%s %d: Δ=%+.2f min exceeds %.0f min", as.Season, year, delta, tolMinutes)
				}

				found = true
				compared++

				break
			}

			if !found {
				t.Errorf("%s at %s: no matching astrogo season found", usSeason.Phenom, usnoTime.Format("2006-01-02"))
			}
		}

		if compared != 4 {
			t.Errorf("%d: compared %d seasons, want 4", year, compared)
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

// Sidereal time is TestUSNO_SiderealTime in usno_sidereal_test.go: an offline
// fixture of USNO's own values, so it runs in every build rather than only
// under this file's integration tag.

// ── Test: Perihelion/Aphelion ────────────────────────────────────────────────

func TestUSNO_Apsides(t *testing.T) {
	requireUSNO(t)

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
// In every response these tests receive, USNO leaves out an event that does
// not happen rather than listing it as null, so compareUSNOEvents's counts
// carry the check: no sunrise listed means astrogo must find none, and an
// upper transit is listed only in the midnight sun.

func TestUSNO_PolarSun(t *testing.T) {
	requireUSNO(t)

	eph := newEph(t)

	cases := []struct {
		name string
		loc  testLocation
		date string
	}{
		// North Pole — summer (midnight sun)
		{"NorthPole/MidnightSun", edgeCaseLocations[0], "2026-06-21"},
		// North Pole — winter (polar night)
		{"NorthPole/PolarNight", edgeCaseLocations[0], "2026-12-21"},
		// South Pole — winter (polar night for south = June)
		{"SouthPole/PolarNight", edgeCaseLocations[1], "2026-06-21"},
		// South Pole — summer (midnight sun for south = December)
		{"SouthPole/MidnightSun", edgeCaseLocations[1], "2026-12-21"},
		// Tromsø — summer (midnight sun)
		{"Tromsø/MidnightSun", edgeCaseLocations[4], "2026-06-21"},
		// Tromsø — spring equinox (normal rise/set)
		{"Tromsø/Equinox", edgeCaseLocations[4], "2026-03-20"},
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

			// Set up astrogo — use UTC to match USNO query timezone
			var y, mo, d int
			if _, err := fmt.Sscanf(tc.date, "%d-%d-%d", &y, &mo, &d); err != nil {
				t.Fatalf("parsing the fixture date: %v", err)
			}

			// Even for locations with a named timezone, we use UTC for the
			// computation interval to match the USNO query (tz=0).
			geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), unit.Meters(loc.Height))
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}

			site, err := plan.NewSite(loc.Name, geodetic, plan.WithTimeZone(time.LocationUTC))
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.LocationUTC)
			end := start.Add(unit.Hours(24))

			sunEvents, err := plan.SunEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("SunEvents failed: %v", err)
			}

			compareUSNOEvents(t, "Sun", resp.Properties.Data.SunData, sunEvents, site.SunRiseSetThreshold(), start, time.LocationUTC)
		})
	}
}

// ── Test: High Altitude — Mount Everest ──────────────────────────────────────
// At 8849m altitude, the horizon dip is ~2.76° (1.76′√h, with terrestrial
// refraction; the geometric dip would be 3.02°), which significantly
// shifts sunrise/sunset times (the Sun appears to rise earlier and set later).
//
// IMPORTANT: The USNO rstt/oneday API ignores the height parameter for rise/set
// times (verified empirically: height=0 and height=8849 return identical results).
// Therefore this test:
//  1. Compares USNO (sea-level) against astrogo at sea-level (height=0), within usnoTolerance.
//  2. Compares astrogo at 8849m vs astrogo at 0m — validates altitude correction is physical
//     (sunrise earlier, sunset later, shift ≈ 10–15 min at Everest latitude).
//  3. Transit times (height-independent) are compared against USNO, within usnoTolerance.

func TestUSNO_HighAltitude(t *testing.T) {
	requireUSNO(t)

	eph := newEph(t)
	loc := edgeCaseLocations[2] // Everest

	dates := []string{"2026-03-20", "2026-06-21", "2026-12-21"}

	for _, dateStr := range dates {
		t.Run("Everest/"+dateStr, func(t *testing.T) {
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
			if _, err := fmt.Sscanf(dateStr, "%d-%d-%d", &y, &mo, &d); err != nil {
				t.Fatalf("parsing the fixture date: %v", err)
			}

			tz, err := time.LoadLocation(loc.TZName)
			if err != nil {
				t.Fatalf("Failed to load timezone: %v", err)
			}

			// Build sites at sea level AND at summit
			geodetic0, _ := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), 0)
			site0, _ := plan.NewSite(loc.Name+" (0m)", geodetic0, plan.WithTimeZone(tz))

			geodetic, _ := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), unit.Meters(loc.Height))
			site, _ := plan.NewSite(loc.Name+" (8849m)", geodetic, plan.WithTimeZone(tz))

			t.Logf("Horizon dip (8849m): %.4f°", site.HorizonDip().Degrees())
			t.Logf("Sun threshold (0m):    %.4f°", site0.SunRiseSetThreshold().Degrees())
			t.Logf("Sun threshold (8849m): %.4f°", site.SunRiseSetThreshold().Degrees())

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, tz)
			end := start.Add(unit.Hours(24))

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

			// ── Part 1: Sea-level astrogo vs USNO, within usnoTolerance ──
			t.Log("── Sea-level comparison (astrogo 0m vs USNO) ──")

			compareUSNOEvents(t, "Sun", resp.Properties.Data.SunData, sunEvents0, site0.SunRiseSetThreshold(), start, tz)
			compareUSNOEvents(t, "Moon", resp.Properties.Data.MoonData, moonEvents0, site0.MoonRiseSetThreshold(), start, tz)

			// ── Part 2: Altitude correction (astrogo 8849m vs 0m) ──
			t.Log("── Altitude correction (8849m vs 0m) ──")

			logAltEvents := func(label string, events0, events []plan.Event) {
				var (
					rise0, riseH, set0, setH       float64
					haveR0, haveRH, haveS0, haveSH bool
				)
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
						t.Errorf("%s sunrise should be earlier at 8849m (shift=%.1f minutes)", label, shift)
					}
				}

				if haveS0 && haveSH {
					shift := setH - set0
					t.Logf("%s sunset shift: %.1f min later at 8849m", label, shift)

					if shift < 3 {
						t.Errorf("%s sunset should be later at 8849m (shift=%.1f minutes)", label, shift)
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
	requireUSNO(t)

	eph := newEph(t)
	loc := edgeCaseLocations[3] // Equator (0°, 0°)

	dates := []string{"2026-03-20", "2026-06-21", "2026-12-21"}

	for _, dateStr := range dates {
		t.Run("Equator/"+dateStr, func(t *testing.T) {
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
			if _, err := fmt.Sscanf(dateStr, "%d-%d-%d", &y, &mo, &d); err != nil {
				t.Fatalf("parsing the fixture date: %v", err)
			}

			geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), unit.Meters(loc.Height))
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}

			site, err := plan.NewSite(loc.Name, geodetic, plan.WithTimeZone(time.LocationUTC))
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.LocationUTC)
			end := start.Add(unit.Hours(24))

			// Sun events
			sunEvents, err := plan.SunEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("SunEvents failed: %v", err)
			}

			compareUSNOEvents(t, "Sun", resp.Properties.Data.SunData, sunEvents, site.SunRiseSetThreshold(), start, time.LocationUTC)

			// At the equator, day length should always be ~12h (± 10 minutes)
			var (
				riseMin, setMin   float64
				haveRise, haveSet bool
			)
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
	requireUSNO(t)

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

			// Set up astrogo
			var y, mo, d int
			if _, err := fmt.Sscanf(tc.date, "%d-%d-%d", &y, &mo, &d); err != nil {
				t.Fatalf("parsing the fixture date: %v", err)
			}

			tz := time.LocationUTC

			if loc.TZName != "" {
				var err error

				tz, err = time.LoadLocation(loc.TZName)
				if err != nil {
					t.Fatalf("Failed to load timezone: %v", err)
				}
			}

			geodetic, err := coord.NewGeodetic(angle.Deg(loc.Lon), angle.Deg(loc.Lat), unit.Meters(loc.Height))
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}

			site, err := plan.NewSite(loc.Name, geodetic, plan.WithTimeZone(tz))
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			start := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, tz)
			end := start.Add(unit.Hours(24))

			moonEvents, err := plan.MoonEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("MoonEvents failed: %v", err)
			}

			// In every response these tests receive, USNO leaves out an event that
			// does not happen rather than listing it as null, so a Moon that never
			// rises is a day with no "Rise" entry.
			// The test used to look for the null, so an astrogo moonrise on such a
			// day could not fail it. compareUSNOEvents holds the counts both ways.
			compareUSNOEvents(t, "Moon", resp.Properties.Data.MoonData, moonEvents, site.MoonRiseSetThreshold(), start, tz)
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

	var (
		riseTimes []float64
		setTimes  []float64
	)

	for _, ac := range altCases {
		t.Run(ac.name, func(t *testing.T) {
			geodetic, err := coord.NewGeodetic(angle.Deg(86.925), angle.Deg(27.9881), unit.Meters(ac.height))
			if err != nil {
				t.Fatalf("Failed to create geodetic: %v", err)
			}

			tz, _ := time.LoadLocation("Asia/Kathmandu")

			site, err := plan.NewSite(ac.name, geodetic, plan.WithTimeZone(tz))
			if err != nil {
				t.Fatalf("Failed to create site: %v", err)
			}

			t.Logf("Height=%.0fm  HorizonDip=%.4f°  SunThreshold=%.4f°",
				ac.height, site.HorizonDip().Degrees(), site.SunRiseSetThreshold().Degrees())

			start := time.Date(2026, time.March, 20, 0, 0, 0, 0, tz)
			end := start.Add(unit.Hours(24))

			sunEvents, err := plan.SunEvents(start, end, site, eph)
			if err != nil {
				t.Fatalf("SunEvents failed: %v", err)
			}

			for _, ev := range sunEvents {
				minutes := eventMinutesIn(ev.Time, tz)
				//nolint:exhaustive // counts the named kinds; the rest are legitimately not this test's subject
				switch ev.Kind {
				case plan.EventRise:
					t.Logf("Sunrise: %s (%.1f min from midnight)", ev.Time.In(tz).Format("15:04:05"), minutes)
					riseTimes = append(riseTimes, minutes)
				case plan.EventSet:
					t.Logf("Sunset:  %s (%.1f min from midnight)", ev.Time.In(tz).Format("15:04:05"), minutes)
					setTimes = append(setTimes, minutes)
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

// ── Helper: compareUSNOEvents ────────────────────────────────────────────────

// usnoTolerance is how far, in minutes, an astrogo rise, set or upper transit
// may sit from USNO's: for the Sun and the Moon alike, at every site here.
//
// USNO publishes whole minutes, rounded, so half a minute of any residual is
// USNO's rounding. The other half is for what the two models do differently,
// chiefly the Moon's semi-diameter, which astrogo holds at its mean (#693):
// up to 28 s at 60°N. Measured on 2026-10-09 over every comparison in this
// file, Tromsø at 69.6°N included: 0.6 min at worst.
//
// It used to be 2 minutes for the Sun and 3 for the Moon, and 5 at the polar
// sites. Neither caught a body rising on its center instead of its upper
// limb: with each semi-diameter removed, the Sun's worst residual was 2.1 min
// and the Moon's 2.3, and the Moon's test still passed (#684).
const usnoTolerance = 1.0

// usnoEventKinds maps the USNO phenomena compareUSNOEvents holds astrogo to.
var usnoEventKinds = map[string]plan.EventKind{
	"Rise":          plan.EventRise,
	"Set":           plan.EventSet,
	"Upper Transit": plan.EventTransit,
}

// compareUSNOEvents holds every rise, set and upper transit USNO lists for
// one day to astrogo's, within usnoTolerance. day is that day's local
// midnight in tz, the zone USNO was asked for; astro is astrogo's events over
// the same day; horizon is the body's rise/set altitude.
//
// Each USNO event is paired with the astrogo event of its kind nearest to it,
// not the first of its kind, which is another event whenever astrogo reports
// one USNO does not. Both directions are held: a USNO event with no astrogo
// event of its kind is an error, since the site, day and convention are the
// same, and so is an astrogo event USNO does not list.
//
// USNO lists an upper transit only while the body is up: none at the North
// Pole in the polar night, one in the midnight sun. astrogo reports every
// culmination, as [plan.EventTransit] says, so the transits held here are the
// ones above horizon.
func compareUSNOEvents(
	t *testing.T, body string, usno []usnoPhenomenon, astro []plan.Event, horizon angle.Angle, day time.Time, tz *time.Location,
) {
	t.Helper()

	var listable []plan.Event

	for _, ev := range astro {
		if ev.Kind != plan.EventTransit || ev.GeometricAltitude > horizon {
			listable = append(listable, ev)
		}
	}

	astro = listable
	local := day.GoTime().In(tz)
	listed := map[plan.EventKind]int{}

	for _, p := range usno {
		kind, ok := usnoEventKinds[p.Phen]
		if !ok {
			continue
		}

		h, m, ok := parseUSNOTime(p.Time)
		if !ok {
			continue
		}

		listed[kind]++

		at := time.Date(local.Year(), local.Month(), local.Day(), h, m, 0, 0, tz).GoTime()

		var nearest *plan.Event

		best := math.Inf(1)

		for i := range astro {
			if astro[i].Kind != kind {
				continue
			}

			if d := astro[i].Time.GoTime().Sub(at).Abs().Minutes(); d < best {
				best, nearest = d, &astro[i]
			}
		}

		if nearest == nil {
			t.Errorf("%s %s: USNO lists %02d:%02d, astrogo found none", body, p.Phen, h, m)

			continue
		}

		t.Logf("%s %-13s  USNO=%02d:%02d  astrogo=%s  Δ=%.2f min",
			body, p.Phen, h, m, nearest.Time.In(tz).Format("15:04:05"), best)

		if best > usnoTolerance {
			t.Errorf("%s %s: Δ=%.2f min exceeds %.0f min", body, p.Phen, best, usnoTolerance)
		}
	}

	for phen, kind := range usnoEventKinds {
		found := 0

		for _, ev := range astro {
			if ev.Kind == kind {
				found++
			}
		}

		if found != listed[kind] {
			t.Errorf("%s %s: astrogo found %d, USNO lists %d", body, phen, found, listed[kind])
		}
	}
}

// TestUSNOReachabilityDoesNotRememberAFailure pins the defect this file's
// pre-check used to have, without needing a network.
//
// Both answers used to live in a sync.Once, so one unlucky probe skipped all
// fourteen TestUSNO_* functions for the rest of the binary — reported as SKIP,
// which reads as a pass. The host resolves to a single address and drops SYNs
// intermittently: measured while it answered curl in 2.2s, a 5s dial overran
// to 17.3s and failed, and the very next one connected in 140ms (#225).
//
// So a failure has to be retried and a success may be kept. Asserting both
// directions, because remembering nothing would be correct but would pay a
// probe per call forever, and remembering everything is the bug.
func TestUSNOReachabilityDoesNotRememberAFailure(t *testing.T) {
	usnoReachableMu.Lock()
	saved := usnoReachableOK
	usnoReachableOK = false
	usnoReachableMu.Unlock()

	t.Cleanup(func() {
		usnoReachableMu.Lock()
		usnoReachableOK = saved
		usnoReachableMu.Unlock()
	})

	calls := 0
	answer := false
	probe := func() bool { calls++; return answer }

	if usnoReachable(probe) {
		t.Fatal("reported reachable while the probe was failing")
	}

	if usnoReachable(probe) {
		t.Fatal("reported reachable while the probe was failing")
	}

	if calls != 2 {
		t.Errorf("probe ran %d times across two failures, want 2 — a cached failure "+
			"is what retired the whole suite on one dropped packet", calls)
	}

	// The host comes back, as it did 140 ms later.
	answer = true

	if !usnoReachable(probe) {
		t.Fatal("a recovered host was still reported unreachable")
	}

	if calls != 3 {
		t.Errorf("probe ran %d times, want 3", calls)
	}

	// And a success is kept, so a real outage costs one probe per top-level
	// test rather than one per usnoGet call site.
	if !usnoReachable(probe) || calls != 3 {
		t.Errorf("probe ran %d times after succeeding, want it remembered at 3", calls)
	}
}
