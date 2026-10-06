package docsguard_test

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestGoDocLinksResolve holds every doc link in the module's documentation to
// a symbol that exists, the way go/doc resolves it (#507).
//
// # Why nothing else catches this
//
// A doc link that does not resolve is not an error anywhere. go doc and
// pkg.go.dev render it as bracketed plain text, and the compiler, go vet and
// golangci-lint say nothing. So when a symbol is removed, every [Link] to it
// goes on reading as documentation. remote's package doc listed the front door
// it had before the rebuild — [Bucket], [OpenBucket], [Save], [APIClient] —
// and satellite's linked [Satellite.Verified] long after it was removed.
//
// # What is checked, and why that much
//
// Doc comments only: a package's, a declaration's, a spec's and a field's,
// which are what go doc renders. Test files never reach rendered
// documentation, and a comment inside a function body is never shown, so a
// link in either cannot mislead a reader of the API.
//
// A link resolves as go/doc resolves it: a package-level name, or
// Type.Member for a method, a struct field or an interface method, in the
// file's own package or a package of this module it imports. A field reached
// through a variable, [IAU2015.ObliquityJ2000], is not something Go can link,
// and neither is a bare method name. A link qualified by a package the file
// does not import renders as plain text too, and is reported as such.
//
// Only exported names are checked, because only they are documented, and
// that keeps prose in brackets out of it. Links outside this module are not
// checked.
func TestGoDocLinksResolve(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	module := modulePath(t, root)
	idx := moduleSymbols(t)

	var broken []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path contributes nothing and is not fatal
		}

		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "testdata", "examples":
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		broken = append(broken, unresolvedDocLinks(t, root, module, path, idx)...)

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	for _, b := range broken {
		t.Error(b)
	}
}

// unresolvedDocLinks lists the doc links in one file that do not resolve.
func unresolvedDocLinks(t *testing.T, root, module, path string, idx *symbolIndex) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil // an unparseable file is the compiler's problem, not this guard's
	}

	self := strings.TrimPrefix(filepath.ToSlash(filepath.Dir(path)), filepath.ToSlash(root))
	selfPath := module + self

	dir := filepath.Dir(path)
	imports := packageImports(t, dir)

	// Every name a module package goes by, so that a link qualified by a
	// package the file does not import is recognized as a link at all.
	unimported := func(name string) bool { return len(idx.dirsByName[name]) > 0 }

	parse := &comment.Parser{
		LookupPackage: func(name string) (string, bool) {
			switch {
			case strings.Contains(name, "/"):
				return name, true
			case imports[name] != "":
				return imports[name], true
			case name == file.Name.Name:
				return "", true
			case unimported(name):
				return "unimported:" + name, true
			}

			// What go/doc falls back on: a standard-library package with a
			// one-element path, math or errors, needs no import to link.
			if p, ok := comment.DefaultLookupPackage(name); ok {
				return p, true
			}

			// A qualifier that is a plain identifier and not one of this
			// package's own names reads as a package, and renders as plain
			// text: most often a conventional alias, eph for ephemeris,
			// written where it is not imported. One the package declares,
			// [localFS.Create], is a receiver, and bracketed prose, [0, 1],
			// is neither.
			if token.IsIdentifier(name) && !idx.decls[dir][name] {
				return "alias:" + name, true
			}

			return "", false
		},
		LookupSym: func(string, string) bool { return true },
	}

	var out []string

	for _, cg := range docComments(file) {
		for _, link := range docLinks(parse.Parse(cg.Text())) {
			if !exported(link.Name) || (link.Recv != "" && !exported(link.Recv)) {
				continue
			}

			where := fset.Position(cg.Pos())
			text := linkString(link)

			if pkg, ok := strings.CutPrefix(link.ImportPath, "unimported:"); ok {
				out = append(out, fmt.Sprintf("%s:%d: [%s]: %s is not imported here, so this renders as plain text; "+
					"use the full import path", filepath.ToSlash(where.Filename), where.Line, text, pkg))

				continue
			}

			if pkg, ok := strings.CutPrefix(link.ImportPath, "alias:"); ok {
				// [name] alone is bracketed prose as often as a package
				// link, so only a qualified [name.Symbol] is held to it.
				if link.Name != "" {
					out = append(out, fmt.Sprintf("%s:%d: [%s]: no package %s is imported here, so this renders "+
						"as plain text; use the full import path", filepath.ToSlash(where.Filename), where.Line, text, pkg))
				}

				continue
			}

			target := link.ImportPath
			if target == "" {
				target = selfPath
			}

			rel, inModule := strings.CutPrefix(target, module)
			if !inModule {
				continue
			}

			targetDir := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(rel, "/")))

			if link.Name == "" { // a link to a package, which must exist
				if _, ok := idx.decls[targetDir]; !ok {
					out = append(out, fmt.Sprintf("%s:%d: [%s] does not resolve: no package %s in this module",
						filepath.ToSlash(where.Filename), where.Line, text, target))
				}

				continue
			}

			key := link.Name
			if link.Recv != "" {
				key = link.Recv + "." + link.Name
			}

			if !idx.decls[targetDir][key] {
				out = append(out, fmt.Sprintf("%s:%d: [%s] does not resolve: %s declares no %s",
					filepath.ToSlash(where.Filename), where.Line, text, target, key))
			}
		}
	}

	return out
}

