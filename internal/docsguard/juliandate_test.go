package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// gregorianDateLiteral matches a time.Date call written with a literal year,
// and captures the year.
var gregorianDateLiteral = regexp.MustCompile(`time\.Date\(\s*(-?\d+)\s*,`)

// TestHistoricalDatesUseTheJulianCalendar keeps the code a user copies from
// building a historical date with the wrong calendar.
//
// # Why
//
// time.Date is the proleptic Gregorian calendar. A date a historical source
// gives before the reform of 15 October 1582 is a Julian one, and
// time.DateJulianCal builds it. In the first century the two are two days
// apart. examples/10_jesus_christ/eclipse found the eclipse of 3 April AD 33
// correctly, then built its evening with time.Date and printed the sunset and
// moonrise of Julian 5 April: a moonrise 1 h 50 min late, with the Moon 21°
// up when it printed "rises" (#841).
//
// # Scope
//
// What a user reads to learn the API: the examples/ module, every Markdown
// file, and every Go file that holds an Example function, as for
// TestExamplesBuildSitesWithNewEarthLocation. Only a literal year below 1583
// is read. A date built from a computed event's own calendar parts is a
// variable, and passes: the crux example does that, consistently both ways.
func TestHistoricalDatesUseTheJulianCalendar(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	var checked int

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		rel = filepath.ToSlash(rel)
		if rel == "CHANGELOG.md" || (!strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".md")) {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		text := string(data)

		userFacing := strings.HasPrefix(rel, "examples/") || strings.HasSuffix(rel, ".md") ||
			(strings.HasSuffix(rel, "_test.go") && strings.Contains(text, "\nfunc Example"))
		if !userFacing {
			return nil
		}

		checked++

		for i, line := range strings.Split(text, "\n") {
			for _, m := range gregorianDateLiteral.FindAllStringSubmatch(line, -1) {
				if year, aerr := strconv.Atoi(m[1]); aerr == nil && year < 1583 {
					t.Errorf("%s:%d: time.Date(%d, …) in code a user copies is the proleptic Gregorian calendar; "+
						"a date before the 1582 reform is a Julian one: build it with time.DateJulianCal", rel, i+1, year)
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 50 {
		t.Fatalf("checked only %d user-facing files; the walk is not seeing the repository", checked)
	}
}
