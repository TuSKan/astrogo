package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// examplesModule guards the four properties that make examples/ a separate
// module (#124) worth having rather than a place for code to rot.
//
// # Why a guard at all
//
// Before the split, `go build ./...` compiled the 32 example programs along
// with everything else, so an API change that broke one failed immediately and
// nobody had to remember anything. A module boundary ends that: `./...` stops
// at it, and the examples are now verified only because several things stay
// true at once. Each of them is one careless edit from being false, and none
// of them fails loudly on its own.
//
//  1. examples/go.mod replaces the library with ../. Drop it and the examples
//     build against the last *released* astrogo instead of this commit — they
//     would keep compiling, keep passing CI, and stop testing the change under
//     review. That is the worst of the three, because it looks like success.
//
//  2. Its `go` directive matches the root's. Two modules in one repository on
//     different toolchain minimums is a build that works for whoever ran it
//     last.
//
//  3. CI actually builds it. A perfectly correct module nothing compiles is
//     the same as no module.
//
//  4. The versions the two modules share stay equal. examples/ requires only
//     astrogo, so every other line in its require block is an indirect copy of
//     one of the root's — a hand-maintained duplicate that drifts the moment a
//     dependency is bumped in one file and not the other, and then refuses to
//     build at all.
//
// The README's claim that every sample in it was compiled and run rests on
// the same facts, which is the other reason to assert them here rather
// than trust the workflow file to keep saying what it says today.
var (
	goDirectiveLine = regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)\s*$`)
	localReplace    = regexp.MustCompile(`(?m)^replace\s+github\.com/TuSKan/astrogo\s+=>\s+\.\./\s*$`)

	// A requirement in either of go.mod's two spellings: a tab-indented line
	// inside a `require (…)` block, or a standalone `require path version`.
	// Both forms appear in this repository — examples/go.mod states its one
	// direct requirement on a single line — and matching only the block form
	// would skip a requirement silently rather than report it, which is the
	// failure mode a guard must not have.
	//
	// Comment-only lines and the block delimiters do not match, which is what
	// keeps the gocloud pin's explanatory comment from being read as a
	// requirement.
	requireLine = regexp.MustCompile(`(?m)^(?:\t|require +)([^\s/][^\s]*) +(v[^\s]+)(?: +//.*)?$`)
)

// requires maps module path to version for every require line in a go.mod.
func requires(mod []byte) map[string]string {
	out := make(map[string]string)

	for _, m := range requireLine.FindAllSubmatch(mod, -1) {
		out[string(m[1])] = string(m[2])
	}

	return out
}

// TestRequiresReadsBothSpellings pins the parser the drift check is built on.
// Its dangerous failure is not a wrong version but a missed line: a
// requirement the regexp skips is a requirement the drift check reports as
// absent rather than as unequal, and the guard passes while the two modules
// disagree.
func TestRequiresReadsBothSpellings(t *testing.T) {
	t.Parallel()

	const mod = "module example.com/m\n" +
		"\n" +
		"go 1.27\n" +
		"\n" +
		"require standalone.example/a v1.2.3\n" +
		"\n" +
		"require (\n" +
		"\t// A comment inside the block, naming v9.9.9, is not a requirement.\n" +
		"\tblocked.example/b v0.4.0\n" +
		"\tindirect.example/c v1.0.0 // indirect\n" +
		")\n" +
		"\n" +
		"replace replaced.example/d => ../d\n"

	want := map[string]string{
		"standalone.example/a": "v1.2.3",
		"blocked.example/b":    "v0.4.0",
		"indirect.example/c":   "v1.0.0",
	}

	got := requires([]byte(mod))

	for path, ver := range want {
		if got[path] != ver {
			t.Errorf("requires()[%q] = %q, want %q", path, got[path], ver)
		}
	}

	for path := range got {
		if _, expected := want[path]; !expected {
			t.Errorf("requires() reported %q = %q, which is not a requirement", path, got[path])
		}
	}
}

