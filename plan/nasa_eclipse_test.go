//go:build integration

package plan_test

// Package plan_test contains integration tests that validate astrogo's
// eclipse detection against NASA's Five Millennium Catalogs of
// Solar and Lunar Eclipses (eclipse.gsfc.nasa.gov).
//
// Run with: go test -tags integration -run TestNASA -v -timeout 60m ./plan/
//
// These tests require an active internet connection to reach
// https://eclipse.gsfc.nasa.gov/ catalog pages.
// They also require a JPL DE441 kernel (auto-downloaded on first run, ~1.5 GB).
//
// Under a shorter ambient -timeout (e.g. CI's generic 10m integration job,
// well under the 60m this file needs from a cold cache), requireNASA and
// nasaBudgetOK makes these tests skip cleanly — with a clear message —
// rather than let the ambient timeout kill the whole test binary mid
// request. Run with the full 60m budget locally for complete coverage.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// ── NASA Eclipse Reference Types ─────────────────────────────────────────────

type nasaEclipseRef struct {
	Year, Month, Day int
	Hour, Min, Sec   int
	DeltaT           float64 // ΔT in seconds (from NASA catalog)
	EclipseType      string  // "T", "P", "N" (lunar) or "T", "A", "H", "P" (solar)
	JDtd             float64 // Julian Day in TD

	// Magnitude is the canon's eclipse magnitude: the umbral magnitude for a
	// lunar eclipse, the eclipse magnitude at greatest eclipse for a solar
	// one. PenumbralMagnitude is the lunar penumbral magnitude.
	Magnitude, PenumbralMagnitude float64
}

// ── NASA Eclipse Catalog Parser ──────────────────────────────────────────────

// catalogNumber matches the five-digit catalog number that opens every row of
// NASA's eclipse catalog pages.
var catalogNumber = regexp.MustCompile(`^\d{5}$`)

// errCatalogField is a field of a recognized catalog row that does not parse.
var errCatalogField = errors.New("malformed catalog field")

// catalogFields are the date, time and ΔT columns every catalog row carries,
// in the same place on the lunar and solar pages.
type catalogFields struct {
	year, month, day, hour, minute, sec int
	deltaT                              float64
}

// parseCatalogFields reads parts[1:6] of a recognized catalog row.
func parseCatalogFields(parts []string) (catalogFields, error) {
	var f catalogFields

	year, err := strconv.Atoi(parts[1])
	if err != nil {
		return f, fmt.Errorf("%w: year %q", errCatalogField, parts[1])
	}

	month, ok := monthMap[parts[2]]
	if !ok {
		return f, fmt.Errorf("%w: month %q", errCatalogField, parts[2])
	}

	day, err := strconv.Atoi(parts[3])
	if err != nil {
		return f, fmt.Errorf("%w: day %q", errCatalogField, parts[3])
	}

	hms := strings.Split(parts[4], ":")
	if len(hms) != 3 {
		return f, fmt.Errorf("%w: time %q", errCatalogField, parts[4])
	}

	var clock [3]int

	for i, v := range hms {
		if clock[i], err = strconv.Atoi(v); err != nil {
			return f, fmt.Errorf("%w: time %q", errCatalogField, parts[4])
		}
	}

	deltaT, err := strconv.ParseFloat(parts[5], 64)
	if err != nil {
		return f, fmt.Errorf("%w: ΔT %q", errCatalogField, parts[5])
	}

	return catalogFields{year, month, day, clock[0], clock[1], clock[2], deltaT}, nil
}

// lunarType and solarType are the eclipse-type column (the ninth field of a
// row) as each catalog's key defines it: the type letter, then at most one
// qualifier. Lunar (LEcat5/LEcatkey.html): N penumbral, P partial, T total;
// m middle of its saros, + and - central total north and south of the shadow
// axis, * total penumbral, b and e first and last of its saros. Solar
// (SEcat5/SEcatkey.html): P partial, A annular, T total, H hybrid; m middle of
// its saros, n and s central with no northern or southern limit, + and -
// non-central with none, 2 and 3 hybrid beginning total or annular, b and e
// first and last of its saros.
//
// The pages print the lunar key's * as x: all 19 Nx rows on the six lunar
// pages these tests read have a penumbral magnitude of 1 or more, which is what
// a total penumbral eclipse is.
//
// Until #398 each parser kept only the codes it listed, and dropped any other
// row without a word: 28 lunar and 50 solar rows, the marginal penumbrals and
// the non-central and hybrid eclipses a detector is likeliest to miss.
var (
	lunarType = regexp.MustCompile(`^([NPT])[m+*bex-]?$`)
	solarType = regexp.MustCompile(`^([PATH])[mns+23be-]?$`)
)

