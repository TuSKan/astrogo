package docsguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestSetDataDirIsRestored stops a test from leaving remote's data directory
// pointed at a bucket of its own.
//
// # What was wrong
//
// The data directory is process-wide. A test that points it at t.TempDir()
// and never puts it back leaves every later test in the binary with a
// directory Go has already deleted. create_dir=true does not save it, since
// the test's parent temporary directory is gone too: measured, the next write
// fails with "The system cannot find the path specified", in whichever test
// happens to run next. catalog/mpcorb did this through remote.Reset, which
// leaves the data directory alone, and broke TestOpenParsesTheCometFile on
// every run (#443). This guard's first version caught only that form: a test
// that set the directory and cleaned up with nothing at all passed it, and
// five did (#971).
//
// # How it works
//
// A test function that calls remote.SetDataDir with anything but "" must put
// the directory back: by capturing it first, remote.Capture(...).Restore, or by
// restoring the default with remote.SetDataDir(""), as time's EOP tests do. A
// helper in the same package that does either counts for every function that
// calls it, so serveProfile-style fixtures need no second capture in each test.
//
// remote's own tests are exempt, being the package under test, and so is this
// package, which names both calls in its messages.
func TestSetDataDirIsRestored(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fset := token.NewFileSet()
	packages := make(map[string][]*ast.File)
	scanned := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("relative path of %s: %w", path, relErr)
		}

		rel = filepath.ToSlash(rel)

		if d.IsDir() {
			switch {
			case d.Name() == ".git", d.Name() == "testdata", d.Name() == "vendor",
				rel == "remote", rel == "internal/docsguard":
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

		dir := filepath.ToSlash(filepath.Dir(rel))
		packages[dir] = append(packages[dir], file)

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if scanned < 100 {
		t.Fatalf("only %d test files scanned; the walk is not reaching the module", scanned)
	}

	dirs := make([]string, 0, len(packages))
	for dir := range packages {
		dirs = append(dirs, dir)
	}

	sort.Strings(dirs)

	offenders := 0

	for _, dir := range dirs {
		files := packages[dir]

		// Helpers that restore, by name, so their callers count as restoring.
		restorers := make(map[string]bool)

		for _, file := range files {
			for _, decl := range file.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Body != nil && restoresDataDir(fn.Body, nil) {
					restorers[fn.Name.Name] = true
				}
			}
		}

		for _, file := range files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}

				set := setsDataDir(fn.Body)
				if set == nil || restoresDataDir(fn.Body, restorers) {
					continue
				}

				offenders++

				pos := fset.Position(set.Pos())
				rel, _ := filepath.Rel(root, pos.Filename)

				t.Errorf("%s:%d: func %s sets remote's data directory and never puts it back.\n"+
					"  Every later test in the binary then inherits this test's temporary "+
					"bucket, which is deleted with the test, so the next cache write fails. "+
					"Capture first, t.Cleanup(remote.Capture(ids...).Restore). See #443, #971.",
					filepath.ToSlash(rel), pos.Line, fn.Name.Name)
			}
		}
	}

	t.Logf("%d test files scanned in %d packages, %d functions leaving the data directory set",
		scanned, len(packages), offenders)
}

// setsDataDir returns the first remote.SetDataDir call in body whose argument
// is anything but the empty string literal, or nil.
func setsDataDir(body ast.Node) *ast.CallExpr {
	var found *ast.CallExpr

	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		if call, ok := n.(*ast.CallExpr); ok && isRemoteCall(call, "SetDataDir") && !isEmptyStringArg(call) {
			found = call
		}

		return found == nil
	})

	return found
}

// restoresDataDir reports whether body captures remote's state, restores the
// default data directory, or calls one of the named helpers that does.
func restoresDataDir(body ast.Node, restorers map[string]bool) bool {
	restores := false

	ast.Inspect(body, func(n ast.Node) bool {
		if restores {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch {
		case isRemoteCall(call, "Capture"):
			restores = true
		case isRemoteCall(call, "SetDataDir") && isEmptyStringArg(call):
			restores = true
		default:
			if id, ok := call.Fun.(*ast.Ident); ok && restorers[id.Name] {
				restores = true
			}
		}

		return !restores
	})

	return restores
}

// isRemoteCall reports whether call is remote.<name>(...).
func isRemoteCall(call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}

	pkg, ok := sel.X.(*ast.Ident)

	return ok && pkg.Name == "remote"
}

// isEmptyStringArg reports whether call's only argument is the literal "".
func isEmptyStringArg(call *ast.CallExpr) bool {
	if len(call.Args) != 1 {
		return false
	}

	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}

	s, err := strconv.Unquote(lit.Value)

	return err == nil && s == ""
}
