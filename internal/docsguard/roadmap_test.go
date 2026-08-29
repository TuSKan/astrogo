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

	pkgs := packageDirs(t)

	var checked int

	for i, line := range strings.Split(string(raw), "\n") {
		m := deliverable.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		done, pkgPath, symbol := m[1] == "x", m[2], m[3]

		// The last path element is the package name; a qualifier this guard
		// cannot resolve is skipped rather than guessed at.
		name := pkgPath[strings.LastIndex(pkgPath, "/")+1:]

		dirs := pkgs[name]
		if len(dirs) == 0 {
			continue
		}

		checked++

		leaf := symbol[strings.LastIndex(symbol, ".")+1:]
		exists := declaredIn(t, dirs, leaf)

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

// packageDirs maps a package name to the directories declaring it.
func packageDirs(t *testing.T) map[string][]string {
	t.Helper()

	clause := regexp.MustCompile(`(?m)^package ([a-z][A-Za-z0-9_]*)`)
	out := make(map[string][]string)
	seen := make(map[string]bool)

	root := filepath.Join("..", "..")

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") {
			return nil //nolint:nilerr // an unreadable path is skipped, not fatal
		}

		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil //nolint:nilerr // a file we cannot read contributes no package, and is not worth failing the walk for
		}

		m := clause.FindSubmatch(src)
		if m == nil {
			return nil
		}

		name, dir := string(m[1]), filepath.Dir(path)

		key := name + "\x00" + dir
		if seen[key] {
			return nil
		}

		seen[key] = true

		out[name] = append(out[name], dir)

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	return out
}

// declaredIn reports whether leaf is declared in any of dirs, as a function,
// method, type, var, const or struct field.
func declaredIn(t *testing.T, dirs []string, leaf string) bool {
	t.Helper()

	decl := regexp.MustCompile(`(?m)^\s*(func\s+(\([^)]*\)\s*)?` + leaf + `\b` +
		`|(type|var|const)\s+` + leaf + `\b` +
		`|` + leaf + `\s+(\[\]|\*|map\[)?[A-Za-z][A-Za-z0-9_.\[\]]*\s*(` + "`" + `[^` + "`" + `]*` + "`" + `)?\s*$)`)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}

			src, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
			if rerr == nil && decl.Match(src) {
				return true
			}
		}
	}

	return false
}
