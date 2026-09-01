package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testDecl matches a test function's declaration, in any file, under any
// build tag — the files are read as text rather than compiled, so a
// network-tagged test counts as declared just like an ordinary one.
var testDecl = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]+)\s*\(`)

// testCitation matches a name that reads as a reference to a test.
//
// The five-character tail keeps it from firing on `TestMain` or on a bare
// `Test`, and requiring an upper-case letter after `Test` keeps it from
// matching prose like "Tests".
var testCitation = regexp.MustCompile(`\b(Test[A-Z][A-Za-z0-9_]{4,})`)

// historicalMention marks a line that names a test in the past tense.
//
// Some of the most useful prose in this repository is about a test that no
// longer exists: docs/skybrightness.md explains a sign inversion by pointing
// at the test whose premise was the bug, which was deleted precisely because
// it was wrong. Requiring that name to resolve would force the history out of
// the document, which is worse than the drift this guard prevents.
//
// The escape hatch is a word on the same line, so it is visible where it is
// used rather than hidden in a list here.
var historicalMention = regexp.MustCompile(`(?i)\b(since deleted|since removed|since renamed|no longer exists|was deleted|was removed)\b`)

// TestCitedTestsExist checks that every test named in a comment or a document
// is a test that exists.
//
// # Why
//
// Because a name is how a reader checks whether a claim is guarded, and a
// name that resolves to nothing ends the check with the wrong answer. The
// worst case found when this guard was written was skybrightness's
// scatterKernel: a rearrangement of the model's innermost loop whose doc
// comment said it "is not allowed to drift from the functions it rearranges"
// and named the test holding it to that. There was no such test — and no test
// of the kernel at all. A reader following the citation would have concluded
// the opposite of the truth.
//
// It found nine more. Three equation-to-test maps in docs/skybrightness.md
// naming tests that had been renamed; a CLAUDE.md command line offering
// `-run TestApparentPlace` (no longer exists) to run "a single test by name";
// and TestCharonNeedsTheSystemParameter, cited by a neighbouring test as the
// counter-case establishing a scientific claim this repository had previously
// shipped backwards twice — and never written.
//
// That first line is itself exempt, by the marker sitting on it. The marker
// has to share the line with the name, which is the whole reason it reads as
// an aside rather than a footnote: a reader meets the caveat where they meet
// the name.
//
// # What is exempt
//
// CHANGELOG.md, because it is forward-only by policy: an entry describes the
// release it shipped in, and the tests it named were real then. Rewriting a
// released entry to track a rename would falsify the record to satisfy a
// guard.
//
// A line marked as historical — see [historicalMention].
//
// A citation that is a prefix of a real test also passes, since
// `TestAstroPixels` is a fair way to refer to the `TestAstroPixels_*` family.
func TestCitedTestsExist(t *testing.T) {
	root := filepath.Join("..", "..")

	goFiles, docFiles := collectSources(t, root)

	declared := make(map[string]bool)

	for _, path := range goFiles {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		for _, m := range testDecl.FindAllStringSubmatch(string(src), -1) {
			declared[m[1]] = true
		}
	}

	if len(declared) < 100 {
		t.Fatalf("only %d test declarations found; the walk root is probably wrong", len(declared))
	}

	// A citation resolves if it names a test outright, or names the common
	// prefix of a family of them.
	resolves := func(name string) bool {
		if declared[name] {
			return true
		}

		for d := range declared {
			if strings.HasPrefix(d, name) {
				return true
			}
		}

		return false
	}

	var checked, offenders int

	for _, path := range append(goFiles, docFiles...) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}

		slash := filepath.ToSlash(rel)
		if slash == "CHANGELOG.md" {
			continue
		}

		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		isGo := strings.HasSuffix(slash, ".go")
		checked++

		for n, line := range strings.Split(string(src), "\n") {
			// In Go, only comments are prose; a bare identifier in code is
			// checked by the compiler already.
			if isGo && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}

			if historicalMention.MatchString(line) {
				continue
			}

			for _, m := range testCitation.FindAllStringSubmatch(line, -1) {
				if resolves(m[1]) {
					continue
				}

				offenders++

				t.Errorf("%s:%d: cites %s, which no test declares.\n"+
					"  Point at the test that covers this now, or write it. If the name is "+
					"deliberately historical, say so on the same line — \"since deleted\", "+
					"\"since renamed\".", slash, n+1, m[1])
			}
		}
	}

	t.Logf("%d files checked, %d test declarations, %d unresolved citations",
		checked, len(declared), offenders)
}

// collectSources returns every Go and Markdown file under root.
func collectSources(t *testing.T, root string) (goFiles, docFiles []string) {
	t.Helper()

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

		switch {
		case strings.HasSuffix(path, ".go"):
			goFiles = append(goFiles, path)
		case strings.HasSuffix(path, ".md"):
			docFiles = append(docFiles, path)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	return goFiles, docFiles
}