// packageImports maps each name a package's non-test files import under to
// its import path, as go/doc builds it: from every file of the package, not
// only the one a comment is in. A name two files import as different paths
// is ambiguous, and go/doc resolves neither; it maps to "".
func packageImports(t *testing.T, dir string) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Errorf("read %s: %v", dir, err)

		return nil
	}

	fset := token.NewFileSet()
	imports := map[string]string{}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", filepath.Join(dir, name), err)

			continue
		}

		for _, im := range file.Imports {
			p := strings.Trim(im.Path.Value, `"`)

			as := p[strings.LastIndex(p, "/")+1:]
			if im.Name != nil {
				as = im.Name.Name
			}

			if prev, seen := imports[as]; seen && prev != p {
				imports[as] = ""
			} else if !seen {
				imports[as] = p
			}
		}
	}

	return imports
}

// docComments are the comment groups go doc renders: the package's, and each
// declaration's, spec's and field's.
func docComments(file *ast.File) []*ast.CommentGroup {
	var groups []*ast.CommentGroup

	add := func(cg *ast.CommentGroup) {
		if cg != nil {
			groups = append(groups, cg)
		}
	}

	add(file.Doc)

	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			add(x.Doc)

			return false // a body's comments are never rendered
		case *ast.GenDecl:
			add(x.Doc)
		case *ast.TypeSpec:
			add(x.Doc)
		case *ast.ValueSpec:
			add(x.Doc)
		case *ast.Field:
			add(x.Doc)
		}

		return true
	})

	return groups
}

// docLinks are every doc link in a parsed comment.
func docLinks(doc *comment.Doc) []*comment.DocLink {
	var links []*comment.DocLink

	var walk func(texts []comment.Text)

	walk = func(texts []comment.Text) {
		for _, t := range texts {
			switch x := t.(type) {
			case *comment.DocLink:
				links = append(links, x)
			case *comment.Link:
				walk(x.Text)
			}
		}
	}

	var blocks func(bs []comment.Block)

	blocks = func(bs []comment.Block) {
		for _, b := range bs {
			switch x := b.(type) {
			case *comment.Paragraph:
				walk(x.Text)
			case *comment.Heading:
				walk(x.Text)
			case *comment.List:
				for _, item := range x.Items {
					blocks(item.Content)
				}
			}
		}
	}

	blocks(doc.Content)

	return links
}

// linkString is a link as it was written.
func linkString(l *comment.DocLink) string {
	var b strings.Builder

	for _, t := range l.Text {
		if p, ok := t.(comment.Plain); ok {
			b.WriteString(string(p))
		}
	}

	return b.String()
}

// exported reports whether a doc link names an exported identifier. An empty
// name is a link to a package, which is checked against the module.
func exported(name string) bool {
	if name == "" {
		return true
	}

	for _, r := range name {
		return unicode.IsUpper(r)
	}

	return false
}

// modulePath reads the module path from go.mod.
func modulePath(t *testing.T, root string) string {
	t.Helper()

	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("open go.mod: %v", err)
	}

	defer func() { _ = f.Close() }()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if mod, ok := strings.CutPrefix(strings.TrimSpace(s.Text()), "module "); ok {
			return strings.TrimSpace(mod)
		}
	}

	t.Fatal("go.mod declares no module")

	return ""
}
