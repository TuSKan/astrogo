package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// stdTimeImport matches an import of the standard library's time package, in
// any of the forms it is written: on its own, inside a block, and with or
// without a local name.
var stdTimeImport = regexp.MustCompile(`(?m)^(?:import[ 	]+|[ 	]*)(?:[a-z][a-z0-9]*[ 	]+)?"time"[ 	]*$`)

// aliasedAstroTime matches an import of this module's time package under a
// local name, in a block or on its own line.
//
// Spaces and tabs rather than \s, because \s matches a newline: with it, the
// pattern could begin on the blank line above an unaliased single-line import
// and read the `import` keyword itself as the alias. This guard reported
// exactly that against atmosphere/provenance.go before the classes were
// narrowed.
var aliasedAstroTime = regexp.MustCompile(
	`(?m)^(?:import[ 	]+|[ 	]+)[a-z][a-z0-9]*[ 	]+"github\.com/TuSKan/astrogo/time"[ 	]*$`)

// TestNoStandardLibraryTimeOutsideTimePackage enforces the rule that
// astrogo/time is the module's only door to the standard library's clock.
//
// # Why it is a rule and not a preference
//
// A package that imports both ends up with two types spelled `time.Time` and
// a local name to tell them apart. Then the local name is what carries the
// meaning, and a field declared `Time time.Time` says nothing on its own
// about which one it is — the answer is at the top of the file, in an import
// line nobody re-reads.
//
// That is not hypothetical here. Converting the module to this rule, an
// intermediate version of the change swapped an import without rewriting the
// body, and skybrightness.Scene.Time silently changed from the standard
// library's type to this module's. The source line was character-for-character
// identical before and after.
//
// astrogo/time aliases everything needed for that not to be a trade: GoTime
// for the standard library's type, LocationUTC for the location, GoDate
// beside Date, and the duration, month and layout constants unchanged.
//
// The one exception is internal/testutil, which cannot import astrogo/time
// without closing a cycle through time/internal/iers' own tests. It uses an
// untyped constant instead and says so.
func TestNoStandardLibraryTimeOutsideTimePackage(t *testing.T) {
	root := filepath.Join("..", "..")

	var checked, offenders int

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

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// astrogo/time is the one package allowed to import it — that is
		// the whole point of the rule.
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		if slash := filepath.ToSlash(rel); slash == "time" || strings.HasPrefix(slash, "time/") {
			return nil
		}

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // a file we cannot read is skipped, not fatal
		}

		checked++

		body := string(src)

		if loc := stdTimeImport.FindStringIndex(body); loc != nil {
			offenders++

			t.Errorf("%s:%d: imports the standard library's time.\n"+
				"  Use github.com/TuSKan/astrogo/time: GoTime for the standard library's\n"+
				"  Time, LocationUTC for its UTC location, GoDate beside Date, and the\n"+
				"  duration, month and layout constants under their own names.",
				filepath.ToSlash(rel), lineOf(body, loc[0]))
		}

		if loc := aliasedAstroTime.FindStringIndex(body); loc != nil {
			offenders++

			t.Errorf("%s:%d: imports astrogo/time under a local name.\n"+
				"  With the standard library's time gone there is nothing to disambiguate,\n"+
				"  and the alias only makes the package read differently from its neighbours.",
				filepath.ToSlash(rel), lineOf(body, loc[0]))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked == 0 {
		t.Fatal("no Go files examined; the walk root may be wrong")
	}

	t.Logf("%d files checked, %d offenders", checked, offenders)
}

// lineOf returns the 1-indexed line containing the byte at off.
func lineOf(s string, off int) int {
	return 1 + strings.Count(s[:off], "\n")
}
