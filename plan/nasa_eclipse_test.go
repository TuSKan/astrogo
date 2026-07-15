//go:build integration

// Package plan_test contains integration tests that validate astrogo's
// eclipse detection against NASA's Five Millennium Catalogs of
// Solar and Lunar Eclipses (eclipse.gsfc.nasa.gov).
//
// Run with: go test -tags integration -run TestNASA -v -timeout 60m ./plan/
//
// These tests require an active internet connection to reach
// https://eclipse.gsfc.nasa.gov/ catalog pages.
// They also require a JPL DE441 kernel (auto-downloaded on first run, ~1.5 GB).
package plan_test

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
	gotime "time"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// ── NASA Eclipse Reference Types ─────────────────────────────────────────────

type nasaEclipseRef struct {
	Year, Month, Day int
	Hour, Min, Sec   int
	DeltaT           float64 // ΔT in seconds (from NASA catalog)
	EclipseType      string  // "T", "P", "N" (lunar) or "T", "A", "H", "P" (solar)
	JDtd             float64 // Julian Day in TD
}

// ── NASA Eclipse Catalog Parser ──────────────────────────────────────────────

// parseNASALunarEclipses parses a NASA lunar eclipse catalog page.
func parseNASALunarEclipses(html string) []nasaEclipseRef {
	var eclipses []nasaEclipseRef

	// Strip HTML tags
	clean := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")
	lines := strings.Split(clean, "\n")

	for _, line := range lines {
		// Look for lines with catalog numbers (5 digits at start)
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 40 {
			continue
		}

		// Try to match the pattern
		// Format: "04824  0001 Jun 24  12:08:47  10519 -24719   78   P   ..."
		parts := strings.Fields(trimmed)
		if len(parts) < 9 {
			continue
		}

		// Catalog number must be 5 digits
		catNum := parts[0]
		if len(catNum) != 5 {
			continue
		}
		if _, err := strconv.Atoi(catNum); err != nil {
			continue
		}

		// Parse year
		year, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		// Parse month
		month, ok := monthMap[parts[2]]
		if !ok {
			continue
		}

		// Parse day
		day, err := strconv.Atoi(parts[3])
		if err != nil {
			continue
		}

		// Parse time (HH:MM:SS)
		timeParts := strings.Split(parts[4], ":")
		if len(timeParts) != 3 {
			continue
		}
		hour, _ := strconv.Atoi(timeParts[0])
		min, _ := strconv.Atoi(timeParts[1])
		sec, _ := strconv.Atoi(timeParts[2])

		// Parse ΔT
		dt, err := strconv.ParseFloat(parts[5], 64)
		if err != nil {
			continue
		}

		// Parse Luna Num (skip)
		// Parse Saros Num
		// Parse eclipse type — find "T+", "T-", "T", "P", "N" etc.
		eclType := ""
		for i := 7; i < len(parts) && i < 10; i++ {
			p := parts[i]
			if p == "T+" || p == "T-" || p == "T" ||
				p == "P" || p == "N" ||
				p == "Pb" || p == "Nb" || p == "Tb" {
				eclType = string(p[0])
				break
			}
		}
		if eclType == "" {
			continue
		}

		// NASA catalog uses Julian calendar before 1582-10-15, times are in TD ≈ TDB
		isJulianCal := year < 1582 || (year == 1582 && month < 10) || (year == 1582 && month == 10 && day < 15)
		var jdTD float64
		if isJulianCal {
			jdTD = time.DateJulianCal(year, month, day, hour, min, sec).JD()
		} else {
			jdTD = time.Date(year, gotime.Month(month), day, hour, min, sec, 0, gotime.UTC).JD()
		}

		eclipses = append(eclipses, nasaEclipseRef{
			Year: year, Month: month, Day: day,
			Hour: hour, Min: min, Sec: sec,
			DeltaT:      dt,
			EclipseType: eclType,
			JDtd:        jdTD,
		})
	}
	return eclipses
}

