package docsguard_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestConsentGrantingPackagesDoNotResetRemote stops one test in a package
// from revoking the download consent every other test in it depends on.
//
// # Why
//
// remote's consent, offline flag and cache directory are process-wide, and
// several packages grant consent once in TestMain for the whole test binary —
// real DE441/DE442 fetches need it. remote.Reset restores the *process
// default*, which is no consent at all, so a test that cleans up with
// t.Cleanup(remote.Reset) does not undo itself: it undoes TestMain, for every
// test that runs afterwards.
//
// Reset also does not touch the data directory, which is a separate global,
// so a test that repoints the cache at an empty t.TempDir and then Resets
// leaves it pointed there. A later test then finds neither consent nor cache
// and fails instantly, in a package it has nothing to do with.
//
// This is not hypothetical and not new. It broke the same three eclipse and
// moon-phase tests twice: once from plan/visible_tonight_internal_test.go,
// whose doc comment records the diagnosis, and again from
// plan/mpc_network_test.go (#239), written afterwards, which used the form
// that comment warns about. remote.Reset's own doc comment says to prefer
// Capture/Restore. The knowledge was written down three times and still did
// not travel, so this checks instead.
//
// # Scope
//
// Only packages that grant consent in TestMain, because only they have
// something for a Reset to destroy. remote's own tests Reset freely and
// should — that is the package under test.
//
// The fix at any site this reports is remote.Capture(ids...).Restore, which
// snapshots exactly what the test is about to change, the data directory
// included, and puts back exactly that.
func TestConsentGrantingPackagesDoNotResetRemote(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	granting := consentGrantingPackages(t, root)
	if len(granting) < 3 {
		t.Fatalf("only %d packages found granting download consent in TestMain; "+
			"the walk is not reaching the module", len(granting))
	}

	var scanned, offenders int

	for _, dir := range granting {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}

			path := filepath.Join(dir, e.Name())

			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}

			scanned++

			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)

			for n, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)

				// A comment explaining why Reset is wrong is exactly the
				// prose this guard wants to keep, so only code counts.
				if strings.HasPrefix(trimmed, "//") || !strings.Contains(trimmed, "remote.Reset") {
					continue
				}

				offenders++

				t.Errorf("%s:%d uses remote.Reset in a package whose TestMain grants "+
					"download consent.\n"+
					"  Reset restores the process default — no consent — so this revokes "+
					"TestMain's grant for every test running afterwards, and leaves any "+
					"SetDataDir pointed at this test's temporary bucket. Use "+
					"remote.Capture(ids...).Restore, which puts back exactly what was "+
					"captured. See #239.", rel, n+1)
			}
		}
	}

	t.Logf("%d test files scanned across %d consent-granting packages, %d offending lines",
		scanned, len(granting), offenders)
}

// consentGrantingPackages returns the directories holding a TestMain that
// grants download consent — the only ones where a Reset has something to
// destroy.
//
// Both markers must appear in the same file, so a package with an ordinary
// TestMain and an unrelated EnableDownloads elsewhere is not swept in.
func consentGrantingPackages(t *testing.T, root string) []string {
	t.Helper()

	var dirs []string

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

		// A declaration at column zero and a qualified call, rather than bare
		// substrings: this file names both markers in its own strings and doc
		// comment, and a guard that reports itself reports nothing useful.
		src := string(data)
		if !strings.HasPrefix(src, "func TestMain(") && !strings.Contains(src, "\nfunc TestMain(") {
			return nil
		}

		if !strings.Contains(src, "remote.EnableDownloads(") {
			return nil
		}

		dir := filepath.Dir(path)
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	return dirs
}

// TestSetDataDirIsNotResetAway catches the other half of the trap above in
// every package, not only those whose TestMain grants consent: a test that
// points the data directory at its own temporary bucket and cleans up with
// remote.Reset leaves it pointed there, since Reset does not touch the data
// directory. The bucket is deleted with the test, and the next test to cache
// anything fails to create its lock inside it. catalog/mpcorb, which grants
// consent inside its tests rather than in TestMain, did exactly this, and
// broke TestOpenParsesTheCometFile on every run (#443).
//
// A test function that calls both is reported, unless it also puts the
// default back with remote.SetDataDir(""), as time's EOP tests do. remote's
// own tests are exempt, being the package under test, and so is this package,
// which names both in its messages.
func TestSetDataDirIsNotResetAway(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	var scanned, offenders int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" || rel == "remote" || rel == "internal/docsguard" {
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

		scanned++

		// One chunk per top-level function, and only code lines in it: a
		// comment explaining why Reset is wrong is prose this guard keeps.
		for fn := range strings.SplitSeq(string(data), "\nfunc ") {
			var sets, resets, restores bool

			for line := range strings.SplitSeq(fn, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") {
					continue
				}

				sets = sets || strings.Contains(trimmed, "remote.SetDataDir(")
				resets = resets || strings.Contains(trimmed, "remote.Reset")
				restores = restores || strings.Contains(trimmed, `remote.SetDataDir("")`)
			}

			if sets && resets && !restores {
				offenders++

				name, _, _ := strings.Cut(fn, "(")

				t.Errorf("%s: func %s sets remote's data directory and cleans up with remote.Reset.\n"+
					"  Reset leaves the data directory alone, so it stays pointed at this "+
					"test's temporary bucket after the bucket is deleted. Use "+
					"remote.Capture(ids...).Restore, which restores the data directory too. "+
					"See #443.", rel, name)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if scanned < 100 {
		t.Fatalf("only %d test files scanned; the walk is not reaching the module", scanned)
	}

	t.Logf("%d test files scanned, %d offending functions", scanned, offenders)
}