// eclipseType returns the type letter of a catalog row's type field, or an
// error wrapping errCatalogField when the key does not define the field.
func eclipseType(key *regexp.Regexp, field string) (string, error) {
	m := key.FindStringSubmatch(field)
	if m == nil {
		return "", fmt.Errorf("%w: eclipse type %q", errCatalogField, field)
	}

	return m[1], nil
}

// parseNASALunarEclipses parses a NASA lunar eclipse catalog page.
func parseNASALunarEclipses(t *testing.T, html string) []nasaEclipseRef {
	t.Helper()

	return parseNASACatalog(t, html, lunarType)
}

// parseNASASolarEclipses parses a NASA solar eclipse catalog page.
func parseNASASolarEclipses(t *testing.T, html string) []nasaEclipseRef {
	t.Helper()

	return parseNASACatalog(t, html, solarType)
}

// parseNASACatalog parses a NASA eclipse catalog page, lunar or solar. The two
// lay out every column this reads identically and differ only in the eclipse
// types their keys define; key selects which.
func parseNASACatalog(t *testing.T, html string, key *regexp.Regexp) []nasaEclipseRef {
	t.Helper()

	var eclipses []nasaEclipseRef

	clean := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")
	lines := strings.SplitSeq(clean, "\n")

	for line := range lines {
		trimmed := strings.TrimSpace(line)

		// A catalog row starts with its five-digit catalog number; every other
		// line on the page is prose, headers or rules. Past this point the line
		// is a row, and a field in it that does not parse is a malformed row,
		// reported rather than dropped: a quietly shorter reference set still
		// compares, and passes, on whatever rows are left.
		//
		// Format: "04824  0001 Jun 24  12:08:47  10519 -24719   78   P   ..."
		parts := strings.Fields(trimmed)
		if len(parts) == 0 || !catalogNumber.MatchString(parts[0]) {
			continue
		}

		if len(parts) < 9 {
			t.Errorf("NASA catalog row %q: %v: %d fields, the key defines 9 before the type-specific ones",
				trimmed, errCatalogField, len(parts))

			continue
		}

		fields, err := parseCatalogFields(parts)
		if err != nil {
			t.Errorf("NASA catalog row %q: %v", trimmed, err)

			continue
		}

		year, month, day := fields.year, fields.month, fields.day
		hour, minute, sec, dt := fields.hour, fields.minute, fields.sec, fields.deltaT

		eclType, err := eclipseType(key, parts[8])
		if err != nil {
			t.Errorf("NASA catalog row %q: %v", trimmed, err)

			continue
		}

		magnitude, penumbral, err := catalogMagnitudes(key, parts)
		if err != nil {
			t.Errorf("NASA catalog row %q: %v", trimmed, err)

			continue
		}

		// NASA catalog uses Julian calendar before 1582-10-15, times are in TD ≈ TDB
		isJulianCal := year < 1582 || (year == 1582 && month < 10) || (year == 1582 && month == 10 && day < 15)

		var jdTD float64
		if isJulianCal {
			jdTD = time.DateJulianCal(year, month, day, hour, minute, sec).JD()
		} else {
			jdTD = time.Date(year, time.Month(month), day, hour, minute, sec, 0, time.LocationUTC).JD()
		}

		eclipses = append(eclipses, nasaEclipseRef{
			Year: year, Month: month, Day: day,
			Hour: hour, Min: minute, Sec: sec,
			DeltaT:             dt,
			EclipseType:        eclType,
			JDtd:               jdTD,
			Magnitude:          magnitude,
			PenumbralMagnitude: penumbral,
		})
	}

	return eclipses
}

