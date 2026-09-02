package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const roadmapDoc = "../../docs/ROADMAP.md"

// deliverable matches a roadmap checkbox whose text *begins* with a
// package-qualified symbol in backticks — the form that means "this box
// delivers this thing", as opposed to one that merely mentions it.
//
// The distinction is the whole reason this guard is narrow enough to be
// worth having. An audit that matched any backticked name in any box
// reported four stale entries, and three were wrong: "Integration with
// `SatellitePasses`" and "Optional coupling with `MoonSep`" name symbols the
// work would build *on*, not build, and a bare "`Horizon` constraint" matched
// atmosphere.HorizonProfile, a different thing in a different package. A
// guard that cries wolf three times out of four gets switched off.
var deliverable = regexp.MustCompile("^- \\[([ x])\\] `([a-z][A-Za-z0-9_]*(?:/[a-z][A-Za-z0-9_]*)*)\\.([A-Za-z][A-Za-z0-9_.]*)`")

// TestRoadmapBoxesMatchTheCode checks a roadmap checkbox against the package
// it names.
//
// A stale *unchecked* box is the mirror of a stale tick, and the worse of the
// two: it hides finished work and sends the next contributor to build
// something twice. resolve.Target.HasRadialVelocity sat unchecked while being
// declared, populated by catalog/simbad, preserved through the merge and
// consumed by plan.
//
// Both directions are checked. A tick whose symbol has been deleted is a
// promise the code no longer keeps.
func TestRoadmapBoxesMatchTheCode(t *testing.T) {
	raw, err := os.ReadFile(roadmapDoc)
	if err != nil {
		t.Fatalf("read %s: %v", roadmapDoc, err)
	}

	root := filepath.Join("..", "..")
	idx := moduleSymbols(t)

	var checked int

	for i, line := range strings.Split(string(raw), "\n") {
		m := deliverable.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		done, pkgPath, symbol := m[1] == "x", m[2], m[3]

		// A qualifier naming no package in this module is skipped rather
		// than guessed at.
		exists, dirs := idx.lookup(root, pkgPath, symbol)
		if len(dirs) == 0 {
			continue
		}

		checked++

		switch {
		case !done && exists:
			t.Errorf("%s:%d: box is unchecked but %s.%s is already declared — "+
				"finished work left looking open sends the next contributor to build it twice",
				roadmapDoc, i+1, pkgPath, symbol)
		case done && !exists:
			t.Errorf("%s:%d: box is checked but %s.%s is not declared in %v",
				roadmapDoc, i+1, pkgPath, symbol, dirs)
		}
	}

	if checked == 0 {
		t.Fatal("no roadmap box named a resolvable package-qualified symbol; the format may have changed")
	}

	t.Logf("%d roadmap boxes checked against the code", checked)
}