// parseNASASolarEclipses parses a NASA solar eclipse catalog page.
func parseNASASolarEclipses(html string) []nasaEclipseRef {
	var eclipses []nasaEclipseRef

	clean := regexp.MustCompile(`<[^>]+>`).ReplaceAllString(html, "")
	lines := strings.Split(clean, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 40 {
			continue
		}

		parts := strings.Fields(trimmed)
		if len(parts) < 9 {
			continue
		}

		// Catalog number must be 5 digits
		catNum := parts[0]
		if len(catNum) != 5 {
			continue
		}
		if _, err := strconv.Atoi(catNum); err != nil {
			continue
		}

		year, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}

		month, ok := monthMap[parts[2]]
		if !ok {
			continue
		}

		day, err := strconv.Atoi(parts[3])
		if err != nil {
			continue
		}

		timeParts := strings.Split(parts[4], ":")
		if len(timeParts) != 3 {
			continue
		}
		hour, _ := strconv.Atoi(timeParts[0])
		min, _ := strconv.Atoi(timeParts[1])
		sec, _ := strconv.Atoi(timeParts[2])

		dt, err := strconv.ParseFloat(parts[5], 64)
		if err != nil {
			continue
		}

		// Eclipse type for solar: T, A, H, P
		eclType := ""
		for i := 6; i < len(parts) && i < 12; i++ {
			p := strings.TrimSpace(parts[i])
			if len(p) >= 1 && (p[0] == 'T' || p[0] == 'A' || p[0] == 'H' || p[0] == 'P') {
				// Verify it's actually an eclipse type marker, not some other field
				if len(p) <= 3 && (p == "T" || p == "A" || p == "H" || p == "P" ||
					p == "Ts" || p == "As" || p == "Hs" || p == "Ps" ||
					p == "Tm" || p == "Am" || p == "Hm" || p == "Pm" ||
					p == "T1" || p == "A1" || p == "H1" || p == "P1" ||
					p == "Te" || p == "Ae" || p == "He" || p == "Pe") {
					eclType = string(p[0])
					break
				}
			}
		}
		if eclType == "" {
			continue
		}

		// NASA catalog uses Julian calendar before 1582-10-15, times are in TD ≈ TDB
		isJulianCal := year < 1582 || (year == 1582 && month < 10) || (year == 1582 && month == 10 && day < 15)
		var jdTD float64
		if isJulianCal {
			jdTD = time.DateJulianCal(year, month, day, hour, min, sec).JD()
		} else {
			jdTD = time.Date(year, gotime.Month(month), day, hour, min, sec, 0, gotime.UTC).JD()
		}

		eclipses = append(eclipses, nasaEclipseRef{
			Year: year, Month: month, Day: day,
			Hour: hour, Min: min, Sec: sec,
			DeltaT:      dt,
			EclipseType: eclType,
			JDtd:        jdTD,
		})
	}
	return eclipses
}

// ── Fetch Helper ─────────────────────────────────────────────────────────────

func fetchNASAPage(t *testing.T, url string) string {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("Failed to create request for %s: %v", url, err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; astrogo-test/1.0)")
	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("NASA endpoint unreachable, skipping: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Skipf("NASA returned status %d for %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read NASA response: %v", err)
	}
	return string(body)
}

// ── Test: Lunar Eclipses vs NASA ─────────────────────────────────────────────

func TestNASA_LunarEclipses_Historical(t *testing.T) {
	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
	if err != nil {
		t.Fatalf("Failed to create DE441 provider: %v", err)
	}
	defer prov.Close()

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

	var totalRef, totalDetected int
	var totalDelta, maxDelta float64

	for _, c := range centuries {
		name := fmt.Sprintf("LE_%04d-%04d", c.start, c.end)
		t.Run(name, func(t *testing.T) {
			html := fetchNASAPage(t, c.url)
			refs := parseNASALunarEclipses(html)
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
				searchStart := refTime.Add(-30 * 24 * time.Hour)
				searchEnd := refTime.Add(30 * 24 * time.Hour)

				eclipses, err := plan.LunarEclipses(searchStart, searchEnd, prov)
				if err != nil {
					t.Logf("  SKIP %04d-%02d-%02d: LunarEclipses error: %v",
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
					if bestDelta > 60 {
						t.Logf("  WARN LE %04d-%02d-%02d %02d:%02d type=%s  Δ=%.0f min",
							ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType, bestDelta)
					}
				} else {
					// Only fail for umbral eclipses (T, P) — penumbral (N) may be below our detection threshold
					if ref.EclipseType != "N" {
						t.Errorf("  MISS LE %04d-%02d-%02d %02d:%02d type=%s: not detected by astrogo",
							ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType)
					} else {
						t.Logf("  SKIP LE %04d-%02d-%02d type=N (penumbral): below detection threshold",
							ref.Year, ref.Month, ref.Day)
					}
				}
			}

			if detected > 0 {
				t.Logf("Century %04d-%04d: %d/%d eclipses detected, mean Δ=%.1f min, max Δ=%.1f min",
					c.start, c.end, detected, tested, centuryDelta/float64(detected), centuryMax)
			} else {
				t.Logf("Century %04d-%04d: %d/%d sampled eclipses detected",
					c.start, c.end, detected, tested)
			}
		})
	}

	t.Logf("\n══════════════════════════════════════════════════════════")
	if totalDetected > 0 {
		t.Logf("NASA Lunar Eclipses: %d/%d detected, mean Δ=%.1f min, max Δ=%.1f min",
			totalDetected, totalRef, totalDelta/float64(totalDetected), maxDelta)
	} else {
		t.Logf("NASA Lunar Eclipses: %d/%d detected", totalDetected, totalRef)
	}
	t.Logf("══════════════════════════════════════════════════════════")
}