// catalogMagnitudes reads a row's magnitude columns: on the lunar pages the
// penumbral and umbral magnitudes (fields 12 and 13), on the solar pages the
// eclipse magnitude (field 12).
func catalogMagnitudes(key *regexp.Regexp, parts []string) (magnitude, penumbral float64, err error) {
	if len(parts) < 13 {
		return 0, 0, fmt.Errorf("%w: %d fields, too few for the magnitudes", errCatalogField, len(parts))
	}

	first, err := strconv.ParseFloat(parts[11], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: magnitude %q", errCatalogField, parts[11])
	}

	if key != lunarType {
		return first, 0, nil
	}

	umbral, err := strconv.ParseFloat(parts[12], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("%w: umbral magnitude %q", errCatalogField, parts[12])
	}

	return umbral, first, nil
}

// catalogKinds maps the canon's type letters to astrogo's kinds.
var catalogKinds = map[string]plan.EclipseKind{
	"N": plan.EclipsePenumbral,
	"P": plan.EclipsePartial,
	"T": plan.EclipseTotal,
	"A": plan.EclipseAnnular,
	"H": plan.EclipseHybrid,
}

// catalogMagnitudeTol bounds astrogo's eclipse magnitudes against the canon's,
// which prints four decimals. Measured against DE441 over the six centuries
// this file reads and 2001–2100 besides: 0.0004 at worst for the lunar
// penumbral and umbral magnitudes, 0.00026 for the solar. Every kind agrees,
// the 77 hybrids on these pages included.
const catalogMagnitudeTol = 0.002

// checkKindAndMagnitude compares an eclipse astrogo found with the catalog
// row it matched (#405).
func checkKindAndMagnitude(t *testing.T, kind string, ref nasaEclipseRef, e plan.EclipseEvent) {
	t.Helper()

	date := fmt.Sprintf("%s %04d-%02d-%02d", kind, ref.Year, ref.Month, ref.Day)

	if want := catalogKinds[ref.EclipseType]; e.Kind != want {
		t.Errorf("  KIND %s: %v, the catalog says %v", date, e.Kind, want)
	}

	if d := e.Magnitude - ref.Magnitude; math.Abs(d) > catalogMagnitudeTol {
		t.Errorf("  MAG %s: magnitude %.4f, the catalog gives %.4f (%+.4f)", date, e.Magnitude, ref.Magnitude, d)
	}

	if d := e.PenumbralMagnitude - ref.PenumbralMagnitude; math.Abs(d) > catalogMagnitudeTol {
		t.Errorf("  MAG %s: penumbral magnitude %.4f, the catalog gives %.4f (%+.4f)", date, e.PenumbralMagnitude, ref.PenumbralMagnitude, d)
	}
}

// greatestEclipseTolMinutes bounds how far astrogo's greatest eclipse may fall
// from the canon's, which prints it to the second. Measured over all 2885
// eclipses on these pages: 0.16 minutes on average, 0.55 at worst. Drawing the
// shadow axis from the geometric Sun instead of the apparent one moves it by
// 0.8 minutes on average, which is what this is set to catch (#401). It used
// to be logged, and only past an hour.
const greatestEclipseTolMinutes = 1.0

// checkNoEclipseOutsideCatalog is the comparison's other direction: every
// eclipse astrogo reports over a catalog page's span is one the page lists.
//
// Until #401 only the catalog's side was asked — is each listed eclipse
// found? — and astrogo, deciding by a fixed 1.58° latitude limit, reported 76
// lunar and 97 solar eclipses over these six centuries that do not happen.
// Nothing looked. The window is the page's first to last eclipse, a day
// either side, so no eclipse belonging to a neighboring page can fall in it.
func checkNoEclipseOutsideCatalog(t *testing.T, kind string,
	find func(start, end time.Time, prov eph.Provider) ([]plan.EclipseEvent, error),
	refs []nasaEclipseRef, prov eph.Provider,
) {
	t.Helper()

	start := time.FromJD(refs[0].JDtd-1, time.TDB)
	end := time.FromJD(refs[len(refs)-1].JDtd+1, time.TDB)

	events, err := find(start, end, prov)
	if err != nil {
		t.Errorf("  FAIL %s over the page: %v", kind, err)

		return
	}

	var extra int

	for _, e := range events {
		listed := false

		for _, ref := range refs {
			if math.Abs(e.Time.JD()-ref.JDtd) < 2 {
				listed = true

				break
			}
		}

		if !listed {
			extra++

			t.Errorf("  EXTRA %s JD %.4f (TDB): astrogo reports an eclipse the catalog does not list (Gamma %.4f, |β| %.3f°)",
				kind, e.Time.JD(), e.Gamma, math.Abs(e.EclipticLatitude.Degrees()))
		}
	}

	t.Logf("%s: %d eclipses reported over the page, %d not in the catalog", kind, len(events), extra)
}

