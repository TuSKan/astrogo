//go:build integration

// Package plan_test contains integration tests that validate astrogo's
// moon phase computations against Fred Espenak's Six Millennium Catalog
// of Phases of the Moon (AstroPixels).
//
// Run with: go test -tags integration -run TestAstroPixels -v -timeout 60m ./plan/
//
// These tests require an active internet connection to reach
// https://astropixels.com/ephemeris/phasescat/ pages.
// They also require a JPL DE441 kernel (auto-downloaded on first run, ~1.5 GB).
package plan_test

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	gotime "time"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
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
			end := col.end
			if end > len(line) {
				end = len(line)
			}
			field := line[col.start:end]

			matches := phaseEntryRegex.FindStringSubmatch(field)
			if matches == nil {
				continue
			}

			month := monthMap[matches[1]]
			day, _ := strconv.Atoi(matches[2])
			hour, _ := strconv.Atoi(matches[3])
			min, _ := strconv.Atoi(matches[4])

			if month == 0 {
				continue
			}

			isJulian := currentYear < 1582 || (currentYear == 1582 && month < 10) ||
				(currentYear == 1582 && month == 10 && day < 15)

			// Build UT time from calendar; .TDB() auto-applies ΔT for historical dates
			var tUT time.Time
			if isJulian {
				tUT = time.DateJulianCal(currentYear, month, day, hour, min, 0)
			} else {
				tUT = time.Date(currentYear, gotime.Month(month), day, hour, min, 0, 0, gotime.UTC)
			}

			events = append(events, apPhaseEvent{
				Year: currentYear, Month: month, Day: day,
				Hour: hour, Min: min,
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
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request for %s: %v", url, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; astrogo-test/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("AstroPixels unreachable, skipping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Skipf("AstroPixels returned status %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read AstroPixels response: %v", err)
	}
	return string(body)
}

// ── Test: Moon Phases vs AstroPixels ─────────────────────────────────────────

func TestAstroPixels_MoonPhases(t *testing.T) {
	// Load both DE441 parts for full coverage: part-1 (deep historical) + part-2 (modern/future)
	prov, err := eph.NewProvider(eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
	if err != nil {
		t.Fatalf("Failed to create DE441 provider: %v", err)
	}
	defer prov.Close()

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

	// Track global statistics
	var totalEvents, matchedEvents int
	var totalDelta, maxDelta float64
	var maxDeltaEvent string

	for _, centuryStart := range centuryStarts {
		t.Run(fmt.Sprintf("Century_%04d", centuryStart), func(t *testing.T) {
			html := fetchAstroPixelsPage(t, centuryStart)
			refEvents := parseAstroPixelsPage(html)
			t.Logf("Parsed %d phase events from AstroPixels century %04d-%04d",
				len(refEvents), centuryStart, centuryStart+99)

			if len(refEvents) == 0 {
				t.Fatalf("No events parsed — parser may be broken")
			}

			var centuryDelta float64
			var centuryMax float64
			var centuryCount int

			for _, ref := range refEvents {
				totalEvents++

				// Use TDB scale for TD reference time (TDB ≈ TT to ~1.7ms)
				// This avoids the LSK adding 32.184s on top of the already-corrected TD JD
				refTime := time.FromJD(ref.JDtd, time.TDB)
				searchStart := refTime.Add(-2 * 24 * time.Hour)
				searchEnd := refTime.Add(2 * 24 * time.Hour)

				phases, err := plan.MoonPhases(searchStart, searchEnd, prov)
				if err != nil {
					t.Logf("  SKIP %04d-%02d-%02d %02d:%02d %s: MoonPhases error: %v",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.Phase, err)
					continue
				}

				// Find matching phase
				bestDelta := math.MaxFloat64
				found := false
				for _, p := range phases {
					if p.Phase != ref.Phase {
						continue
					}
					delta := math.Abs(p.Time.JD()-ref.JDtd) * 24 * 60 // minutes
					if delta < bestDelta {
						bestDelta = delta
						found = true
					}
				}

				if !found {
					t.Errorf("  MISS %04d-%02d-%02d %02d:%02d %-14s: no matching phase found in ±2d window",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.Phase)
					continue
				}

				matchedEvents++
				centuryDelta += bestDelta
				centuryCount++
				if bestDelta > centuryMax {
					centuryMax = bestDelta
				}
				totalDelta += bestDelta
				if bestDelta > maxDelta {
					maxDelta = bestDelta
					maxDeltaEvent = fmt.Sprintf("%04d-%02d-%02d %s", ref.Year, ref.Month, ref.Day, ref.Phase)
				}

				// Log individual events with large residuals
				if bestDelta > 5.0 {
					t.Logf("  WARN %04d-%02d-%02d %02d:%02d %-14s  Δ=%.1f min",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.Phase, bestDelta)
				}

				if bestDelta > 30.0 {
					t.Errorf("  FAIL %04d-%02d-%02d %02d:%02d %-14s  Δ=%.1f min exceeds 30 min tolerance",
						ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.Phase, bestDelta)
				}
			}

			if centuryCount > 0 {
				t.Logf("Century %04d: %d events, mean Δ=%.1f min, max Δ=%.1f min",
					centuryStart, centuryCount, centuryDelta/float64(centuryCount), centuryMax)
			}
		})
	}

	t.Logf("\n══════════════════════════════════════════════════════════")
	t.Logf("AstroPixels Summary: %d/%d events matched", matchedEvents, totalEvents)
	if matchedEvents > 0 {
		t.Logf("Mean Δ = %.2f min, Max Δ = %.2f min (%s)",
			totalDelta/float64(matchedEvents), maxDelta, maxDeltaEvent)
	}
	t.Logf("══════════════════════════════════════════════════════════")
}

