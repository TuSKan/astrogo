package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// remoteSetter matches a call that changes remote's process-wide state: what a
// TestMain sets for its whole binary and remote.Reset would undo.
var remoteSetter = regexp.MustCompile(`\bremote\.(Set\w*|Disable|Enable)\(`)

// TestPackagesWhoseTestMainSetsRemoteDoNotResetIt stops one test in a package
// from undoing what its TestMain set for every other test.
//
// # Why
//
// remote's offline flag, download policy and cache directory are process-wide.
// remote.Reset restores the *process default*, so a test that cleans up with
// t.Cleanup(remote.Reset) does not undo itself: it undoes what TestMain set,
// for every test that runs afterwards.
//
// Reset also does not touch the data directory, which is a separate global,
// so a test that repoints the cache at an empty t.TempDir and then Resets
// leaves it pointed there. A later test then finds no cache and fails, in a
// package it has nothing to do with.
//
// This is not hypothetical. When a TestMain granted download consent it broke
// the same three eclipse and moon-phase tests twice: once from
// plan/visible_tonight_internal_test.go, whose doc comment records the
// diagnosis, and again from plan/mpc_network_test.go (#239), written
// afterwards, which used the form that comment warns about. remote.Reset's own
// doc comment says to prefer Capture/Restore. The knowledge was written down
// three times and still did not travel, so this checks instead.
//
// # Scope
//
// Only packages whose TestMain changes remote's state, because only they have
// something for a Reset to destroy. remote's own tests Reset freely and should
// — that is the package under test. There may be no such package at a given
// moment; the walk is still held to reaching the module.
//
// The fix at any site this reports is remote.Capture(ids...).Restore, which
// snapshots exactly what the test is about to change, the data directory
// included, and puts back exactly that.
func TestPackagesWhoseTestMainSetsRemoteDoNotResetIt(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	testFiles := map[string][]string{} // directory → its test files
	setting := map[string]bool{}       // directory → its TestMain sets remote

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
		testFiles[dir] = append(testFiles[dir], path)

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // an unreadable file is skipped, not fatal
		}

		if body, ok := testMainBody(string(data)); ok && remoteSetter.MatchString(body) {
			setting[dir] = true
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	scanned := 0
	for _, files := range testFiles {
		scanned += len(files)
	}

	if scanned < 100 {
		t.Fatalf("only %d test files found; the walk is not reaching the module", scanned)
	}

	offenders := 0

	for dir := range setting {
		if rel, _ := filepath.Rel(root, dir); filepath.ToSlash(rel) == "remote" {
			continue
		}

		for _, path := range testFiles[dir] {
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatalf("read %s: %v", path, rerr)
			}

			rel, _ := filepath.Rel(root, path)

			for n, line := range strings.Split(string(data), "\n") {
				trimmed := strings.TrimSpace(line)

				// A comment explaining why Reset is wrong is exactly the prose
				// this guard wants to keep, so only code counts.
				if strings.HasPrefix(trimmed, "//") || !strings.Contains(trimmed, "remote.Reset") {
					continue
				}

				offenders++

				t.Errorf("%s:%d uses remote.Reset in a package whose TestMain sets remote's state.\n"+
					"  Reset restores the process default, so this undoes TestMain's setting "+
					"for every test running afterwards, and leaves any SetDataDir pointed at "+
					"this test's temporary bucket. Use remote.Capture(ids...).Restore, which "+
					"puts back exactly what was captured. See #239.", filepath.ToSlash(rel), n+1)
			}
		}
	}

	t.Logf("%d test files scanned, %d packages whose TestMain sets remote, %d offending lines",
		scanned, len(setting), offenders)
}

// testMainBody returns the body of the TestMain src declares, if it declares
// one: everything from its signature to the first line that closes a function.
func testMainBody(src string) (string, bool) {
	start := strings.Index(src, "\nfunc TestMain(")
	if start < 0 {
		return "", false
	}

	body := src[start+1:]
	if end := strings.Index(body, "\n}\n"); end >= 0 {
		body = body[:end]
	}

	return body, true
}
