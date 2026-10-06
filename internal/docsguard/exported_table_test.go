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

// TestNoExportedPackageLevelTables keeps every lookup table in the module
// behind a function: no exported top-level var may be a map, a slice or an
// array.
//
// # What was wrong
//
// `plan.KnownSites`, `plan.MeteorShowers`, `plan.TwilightThresholds` and
// `jpl.BodyIDToNAIF` were exported maps (all four since removed, #536), and
// `core.Bodies` an exported slice (since removed, #539). Each is process-wide
// mutable state twice over: any importer can reassign the variable, and any
// importer can change an element in place without reassigning anything. One
// package deleting "paranal" or setting the civil threshold to −5° changed
// what every other caller in the binary saw, with nothing at the call site to
// show it. The last two exported slices, in internal/changelog and
// remote/file, were read only inside their own packages and are unexported
// (#542).
//
// # Why a guard rather than just the fix
//
// `var Table = map[...]...{...}` is the shortest way to publish a table and
// reads as harmless, so the next one would arrive the same way. Unlike
// [TestTimeExportsNoMutableFunctionValues] this has no allow list: a table
// has no case Go forces on us, because a function that looks a value up in
// an unexported table, or returns a copy of it, is always available.
func TestNoExportedPackageLevelTables(t *testing.T) {
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

		for _, v := range exportedTableVars(file) {
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				rel = path
			}

			t.Errorf("%s:%d: exported package-level %s %s.\n"+
				"  Any importer can change its elements, process-wide, without reassigning it. "+
				"Keep it unexported and export a lookup function (and, if callers need to "+
				"enumerate, a function returning a sorted copy) — see "+
				"plan.KnownSiteNames/NewKnownSite.",
				filepath.ToSlash(rel), fset.Position(v.name.Pos()).Line, v.kind, v.name.Name)
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

type tableVar struct {
	name *ast.Ident
	kind string // "map", "slice" or "array"
}

// exportedTableVars returns every exported name a top-level `var` in file
// declares as a map, slice or array: by its declared type, a composite
// literal of one, or make(...).
func exportedTableVars(file *ast.File) []tableVar {
	var out []tableVar

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

				kind := tableKind(vs.Type)

				if kind == "" && i < len(vs.Values) {
					kind = tableValueKind(vs.Values[i])
				}

				if kind != "" {
					out = append(out, tableVar{name: name, kind: kind})
				}
			}
		}
	}

	return out
}

// tableKind names the table type e is, or returns "" for any other type.
func tableKind(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.MapType:
		return "map"
	case *ast.ArrayType:
		if v.Len == nil {
			return "slice"
		}

		return "array"
	}

	return ""
}

// tableValueKind names the table a composite literal or make(...) builds.
func tableValueKind(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return tableKind(v.Type)
	case *ast.CallExpr:
		fn, ok := v.Fun.(*ast.Ident)
		if ok && fn.Name == "make" && len(v.Args) > 0 {
			return tableKind(v.Args[0])
		}
	}

	return ""
}
