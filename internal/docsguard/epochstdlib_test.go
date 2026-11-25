package docsguard_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestEpochReExportsNothingFromTheStandardLibrary keeps epoch from growing
// back the layer it was renamed to remove.
//
// # Why
//
// When epoch was named time it re-exported part of the standard library so
// that no other package had to import it: GoTime, GoDate, LocationUTC, the
// duration, month and layout constants, and a Now that returned Go's time
// beside a NowUTC that returned astrogo's. Now meant two things depending on
// which package a reader thought they were in. The two packages sit side by
// side now, and a caller wanting Go's time imports it.
//
// A re-export is the shortest way to make a call site read well and looks
// harmless, so the next one would be added the same way. This fails on any
// exported name in epoch that is only the standard library's under another
// roof: a type declared as a time type, a var or const copied from one, or a
// function or method whose whole body is a call into time.
func TestEpochReExportsNothingFromTheStandardLibrary(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(repoRoot(t), "epoch")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read epoch/: %v", err)
	}

	fset := token.NewFileSet()
	parsed := 0

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", name, err)

			continue
		}

		parsed++

		std := stdTimeName(file)
		if std == "" {
			continue
		}

		for _, found := range reExports(file, std) {
			t.Errorf("%s:%d: %s re-exports the standard library's time.\n"+
				"  epoch and time sit side by side; a caller wanting Go's time imports it, "+
				"so epoch does not hand it out again under its own name.",
				name, fset.Position(found.Pos()).Line, found.Name)
		}
	}

	if parsed == 0 {
		t.Fatal("scanned no files in epoch/; the guard is guarding nothing")
	}
}

// stdTimeName is the name file imports the standard library's time under, or
// "" if it does not import it.
func stdTimeName(file *ast.File) string {
	for _, spec := range file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err != nil || path != "time" {
			continue
		}

		if spec.Name != nil {
			return spec.Name.Name
		}

		return "time"
	}

	return ""
}

// reExports returns every exported name file declares as the standard
// library's time under another roof.
func reExports(file *ast.File, std string) []*ast.Ident {
	isStd := func(e ast.Expr) bool {
		sel, ok := e.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		id, ok := sel.X.(*ast.Ident)

		return ok && id.Name == std
	}

	var out []*ast.Ident

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					// type X = time.Y, and type X time.Y: either is the
					// standard library's type under epoch's name.
					if s.Name.IsExported() && isStd(s.Type) {
						out = append(out, s.Name)
					}
				case *ast.ValueSpec:
					for i, n := range s.Names {
						if n.IsExported() && i < len(s.Values) && isStd(s.Values[i]) {
							out = append(out, n)
						}
					}
				}
			}
		case *ast.FuncDecl:
			if !d.Name.IsExported() || d.Body == nil || len(d.Body.List) != 1 {
				continue
			}

			var call ast.Expr

			switch st := d.Body.List[0].(type) {
			case *ast.ReturnStmt:
				if len(st.Results) == 1 {
					call = st.Results[0]
				}
			case *ast.ExprStmt:
				call = st.X
			}

			if c, ok := call.(*ast.CallExpr); ok && isStd(c.Fun) {
				out = append(out, d.Name)
			}
		}
	}

	return out
}
