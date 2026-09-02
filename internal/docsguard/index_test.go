package docsguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// symbolIndex is every declaration this module makes, read with Go's own
// parser rather than matched with a regular expression.
//
// # Why not a regular expression
//
// Because the one this replaces was wrong in both directions and neither was
// visible without running it across every document.
//
// It missed entries in grouped const/var/type blocks, which is most of the
// module's constants — every endpoint id in remote, every Kind in resolve,
// every type alias in catalog — and reported two dozen names that plainly
// exist. Widening it to match `Name =` then made it match plain assignment
// inside a function body, so a local variable could satisfy a citation of an
// exported symbol.
//
// It also could not tell a method from a name that merely appears somewhere
// in the package: `atmosphere.Aerosol.TauAt` was checked by looking for
// `TauAt` anywhere in atmosphere, so citing a real method on the wrong type
// passed.
//
// A parser has neither problem, costs about the same, and cannot be talked
// into a false positive by an unusual formatting choice.
type symbolIndex struct {
	// decls maps a package directory to every name it declares: top-level
	// names on their own, and members qualified as "Type.Member" — a method
	// by its receiver, a struct field by its struct, an interface method by
	// its interface.
	decls map[string]map[string]bool

	// dirsByName maps a package name to the directories declaring it. Three
	// directories declare package plan, so a citation of `plan.Foo` has to
	// be resolved against all of them.
	dirsByName map[string][]string

	// varTypes maps a package-level variable to its named type, so that a
	// field reached through it resolves.
	//
	// `constants.WGS84.AngularVelocity` is how anyone would write that
	// citation and it is correct Go, but WGS84 is a var and AngularVelocity
	// is a field of WGS84Set, so the two names never meet without this. The
	// same shape covers constants.IAU, constants.Derived and every other
	// published table in the module.
	varTypes map[string]map[string]string
}

//nolint:gochecknoglobals // a parse of the whole module, built once per test binary
var (
	indexOnce  sync.Once
	indexValue *symbolIndex
	errIndex   error
)

// moduleSymbols parses the module once and shares the result.
//
// Both guards that consume it walk every document in the repository, so
// re-parsing per lookup — or per guard — is the difference between a check
// people run and one they skip.
func moduleSymbols(t *testing.T) *symbolIndex {
	t.Helper()

	indexOnce.Do(func() { indexValue, errIndex = buildSymbolIndex(filepath.Join("..", "..")) })

	if errIndex != nil {
		t.Fatalf("index the module: %v", errIndex)
	}

	return indexValue
}

func buildSymbolIndex(root string) (*symbolIndex, error) {
	idx := &symbolIndex{
		decls:      make(map[string]map[string]bool),
		dirsByName: make(map[string][]string),
		varTypes:   make(map[string]map[string]string),
	}

	fset := token.NewFileSet()

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

		// Test files declare helpers that are not part of any package's
		// documented surface, so a document citing one is still wrong.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		// Parsed with no build context, so a file behind any tag still
		// counts: a symbol declared only under `network` is declared.
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return nil //nolint:nilerr // an unparseable file is the compiler's problem, not this guard's
		}

		dir := filepath.Dir(path)
		idx.add(dir, file.Name.Name, file)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}

	return idx, nil
}

// add records every declaration in one file.
func (idx *symbolIndex) add(dir, pkgName string, file *ast.File) {
	names, ok := idx.decls[dir]
	if !ok {
		names = make(map[string]bool)
		idx.decls[dir] = names

		idx.dirsByName[pkgName] = append(idx.dirsByName[pkgName], dir)
		idx.varTypes[dir] = make(map[string]string)
	}

	for _, d := range file.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil || len(decl.Recv.List) == 0 {
				names[decl.Name.Name] = true

				continue
			}

			// A method belongs to its receiver, and is recorded only there:
			// citing pkg.Method without the type is not a thing Go lets a
			// caller write, so it should not satisfy a citation either.
			if recv := receiverName(decl.Recv.List[0].Type); recv != "" {
				names[recv+"."+decl.Name.Name] = true
			}

		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				addSpec(names, idx.varTypes[dir], spec)
			}
		}
	}
}

