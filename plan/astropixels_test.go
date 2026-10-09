//go:build integration

package plan_test

// Package plan_test contains integration tests that validate astrogo's
// moon phase computations against Fred Espenak's Six Millennium Catalog
// of Phases of the Moon (AstroPixels).
//
// Run with: go test -tags integration -run TestAstroPixels -v -timeout 60m ./plan/
//
// These tests require an active internet connection to reach
// https://astropixels.com/ephemeris/phasescat/ pages.
// They also require a JPL DE441 kernel (auto-downloaded on first run, ~1.5 GB).

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

// ── AstroPixels Reference Types ──────────────────────────────────────────────

// apPhaseEvent represents a single moon phase event from AstroPixels.
type apPhaseEvent struct {
	Year, Month, Day int
	Hour, Min        int
	Phase            plan.MoonPhase
	JDut             float64 // Julian Day in UT
	JDtd             float64 // Julian Day in TD (after ΔT correction)
	IsJulianCal      bool    // true if date is Julian calendar (before 1582 Oct 15)
}

// ── AstroPixels HTML Parser ──────────────────────────────────────────────────

var monthMap = map[string]int{
	"Jan": 1, "Feb": 2, "Mar": 3, "Apr": 4, "May": 5, "Jun": 6,
	"Jul": 7, "Aug": 8, "Sep": 9, "Oct": 10, "Nov": 11, "Dec": 12,
}

// phaseEntryRegex matches entries like "Jan 13  10:58" or "Jun 10  03:41 T"
var phaseEntryRegex = regexp.MustCompile(`([A-Z][a-z]{2})\s+(\d{1,2})\s+(\d{2}):(\d{2})(?:\s+([TAPHtpn]))?`)

// parseAstroPixelsPage parses a century page and returns all phase events.
func parseAstroPixelsPage(html string) []apPhaseEvent {
	var events []apPhaseEvent

	currentYear := 0

	// Find all <pre> blocks
	lines := strings.Split(html, "\n")
	inPre := false

	for _, rawLine := range lines {
		if strings.Contains(rawLine, "<pre>") {
			inPre = true
			continue
		}

		if strings.Contains(rawLine, "</pre>") {
			inPre = false
			continue
		}

		if !inPre {
			continue
		}

		// Strip HTML tags (like <br/>, <a>)
		line := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(rawLine, "")

		// Skip header lines
		if strings.Contains(line, "Year") && strings.Contains(line, "New Moon") {
			continue
		}

		// Check for year line: " YYYY " at the start
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Try to parse year from the beginning of the line
		if len(line) >= 5 {
			yearStr := strings.TrimSpace(line[0:6])
			if y, err := strconv.Atoi(yearStr); err == nil && y > 0 && y <= 4100 {
				currentYear = y
			}
		}

		if currentYear == 0 {
			continue
		}

		// Parse phase entries by column position.
		// The columns are approximately:
		//   New Moon:      chars 8-22
		//   First Quarter: chars 23-41
		//   Full Moon:     chars 42-57
		//   Last Quarter:  chars 58-77
		type colDef struct {
			start, end int
			phase      plan.MoonPhase
		}

		cols := []colDef{
			{8, 24, plan.PhaseNewMoon},
			{24, 43, plan.PhaseFirstQuarter},
			{43, 59, plan.PhaseFullMoon},
			{59, 78, plan.PhaseLastQuarter},
		}

		for _, col := range cols {
			if len(line) < col.start {
				continue
			}

			end := min(col.end, len(line))

			field := line[col.start:end]

			matches := phaseEntryRegex.FindStringSubmatch(field)
			if matches == nil {
				continue
			}

			month := monthMap[matches[1]]
			day, _ := strconv.Atoi(matches[2])
			hour, _ := strconv.Atoi(matches[3])
			minute, _ := strconv.Atoi(matches[4])

			if month == 0 {
				continue
			}

			isJulian := currentYear < 1582 || (currentYear == 1582 && month < 10) ||
				(currentYear == 1582 && month == 10 && day < 15)

			// AstroPixels prints Universal Time, UT1, from the ΔT it states for
			// each century. The label is therefore read as UT1 and converted by
			// time's one ΔT (#696). It used to be read as UTC, which from 1972
			// adds TT − UTC rather than ΔT: the same number while leap seconds
			// keep UT1 near UTC, but not a statement of the same thing.
			var label time.Time
			if isJulian {
				label = time.DateJulianCal(currentYear, month, day, hour, minute, 0)
			} else {
				label = time.Date(currentYear, time.Month(month), day, hour, minute, 0, 0, time.LocationUTC)
			}

			j1, j2 := label.JDParts()
			tUT := time.FromJDParts(j1, j2, time.UT1)

			events = append(events, apPhaseEvent{
				Year: currentYear, Month: month, Day: day,
				Hour: hour, Min: minute,
				Phase:       col.phase,
				JDut:        tUT.JD(),
				JDtd:        tUT.TDB().JD(),
				IsJulianCal: isJulian,
			})
		}
	}

	return events
}

