package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
func checkEvidence(t *testing.T, line int, cited string, suites map[string]bool) {
	t.Helper()

	if strings.HasSuffix(cited, ".go") {
		if _, err := os.Stat(filepath.Join("..", "..", cited)); err != nil {
			t.Errorf("%s:%d: Evidence cites %q, which does not exist", validationDoc, line, cited)
		}

		return
	}

	if !suites[cited] {
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

func collectGeneratedSuites(lines []string) map[string]bool {
	suites := make(map[string]bool)

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
			suites[m[1]] = true
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

func truncate(s string) string {
	const limit = 90
	if len(s) <= limit {
		return s
	}

	return s[:limit] + "…"
}
