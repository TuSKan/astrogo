package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// namedTimeImport matches an import of either time package under a local
// name: the standard library's time or this module's epoch, in a block or on
// its own line. It captures the name and the path.
//
// Spaces and tabs rather than \s, because \s matches a newline: with it, the
// pattern could begin on the blank line above an unaliased single-line import
// and read the `import` keyword itself as the alias. The guard this replaced
// reported exactly that against atmosphere/provenance.go before its classes
// were narrowed.
var namedTimeImport = regexp.MustCompile(
	`(?m)^(?:import[ \t]+|[ \t]+)([A-Za-z_][A-Za-z0-9_]*)[ \t]+"(time|github\.com/TuSKan/astrogo/epoch)"[ \t]*$`)

// TestTimePackagesKeepTheirNames keeps time.X meaning the standard library and
// epoch.X meaning this module, in every file.
//
// # Why
//
// Until this module's epoch package was renamed from time, a file needing both
// imported one under a local name: gotime, stdtime, atime, astrotime. Then the
// local name was what carried the meaning, and a field declared `Time
// time.Time` said nothing on its own about which type it was. The answer was
// at the top of the file, in an import line nobody re-reads.
//
// That was not hypothetical. Converting the module to an earlier rule, an
// intermediate version of the change swapped an import without rewriting the
// body, and skybrightness.Scene.Time silently changed from the standard
// library's type to this module's. The source line was character-for-character
// identical before and after.
//
// The rename removed the collision: time.Time and epoch.Time read differently
// at every use. A local name for either brings the ambiguity back for nothing,
// so neither has one anywhere in the module.
//
// It replaces the rule that only this package might import the standard
// library's time, which cost re-exports of the standard library (GoTime,
// LocationUTC, GoDate, the duration, month and layout constants), exemptions
// for the packages below it, and untyped nanosecond constants in
// internal/testutil where it could import neither.
func TestTimePackagesKeepTheirNames(t *testing.T) {
	t.Parallel()

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

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // a file we cannot read is skipped, not fatal
		}

		checked++

		body := string(src)

		for _, m := range namedTimeImport.FindAllStringSubmatchIndex(body, -1) {
			offenders++

			t.Errorf("%s:%d: imports %q as %s.\n"+
				"  time is the standard library's and epoch is astrogo's, and each reads as "+
				"itself at every use; a local name makes the reader look up which one it is.",
				filepath.ToSlash(rel), lineOf(body, m[0]), body[m[4]:m[5]], body[m[2]:m[3]])
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 100 {
		t.Fatalf("only %d Go files examined; the walk root may be wrong", checked)
	}

	t.Logf("%d files checked, %d offenders", checked, offenders)
}

// lineOf returns the 1-indexed line containing the byte at off.
func lineOf(s string, off int) int {
	return 1 + strings.Count(s[:off], "\n")
}
