package docsguard_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestInternalPackagesHaveNoUnusedExports reports an exported symbol in an
// internal package that nothing in the module names.
//
// # Why only internal packages
//
// Because everywhere else the answer is meaningless, and the measurement says
// so rather than the intuition.
//
// #106 proposed reporting any exported symbol with no non-test caller. Run
// against this module that reports **173 of 581 exported functions** — 60 in
// plan alone, including Episode, LunarEclipses, DayEvents, Conjunctions and
// Apsides. Those are the library's public API. astrogo is a library: an
// exported symbol with no internal caller is the normal case, not a defect,
// and a guard needing a 173-entry allowlist is one that gets switched off —
// the failure the roadmap guard's own comment already warns about.
//
// An internal package has no external audience. Nothing outside this module
// can import it, so a name nothing here uses is used by nobody, and that is
// a fact rather than a judgement.
//
// # Why a test caller counts
//
// For internal/testutil especially: being called only from tests is what those
// helpers are *for*. Requiring a production caller would report the entire
// package. The question here is whether anything at all names the symbol.
//
// # What this found on its first run
//
// Three, all in internal/metrology: KindIERS, KindNASA and KindUSNO, members
// of the ReferenceKind taxonomy that nothing names. They are allowed below
// with reasons rather than deleted, because two of them have suites waiting —
// plan/usno_test.go and plan/nasa_eclipse_test.go already compare against
// exactly those sources.
//
// Worth noting how they were nearly missed: a first pass over exported
// *functions* alone reported zero, and every hit here is a constant. A guard
// that only looks at the shape of symbol it expects to find will report that
// there is nothing to find.
//
// # What it deliberately does not catch
//
// The defect that motivated #106 is not dead code. plan.IsCircumpolar has 28
// references from tests and plan.IsNeverUp 12 — they are public API that
// works. The complaint in #110 is that Episode brute-forces what they compute
// in closed form, which is a duplicated *meaning*, invisible to any
// call-graph analysis. No guard of this shape would have found it, and the
// numbers above are recorded so the idea is not proposed a second time on the
// same reasoning.
func TestInternalPackagesHaveNoUnusedExports(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")

	declared, uses, files := scanExports(t, root)
	if files < 300 {
		t.Fatalf("only %d Go files scanned; the walk is not reaching the module", files)
	}

	if len(declared) == 0 {
		t.Fatal("no exported symbols found in internal packages; the scan is not " +
			"looking at anything")
	}

	var unused []string

	for _, d := range declared {
		// A name is used when it appears more often than it is declared.
		// Comparing counts rather than looking for "a use" is what keeps a
		// declaration from counting as its own caller.
		if uses[d.name] <= d.declCount {
			unused = append(unused, d.pkg+"."+d.name)
		}
	}

	sort.Strings(unused)

	for _, u := range unused {
		if _, allowed := unusedInternalAllowed[u]; allowed {
			continue
		}

		t.Errorf("%s is exported from an internal package and named nowhere else in "+
			"the module.\n"+
			"  Nothing outside astrogo can import an internal package, so this is "+
			"either dead code to delete or something that was meant to be wired up "+
			"and was not. Add an entry to unusedInternalAllowed with the reason if "+
			"it is neither. See #106.", u)
	}

	// A stale entry is reported too, so the allowlist cannot quietly outlive
	// what it excuses. This is the half that keeps an allowlist honest: the
	// usual failure is not adding an entry, it is never removing one.
	for name, reason := range unusedInternalAllowed {
		if !slices.Contains(unused, name) {
			t.Errorf("unusedInternalAllowed lists %s, which is now used somewhere in "+
				"the module. Remove the entry — its reason (%q) no longer applies.",
				name, reason)
		}
	}

	t.Logf("%d Go files scanned, %d exported symbols in internal packages, %d unused "+
		"(%d allowed)", files, len(declared), len(unused), len(unusedInternalAllowed))
}

// internalExport is one exported top-level symbol of an internal package.
type internalExport struct {
	pkg  string
	name string

	// declCount is how many times this name is declared anywhere in the
	// module. Two packages may each declare a New, and the use count below
	// is by bare name, so the comparison has to know how many declarations
	// it is accounting for.
	declCount int
}

// scanExports returns the exported symbols of internal packages, how often
// every exported name appears module-wide, and how many files were read.
//
// Counting bare names rather than resolving them to packages is deliberate.
// A full resolve would need type information; matching by name errs toward
// *under*-reporting, since an unrelated package's identically named symbol
// makes this one look used. For a ratchet that is the safe direction: a
// missed hit is a guard that stays quiet, a false hit is a guard that gets
// deleted.
func scanExports(t *testing.T, root string) (declared []internalExport, uses map[string]int, files int) {
	t.Helper()

	uses = make(map[string]int)
	declCount := make(map[string]int)

	type pending struct{ pkg, name string }

	var found []pending

	fset := token.NewFileSet()

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" || name == "testdata" {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil //nolint:nilerr // an unparseable file is skipped, not fatal
		}

		files++

		rel := filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator)))
		dir := filepath.ToSlash(filepath.Dir(rel))

		// Every exported identifier anywhere, in any position — a call, a
		// field type, a composite literal, a function signature. Restricting
		// this to function bodies is what would produce false positives for a
		// type used only as a parameter.
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.IsExported() {
				uses[id.Name]++
			}

			return true
		})

		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		for _, name := range exportedTopLevel(f) {
			declCount[name]++

			if isInternalDir(dir) {
				found = append(found, pending{dir, name})
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	for _, p := range found {
		declared = append(declared, internalExport{pkg: p.pkg, name: p.name, declCount: declCount[p.name]})
	}

	return declared, uses, files
}

// exportedTopLevel lists a file's exported top-level declarations: functions
// without a receiver, types, constants and variables.
//
// Methods are left out. A method can be reached through an interface this
// analysis cannot resolve, so reporting one would be a guess.
func exportedTopLevel(f *ast.File) []string {
	var names []string

	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name.IsExported() {
				names = append(names, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name.IsExported() {
						names = append(names, s.Name.Name)
					}
				case *ast.ValueSpec:
					for _, id := range s.Names {
						if id.IsExported() {
							names = append(names, id.Name)
						}
					}
				}
			}
		}
	}

	return names
}

// isInternalDir reports whether a module-relative directory is inside an
// internal package tree.
func isInternalDir(dir string) bool {
	return dir == "internal" || strings.HasPrefix(dir, "internal/") ||
		slices.Contains(strings.Split(dir, "/"), "internal")
}

// unusedInternalAllowed lists exported internal symbols that are named
// nowhere else and should stay anyway, each with the reason.
//
// Keep this short. An allowlist of a few entries with reasons is a record of
// deliberate decisions; one of a hundred is a disabled check wearing a
// disguise, which is why the broad version of this guard was not built (see
// this file's leading comment).
//
// Every entry is verified still-unused above, so an excuse cannot outlive
// what it excuses.
var unusedInternalAllowed = map[string]string{
	"internal/metrology.KindIERS": "ReferenceKind is a closed taxonomy of what astrogo " +
		"validates against, and time's EOP handling is one of the things it validates. " +
		"The member exists so the suite naming it does not have to widen the type first.",
	"internal/metrology.KindNASA": "Same: plan/nasa_eclipse_test.go already compares " +
		"against NASA's Five Millennium canon and is a retrofit target for metrology.",
	"internal/metrology.KindUSNO": "Same: plan/usno_test.go already compares against " +
		"USNO and is a retrofit target for metrology.",
}
