package docsguard_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// canonicalLiterals are values that have a home in the constants package and
// must not be written out anywhere else.
//
// Each is a unit conversion, not a coefficient of a published formula.
// CLAUDE.md's "magic numbers are physical constants" rule protects the latter
// — a Saemundsson coefficient belongs beside its formula, where a reader can
// check it against the paper. A conversion factor with a canonical home is the
// opposite case: every copy is a place the value can fail to be updated.
var canonicalLiterals = map[string]string{
	"149597870.7":   "the astronomical unit in km — use constants.IAU.AstronomicalUnit.Value / 1e3",
	"173.144632674": "the speed of light in AU/day — derive it from constants.SI2019.SpeedOfLight and constants.IAU.AstronomicalUnit",
}

// TestCanonicalConstantsAreNotWrittenOut keeps the constants package the single
// source of truth for values that live there.
//
// # The failure this prevents
//
// The AU was written out in four production files and the light-time constant
// in two, all agreeing with constants and with each other. Agreeing today is
// the whole problem: the failure mode is a future revision applied in
// constants while five other copies quietly do not move. Nothing would fail,
// and the disagreement would surface as a slow drift between subsystems.
//
// One of those copies was exported, as jpl.KMPerAU, so a downstream caller
// could pin the stale value from outside the module entirely.
//
// # Why a source scan
//
// The values cannot be compared at runtime, because after the fix there is
// nothing left to compare — each package now reads the same variable. The
// property being defended is textual: the literal must not reappear. A scan is
// the only thing that can see that.
//
// Comments are exempt. A doc comment saying "1 AU = 149597870.7 km" is helping
// a reader, not creating a second source of truth, and the packages that read
// from constants annotate their derivation exactly that way.
func TestCanonicalConstantsAreNotWrittenOut(t *testing.T) {
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

		rel = filepath.ToSlash(rel)

		// constants is where these live, so it is the one place they are
		// written out. Tests may pin a literal on purpose — that is what makes
		// a test a check rather than a restatement.
		if strings.HasPrefix(rel, "constants/") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		checked++

		for i, line := range strings.Split(string(data), "\n") {
			code := strings.TrimSpace(line)
			if strings.HasPrefix(code, "//") {
				continue // a comment quoting the value is documentation
			}

			// Strip a trailing line comment, so `x / 1e3 // 149597870.7 km`
			// — the annotation the fixed call sites carry — is not a hit.
			if idx := strings.Index(code, "//"); idx >= 0 {
				code = code[:idx]
			}

			for literal, guidance := range canonicalLiterals {
				if !strings.Contains(code, literal) {
					continue
				}

				offenders++

				t.Errorf("%s:%d writes out %s.\n  %s.\n  Every copy is a place the "+
					"value can fail to be updated; constants is the single source.",
					rel, i+1, literal, guidance)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if checked < 200 {
		t.Fatalf("only %d Go files scanned; the walk is not reaching the module", checked)
	}

	t.Logf("%d files scanned, %d offending lines", checked, offenders)
}
