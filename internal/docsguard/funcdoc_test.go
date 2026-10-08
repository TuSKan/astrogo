package docsguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestNoFuncDocOpensWithAnotherFunc holds every function's doc comment to its
// own function (#642).
//
// # Why
//
// A function inserted between an existing function and its doc comment takes
// the comment over: the old doc now opens the new function's, and the old
// function has none. Five had happened on main, and nothing noticed. plan's
// raOfDateDifference opened with "wrap180 is an angle …", and starlight's
// parquetRows.Has was documented as reading a column as a float. revive's
// exported check looks only at exported functions, so an unexported one
// documented as another passes everything.
//
// # What is checked
//
// A function's doc must not begin with the name of a different function
// declared in the same file. It cannot require every doc to begin with its
// own name: about one in seven, most of them tests, opens with prose ("The …",
// "A …") on purpose. Test files are included. Three of the five were in them,
// and a test's doc is where this module records why a check exists.
func TestNoFuncDocOpensWithAnotherFunc(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	var checked int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path contributes nothing and is not fatal
		}

		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "testdata":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		checked++

		for _, msg := range funcDocsOpeningWithAnotherFunc(root, path) {
			t.Error(msg)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	if checked < 500 {
		t.Fatalf("only %d Go files scanned; the walk is not reaching the module", checked)
	}
}

// funcDocsOpeningWithAnotherFunc lists the functions in one file whose doc
// comment opens with the name of another function in that file.
func funcDocsOpeningWithAnotherFunc(root, path string) []string {
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil // an unparseable file is the compiler's problem, not this guard's
	}

	funcs := make(map[string]bool)

	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcs[fn.Name.Name] = true
		}
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}

	var found []string

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Doc == nil {
			continue
		}

		first := firstWord(fn.Doc.Text())
		if first == fn.Name.Name || !funcs[first] {
			continue
		}

		found = append(found, fmt.Sprintf("%s:%d: func %s's doc comment opens with %s, another function in this file.\n"+
			"  A function inserted between %s and its doc took the doc over; move it back above func %s.",
			filepath.ToSlash(rel), fset.Position(fn.Pos()).Line, fn.Name.Name, first, first, first))
	}

	return found
}

// firstWord returns the first identifier-like word of text.
func firstWord(text string) string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})

	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}
