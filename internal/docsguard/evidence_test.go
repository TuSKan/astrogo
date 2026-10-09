package docsguard_test

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// validationDoc is the document whose status table is checked below.
const validationDoc = "../../docs/VALIDATION.md"

// backtickName matches one citation inside an Evidence cell. A cell may hold
// several: a suite is often measured in companion parts — position and
// velocity, separation and cross-track — that one status-table row summarises
// together.
var backtickName = regexp.MustCompile("`([^`]+)`")

// generatedSuite matches a suite name in the generated accuracy table.
var generatedSuite = regexp.MustCompile("^\\| `([a-z0-9._]+)` \\|")

// toleranceTerm matches one bound stated in a Tolerance cell: a number and the
// unit written straight after it, as in "3 arcsec" or "1e-10 AU/day".
var toleranceTerm = regexp.MustCompile(`(\d+(?:\.\d+)?(?:[eE][-+]?\d+)?)\s*([^\s;,()]+)`)

// Column positions after splitting a table row on "|", which leaves an empty
// cell before the first column.
const (
	statusToleranceCell   = 5 // Area, Status, Evidence, Reference, Tolerance
	generatedContractCell = 9 // Suite, Reference, Independence, N, p50, p95, p99, Max, Contract
)

// TestStatusTableEvidenceResolves is why the Evidence column exists.
//
// The status table is hand-written and stays that way — the reasoning about
// why a number is what it is cannot be generated. But 52 of its 53 rows say
// "validated", undated, in the same document as a generated table whose rows
// carry a contract, a measured distribution and a commit stamp. A reader had
// no way to tell which ticks were evidence and which were assertions, and no
// way to get from a tick to the thing that establishes it.
//
// Each row now cites either a test file or a generated suite, and this checks
// that the citation resolves. A test file that is renamed or deleted, or a
// suite that stops being produced, fails the build rather than leaving a
// pointer into nothing — which is the same failure the version claims in
// version_test.go guard against, one level up.
func TestStatusTableEvidenceResolves(t *testing.T) {
	raw, err := os.ReadFile(validationDoc)
	if err != nil {
		t.Fatalf("read %s: %v", validationDoc, err)
	}

	lines := strings.Split(string(raw), "\n")
	suites := collectGeneratedSuites(lines)

	if len(suites) == 0 {
		t.Fatal("no generated suites found; the accuracy table markers may have moved")
	}

	var (
		inTable bool
		rows    int
	)

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "## Status Table"):
			inTable = true
		case strings.HasPrefix(line, "## Known Incomplete"):
			inTable = false
		}

		if !inTable || !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Area") {
			continue
		}

		cited := evidenceCitations(line)
		if len(cited) == 0 {
			t.Errorf("%s:%d: status-table row has no Evidence cell:\n  %s",
				validationDoc, i+1, truncate(line))

			continue
		}

		rows++

		for _, c := range cited {
			checkEvidence(t, i+1, c, suites)
		}
	}

	if rows == 0 {
		t.Fatal("no status-table rows found; the table heading may have moved")
	}

	t.Logf("%d status-table rows, %d generated suites", rows, len(suites))
}

// checkEvidence resolves one citation: a dotted name must be a generated
// suite, anything else a file that exists.
func checkEvidence(t *testing.T, line int, cited string, suites map[string]string) {
	t.Helper()

	if strings.HasSuffix(cited, ".go") {
		if _, err := os.Stat(filepath.Join("..", "..", cited)); err != nil {
			t.Errorf("%s:%d: Evidence cites %q, which does not exist", validationDoc, line, cited)
		}

		return
	}

	if _, ok := suites[cited]; !ok {
		t.Errorf("%s:%d: Evidence cites suite %q, which the generated accuracy table does not contain",
			validationDoc, line, cited)
	}
}

// evidenceCitations returns every name cited in a row's Evidence cell, which
// is the third column.
func evidenceCitations(line string) []string {
	cells := strings.Split(line, "|")
	if len(cells) < 4 {
		return nil
	}

	var out []string
	for _, m := range backtickName.FindAllStringSubmatch(cells[3], -1) {
		out = append(out, m[1])
	}

	return out
}

