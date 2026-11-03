package docsguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestCleanupsDoNotGrantConsent stops a test from cleaning up with a grant.
//
// # What was wrong
//
// remote.EnableDownloads(0) with no ids grants consent to every Downloadable
// endpoint, with no size cap. Three network tests granted CALSPEC consent
// and then ran defer remote.EnableDownloads(0), as if it took the grant back.
// It made it unlimited and universal instead, for every test that ran
// afterwards in the same binary. A fourth site, meant to withdraw consent
// before checking the fetch was refused, granted it the same way (#967).
//
// # How it works
//
// A cleanup runs after the test, to put back the state the test found. A
// cleanup that grants consent cannot be doing that: the state before the test
// is whatever TestMain or the caller left, which the cleanup does not know.
// remote.Capture(ids...).Restore does know, because it snapshots before the
// test changes anything. So every deferred call and every t.Cleanup function
// is searched for a call to EnableDownloads, through any receiver.
//
// The check parses files rather than reading lines, so it sees a
// t.Cleanup(func() { ... }) written across lines. It reads every _test.go
// file regardless of build tags, since the three sites ran only under the
// network tag.
func TestCleanupsDoNotGrantConsent(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fset := token.NewFileSet()

	var scanned, offenders int

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "testdata", "vendor", "node_modules":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		scanned++

		ast.Inspect(file, func(n ast.Node) bool {
			cleanup := cleanupBody(n)
			if cleanup == nil {
				return true
			}

			grant := findConsentGrant(cleanup)
			if grant == nil {
				return true
			}

			offenders++

			pos := fset.Position(grant.Pos())
			rel, _ := filepath.Rel(root, pos.Filename)

			t.Errorf("%s:%d: a cleanup calls EnableDownloads.\n"+
				"  It grants consent rather than restoring what the test found, and "+
				"EnableDownloads(0) with no ids grants every endpoint with no size cap. "+
				"Capture before changing anything, t.Cleanup(remote.Capture(ids...).Restore). "+
				"See #967.", filepath.ToSlash(rel), pos.Line)

			// One report per cleanup, however many grants it holds.
			return false
		})

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if scanned < 100 {
		t.Fatalf("only %d test files scanned; the walk is not reaching the module", scanned)
	}

	t.Logf("%d test files scanned, %d cleanups granting consent", scanned, offenders)
}

// cleanupBody returns what n runs at cleanup: a deferred call, or the
// argument of a Cleanup call such as t.Cleanup or b.Cleanup. It returns nil
// for any other node.
func cleanupBody(n ast.Node) ast.Node {
	switch n := n.(type) {
	case *ast.DeferStmt:
		return n.Call
	case *ast.CallExpr:
		if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Cleanup" && len(n.Args) == 1 {
			return n.Args[0]
		}
	}

	return nil
}

// findConsentGrant returns the first call to EnableDownloads inside n, as
// remote.EnableDownloads, a Client's method, or the bare name in remote's own
// tests, or nil when there is none.
func findConsentGrant(n ast.Node) *ast.CallExpr {
	var found *ast.CallExpr

	ast.Inspect(n, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel.Name == "EnableDownloads" {
				found = call
			}
		case *ast.Ident:
			if fun.Name == "EnableDownloads" {
				found = call
			}
		}

		return found == nil
	})

	return found
}
