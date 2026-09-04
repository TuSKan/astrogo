package docsguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// goBlock matches a fenced Go code block and captures its body.
var goBlock = regexp.MustCompile("(?s)```go\\n(.*?)```")

// qualifier matches a package-qualified reference — coord.NewContext,
// remote.SetOffline — so a fragment's imports can be reconstructed from what
// it actually uses. Only a lower-case leading identifier can be a package.
var qualifier = regexp.MustCompile(`\b([a-z][a-z0-9]*)\.[A-Z]`)

// stdlibForREADME maps the standard-library packages the samples use to their
// import paths. Deliberately a short allowlist rather than a resolver: a
// sample reaching for something outside it is a sample that has grown past
// what a README should carry.
//
//nolint:gochecknoglobals // a lookup table, not state
var stdlibForREADME = map[string]string{
	"fmt":     "fmt",
	"log":     "log",
	"os":      "os",
	"context": "context",
	"math":    "math",
	"sort":    "sort",
	"strings": "strings",
	"errors":  "errors",
}

// wrappingArtefact matches the errors that come from turning a README
// fragment into a function body rather than from the sample being wrong.
//
// A free variable is expected: a fragment shows a few lines in context and
// does not re-derive everything it references. "Declared and not used" is the
// same thing seen from the other side — prose continues where the code block
// stops, so the last value a sample introduces is often never consumed. Go
// rejects both inside a function and neither says anything about the sample.
var wrappingArtefact = regexp.MustCompile(`(undefined: \w+|declared and not used)`)

// TestREADMECodeBlocksCompile type-checks every Go block in the README.
//
// # Why symbol existence was not enough
//
// TestCitedSymbolsExist proves a name a document mentions is declared. It
// cannot prove a program builds, and the difference is not academic — the
// README carried
//
//	coord.NewContext(epoch, observer, atmosphere.Atmosphere{})
//
// for over a release. atmosphere.Atmosphere exists, so symbol checking passed
// it; NewContext takes an atmosphere.Refraction, so it never compiled. Two
// independent reviewers found that class from opposite directions, one via the
// type error and one by extracting and compiling the blocks, which is the
// argument for checking it mechanically.
//
// All of this sits under the sentence "Every code sample in this README is
// copy-pasted from a program that was actually compiled and run; none of it is
// aspirational."
//
// # How a fragment is checked
//
// Two blocks are whole programs and are compiled as they stand. The other
// fifteen are statement sequences that reference variables they do not
// declare, so each is wrapped in a function and compiled with its imports
// reconstructed from the package qualifiers it uses. Errors reporting those
// free variables are filtered; everything else is a real defect.
//
// That filtering is what makes the check possible and also bounds it. Go
// reports an argument-type mismatch even when other operands are undefined —
// the NewContext case above is caught — but it cannot check a method on an
// undefined receiver, so site.Atmosphere() in an unrooted fragment is invisible
// until the fragment declares site. The count of unchecked identifiers is
// therefore reported and asserted, so that surface shrinks rather than growing
// quietly.
//
// # Why a temp package inside the module
//
// It compiles against this repository directly — no second module, no
// go.mod, no network, and no risk of testing a published version instead of
// the working tree.
func TestREADMECodeBlocksCompile(t *testing.T) {
	root := filepath.Join("..", "..")

	raw, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	blocks := goBlock.FindAllStringSubmatch(string(raw), -1)
	if len(blocks) < 10 {
		t.Fatalf("found %d Go blocks in README.md; the fence format has probably changed",
			len(blocks))
	}

	idx := moduleSymbols(t)

	// A package directory inside the module, so astrogo imports resolve with
	// no module of its own.
	//
	//nolint:usetesting // t.TempDir() is outside the module; these files must
	// live inside it or "github.com/TuSKan/astrogo/..." does not resolve.
	dir, err := os.MkdirTemp(root, ".readmecheck")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	// A main package cannot share a directory with anything else, so each
	// whole program gets its own; the fragments share one.
	fragDir := filepath.Join(dir, "fragments")
	if err := os.MkdirAll(fragDir, 0o750); err != nil {
		t.Fatalf("fragment dir: %v", err)
	}

	var (
		programs  int
		fragments int
		dirs      = []string{fragDir}
	)

	for i, m := range blocks {
		body := m[1]

		if strings.Contains(body, "package main") {
			programs++

			progDir := filepath.Join(dir, "prog"+itoa(i))
			if err := os.MkdirAll(progDir, 0o750); err != nil {
				t.Fatalf("program dir: %v", err)
			}

			if err := os.WriteFile(filepath.Join(progDir, "main.go"), []byte(body), 0o600); err != nil {
				t.Fatalf("write block %d: %v", i+1, err)
			}

			dirs = append(dirs, progDir)

			continue
		}

		fragments++

		file := filepath.Join(fragDir, "frag"+itoa(i)+".go")
		if err := os.WriteFile(file, []byte(wrapFragment(idx, root, i, body)), 0o600); err != nil {
			t.Fatalf("write block %d: %v", i+1, err)
		}
	}

	t.Logf("%d Go blocks: %d whole programs, %d fragments", len(blocks), programs, fragments)

	var unrooted int
	for _, d := range dirs {
		unrooted += reportBuild(t, root, d)
	}

	// Every free variable is a line this check cannot fully verify — a method
	// call on one is invisible to the compiler, which is exactly how
	// site.Atmosphere() survived. Pinning the count stops that surface growing
	// unnoticed; lowering it means making a block self-contained, which also
	// makes it copy-pasteable.
	const maxUnrooted = 60

	if unrooted > maxUnrooted {
		t.Errorf("%d undefined identifiers across the README fragments, above the %d "+
			"budget.\n  Each one is a value this check cannot follow, so a wrong method "+
			"call on it goes unseen. Declare it in the sample.", unrooted, maxUnrooted)
	}

	t.Logf("%d free identifiers across fragments (budget %d)", unrooted, maxUnrooted)
}