// collectGeneratedSuites maps every suite in the generated accuracy table to
// its contract, as that table prints it: a number and a unit, "3 arcsec".
func collectGeneratedSuites(lines []string) map[string]string {
	suites := make(map[string]string)

	var inGenerated bool

	for _, line := range lines {
		switch {
		case strings.Contains(line, "BEGIN GENERATED ACCURACY"):
			inGenerated = true

			continue
		case strings.Contains(line, "END GENERATED ACCURACY"):
			inGenerated = false

			continue
		}

		if !inGenerated {
			continue
		}

		if m := generatedSuite.FindStringSubmatch(line); m != nil {
			var contract string
			if cells := strings.Split(line, "|"); len(cells) > generatedContractCell {
				contract = strings.TrimSpace(cells[generatedContractCell])
			}

			suites[m[1]] = contract
		}
	}

	return suites
}

// TestEveryGeneratedSuiteIsCited runs the check the other way.
//
// A suite that no status-table row points at is measured evidence nobody can
// find from the summary, which is how the two halves of this document drift
// apart: the generated table grows, the hand-written one keeps describing an
// older shape of the library.
func TestEveryGeneratedSuiteIsCited(t *testing.T) {
	raw, err := os.ReadFile(validationDoc)
	if err != nil {
		t.Fatalf("read %s: %v", validationDoc, err)
	}

	lines := strings.Split(string(raw), "\n")
	suites := collectGeneratedSuites(lines)

	for suite := range suites {
		if !citedAsEvidence(lines, suite) {
			t.Errorf("suite %q is measured but no status-table row cites it", suite)
		}
	}
}

func citedAsEvidence(lines []string, suite string) bool {
	var inTable bool

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "## Status Table"):
			inTable = true
		case strings.HasPrefix(line, "## Known Incomplete"):
			inTable = false
		}

		if !inTable {
			continue
		}

		if slices.Contains(evidenceCitations(line), suite) {
			return true
		}
	}

	return false
}

// TestStatusTableToleranceIsTheCitedContract holds the Tolerance column to its
// own definition.
//
// The document defines that column as the bound a test asserts, not an
// achieved accuracy. When the Evidence column was added, rows written earlier
// kept their old Tolerance beside the suites they now cited, and three of them
// stated a bound nothing asserts: 0.05 mag for a GAMBONS comparison held to
// 1 mag, 1e-7 deg for a Horizons comparison held to 3 arcsec, and 1e-12 d for
// round trips held to 1e-6 s and 5 s (#665). The first was repeated in the
// README as what the natural sky is validated to.
//
// So a row that cites a generated suite must state that suite's contract, with
// the value and unit the generated table prints. It may state more, such as a
// bound from a test file it also cites, but not less. A Tolerance with no
// number in it, such as "per body, from SOFA's own table", makes no numeric
// claim and is not checked.
func TestStatusTableToleranceIsTheCitedContract(t *testing.T) {
	raw, err := os.ReadFile(validationDoc)
	if err != nil {
		t.Fatalf("read %s: %v", validationDoc, err)
	}

	lines := strings.Split(string(raw), "\n")
	contracts := collectGeneratedSuites(lines)

	var (
		inTable bool
		checked int
	)

	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "## Status Table"):
			inTable = true
		case strings.HasPrefix(line, "## Known Incomplete"):
			inTable = false
		}

		if !inTable || !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Area") {
			continue
		}

		cells := strings.Split(line, "|")
		if len(cells) <= statusToleranceCell {
			continue
		}

		tolerance := strings.TrimSpace(cells[statusToleranceCell])
		if !strings.ContainsAny(tolerance, "0123456789") {
			continue
		}

		for _, cited := range evidenceCitations(line) {
			// A file, or a suite TestStatusTableEvidenceResolves reports.
			contract, ok := contracts[cited]
			if !ok {
				continue
			}

			checked++

			if !statesBound(tolerance, contract) {
				t.Errorf("%s:%d: Tolerance %q does not state the contract of %s, which is %s",
					validationDoc, i+1, tolerance, cited, contract)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no row cites a generated suite beside a numeric Tolerance; the table layout may have changed")
	}

	t.Logf("%d suite citations checked against their row's Tolerance", checked)
}

// statesBound reports whether a Tolerance cell states contract, which is a
// number and a unit as the generated table prints it.
func statesBound(tolerance, contract string) bool {
	value, unit, ok := strings.Cut(contract, " ")
	if !ok {
		return false
	}

	want, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false
	}

	for _, m := range toleranceTerm.FindAllStringSubmatch(tolerance, -1) {
		got, err := strconv.ParseFloat(m[1], 64)
		if err == nil && m[2] == unit && math.Abs(got-want) <= 1e-9*math.Abs(want) {
			return true
		}
	}

	return false
}

func truncate(s string) string {
	const limit = 90
	if len(s) <= limit {
		return s
	}

	return s[:limit] + "…"
}