func TestExamplesModuleIsVerifiedAgainstTheWorkingTree(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	rootMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	exMod, err := os.ReadFile(filepath.Join(root, "examples", "go.mod"))
	if err != nil {
		t.Fatalf("read examples/go.mod: %v.\n"+
			"  examples/ is a separate module (#124). If that was undone, this guard "+
			"and ci.yml's examples steps should go with it.", err)
	}

	t.Run("the library is replaced with the working tree", func(t *testing.T) {
		t.Parallel()

		if !localReplace.Match(exMod) {
			t.Error("examples/go.mod has no `replace github.com/TuSKan/astrogo => ../`.\n" +
				"  Without it the examples compile against the last released version, so an " +
				"API break in this commit leaves them green while proving nothing. The require " +
				"line names a real release only so the module still resolves for anyone who " +
				"fetches it directly, where a replace is ignored.")
		}
	})

	t.Run("both modules ask for the same Go", func(t *testing.T) {
		t.Parallel()

		want := goDirectiveLine.FindSubmatch(rootMod)
		got := goDirectiveLine.FindSubmatch(exMod)

		switch {
		case want == nil:
			t.Fatal("go.mod has no `go` directive this guard can read")
		case got == nil:
			t.Fatal("examples/go.mod has no `go` directive this guard can read")
		case string(want[1]) != string(got[1]):
			t.Errorf("examples/go.mod declares go %s, go.mod declares go %s.\n"+
				"  Two modules in one repository on different toolchain minimums build "+
				"differently depending on who ran them.", got[1], want[1])
		}
	})

	t.Run("shared dependency versions have not drifted", func(t *testing.T) {
		t.Parallel()

		// examples/ requires astrogo and nothing else, so with the `replace`
		// above its build list is the root's build list. Every other entry in
		// its require block is therefore a copy, and a copy can go stale: a
		// dependency bump in the root leaves examples/go.mod naming the old
		// version, and `go build ./...` inside examples/ then refuses with
		// "updates to go.mod needed" rather than building against the wrong
		// one. CI's own `go mod tidy` check catches that, but only after a
		// push; this catches it in `go test ./...`.
		//
		// The fix is always the same and is the missing half of any dependency
		// bump here: `go mod tidy` in the root, then `go mod tidy` in
		// examples/.
		root := requires(rootMod)

		var stale []string

		for path, exVer := range requires(exMod) {
			if rootVer, shared := root[path]; shared && rootVer != exVer {
				stale = append(stale, path+": examples has "+exVer+", go.mod has "+rootVer)
			}
		}

		sort.Strings(stale)

		for _, s := range stale {
			t.Errorf("examples/go.mod is behind the root module — %s.\n"+
				"  Run `go mod tidy` in examples/ after every dependency bump.", s)
		}
	})

	t.Run("CI compiles it", func(t *testing.T) {
		t.Parallel()

		ci, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
		if err != nil {
			t.Fatalf("read ci.yml: %v", err)
		}

		// working-directory is the load-bearing half: a `go build ./...` that
		// runs at the repository root does not reach across the module
		// boundary and would pass without compiling a single example.
		if !strings.Contains(string(ci), "working-directory: examples") {
			t.Error("ci.yml has no step with `working-directory: examples`.\n" +
				"  Nothing in the root module compiles examples/ — `go build ./...`, " +
				"`go vet ./...` and golangci-lint all stop at the module boundary — so " +
				"without a step of its own the examples are shipped, not verified.")
		}
	})

	t.Run("the trial-merge job compiles it too", func(t *testing.T) {
		t.Parallel()

		// open-prs-still-build.sh answers a question the pull-request checks
		// cannot: whether an already-green PR still builds now that main has
		// moved. Its `go build ./...` stops at the module boundary like every
		// other, so an API change that breaks an example would merge and the
		// job would report every open PR as fine.
		script := filepath.Join(root, ".github", "scripts", "open-prs-still-build.sh")

		sh, err := os.ReadFile(script)
		if err != nil {
			t.Fatalf("read open-prs-still-build.sh: %v", err)
		}

		if !strings.Contains(string(sh), "go -C examples build") {
			t.Error("open-prs-still-build.sh does not build the examples module.\n" +
				"  It trial-merges each open pull request and runs `go build ./...`, which " +
				"no longer reaches examples/. Without `go -C examples build ./...` the job " +
				"reports a PR as building against main when an example it broke does not.")
		}
	})
}