// wrapFragment turns a statement sequence into a compilable function.
func wrapFragment(idx *symbolIndex, root string, i int, body string) string {
	// A fragment that carries its own import line is showing the import, not
	// using it; the reconstruction below covers what it actually references.
	var stmts []string

	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "package ") {
			continue
		}

		stmts = append(stmts, line)
	}

	seen := map[string]bool{}

	var imports []string

	// Comments are stripped before the scan. A package named only in a comment
	// is not used, and importing it makes the wrapped fragment fail to build
	// for a reason the sample is not responsible for.
	for _, q := range qualifier.FindAllStringSubmatch(stripComments(stmts), -1) {
		pkg := q[1]
		if seen[pkg] {
			continue
		}

		seen[pkg] = true

		switch {
		case stdlibForREADME[pkg] != "":
			imports = append(imports, `	"`+stdlibForREADME[pkg]+`"`)
		case len(idx.dirsByName[pkg]) == 1:
			rel, err := filepath.Rel(root, idx.dirsByName[pkg][0])
			if err != nil {
				continue
			}

			imports = append(imports, `	`+pkg+` "github.com/TuSKan/astrogo/`+
				filepath.ToSlash(rel)+`"`)
		}
	}

	var b strings.Builder

	b.WriteString("package readmecheck\n\n")

	if len(imports) > 0 {
		b.WriteString("import (\n" + strings.Join(imports, "\n") + "\n)\n\n")
	}

	b.WriteString("func fragment" + itoa(i) + "() {\n")
	b.WriteString(strings.Join(stmts, "\n"))
	b.WriteString("\n}\n")

	return b.String()
}

// stripComments removes line comments so a package named only in prose is not
// mistaken for one the fragment uses.
func stripComments(lines []string) string {
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		if i := strings.Index(line, "//"); i > 0 && line[i-1] != ':' {
			line = line[:i]
		} else if i == 0 {
			line = ""
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n")
}

// reportBuild compiles the extracted blocks and reports every error that is
// not a free variable.
func reportBuild(t *testing.T, root, dir string) int {
	t.Helper()

	rel, err := filepath.Rel(root, dir)
	if err != nil {
		t.Fatalf("relative dir: %v", err)
	}

	// -e lifts the compiler's ten-error cap: without it one bad block reports
	// "too many errors" and hides every later one, which is how a first pass
	// here saw four problems where there were nine.
	cmd := exec.CommandContext(t.Context(), "go", "build", "-gcflags=-e", "./"+filepath.ToSlash(rel))
	cmd.Dir = root

	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0
	}

	var (
		defects  []string
		unrooted int
	)

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case wrappingArtefact.MatchString(line):
			// An artefact of the wrapping. Expected, and counted so the
			// unchecked surface stays visible.
			unrooted++
		default:
			defects = append(defects, line)
		}
	}

	for _, line := range defects {
		t.Errorf("README code does not compile: %s\n"+
			"  The README states every sample was compiled and run. Fix the sample, "+
			"or make the block self-contained so this can check it.", line)
	}

	return unrooted
}

// itoa avoids pulling strconv in for one call.
func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	var b []byte

	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}

	return string(b)
}
