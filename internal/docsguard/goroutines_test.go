package docsguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestEveryPackageThatStartsGoroutinesChecksForLeaks requires every package
// whose library code starts a goroutine to run its tests under
// leakcheck.RunWithLeakCheck.
//
// # Why
//
// Because the list used to be prose. internal/leakcheck's doc named the
// packages that start goroutines, and the errgroup in skybrightness's
// Model.SkyMap was never added to it (#1099). The guard that catches a worker
// started and never waited for was installed everywhere except where that
// worker was. SkyMap does wait, but nothing would have noticed the day it
// stopped.
//
// A goroutine start is a go statement or a call to a method named Go, the way
// sync.WaitGroup and errgroup.Group start one. Matching every method named Go
// errs toward asking for the check, which costs a package one TestMain line.
// Both sides are read from the syntax tree, so a comment that mentions either
// one cannot count.
func TestEveryPackageThatStartsGoroutinesChecksForLeaks(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	starts := map[string][]string{} // package directory → where it starts goroutines
	guarded := map[string]bool{}    // package directory → its tests call RunWithLeakCheck

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			switch d.Name() {
			case ".git", "examples", "testdata", "node_modules":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(p, ".go") {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("relating %s to the repository: %w", p, err)
		}

		rel = filepath.ToSlash(rel)
		dir := path.Dir(rel)

		src, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("reading %s: %w", rel, err)
		}

		fset := token.NewFileSet()

		f, err := parser.ParseFile(fset, p, src, parser.SkipObjectResolution)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", rel, err)
		}

		isTest := strings.HasSuffix(p, "_test.go")

		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.GoStmt:
				if !isTest {
					starts[dir] = append(starts[dir], fmt.Sprintf("%s:%d", rel, fset.Position(n.Pos()).Line))
				}
			case *ast.CallExpr:
				sel, ok := n.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}

				if isTest {
					if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "leakcheck" && sel.Sel.Name == "RunWithLeakCheck" {
						guarded[dir] = true
					}
				} else if sel.Sel.Name == "Go" {
					starts[dir] = append(starts[dir], fmt.Sprintf("%s:%d", rel, fset.Position(n.Pos()).Line))
				}
			}

			return true
		})

		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}

	dirs := make([]string, 0, len(starts))
	for dir := range starts {
		dirs = append(dirs, dir)
	}

	slices.Sort(dirs)

	// The scan has to be able to see something before its silence means
	// anything. Three packages started goroutines when this was written.
	if len(dirs) == 0 {
		t.Fatal("found no package that starts a goroutine; the scan is not seeing the code")
	}

	for _, dir := range dirs {
		if !guarded[dir] {
			t.Errorf("%s starts goroutines (%s) but its tests do not run under leakcheck.RunWithLeakCheck.\n"+
				"  Add to its TestMain: os.Exit(leakcheck.RunWithLeakCheck(m)). A goroutine it starts and "+
				"does not wait for would otherwise pass every test.", dir, strings.Join(starts[dir], ", "))
		}
	}

	t.Logf("%d packages start goroutines: %s", len(dirs), strings.Join(dirs, ", "))
}