// fetchAstroPixelsPage downloads a single AstroPixels century page.
func fetchAstroPixelsPage(t *testing.T, startYear int) string {
	t.Helper()

	url := fmt.Sprintf("https://astropixels.com/ephemeris/phasescat/phases%04d.html", startYear)
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("Failed to create request for %s: %v", url, err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; astrogo-test/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		// One call: SkipOnUpstreamFailure consults Unreachable too, so a
		// refused dial and a DNS failure are covered here as well.
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("AstroPixels request for %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// 5xx and 429 are the service having a bad day. Every other status —
		// a 404 above all — means this test asked for the wrong page, which is
		// the defect it exists to catch and used to skip past.
		if resp.StatusCode >= http.StatusInternalServerError ||
			resp.StatusCode == http.StatusTooManyRequests {
			t.Skipf("AstroPixels answered %d for %s", resp.StatusCode, url)
		}

		t.Fatalf("AstroPixels answered %d for %s", resp.StatusCode, url)
	}

	// The request succeeded and the status was 200, so the two guards above
	// have already passed; a failure here is the connection dropping
	// mid-body, which is the server's doing and not a defect in this parser.
	body, err := io.ReadAll(resp.Body)
	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("Failed to read AstroPixels response: %v", err)
	}

	return string(body)
}

// ── Test: Moon Phases vs AstroPixels ─────────────────────────────────────────

// astroPixelsBound is how far, in minutes, an astrogo phase may sit from
// AstroPixels' in the centuries the comparison is like for like: half a
// minute of rounding, AstroPixels printing whole minutes, plus half a minute.
const astroPixelsBound = 1.0

