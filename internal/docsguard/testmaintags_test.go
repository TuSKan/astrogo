package docsguard_test

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// tiers are the build tags this repository partitions its tests by.
var tiers = []string{"integration", "network", "validation"}

// TestTestMainCoversEveryTierItsPackageTests stops a package's TestMain from
// being invisible to some of that package's own tests.
//
// # Why
//
// A TestMain does the setup a package's tests cannot do for themselves:
// registering the kernel backend with a blank import, granting download
// consent for the whole binary, installing the goroutine-leak guard. It is
// also an ordinary file with an ordinary build constraint, so a TestMain
// gated to one tag simply is not compiled under another — and the tests that
// needed it then run with none of it, reporting a missing capability rather
// than a missing file.
//
// plan's TestMain was gated to "integration" while the tests depending on it
// were tagged "network" and "validation". Under `go test -tags=network
// ./plan/` — the command CLAUDE.md documents, and the one pre-release.yml
// runs on its own — nothing registered the kernel backend, and 24 sub-checks
// across two tests failed with "this build has no kernel backend" (#238).
//
// It was invisible locally because the full gate runs all three tags at once,
// which is the configuration where the file does compile. The failing
// configuration was the one only CI used, which is the wrong way round: a
// scheduled job that always fails is a scheduled job nobody reads.
//
// # What this checks
//
// For each package holding a constrained TestMain, every tier that any of
// that package's other test files is gated to must also appear in TestMain's
// own constraint. An unconstrained TestMain is compiled into every build and
// is always fine.
//
// This does not evaluate build expressions in general — it asks whether a
// tier's name appears, which is exact for the "a || b || c" and single-tag
// forms this repository uses, and deliberately not a constraint solver.
func TestTestMainCoversEveryTierItsPackageTests(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	pkgs := packagesWithTestFiles(t, root)
	if len(pkgs) < 20 {
		t.Fatalf("only %d packages with test files found; the walk is not reaching "+
			"the module", len(pkgs))
	}

	var checked int

	for _, dir := range slices.Sorted(maps.Keys(pkgs)) {
		files := pkgs[dir]

		mainFile, mainTiers, ok := testMainConstraint(t, dir, files)
		if !ok {
			continue // no TestMain, or one compiled into every build
		}

		checked++

		for _, f := range files {
			if f == mainFile {
				continue
			}

			for _, tier := range fileTiers(t, filepath.Join(dir, f)) {
				if slices.Contains(mainTiers, tier) {
					continue
				}

				rel, _ := filepath.Rel(root, filepath.Join(dir, f))

				t.Errorf("%s is tagged %q, but this package's TestMain (%s) is not.\n"+
					"  Under `go test -tags=%s` that TestMain is not compiled, so this "+
					"file's tests run without whatever it sets up — a registered backend, "+
					"download consent, a leak guard. Widen the TestMain's constraint to "+
					"cover every tier the package has tests in. See #238.",
					filepath.ToSlash(rel), tier, mainFile, tier)
			}
		}
	}

	if checked == 0 {
		t.Fatal("no package with a constrained TestMain was examined; the check is " +
			"not looking at anything")
	}

	t.Logf("%d packages with a constrained TestMain checked", checked)
}

// testMainConstraint reports the file declaring the package's TestMain and
// the tiers its build constraint names.
//
// The third result is false when there is no TestMain, or when it carries no
// constraint at all — that one is compiled into every build and so covers
// every tier by construction.
func testMainConstraint(t *testing.T, dir string, files []string) (string, []string, bool) {
	t.Helper()

	for _, f := range files {
		path := filepath.Join(dir, f)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		src := string(data)
		if !strings.HasPrefix(src, "func TestMain(") && !strings.Contains(src, "\nfunc TestMain(") {
			continue
		}

		got := fileTiers(t, path)
		if len(got) == 0 {
			return f, nil, false
		}

		return f, got, true
	}

	return "", nil, false
}

// fileTiers reports which tiers a file's //go:build line names.
func fileTiers(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var found []string

	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)

		if !strings.HasPrefix(line, "//go:build") {
			// The constraint has to precede the package clause, so once that
			// is reached there is nothing left to find.
			if strings.HasPrefix(line, "package ") {
				break
			}

			continue
		}

		for _, tier := range tiers {
			if strings.Contains(line, tier) {
				found = append(found, tier)
			}
		}

		break
	}

	return found
}

// packagesWithTestFiles maps each directory holding _test.go files to their
// base names.
func packagesWithTestFiles(t *testing.T, root string) map[string][]string {
	t.Helper()

	pkgs := make(map[string][]string)

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

		dir := filepath.Dir(path)
		pkgs[dir] = append(pkgs[dir], filepath.Base(path))

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	return pkgs
}
