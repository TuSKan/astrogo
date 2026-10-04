package docsguard

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	// fuzzFunc is a fuzz target's declaration.
	fuzzFunc = regexp.MustCompile(`(?m)^func (Fuzz\w+)\(f \*testing\.F\)`)

	// fuzzTableRow is a row of CLAUDE.md's table of fuzzed packages: the
	// package, then its targets, each in backticks.
	fuzzTableRow = regexp.MustCompile("(?m)^\\| `([a-z0-9/]+)` \\| ((?:`Fuzz\\w+`(?:, )?)+) \\|")

	// fuzzPackageCount is the sentence introducing that table.
	fuzzPackageCount = regexp.MustCompile(`(?m)^(\w+) packages carry targets\b`)

	fuzzTarget = regexp.MustCompile("`(Fuzz\\w+)`")
)

// countWords spells the package counts the sentence can plausibly need.
var countWords = map[string]int{
	"Five": 5, "Six": 6, "Seven": 7, "Eight": 8, "Nine": 9, "Ten": 10, "Eleven": 11, "Twelve": 12,
}

// TestEveryFuzzTargetIsDocumented holds CLAUDE.md's table of fuzz targets to
// the code, in both directions, along with the count of packages above it.
//
// The table is where the extended-fuzzing step starts: a target missing from
// it is one nobody runs beyond its seed corpus. It had already drifted. It said
// six packages and left out ephemeris/satellite/sgp4, whose FuzzParseTLE and
// FuzzVerifyTLEChecksums guard the parser that replaced an SGP4 backend the
// fuzzer had crashed in about two seconds.
func TestEveryFuzzTargetIsDocumented(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	inCode := map[string][]string{}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		rel, rerr := filepath.Rel(root, filepath.Dir(path))
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		for _, m := range fuzzFunc.FindAllStringSubmatch(string(data), -1) {
			pkg := filepath.ToSlash(rel)
			inCode[pkg] = append(inCode[pkg], m[1])
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	doc, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}

	inDoc := map[string][]string{}

	for _, row := range fuzzTableRow.FindAllStringSubmatch(string(doc), -1) {
		for _, m := range fuzzTarget.FindAllStringSubmatch(row[2], -1) {
			inDoc[row[1]] = append(inDoc[row[1]], m[1])
		}
	}

	if len(inCode) < 5 || len(inDoc) < 5 {
		t.Fatalf("found fuzz targets in %d packages and %d table rows; one side is no longer "+
			"being read, so this test would pass vacuously", len(inCode), len(inDoc))
	}

	for pkg, targets := range inCode {
		for _, target := range targets {
			if !slices.Contains(inDoc[pkg], target) {
				t.Errorf("%s in %s is not in CLAUDE.md's table of fuzz targets.\n"+
					"  A target missing from the table is one the extended-fuzzing step never runs.",
					target, pkg)
			}
		}
	}

	for pkg, targets := range inDoc {
		for _, target := range targets {
			if !slices.Contains(inCode[pkg], target) {
				t.Errorf("CLAUDE.md lists %s in %s, which has no such fuzz target", target, pkg)
			}
		}
	}

	m := fuzzPackageCount.FindStringSubmatch(string(doc))
	if m == nil {
		t.Fatal("CLAUDE.md no longer says how many packages carry fuzz targets")
	}

	if n, ok := countWords[m[1]]; !ok || n != len(inCode) {
		t.Errorf("CLAUDE.md says %q packages carry fuzz targets; the code has %d", m[1], len(inCode))
	}
}
