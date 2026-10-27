package docsguard_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// goBlock matches a fenced Go code block and captures its body.
var goBlock = regexp.MustCompile("(?s)```go\\n(.*?)```")

// qualifier matches a package-qualified reference — coord.NewContext,
// remote.SetOffline — so a fragment's imports can be reconstructed from what
// it actually uses. Only a lower-case leading identifier can be a package.
var qualifier = regexp.MustCompile(`\b([a-z][a-z0-9]*)\.[A-Z]`)

// stdlibForDocs maps the standard-library packages the samples use to their
// import paths. Deliberately a short allowlist rather than a resolver: a
// sample reaching for something outside it is a sample that has grown past
// what a document should carry, and should show its import.
var stdlibForDocs = map[string]string{
	"fmt":     "fmt",
	"log":     "log",
	"os":      "os",
	"context": "context",
	"math":    "math",
	"sort":    "sort",
	"strings": "strings",
	"errors":  "errors",
}

// conventionalAliases are the names the project's samples give a package that
// are not its directory's: every example and every document writes the
// ephemeris package as eph (#852).
var conventionalAliases = map[string]string{
	"eph": "github.com/TuSKan/astrogo/ephemeris",
}

// wrappingArtifact matches the errors that come from turning a fragment into a
// function body rather than from the sample being wrong.
//
// A free variable is expected: a fragment shows a few lines in context and
// does not re-derive everything it references. "Declared and not used" is the
// same thing seen from the other side — prose continues where the code block
// stops, so the last value a sample introduces is often never consumed. Go
// rejects both inside a function and neither says anything about the sample.
//
// The free variable is a bare identifier, and the pattern is anchored to say
// so. Unanchored it also matched "undefined: jpl.WithSource", which is what Go
// reports for a function a package does not have, so a call to one passed as a
// free variable even through an import the check had made (#852).
var wrappingArtifact = regexp.MustCompile(`(undefined: \w+$|declared and not used)`)

// TestMarkdownCodeBlocksCompile type-checks every Go block in the repository's
// documents: README.md and every other Markdown file with Go blocks, but
// CHANGELOG.md, whose released entries are never rewritten, and files under a
// dot-directory, which are tooling.
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
// All of this sits under the README's sentence "Every code sample in this
// README is copy-pasted from a program that was actually compiled and run;
// none of it is aspirational."
//
// # How a fragment is checked
//
// A block with a package clause is a whole program and is compiled as it
// stands. Any other is a statement sequence that references variables it does
// not declare, so it is wrapped in a function and compiled with its imports:
// the ones it shows, then the project's conventional aliases, then the
// standard library's allowlist, then the one directory in the module with the
// qualifier's name. A qualifier more than one directory declares — plan, jpl —
// fails unless the fragment shows which it means. Errors reporting free
// variables are filtered; everything else is a real defect.
//
// Until #852 a qualifier that matched none of those was left unimported, so
// Go reported the bare name and the error passed as a free variable: no call
// through eph., plan. or jpl. was checked at all, and docs/JESUS.md carried
// two APIs that do not exist through blocks the check would have passed.
//
// That filtering is what makes the check possible and also bounds it. Go
// reports an argument-type mismatch even when other operands are undefined —
// the NewContext case above is caught — but it cannot check a method on an
// undefined receiver, so site.Atmosphere() in an unrooted fragment is invisible
// until the fragment declares site. The README's count of unchecked
// identifiers is therefore asserted, so that surface shrinks rather than
// growing quietly, and every other document's is reported.
//
// # Why a temp package inside the module
//
// It compiles against this repository directly — no second module, no
// go.mod, no network, and no risk of testing a published version instead of
// the working tree.
func TestMarkdownCodeBlocksCompile(t *testing.T) {
	root := filepath.Join("..", "..")
	idx := moduleSymbols(t)
	dir := scratchPackage(t, root)

	docs := markdownWithGoBlocks(t, root)
	if len(docs) < 2 {
		t.Fatalf("found Go blocks in %d Markdown files; the walk or the fence format is wrong", len(docs))
	}

	for _, name := range docs {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}

		blocks := goBlock.FindAllStringSubmatch(string(raw), -1)
		if name == "README.md" && len(blocks) < 10 {
			t.Fatalf("found %d Go blocks in README.md; the fence format has probably changed", len(blocks))
		}

		defects, unrooted, listings := compileDoc(t, idx, root, dir, name, string(raw))

		for _, d := range defects {
			t.Errorf("%s: code does not compile: %s\n"+
				"  Every sample is meant to have been compiled and run. Fix the sample, "+
				"or make the block self-contained so this can check it.", name, d)
		}

		t.Logf("%s: %d Go blocks, %d of them declaration listings not compiled, %d free identifiers across its fragments",
			name, len(blocks), listings, unrooted)

		// Every free variable is a line this check cannot fully verify — a
		// method call on one is invisible to the compiler, which is exactly how
		// site.Atmosphere() survived. Pinning the README's count stops that
		// surface growing unnoticed; lowering it means making a block
		// self-contained, which also makes it copy-pasteable.
		const maxUnrooted = 25

		if name == "README.md" && unrooted > maxUnrooted {
			t.Errorf("%d undefined identifiers across the README fragments, above the %d "+
				"budget.\n  Each one is a value this check cannot follow, so a wrong method "+
				"call on it goes unseen. Declare it in the sample.", unrooted, maxUnrooted)
		}
	}
}