// addSpec records one declaration inside a const, var or type block —
// including the grouped forms that carry no keyword on their own line.
func addSpec(names map[string]bool, varTypes map[string]string, spec ast.Spec) {
	switch s := spec.(type) {
	case *ast.ValueSpec:
		for i, n := range s.Names {
			names[n.Name] = true

			if typ := valueTypeName(s, i); typ != "" {
				varTypes[n.Name] = typ
			}
		}

	case *ast.TypeSpec:
		names[s.Name.Name] = true

		switch t := s.Type.(type) {
		case *ast.StructType:
			addFieldNames(names, s.Name.Name, t.Fields)
		case *ast.InterfaceType:
			addFieldNames(names, s.Name.Name, t.Methods)
		}
	}
}

// addFieldNames records a struct's fields or an interface's methods under
// their owning type.
func addFieldNames(names map[string]bool, owner string, fields *ast.FieldList) {
	if fields == nil {
		return
	}

	for _, f := range fields.List {
		for _, n := range f.Names {
			names[owner+"."+n.Name] = true
		}

		// An embedded field is reachable by its own type name, which is how
		// a document would cite it.
		if len(f.Names) == 0 {
			if embedded := receiverName(f.Type); embedded != "" {
				names[owner+"."+embedded] = true
			}
		}
	}
}

// valueTypeName gives the named type of one entry in a var or const spec,
// from an explicit type or from a composite literal's own.
//
// `var WGS84 = WGS84Set{...}` carries its type only on the literal, which is
// the form every published constant table in this module uses.
func valueTypeName(spec *ast.ValueSpec, i int) string {
	if spec.Type != nil {
		return receiverName(spec.Type)
	}

	if i < len(spec.Values) {
		if lit, ok := spec.Values[i].(*ast.CompositeLit); ok {
			return receiverName(lit.Type)
		}
	}

	return ""
}

// receiverName reduces a receiver or embedded-field expression to the bare
// type name: T, *T, T[P] and *T[P] all give T.
func receiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return receiverName(e.X)
	case *ast.IndexExpr:
		return receiverName(e.X)
	case *ast.IndexListExpr:
		return receiverName(e.X)
	case *ast.SelectorExpr:
		// An embedded type from another package, e.g. unit.Unit.
		return e.Sel.Name
	}

	return ""
}

// lookup reports whether pkgPath declares symbol, and which directories were
// searched.
//
// pkgPath may be a bare package name or a module-relative path. The path form
// is what disambiguates: three directories declare package plan, so `plan.X`
// is satisfied by any of them while `skybrightness/plan.X` is satisfied only
// by the one.
func (idx *symbolIndex) lookup(root, pkgPath, symbol string) (found bool, searched []string) {
	if strings.Contains(pkgPath, "/") {
		dir := filepath.Join(root, filepath.FromSlash(pkgPath))
		if _, ok := idx.decls[dir]; ok {
			return idx.declares(dir, symbol), []string{dir}
		}

		return false, nil
	}

	dirs := idx.dirsByName[pkgPath]
	for _, dir := range dirs {
		if idx.declares(dir, symbol) {
			return true, dirs
		}
	}

	return false, dirs
}

// declares reports whether one directory declares symbol, following a
// package-level variable to its type when the citation reaches through one.
func (idx *symbolIndex) declares(dir, symbol string) bool {
	if idx.decls[dir][symbol] {
		return true
	}

	owner, member, ok := strings.Cut(symbol, ".")
	if !ok {
		return false
	}

	if typ := idx.varTypes[dir][owner]; typ != "" {
		return idx.decls[dir][typ+"."+member]
	}

	return false
}
