package docsguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// contractPackages are the packages whose exported doc comments state their
// contract and cite no issue. A package joins the list when its doc comments
// are rewritten that way (#1193), and the list goes when every package is on
// it.
var contractPackages = []string{
	"plan",
}

// issueReference is an issue or pull request number as prose cites one: "#"
// and digits, not inside a word or an HTML entity.
var issueReference = regexp.MustCompile(`(?:^|[^\w&])(#[0-9]+)\b`)

// TestExportedDocsCiteNoIssues holds the exported doc comments of
// contractPackages, the documentation pkg.go.dev shows, to stating what the
// code does: no issue or pull request number. What a change fixed, and which
// issue reported it, belongs in the changelog and the pull request (#1193).
//
// Unexported declarations and tests are not checked: a test's doc is where
// this module records why a check exists, and that history is the point of
// it.
func TestExportedDocsCiteNoIssues(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	for _, pkg := range contractPackages {
		docs := exportedDocs(t, filepath.Join(root, filepath.FromSlash(pkg)))
		if len(docs) == 0 {
			t.Errorf("%s: no exported doc comments found; the walk is not reaching the package", pkg)
		}

		for _, d := range docs {
			if m := issueReference.FindStringSubmatch(d.text); m != nil {
				t.Errorf("%s: %s's doc cites %s; state the contract, and leave the history to the changelog",
					d.pos, d.name, m[1])
			}
		}
	}
}

// exportedDoc is one doc comment pkg.go.dev shows.
type exportedDoc struct {
	pos  token.Position
	name string
	text string
}

// exportedDocs is every doc comment of dir's package that pkg.go.dev shows:
// the package's own, and those of its exported declarations, their methods,
// struct fields and interface methods.
func exportedDocs(t *testing.T, dir string) []exportedDoc {
	t.Helper()

	fset := token.NewFileSet()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}

	var files []*ast.File

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}

		files = append(files, f)
	}

	var out []exportedDoc

	// A group's doc is offered once for each exported spec in it, and kept
	// once.
	seen := make(map[*ast.CommentGroup]bool)

	add := func(name string, groups ...*ast.CommentGroup) {
		for _, g := range groups {
			if g != nil && !seen[g] {
				seen[g] = true
				out = append(out, exportedDoc{fset.Position(g.Pos()), name, g.Text()})
			}
		}
	}

	for _, f := range files {
		add("package "+f.Name.Name, f.Doc)

		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if name, ok := exportedFuncName(d); ok {
					add(name, d.Doc)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					specDocs(spec, d, add)
				}
			}
		}
	}

	return out
}

// exportedFuncName is d's name as pkg.go.dev lists it, and whether it lists
// it at all: an exported function, or an exported method of an exported type.
func exportedFuncName(d *ast.FuncDecl) (string, bool) {
	if !d.Name.IsExported() {
		return "", false
	}

	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name, true
	}

	recv := docReceiverName(d.Recv.List[0].Type)
	if !ast.IsExported(recv) {
		return "", false
	}

	return recv + "." + d.Name.Name, true
}

// docReceiverName is the type a method's receiver names, without its pointer or
// type parameters.
func docReceiverName(e ast.Expr) string {
	switch r := e.(type) {
	case *ast.StarExpr:
		return docReceiverName(r.X)
	case *ast.IndexExpr:
		return docReceiverName(r.X)
	case *ast.IndexListExpr:
		return docReceiverName(r.X)
	case *ast.Ident:
		return r.Name
	default:
		return fmt.Sprintf("%T", e)
	}
}

// specDocs adds the doc comments of one exported type, constant or variable
// in decl, with a type's exported fields and interface methods.
func specDocs(spec ast.Spec, decl *ast.GenDecl, add func(string, ...*ast.CommentGroup)) {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		if !s.Name.IsExported() {
			return
		}

		add(s.Name.Name, s.Doc, groupDoc(decl))

		var fields *ast.FieldList

		switch t := s.Type.(type) {
		case *ast.StructType:
			fields = t.Fields
		case *ast.InterfaceType:
			fields = t.Methods
		}

		if fields == nil {
			return
		}

		for _, fl := range fields.List {
			for _, n := range fl.Names {
				if n.IsExported() {
					add(s.Name.Name+"."+n.Name, fl.Doc, fl.Comment)
				}
			}
		}
	case *ast.ValueSpec:
		for _, n := range s.Names {
			if n.IsExported() {
				add(n.Name, s.Doc, s.Comment, groupDoc(decl))
			}
		}
	}
}

// groupDoc is the doc comment written above decl: a group's own, which
// pkg.go.dev shows above the group, or a lone declaration's, which the parser
// keeps on the declaration rather than its spec.
func groupDoc(decl *ast.GenDecl) *ast.CommentGroup {
	return decl.Doc
}