// TestMarkdownCheckCatchesWhatItMissed is the canary #852 asked for: a call to
// a function that does not exist, through eph., through plan. with its import
// shown, through jpl. with an import the check made, and an ambiguous plan.
// with none, each in a document other than the README, each a defect.
func TestMarkdownCheckCatchesWhatItMissed(t *testing.T) {
	root := filepath.Join("..", "..")
	idx := moduleSymbols(t)
	dir := scratchPackage(t, root)

	fence := "```"

	doc := fence + "go\nprov := eph.Default()\neph.NoSuchProvider(prov)\n" + fence + "\n\n" +
		fence + "go\nimport \"github.com/TuSKan/astrogo/plan\"\n\nplan.NoSuchTarget()\n" + fence + "\n\n" +
		fence + "go\nimport \"github.com/TuSKan/astrogo/ephemeris/jpl\"\n\njpl.NoSuchOption()\n" + fence + "\n\n" +
		fence + "go\nplan.NewSun(nil)\n" + fence + "\n"

	defects, _, _ := compileDoc(t, idx, root, dir, "docs/canary.md", doc)
	all := strings.Join(defects, "\n")

	for _, want := range []string{"eph.NoSuchProvider", "plan.NoSuchTarget", "jpl.NoSuchOption", "plan. is ambiguous"} {
		if !strings.Contains(all, want) {
			t.Errorf("the check passed %s; its defects were:\n%s", want, all)
		}
	}
}

// scratchPackage makes the directory the blocks are compiled in, inside the
// module, and removes it when the test ends.
func scratchPackage(t *testing.T, root string) string {
	t.Helper()

	// A package directory inside the module, so astrogo imports resolve with
	// no module of its own.
	//
	//nolint:usetesting // t.TempDir() is outside the module; these files must
	// live inside it or "github.com/TuSKan/astrogo/..." does not resolve.
	dir, err := os.MkdirTemp(root, ".readmecheck")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { removeAndWait(t, dir) })

	return dir
}

// markdownWithGoBlocks lists, slash-separated and sorted, every Markdown file
// in the repository with a Go block, but CHANGELOG.md and files under a
// dot-directory.
func markdownWithGoBlocks(t *testing.T, root string) []string {
	t.Helper()

	var out []string

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if name := d.Name(); p != root && (strings.HasPrefix(name, ".") || name == "node_modules") {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(p, ".md") || d.Name() == "CHANGELOG.md" {
			return nil
		}

		raw, err := os.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}

		if !goBlock.Match(raw) {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return fmt.Errorf("relativize %s: %w", p, err)
		}

		out = append(out, filepath.ToSlash(rel))

		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	sort.Strings(out)

	return out
}