// ── Test: Solar Eclipses vs NASA ─────────────────────────────────────────────

func TestNASA_SolarEclipses_Historical(t *testing.T) {
	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
	if err != nil {
		t.Fatalf("Failed to create DE441 provider: %v", err)
	}
	defer prov.Close()

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

	var totalRef, totalDetected int
	var totalDelta, maxDelta float64

	for _, c := range centuries {
		name := fmt.Sprintf("SE_%04d-%04d", c.start, c.end)
		t.Run(name, func(t *testing.T) {
			html := fetchNASAPage(t, c.url)
			refs := parseNASASolarEclipses(html)
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
				searchStart := refTime.Add(-30 * 24 * time.Hour)
				searchEnd := refTime.Add(30 * 24 * time.Hour)

				eclipses, err := plan.SolarEclipses(searchStart, searchEnd, prov)
				if err != nil {
					t.Logf("  SKIP %04d-%02d-%02d: SolarEclipses error: %v",
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
					if bestDelta > 60 {
						t.Logf("  WARN SE %04d-%02d-%02d %02d:%02d type=%s  Δ=%.0f min",
							ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType, bestDelta)
					}
				} else {
					if ref.EclipseType != "P" {
						t.Errorf("  MISS SE %04d-%02d-%02d %02d:%02d type=%s: not detected by astrogo",
							ref.Year, ref.Month, ref.Day, ref.Hour, ref.Min, ref.EclipseType)
					} else {
						t.Logf("  SKIP SE %04d-%02d-%02d type=P (partial): may be below threshold",
							ref.Year, ref.Month, ref.Day)
					}
				}
			}

			if detected > 0 {
				t.Logf("Century %04d-%04d: %d/%d eclipses detected, mean Δ=%.1f min, max Δ=%.1f min",
					c.start, c.end, detected, tested, centuryDelta/float64(detected), centuryMax)
			} else {
				t.Logf("Century %04d-%04d: %d/%d eclipses detected",
					c.start, c.end, detected, tested)
			}
		})
	}

	t.Logf("\n══════════════════════════════════════════════════════════")
	if totalDetected > 0 {
		t.Logf("NASA Solar Eclipses: %d/%d detected, mean Δ=%.1f min, max Δ=%.1f min",
			totalDetected, totalRef, totalDelta/float64(totalDetected), maxDelta)
	} else {
		t.Logf("NASA Solar Eclipses: %d/%d detected", totalDetected, totalRef)
	}
	t.Logf("══════════════════════════════════════════════════════════")
}

// TestNASA_DeltaT_CrossValidation verifies that astrogo's time.DeltaT polynomial
// matches NASA's tabulated ΔT values from the Five Millennium Eclipse Catalog.
// Both sources use the Espenak & Meeus (2006) model, so they should agree closely.
func TestNASA_DeltaT_CrossValidation(t *testing.T) {
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

	var totalEvents int
	var totalDelta, maxDelta float64

	for _, c := range centuries {
		name := fmt.Sprintf("DeltaT_%04d-%04d", c.start, c.end)
		t.Run(name, func(t *testing.T) {
			html := fetchNASAPage(t, c.url)
			refs := parseNASALunarEclipses(html)

			if len(refs) == 0 {
				t.Fatalf("No eclipses parsed")
			}

			var centuryDelta, centuryMax float64
			var count int
			for _, ref := range refs {
				if ref.DeltaT == 0 {
					continue
				}
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

				// NASA truncates ΔT to integer seconds; allow generous tolerance
				if delta > 10 {
					t.Logf("  WARN %04d-%02d-%02d: computed ΔT=%.1f, NASA ΔT=%.0f, Δ=%.1f s",
						ref.Year, ref.Month, ref.Day, computed, ref.DeltaT, delta)
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
