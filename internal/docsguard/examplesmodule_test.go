package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// examplesModule guards the three properties that make examples/ a separate
// module (#124) worth having rather than a place for code to rot.
//
// # Why a guard at all
//
// Before the split, `go build ./...` compiled the 32 example programs along
// with everything else, so an API change that broke one failed immediately and
// nobody had to remember anything. A module boundary ends that: `./...` stops
// at it, and the examples are now verified only because three things stay
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
// The README's claim that every sample in it was compiled and run rests on
// the same three facts, which is the other reason to assert them here rather
// than trust the workflow file to keep saying what it says today.
var (
	goDirectiveLine = regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)\s*$`)
	localReplace    = regexp.MustCompile(`(?m)^replace\s+github\.com/TuSKan/astrogo\s+=>\s+\.\./\s*$`)
)

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
}
