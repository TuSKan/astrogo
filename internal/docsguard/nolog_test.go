package docsguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLibraryCodeWritesToTheGlobalLog keeps astrogo out of its host's
// standard-library logger.
//
// # Why
//
// The global log package writes wherever log.SetOutput last pointed, which is
// the host program's decision and not a library's to make. Three production
// call sites did it anyway — remote's download line, iers' "loaded EOP data",
// and time's warning that Earth Orientation Parameters were unavailable — and
// the third mattered most, because it is the only notice a caller gets that
// accuracy silently degraded and it was going somewhere the caller may never
// have looked.
//
// They now go through [github.com/TuSKan/astrogo/logging], which a caller
// controls. Nothing stops the next one being added with log.Printf, so this
// does.
//
// # Scope
//
// Library code only. Deliberately not covered:
//
//   - examples/ — those are programs, and log.Fatal is exactly right in a main.
//   - _test.go — a test writes to the test log, and t.Log is not this.
//   - the logging package itself, which is the sanctioned destination.
//
// The check is on the import rather than the call, so log.Printf, log.Fatal
// and a var of type *log.Logger are all caught by one rule.
func TestNoLibraryCodeWritesToTheGlobalLog(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	var checked, offenders int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "examples" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil //nolint:nilerr // an unrelatable path is skipped, not fatal
		}

		rel = filepath.ToSlash(rel)

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		checked++

		for line := range strings.SplitSeq(string(data), "\n") {
			trimmed := strings.TrimSpace(line)

			// Only an import line, so a doc comment mentioning log.Printf —
			// including the ones explaining this very change — is not a hit.
			if trimmed != `"log"` && !strings.HasSuffix(trimmed, ` "log"`) {
				continue
			}

			offenders++

			t.Errorf("%s imports the standard library's log package.\n"+
				"  Its output goes wherever the host last pointed log.SetOutput, "+
				"which is not a library's decision. Use "+
				"github.com/TuSKan/astrogo/logging — Info for progress, Warn for "+
				"a result that silently degraded.", rel)

			break
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 200 {
		t.Fatalf("only %d library Go files scanned; the walk is not reaching "+
			"the module", checked)
	}

	t.Logf("%d library files scanned, %d import log", checked, offenders)
}
