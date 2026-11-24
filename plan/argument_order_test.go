package plan

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"
)

// argumentRank is the place a parameter's type takes in an exported plan
// function's arguments: a context first, then subjects (unranked), the site,
// the time or window, and providers; other parameters are unranked.
var argumentRank = map[string]int{
	"context.Context": 0,
	"*Site":           2,
	"time.Time":       3,
	"Window":          3,
	"eph.Provider":    4,
}

// TestEveryFunctionTakesOneArgumentOrder holds plan's exported functions and
// methods to one argument order: subjects, then the site, then the time or
// window, then providers, then the rest, with a context first (#1203). It
// also refuses a start and an end as two time.Time parameters, where a Window
// belongs. plan had three orders for the same triple, written at different
// times, and nothing to say which was right.
func TestEveryFunctionTakesOneArgumentOrder(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading plan: %v", err)
	}

	checked := 0

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}

		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", e.Name(), err)
		}

		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() || !exportedReceiver(fd) {
				continue
			}

			checked++
			name := fset.Position(fd.Pos()).String() + " " + fd.Name.Name
			last, times := -1, 0

			for _, field := range fd.Type.Params.List {
				typ := types.ExprString(field.Type)
				n := max(1, len(field.Names))

				if typ == "time.Time" {
					times += n
				}

				r, ranked := argumentRank[typ]
				if !ranked {
					continue
				}

				if r < last {
					t.Errorf("%s: its %s comes after a parameter that should follow it", name, typ)
				}

				last = r
			}

			if times > 1 {
				t.Errorf("%s takes %d time.Time parameters; a start and an end are a Window", name, times)
			}
		}
	}

	if checked < 100 {
		t.Fatalf("checked %d exported functions; plan has far more, so the parse missed them", checked)
	}
}

// exportedReceiver reports whether fd is a function, or a method of an
// exported type.
func exportedReceiver(fd *ast.FuncDecl) bool {
	if fd.Recv == nil {
		return true
	}

	typ := fd.Recv.List[0].Type
	if star, ok := typ.(*ast.StarExpr); ok {
		typ = star.X
	}

	id, ok := typ.(*ast.Ident)

	return ok && id.IsExported()
}