// TestAstroPixels_MoonPhases holds astrogo's Moon phases to Fred Espenak's
// AstroPixels catalog where the two answer the same question, and reports the
// rest.
//
// # Like for like
//
// Both define a phase by apparent longitudes. From 1801 to 2000 the signed
// mean residual is under a second, and the mean |Δ| of 0.25 min is what minute
// rounding alone gives; geometric longitudes put every century 0.65 min out,
// the Sun's aberration (#694).
//
// Both use the same ΔT before 1960, Espenak & Meeus (2006) with the
// −25.858″/cy² correction: each century page states the range it used, and
// time.DeltaT matches it to the page's printed resolution. AstroPixels prints
// UT1, so the parser reads its labels as UT1 and lets time convert them.
//
// They do not share a lunar theory. AstroPixels is "based on procedures
// described in Astronomical Algorithms by Jean Meeus"; astrogo reads DE441.
// The two agree within the rounding from 1501 to 2000, which is the span held
// to astroPixelsBound. Before 1501 they drift apart by a time offset common to
// all four phases, 0.5 min in the 11th century and 4.4 min in the first, a
// comparison of the reference's theory rather than of astrogo, so it is
// reported. So is 2001–2100: AstroPixels converts with its own ΔT forecast,
// 128 s by 2100, and this binary loads no IERS bulletin, so astrogo's side is
// the zero-DUT1 fallback; modern phases are held to USNO and Skyfield instead.
//
// Every phase still has to be found within two days in every century: a
// calendar or time-scale fault there is a miss, not a residual. Until #694
// every century was held to 30 minutes, 40 times the modern residual, and
// a phase over 5 minutes was only logged.
func TestAstroPixels_MoonPhases(t *testing.T) {
	// Load both DE441 parts for full coverage: part-1 (deep historical) + part-2 (modern/future)
	prov, err := eph.NewProvider(kernelContext(t), eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
	requireKernel(t, "DE441 provider", err)

	defer func() { _ = prov.Close() }()

	// Century start years to test — spans the full catalog
	// AstroPixels covers 0001-4000 CE (common era pages)
	centuryStarts := []int{
		1,    // 1st century CE
		101,  // 2nd century
		501,  // 6th century
		1001, // 11th century
		1501, // 16th century (Julian/Gregorian transition)
		1601, // 17th century
		1801, // 19th century
		1901, // 20th century
		2001, // 21st century
	}

	for _, centuryStart := range centuryStarts {
		t.Run(fmt.Sprintf("Century_%04d", centuryStart), func(t *testing.T) {
			html := fetchAstroPixelsPage(t, centuryStart)
			refEvents := parseAstroPixelsPage(html)

			if len(refEvents) == 0 {
				t.Fatalf("No events parsed — parser may be broken")
			}

			bounded := centuryStart >= 1501 && centuryStart <= 1901

			var (
				signedSum, absSum, maxAbs float64
				count                     int
			)

			for _, ref := range refEvents {
				// Use TDB scale for TD reference time (TDB ≈ TT to ~1.7ms)
				// This avoids the LSK adding 32.184s on top of the already-corrected TD JD
				refTime := time.FromJD(ref.JDtd, time.TDB)

				// DE441 covers every date on these pages, so an error from the
				// search is astrogo's, and a miss rather than a skip.
				phases, err := plan.MoonPhases(refTime.Add(unit.Days(-2)), refTime.Add(unit.Days(2)), prov)
				if err != nil {
					t.Errorf("  FAIL %04d-%02d-%02d %02d:%02d %s: MoonPhases: %v",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.Phase, err)

					continue
				}

				signed, found := nearestPhase(phases, ref)
				if !found {
					t.Errorf("  MISS %04d-%02d-%02d %02d:%02d %-14s: no matching phase found in ±2d window",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.Phase)

					continue
				}

				count++
				signedSum += signed
				absSum += math.Abs(signed)
				maxAbs = math.Max(maxAbs, math.Abs(signed))

				if bounded && math.Abs(signed) > astroPixelsBound {
					t.Errorf("  FAIL %04d-%02d-%02d %02d:%02d %-14s  Δ=%+.2f min exceeds %.0f min",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.Phase, signed, astroPixelsBound)
				}
			}

			scope := "reported"
			if bounded {
				scope = "held to 1 min"
			}

			if count > 0 {
				t.Logf("Century %04d: %d phases, signed mean %+.3f min, mean |Δ| %.3f, max |Δ| %.2f (%s)",
					centuryStart, count, signedSum/float64(count), absSum/float64(count), maxAbs, scope)
			}
		})
	}
}

// nearestPhase is astrogo − AstroPixels in minutes for the phase of ref's kind
// nearest it, and whether there was one.
func nearestPhase(phases []plan.MoonPhaseEvent, ref apPhaseEvent) (signed float64, found bool) {
	best := math.Inf(1)

	for _, p := range phases {
		if p.Phase != ref.Phase {
			continue
		}

		if d := (p.Time.JD() - ref.JDtd) * 24 * 60; math.Abs(d) < best {
			best, signed, found = math.Abs(d), d, true
		}
	}

	return signed, found
}
