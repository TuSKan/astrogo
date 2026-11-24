package docsguard_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// epochImportPath is this module's epoch package.
const epochImportPath = "github.com/TuSKan/astrogo/epoch"

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
//
// # Why the parser and not a pattern
//
// The guard this replaced matched import lines with a regular expression,
// which reads `return "time"` as an import of time named return. The import
// declarations are what is being judged, so they are what is read.
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

		fset := token.NewFileSet()

		file, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			t.Errorf("%s: %v", filepath.ToSlash(rel), perr)

			return nil
		}

		checked++

		for _, spec := range file.Imports {
			importPath, uerr := strconv.Unquote(spec.Path.Value)
			if uerr != nil || spec.Name == nil || (importPath != "time" && importPath != epochImportPath) {
				continue
			}

			offenders++

			t.Errorf("%s:%d: imports %q as %s.\n"+
				"  time is the standard library's and epoch is astrogo's, and each reads as "+
				"itself at every use; a local name makes the reader look up which one it is.",
				filepath.ToSlash(rel), fset.Position(spec.Pos()).Line, importPath, spec.Name.Name)
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
