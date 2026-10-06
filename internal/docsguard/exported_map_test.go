package docsguard_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoExportedPackageLevelMaps keeps every lookup table in the module behind
// a function.
//
// # What was wrong
//
// `plan.KnownSites`, `plan.MeteorShowers`, `plan.TwilightThresholds` and
// `jpl.BodyIDToNAIF` were exported maps (all four since removed, #533). An exported map is
// process-wide mutable state twice over: any importer can reassign the
// variable, and any importer can change an entry without reassigning
// anything. One package deleting "paranal" or setting the civil threshold to
// −5° changed what every other caller in the binary saw, with nothing at the
// call site to show it. Accessors replaced them in 0.16.0, the maps stayed
// three releases as deprecated, and #533 unexported them.
//
// # Why a guard rather than just the fix
//
// `var Table = map[...]...{...}` is the shortest way to publish a table and
// reads as harmless, so the next one would arrive the same way. Unlike
// [TestTimeExportsNoMutableFunctionValues] this has no allow list: a map has
// no case Go forces on us, because a function returning a value looked up in
// an unexported map is always available.
func TestNoExportedPackageLevelMaps(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fset := token.NewFileSet()
	parsed := 0

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if name := d.Name(); name == ".git" || name == "testdata" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			t.Errorf("parse %s: %v", path, perr)

			return nil
		}

		parsed++

		for _, name := range exportedMapVars(file) {
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				rel = path
			}

			t.Errorf("%s:%d: exported package-level map %s.\n"+
				"  Any importer can change its entries, process-wide, without reassigning it. "+
				"Keep the map unexported and export a lookup function (and, if callers need to "+
				"enumerate, a function returning a sorted copy of the keys) — see "+
				"plan.KnownSiteNames/NewKnownSite.",
				filepath.ToSlash(rel), fset.Position(name.Pos()).Line, name.Name)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	// A source-scanning guard that walks nothing passes silently.
	if parsed < 100 {
		t.Fatalf("scanned only %d files under %s; the guard is not reaching the module", parsed, root)
	}
}

// exportedMapVars returns every exported name a top-level `var` in file
// declares as a map: by its declared type, a map composite literal, or
// make(map...).
func exportedMapVars(file *ast.File) []*ast.Ident {
	var out []*ast.Ident

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			for i, name := range vs.Names {
				if !name.IsExported() {
					continue
				}

				var value ast.Expr
				if i < len(vs.Values) {
					value = vs.Values[i]
				}

				if isMapType(vs.Type) || isMapValue(value) {
					out = append(out, name)
				}
			}
		}
	}

	return out
}

func isMapType(e ast.Expr) bool {
	_, ok := e.(*ast.MapType)

	return ok
}

func isMapValue(e ast.Expr) bool {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return isMapType(v.Type)
	case *ast.CallExpr:
		fn, ok := v.Fun.(*ast.Ident)

		return ok && fn.Name == "make" && len(v.Args) > 0 && isMapType(v.Args[0])
	}

	return false
}