// ── Fetch Helper ─────────────────────────────────────────────────────────────

// requireNASA skips the calling test when eclipse.gsfc.nasa.gov is
// unreachable — the same TCP pre-check every other live-network test in
// this repo uses (see e.g. catalog/simbad's requireSimbad), so an
// unreachable/blackholed host is skipped in seconds instead of grinding
// through fetchNASAPage's own per-request timeout across every century
// subtest until the ambient `go test -timeout` kills the whole binary.
func requireNASA(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "eclipse.gsfc.nasa.gov:443")
}

// nasaBudgetOK skips the test unless at least margin remains before the ambient
// `go test -timeout` deadline, skipping the calling (sub)test and
// returning false otherwise. This file's own doc comment recommends
// `-timeout 60m` for a from-cold-cache DE441 download (multi-GB) plus a
// dozen live NASA fetches; CI's generic integration job instead applies a
// blanket 10m across every package, which this suite alone can't reliably
// finish within. Checking the remaining budget before each expensive step
// turns a hard mid-request kill (a confusing goroutine-dump failure) into
// a clean, explained skip.
func nasaBudgetOK(t *testing.T, margin time.Duration) {
	t.Helper()

	deadline, ok := t.Deadline()
	if !ok {
		return
	}

	if time.Until(deadline) < margin {
		t.Skip("not enough time left before the test binary's -timeout for this step — rerun with a longer -timeout (see this file's package doc) for full coverage")
	}
}