// compileDoc type-checks one document's Go blocks in directories of its own
// under dir, and returns every defect, how many free identifiers its fragments
// left unchecked, and how many blocks it passed over as declaration listings.
func compileDoc(t *testing.T, idx *symbolIndex, root, dir, name, content string) (defects []string, unrooted, listings int) {
	t.Helper()

	docDir := filepath.Join(dir, strings.NewReplacer("/", "_", ".", "_").Replace(name))

	// A main package cannot share a directory with anything else, so each
	// whole program gets its own; the fragments share one.
	fragDir := filepath.Join(docDir, "fragments")
	if err := os.MkdirAll(fragDir, 0o750); err != nil {
		t.Fatalf("fragment dir: %v", err)
	}

	var (
		dirs      []string
		fragments int
	)

	for i, m := range goBlock.FindAllStringSubmatch(content, -1) {
		body := m[1]

		if isDeclarationListing(body) {
			listings++

			continue
		}

		if strings.Contains(body, "package main") {
			progDir := filepath.Join(docDir, "prog"+itoa(i))
			if err := os.MkdirAll(progDir, 0o750); err != nil {
				t.Fatalf("program dir: %v", err)
			}

			if err := os.WriteFile(filepath.Join(progDir, "main.go"), []byte(body), 0o600); err != nil {
				t.Fatalf("write block %d: %v", i+1, err)
			}

			dirs = append(dirs, progDir)

			continue
		}

		src, problems := wrapFragment(t, idx, root, i, body)
		defects = append(defects, problems...)
		fragments++

		if err := os.WriteFile(filepath.Join(fragDir, "frag"+itoa(i)+".go"), []byte(src), 0o600); err != nil {
			t.Fatalf("write block %d: %v", i+1, err)
		}
	}

	// A document whose blocks are all listings or programs leaves the
	// fragments directory empty, and an empty package does not build.
	if fragments > 0 {
		dirs = append(dirs, fragDir)
	}

	for _, d := range dirs {
		found, free := buildDefects(t, root, d)
		defects = append(defects, found...)
		unrooted += free
	}

	return defects, unrooted, listings
}

// isDeclarationListing reports whether a block lists declarations rather than
// showing code to run: its first statement declares a type or a function at
// the top level, or its package clause names a package other than main.
//
// docs/ephemeris.md and docs/sgp4.md set out an API that way, with function
// signatures that have no bodies. A listing describes names, which
// TestCitedSymbolsExist checks one by one; it is not a program, and wrapping
// it in a function only makes it a syntax error.
func isDeclarationListing(body string) bool {
	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}

		if strings.HasPrefix(trimmed, "package ") {
			return trimmed != "package main"
		}

		return strings.HasPrefix(line, "type ") || strings.HasPrefix(line, "func ")
	}

	return false
}

// importSpec is one line of an import declaration: an optional name and a
// quoted path.
var importSpec = regexp.MustCompile(`^(?:([A-Za-z_]\w*)\s+)?"([^"]+)"$`)

