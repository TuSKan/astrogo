package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// citedSymbol matches a backticked, package-qualified Go symbol in prose:
// `coord.Separation`, `atmosphere.Aerosol.TauAt`, `remote/file.NewReaderAt`.
//
// Backticks are what make this narrow enough to be worth having. Unquoted
// prose is full of dotted phrases that are not symbols, and a guard that
// reports them gets switched off — the lesson the roadmap guard's own comment
// records after an audit where three of four reports were wrong.
//
// The package must start lower-case and the symbol upper-case, which is Go's
// own rule for an exported name and excludes `basic.rvz_radvel` (a SIMBAD
// column) and `object.kind` (a JSON field) without needing to know what those
// are.
var citedSymbol = regexp.MustCompile(
	"`([a-z][a-z0-9]*(?:/[a-z][a-z0-9_]*)*)\\.([A-Z][A-Za-z0-9_]*(?:\\.[A-Za-z][A-Za-z0-9_]*)*)`")

// unbuiltPromise matches an unchecked roadmap checkbox.
//
// A symbol named in one is the thing the box exists to build, so it is
// supposed not to exist yet — `plan.Weather` and `plan.SatelliteIllumination`
// are both promises, and flagging them would make this guard demand that the
// roadmap only describe finished work.
//
// TestRoadmapBoxesMatchTheCode covers those from the other side: an unchecked
// box whose symbol *has* appeared is finished work left looking open, and it
// fails there.
//
// Deliberately not anchored to the line start. ROADMAP.md documents its own
// checkbox convention by quoting an example inline, mid-sentence, and that
// example names an unbuilt symbol for the same reason the real boxes do. It
// is an illustration of the syntax rather than a claim about the code, which
// reads as obvious to everyone except a regexp.
var unbuiltPromise = regexp.MustCompile(`- \[ \]`)

// TestCitedSymbolsExist checks that a symbol a document names is a symbol the
// code declares.
//
// # Why, given the roadmap is already guarded
//
// Because the roadmap is one document and the guard on it is deliberately
// narrow: it reads only a checkbox whose text *begins* with a symbol, since
// that is the form meaning "this box delivers this thing". Everything else —
// the equation-to-function maps in docs/skybrightness.md, the architecture
// notes in CLAUDE.md, the API tour in the README — was unguarded.
//
// It rots the same way. docs/skybrightness.md carried
// `atmosphere.AerosolOpticalDepth` for a function that has been
// `Aerosol.TauAt` for some time, in a table row whose *test* column was stale
// too. The test half was caught by TestCitedTestsExist and the function half
// was not, because nothing looked at it. A table that maps an equation to the
// code implementing it is worth exactly as much as the accuracy of both
// columns.
//
// # What is skipped, and why that is not a weakness
//
// A qualifier that names no package in this module — `astropy.SkyCoord`,
// `numpy.ndarray`, `gofa.Plan94` — is skipped rather than guessed at. The
// guard reports how many it resolved so a change that silently stops
// resolving anything fails the count check rather than passing vacuously.
//
// CHANGELOG.md is exempt for the reason it always is: it is forward-only, and
// an entry describes the release it shipped in. Rewriting a released entry to
// track a rename would falsify the record to satisfy a guard.
func TestCitedSymbolsExist(t *testing.T) {
	root := filepath.Join("..", "..")
	idx := moduleSymbols(t)

	var docs []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		if info.IsDir() {
			if name := info.Name(); name == ".git" || name == "node_modules" {
				return filepath.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(path, ".md") {
			docs = append(docs, path)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	var checked, offenders int

	for _, path := range docs {
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			continue
		}

		slash := filepath.ToSlash(rel)
		if slash == "CHANGELOG.md" {
			continue
		}

		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			continue
		}

		for n, line := range strings.Split(string(raw), "\n") {
			if historicalMention.MatchString(line) || unbuiltPromise.MatchString(line) {
				continue
			}

			for _, m := range citedSymbol.FindAllStringSubmatch(line, -1) {
				pkgPath, symbol := m[1], m[2]

				exists, searched := idx.lookup(root, pkgPath, symbol)
				if len(searched) == 0 {
					continue // not a package in this module
				}

				checked++

				if exists {
					continue
				}

				offenders++

				t.Errorf("%s:%d: cites %s.%s, which %v does not declare.\n"+
					"  Point at what implements this now, or drop the claim. If the name is "+
					"deliberately historical, say so on the same line — \"since renamed\", "+
					"\"no longer exists\".", slash, n+1, pkgPath, symbol, searched)
			}
		}
	}

	if checked < 50 {
		t.Fatalf("only %d qualified symbols resolved to a package in this module; the "+
			"citation format or the package walk has probably changed", checked)
	}

	t.Logf("%d cited symbols checked across %d documents, %d unresolved",
		checked, len(docs), offenders)
}
