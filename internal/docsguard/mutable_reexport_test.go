package docsguard_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// allowedEpochVars is the complete inventory of exported package-level vars the
// `epoch` package may hold, each with the reason Go leaves no alternative.
//
// A var is reassignable by any importer, process-wide. In the package every
// epoch calculation in the library passes through, that is worth spending a
// line of justification on rather than letting one appear by habit.
var allowedEpochVars = map[string]string{
	// Sentinel errors re-exported from the unexported epoch/internal/iers, so
	// callers can errors.Is against them. An error value cannot be const, and
	// the sentinel-plus-%w idiom is what CLAUDE.md asks for.
	"ErrOutOfRange":  "sentinel error; error values cannot be const",
	"ErrNoRecords":   "sentinel error; error values cannot be const",
	"ErrNoEOPLoader": "sentinel error; error values cannot be const",
	"ErrNoEOPData":   "sentinel error; error values cannot be const",

	// Sentinels the leap-second registry raises when a caller registers a
	// table that contradicts the built-in one. Same reason.
	"ErrLeapSecondConflict": "sentinel error; error values cannot be const",
	"ErrLeapSecondOrder":    "sentinel error; error values cannot be const",

	// Nothing on this list is anything but a sentinel error. LocationUTC was
	// the last, wrapping the standard library's own mutable time.UTC, and went
	// with every other re-export of the standard library. J2000 was another,
	// and #113 turned it into a function: a struct value cannot be const, but
	// a function that returns a copy of an unexported one cannot be
	// reassigned either.
}

// TestEpochExportsNoMutableFunctionValues keeps the package that underpins
// every epoch in the library from re-exporting the standard library through
// reassignable variables.
//
// # What was wrong
//
// Fifteen of this package's re-exports, when it was named `time`, were
// declared `var Parse = time.Parse`,
// `var Sleep = time.Sleep`, `var Now = time.Now` and so on, and the six layout
// strings were a `var` block where the standard library has untyped constants.
// Any package anywhere in the import graph could assign to any of them and
// change the behavior of every epoch calculation in the process — a
// supply-chain footgun with no upside, since a plain function is identical at
// every call site and a const is identical to a string literal.
//
// It also made README.md's "no hidden global state" straightforwardly untrue
// in the one package where it mattered most.
//
// # Why a guard rather than just the fix
//
// `var X = pkg.X` is the shortest way to re-export something and reads as
// entirely harmless, so the next one will be added the same way. The fix is
// invisible once made; this is what keeps it made.
//
// The allow list is an inventory rather than an exclusion list: every entry
// carries why Go offers no alternative, so adding one is a sentence somebody
// has to write and a reviewer gets to disagree with.
func TestEpochExportsNoMutableFunctionValues(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(repoRoot(t), "epoch")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read epoch/: %v", err)
	}

	fset := token.NewFileSet()
	seen := make(map[string]bool)
	parsed := 0

	// os.ReadDir plus ParseFile rather than parser.ParseDir, which Go 1.25
	// deprecated for not considering build tags when grouping files into
	// packages. Nothing here needs that grouping: every non-test .go file in
	// the directory is scanned whatever its tags, which is what a guard wants.
	for _, entry := range entries {
		fileName := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(fileName, ".go") || strings.HasSuffix(fileName, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(dir, fileName), nil, 0)
		if err != nil {
			t.Errorf("parse %s: %v", fileName, err)

			continue
		}

		parsed++

		for _, name := range exportedVarNames(file) {
			seen[name.Name] = true

			if _, allowed := allowedEpochVars[name.Name]; allowed {
				continue
			}

			t.Errorf("%s:%d: exported var %s.\n"+
				"  Any importer can reassign this, process-wide, in the package every epoch in "+
				"the library goes through. Re-export a function AS a function — identical at "+
				"every call site — and a constant value as a const. If it can be neither, add "+
				"it to allowedEpochVars with the reason it cannot.",
				fileName, fset.Position(name.Pos()).Line, name.Name)
		}
	}

	// A source-scanning guard that walks nothing passes silently. Both counts
	// close that: the files were read, and every allowed name was actually
	// found rather than the list guarding an empty set.
	if parsed == 0 {
		t.Fatal("scanned no files in epoch/; the guard is guarding nothing")
	}

	for name, why := range allowedEpochVars {
		if !seen[name] {
			t.Errorf("allowedEpochVars lists %s (%s) but the scan did not find it.\n"+
				"  Either it was removed — shorten the list — or the scan is not reaching epoch/.",
				name, why)
		}
	}
}

// exportedVarNames returns every exported name declared by a top-level `var`
// in file. Constants are not included, which is the point: a const cannot be
// reassigned and needs no justification.
func exportedVarNames(file *ast.File) []*ast.Ident {
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

			for _, name := range vs.Names {
				if name.IsExported() {
					out = append(out, name)
				}
			}
		}
	}

	return out
}