// wrapFragment turns a statement sequence into a compilable function, and
// returns as problems the qualifiers it cannot import without guessing.
func wrapFragment(t *testing.T, idx *symbolIndex, root string, i int, body string) (string, []string) {
	t.Helper()

	// The imports a fragment shows are taken at their word, by the names they
	// give: the package clause and the import lines themselves are not
	// statements.
	shown := map[string]string{}

	var (
		stmts   []string
		inBlock bool
	)

	for line := range strings.SplitSeq(body, "\n") {
		trimmed := strings.TrimSpace(line)

		switch {
		case inBlock && trimmed == ")":
			inBlock = false
		case inBlock:
			addImportSpec(shown, trimmed)
		case trimmed == "import (":
			inBlock = true
		case strings.HasPrefix(trimmed, "import "):
			addImportSpec(shown, strings.TrimSpace(strings.TrimPrefix(trimmed, "import ")))
		case strings.HasPrefix(trimmed, "package "):
		default:
			stmts = append(stmts, line)
		}
	}

	seen := map[string]bool{}

	var imports, problems []string

	// Comments are stripped before the scan. A package named only in a comment
	// is not used, and importing it makes the wrapped fragment fail to build
	// for a reason the sample is not responsible for. The same holds for a
	// shown import the fragment does not use, which is why only qualifiers it
	// does use are imported, shown or not.
	for _, q := range qualifier.FindAllStringSubmatch(stripComments(stmts), -1) {
		pkg := q[1]
		if seen[pkg] {
			continue
		}

		seen[pkg] = true

		switch {
		case shown[pkg] != "":
			imports = append(imports, `	`+pkg+` "`+shown[pkg]+`"`)
		case conventionalAliases[pkg] != "":
			imports = append(imports, `	`+pkg+` "`+conventionalAliases[pkg]+`"`)
		case stdlibForDocs[pkg] != "":
			imports = append(imports, `	"`+stdlibForDocs[pkg]+`"`)
		case len(idx.dirsByName[pkg]) == 1:
			rel, err := filepath.Rel(root, idx.dirsByName[pkg][0])
			if err != nil {
				// Dropping the import would surface as "undefined" in the
				// compiled fragment, naming the wrong cause.
				t.Fatalf("relativize %s: %v", idx.dirsByName[pkg][0], err)
			}

			imports = append(imports, `	`+pkg+` "github.com/TuSKan/astrogo/`+filepath.ToSlash(rel)+`"`)
		case len(idx.dirsByName[pkg]) > 1:
			// Left unimported it would read as a free variable, and nothing
			// called through it would be checked.
			problems = append(problems, "block "+itoa(i+1)+": "+pkg+". is ambiguous, declared by "+
				strings.Join(idx.dirsByName[pkg], ", ")+"; show its import")
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

	return b.String(), problems
}

// addImportSpec records one import line's package under the name it is used
// by: its own, or the last element of its path.
func addImportSpec(shown map[string]string, spec string) {
	m := importSpec.FindStringSubmatch(spec)
	if m == nil {
		return
	}

	name := m[1]
	if name == "" {
		name = path.Base(m[2])
	}

	shown[name] = m[2]
}

// buildDefects compiles one directory of extracted blocks, and returns every
// error that is not a free variable and how many free variables there were.
func buildDefects(t *testing.T, root, dir string) (defects []string, unrooted int) {
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
		return nil, 0
	}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)

		switch {
		case line == "" || strings.HasPrefix(line, "#"):
		case wrappingArtifact.MatchString(line):
			// An artifact of the wrapping. Expected, and counted so the
			// unchecked surface stays visible.
			unrooted++
		default:
			defects = append(defects, line)
		}
	}

	return defects, unrooted
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

// removeAndWait removes the Markdown check's scratch package and does not return
// until it is gone (#838).
//
// On Windows os.RemoveAll can return while a file another process still has
// open, a virus scanner reading the .go file just written, stays listed as
// delete-pending until that handle closes. The next test process to walk the
// repository then finds the fragments: a gate runs ./... untagged and then
// tagged, back to back, and TestNoStandardLibraryTimeOutsideTimePackage
// failed on a leftover frag11.go that way. So this retries, about thirty
// seconds at most and without reading the clock, and says so if the
// directory outlives that rather than discarding the error.
func removeAndWait(t *testing.T, dir string) {
	t.Helper()

	var err error

	for range 600 {
		err = os.RemoveAll(dir)

		if _, serr := os.Stat(dir); errors.Is(serr, fs.ErrNotExist) {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Errorf("the README check's scratch package %s is still there after 30 s: %v\n"+
		"  A walker in this package would read it as part of the repository; remove it by hand.", dir, err)
}

// TestRemoveAndWaitLeavesNothing: the scratch package's layout, a directory of
// fragments beside program directories, is gone when removeAndWait returns.
func TestRemoveAndWaitLeavesNothing(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), ".readmecheck")

	for _, sub := range []string{"fragments", "prog3"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, sub, "main.go"), []byte("package main\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	removeAndWait(t, dir)

	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s is still there: %v", dir, err)
	}
}
