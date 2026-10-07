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

// allowedVarFamilies are the packages whose exported vars are a documented
// design rather than an accident, each with the reason a var is the price of
// it (#537). Every exported var in them passes; a var anywhere else needs its
// own entry in allowedExportedVars.
//
// A var is reassignable by any importer, process-wide: one
// `unit.Meter.ScaleFactor = 0.3048` anywhere changes every length in the
// program. These families keep the form because the alternative costs more
// than it buys, and their doc comments say read-only rather than immutable.
var allowedVarFamilies = map[string]string{
	"constants": "a Constant embeds a unit.Unit, a struct, so neither a Constant nor a set of them can be a Go const; " +
		"the sets are documented read-only and selected from in hundreds of places",
	"unit": "a Unit is a struct; units appear in composite literals and arithmetic throughout the module, " +
		"where an accessor would read worse",
	"unit/dim": "a Dimension is a struct; the same reason as unit",
	"time":     "inventoried name by name in allowedTimeVars, by TestTimeExportsNoMutableFunctionValues",
}

// allowedExportedVars are exported vars outside allowedVarFamilies, keyed by
// "package/dir.Name", each with the reason Go leaves no alternative. Sentinel
// errors need no entry: see isSentinelError.
var allowedExportedVars = map[string]string{}

// TestNoUnlistedExportedVars extends TestTimeExportsNoMutableFunctionValues
// to the whole module (#537).
//
// # What was wrong
//
// After #535 and #536 removed the exported maps and the Body family,
// atmosphere.StandardRefraction and kepler.PlutoElements were still vars: one
// assignment anywhere switched every caller's refraction model or moved
// Pluto. Both are functions now. The unit and dim vars stayed, but their doc
// comments called them "immutable physical constants", which any importer
// could disprove in one line.
//
// # How it works
//
// Every exported top-level var in a non-internal, non-test file must be a
// sentinel error, belong to a family in allowedVarFamilies, or have its own
// entry in allowedExportedVars. Each entry is a sentence somebody has to
// write and a reviewer gets to disagree with.
func TestNoUnlistedExportedVars(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)
	fset := token.NewFileSet()
	seen := make(map[string]bool)
	parsed := 0

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
			// examples/ is a separate module of main packages; internal/
			// cannot be imported from outside it; testdata/ is not code.
			switch d.Name() {
			case ".git", "examples", "internal", "testdata", "vendor":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", rel, err)

			return nil
		}

		parsed++

		dir := filepath.ToSlash(filepath.Dir(rel))
		if _, family := allowedVarFamilies[dir]; family {
			return nil
		}

		for _, spec := range exportedVarSpecs(file) {
			for i, name := range spec.Names {
				if !name.IsExported() {
					continue
				}

				if i < len(spec.Values) && isSentinelError(name.Name, spec.Values[i]) {
					continue
				}

				key := dir + "." + name.Name
				seen[key] = true

				if _, ok := allowedExportedVars[key]; ok {
					continue
				}

				t.Errorf("%s:%d: exported var %s.\n"+
					"  Any importer can reassign it, process-wide. Make it a const, or a function "+
					"returning a copy of an unexported value (#113, #537). If it can be neither, "+
					"add %q to allowedExportedVars with the reason it cannot.",
					rel, fset.Position(name.Pos()).Line, name.Name, key)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	// A guard that scans nothing passes silently.
	if parsed == 0 {
		t.Fatal("scanned no files; the guard is guarding nothing")
	}

	for key, why := range allowedExportedVars {
		if !seen[key] {
			t.Errorf("allowedExportedVars lists %s (%s) but the scan did not find it; shorten the list", key, why)
		}
	}

	// A family whose package is gone would excuse whatever moved in under
	// its name later.
	for dir := range allowedVarFamilies {
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), "*.go"))
		if err != nil || len(matches) == 0 {
			t.Errorf("allowedVarFamilies lists %s, which holds no Go files (%v); shorten the list", dir, err)
		}
	}
}

// exportedVarSpecs returns every top-level var spec in file that declares at
// least one exported name.
func exportedVarSpecs(file *ast.File) []*ast.ValueSpec {
	var out []*ast.ValueSpec

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}

		for _, spec := range gen.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				out = append(out, vs)
			}
		}
	}

	return out
}

// isSentinelError reports whether a var is a sentinel error: named Err...,
// as CLAUDE.md's sentinel convention has it, and initialized by errors.New,
// fmt.Errorf, or another package's sentinel. An error value cannot be const,
// and errors.Is needs a value to compare against.
func isSentinelError(name string, value ast.Expr) bool {
	if !strings.HasPrefix(name, "Err") {
		return false
	}

	switch v := value.(type) {
	case *ast.CallExpr:
		sel, ok := v.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}

		pkg, ok := sel.X.(*ast.Ident)

		return ok && (pkg.Name == "errors" && sel.Sel.Name == "New" || pkg.Name == "fmt" && sel.Sel.Name == "Errorf")
	case *ast.SelectorExpr:
		return strings.HasPrefix(v.Sel.Name, "Err")
	}

	return false
}