func fetchNASAPage(t *testing.T, url string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("Failed to create request for %s: %v", url, err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; astrogo-test/1.0)")

	// Belt-and-suspenders: the request context above already bounds the
	// whole round trip, but Client.Timeout is kept too since it's what
	// every other client in this codebase relies on for cancellation.
	client := &http.Client{Timeout: 25 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		// One call: SkipOnUpstreamFailure consults Unreachable too, so a
		// refused dial and a DNS failure are covered here as well.
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("NASA request for %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// 5xx and 429 are the service having a bad day. Every other status —
		// a 404 above all — means this test asked for the wrong page, which is
		// the defect it exists to catch and used to skip past.
		if resp.StatusCode >= http.StatusInternalServerError ||
			resp.StatusCode == http.StatusTooManyRequests {
			t.Skipf("NASA answered %d for %s", resp.StatusCode, url)
		}

		t.Fatalf("NASA answered %d for %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read NASA response: %v", err)
	}

	return string(body)
}

// ── Test: Lunar Eclipses vs NASA ─────────────────────────────────────────────

//nolint:dupl // parallel blocks over two distinct NASA reference tables; the repetition is what keeps each checkable against its own source
func TestNASA_LunarEclipses_Historical(t *testing.T) {
	requireNASA(t)

	nasaBudgetOK(t, 6*time.Minute)

	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
	requireKernel(t, "DE441 provider", err)

	defer func() { _ = prov.Close() }()

	// Century ranges matching NASA catalog URLs
	centuries := []struct {
		start, end int
		url        string
	}{
		{1, 100, "https://eclipse.gsfc.nasa.gov/LEcat5/LE0001-0100.html"},
		{101, 200, "https://eclipse.gsfc.nasa.gov/LEcat5/LE0101-0200.html"},
		{501, 600, "https://eclipse.gsfc.nasa.gov/LEcat5/LE0501-0600.html"},
		{1001, 1100, "https://eclipse.gsfc.nasa.gov/LEcat5/LE1001-1100.html"},
		{1501, 1600, "https://eclipse.gsfc.nasa.gov/LEcat5/LE1501-1600.html"},
		{1901, 2000, "https://eclipse.gsfc.nasa.gov/LEcat5/LE1901-2000.html"},
	}

	var (
		totalRef, totalDetected int
		totalDelta, maxDelta    float64
	)

	for _, c := range centuries {
		name := fmt.Sprintf("LE_%04d-%04d", c.start, c.end)
		t.Run(name, func(t *testing.T) {
			nasaBudgetOK(t, 45*time.Second)

			html := fetchNASAPage(t, c.url)
			refs := parseNASALunarEclipses(t, html)
			t.Logf("Parsed %d lunar eclipses from NASA %04d-%04d", len(refs), c.start, c.end)

			if len(refs) == 0 {
				t.Fatalf("No eclipses parsed — parser may be broken")
			}

			detected := 0
			tested := 0

			var centuryDelta, centuryMax float64

			for _, ref := range refs {
				tested++
				totalRef++

				// Use TDB scale for TD reference time (TDB ≈ TT to ~1.7ms)
				refTime := time.FromJD(ref.JDtd, time.TDB)
				searchStart := refTime.Add(unit.Days(-30))
				searchEnd := refTime.Add(unit.Days(30))

				// DE441 covers every date on these pages, so an error from the
				// search is astrogo's, and a miss rather than a skip.
				eclipses, err := plan.LunarEclipses(searchStart, searchEnd, prov)
				if err != nil {
					t.Errorf("  FAIL %04d-%02d-%02d: LunarEclipses: %v",
						ref.Year, ref.Month, ref.Day, err)

					continue
				}

				// Find matching eclipse (within ±2 days)
				found := false

				var bestDelta float64

				for _, ecl := range eclipses {
					delta := math.Abs(ecl.Time.JD()-ref.JDtd) * 24 * 60 // minutes
					if delta < 2*24*60 {                                // within 2 days
						found = true
						bestDelta = delta

						checkKindAndMagnitude(t, "LE", ref, ecl)

						break
					}
				}

				if found {
					detected++
					totalDetected++

					centuryDelta += bestDelta
					if bestDelta > centuryMax {
						centuryMax = bestDelta
					}

					totalDelta += bestDelta
					if bestDelta > maxDelta {
						maxDelta = bestDelta
					}

					if bestDelta > greatestEclipseTolMinutes {
						t.Errorf("  LATE LE %04d-%02d-%02d %02d:%02d type=%s: greatest eclipse %.2f min from the catalog's, limit %.1f",
							ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType, bestDelta, greatestEclipseTolMinutes)
					}
				} else {
					// Every miss fails, penumbral ones included. They used to be
					// logged, because astrogo decided an eclipse by a fixed 1.58°
					// latitude limit and missed marginal penumbrals outside it;
					// it now sizes the shadow as the canon does (#401).
					t.Errorf("  MISS LE %04d-%02d-%02d %02d:%02d type=%s: not detected by astrogo",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType)
				}
			}

			checkNoEclipseOutsideCatalog(t, "LE", plan.LunarEclipses, refs, prov)

			if detected > 0 {
				t.Logf("Century %04d-%04d: %d/%d eclipses detected, mean Δ=%.1f minute, max Δ=%.1f min",
					c.start, c.end, detected, tested, centuryDelta/float64(detected), centuryMax)
			} else {
				t.Logf("Century %04d-%04d: %d/%d sampled eclipses detected",
					c.start, c.end, detected, tested)
			}
		})
	}

	t.Logf("\n══════════════════════════════════════════════════════════")

	if totalDetected > 0 {
		t.Logf("NASA Lunar Eclipses: %d/%d detected, mean Δ=%.1f minute, max Δ=%.1f min",
			totalDetected, totalRef, totalDelta/float64(totalDetected), maxDelta)
	} else {
		t.Logf("NASA Lunar Eclipses: %d/%d detected", totalDetected, totalRef)
	}

	t.Logf("══════════════════════════════════════════════════════════")
}

// ── Test: Solar Eclipses vs NASA ─────────────────────────────────────────────

//nolint:dupl // parallel blocks over two distinct NASA reference tables; the repetition is what keeps each checkable against its own source
func TestNASA_SolarEclipses_Historical(t *testing.T) {
	requireNASA(t)

	nasaBudgetOK(t, 6*time.Minute)

	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
	requireKernel(t, "DE441 provider", err)

	defer func() { _ = prov.Close() }()

	centuries := []struct {
		start, end int
		url        string
	}{
		{1, 100, "https://eclipse.gsfc.nasa.gov/SEcat5/SE0001-0100.html"},
		{101, 200, "https://eclipse.gsfc.nasa.gov/SEcat5/SE0101-0200.html"},
		{501, 600, "https://eclipse.gsfc.nasa.gov/SEcat5/SE0501-0600.html"},
		{1001, 1100, "https://eclipse.gsfc.nasa.gov/SEcat5/SE1001-1100.html"},
		{1501, 1600, "https://eclipse.gsfc.nasa.gov/SEcat5/SE1501-1600.html"},
		{1901, 2000, "https://eclipse.gsfc.nasa.gov/SEcat5/SE1901-2000.html"},
	}

	var (
		totalRef, totalDetected int
		totalDelta, maxDelta    float64
	)

	for _, c := range centuries {
		name := fmt.Sprintf("SE_%04d-%04d", c.start, c.end)
		t.Run(name, func(t *testing.T) {
			nasaBudgetOK(t, 45*time.Second)

			html := fetchNASAPage(t, c.url)
			refs := parseNASASolarEclipses(t, html)
			t.Logf("Parsed %d solar eclipses from NASA %04d-%04d", len(refs), c.start, c.end)

			if len(refs) == 0 {
				t.Fatalf("No eclipses parsed — parser may be broken")
			}

			detected := 0
			tested := 0

			var centuryDelta, centuryMax float64

			for _, ref := range refs {
				tested++
				totalRef++

				refTime := time.FromJD(ref.JDtd, time.TDB)
				searchStart := refTime.Add(unit.Days(-30))
				searchEnd := refTime.Add(unit.Days(30))

				// DE441 covers every date on these pages, so an error from the
				// search is astrogo's, and a miss rather than a skip.
				eclipses, err := plan.SolarEclipses(searchStart, searchEnd, prov)
				if err != nil {
					t.Errorf("  FAIL %04d-%02d-%02d: SolarEclipses: %v",
						ref.Year, ref.Month, ref.Day, err)

					continue
				}

				found := false

				var bestDelta float64

				for _, ecl := range eclipses {
					delta := math.Abs(ecl.Time.JD()-ref.JDtd) * 24 * 60
					if delta < 2*24*60 {
						found = true
						bestDelta = delta

						checkKindAndMagnitude(t, "SE", ref, ecl)

						break
					}
				}

				if found {
					detected++
					totalDetected++

					centuryDelta += bestDelta
					if bestDelta > centuryMax {
						centuryMax = bestDelta
					}

					totalDelta += bestDelta
					if bestDelta > maxDelta {
						maxDelta = bestDelta
					}

					if bestDelta > greatestEclipseTolMinutes {
						t.Errorf("  LATE SE %04d-%02d-%02d %02d:%02d type=%s: greatest eclipse %.2f min from the catalog's, limit %.1f",
							ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType, bestDelta, greatestEclipseTolMinutes)
					}
				} else {
					// Every miss fails, partial ones included (#401).
					t.Errorf("  MISS SE %04d-%02d-%02d %02d:%02d type=%s: not detected by astrogo",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType)
				}
			}

			checkNoEclipseOutsideCatalog(t, "SE", plan.SolarEclipses, refs, prov)

			if detected > 0 {
				t.Logf("Century %04d-%04d: %d/%d eclipses detected, mean Δ=%.1f minute, max Δ=%.1f min",
					c.start, c.end, detected, tested, centuryDelta/float64(detected), centuryMax)
			} else {
				t.Logf("Century %04d-%04d: %d/%d eclipses detected",
					c.start, c.end, detected, tested)
			}
		})
	}

	t.Logf("\n══════════════════════════════════════════════════════════")

	if totalDetected > 0 {
		t.Logf("NASA Solar Eclipses: %d/%d detected, mean Δ=%.1f minute, max Δ=%.1f min",
			totalDetected, totalRef, totalDelta/float64(totalDetected), maxDelta)
	} else {
		t.Logf("NASA Solar Eclipses: %d/%d detected", totalDetected, totalRef)
	}

	t.Logf("══════════════════════════════════════════════════════════")
}

// TestNASA_DeltaT_CrossValidation verifies that astrogo's time.DeltaT polynomial
// matches NASA's tabulated ΔT values from the Five Millennium Eclipse Catalog.
// Both sources use the Espenak & Meeus (2006) model, so they should agree closely.
//
// Until #398 it asserted nothing: a difference over 10 s was logged as a
// warning, and the test passed whatever time.DeltaT returned. The bound now
// comes from the two things that separate the sides when the model agrees.
// The catalog prints ΔT in whole seconds, 1 s at most. And this test evaluates
// ΔT at mid-month rather than on the eclipse's date, up to half a month early
// or late, which is 0.4 s in the first century, where ΔT falls fastest, near
// 10 s a year. Measured over 1213 rows: 0.9 s at worst.
func TestNASA_DeltaT_CrossValidation(t *testing.T) {
	const deltaTTolerance = 1.5 // seconds

	requireNASA(t)

	nasaBudgetOK(t, 2*time.Minute)

	centuries := []struct {
		start, end int
		url        string
	}{
		{1, 100, "https://eclipse.gsfc.nasa.gov/LEcat5/LE0001-0100.html"},
		{501, 600, "https://eclipse.gsfc.nasa.gov/LEcat5/LE0501-0600.html"},
		{1001, 1100, "https://eclipse.gsfc.nasa.gov/LEcat5/LE1001-1100.html"},
		{1501, 1600, "https://eclipse.gsfc.nasa.gov/LEcat5/LE1501-1600.html"},
		{1901, 2000, "https://eclipse.gsfc.nasa.gov/LEcat5/LE1901-2000.html"},
	}

	var (
		totalEvents          int
		totalDelta, maxDelta float64
	)

	for _, c := range centuries {
		name := fmt.Sprintf("DeltaT_%04d-%04d", c.start, c.end)
		t.Run(name, func(t *testing.T) {
			nasaBudgetOK(t, 45*time.Second)

			html := fetchNASAPage(t, c.url)
			refs := parseNASALunarEclipses(t, html)

			if len(refs) == 0 {
				t.Fatalf("No eclipses parsed")
			}

			var (
				centuryDelta, centuryMax float64
				count                    int
			)

			// Every row counts. A ΔT of 0 is a value — the catalog prints it
			// for eclipses around 1902, when ΔT crossed zero — and a field
			// that does not parse was already reported by the parser.
			for _, ref := range refs {
				decYear := float64(ref.Year) + (float64(ref.Month)-0.5)/12.0
				computed := time.DeltaT(decYear)
				delta := math.Abs(computed - ref.DeltaT)

				count++
				totalEvents++
				centuryDelta += delta

				totalDelta += delta
				if delta > centuryMax {
					centuryMax = delta
				}

				if delta > maxDelta {
					maxDelta = delta
				}

				if delta > deltaTTolerance {
					t.Errorf("%04d-%02d-%02d: computed ΔT=%.1f s, NASA ΔT=%.0f s, Δ=%.1f s (limit %.1f s)",
						ref.Year, ref.Month, ref.Day, computed, ref.DeltaT, delta, deltaTTolerance)
				}
			}

			if count > 0 {
				t.Logf("Century %04d-%04d: %d eclipses, ΔT mean error=%.1f s, max=%.1f s",
					c.start, c.end, count, centuryDelta/float64(count), centuryMax)
			}
		})
	}

	t.Logf("\n══════════════════════════════════════════════════════════")

	if totalEvents > 0 {
		t.Logf("ΔT Cross-Validation: %d events, mean error=%.1f s, max=%.1f s",
			totalEvents, totalDelta/float64(totalEvents), maxDelta)
	}

	t.Logf("══════════════════════════════════════════════════════════")
}
