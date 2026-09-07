package docsguard_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// workflows are the CI definitions that pin a Go toolchain, relative to the
// repository root.
var workflows = []string{
	filepath.Join(".github", "workflows", "ci.yml"),
	filepath.Join(".github", "workflows", "pre-release.yml"),
}

var (
	// goDirective matches go.mod's own version line: "go 1.25.8".
	goDirective = regexp.MustCompile(`(?m)^go (\d+)\.(\d+)(?:\.\d+)?\s*$`)

	// workflowGoVersion matches a setup-go pin: "go-version: '1.25'".
	workflowGoVersion = regexp.MustCompile(`(?m)^\s*go-version:\s*'?"?(\d+)\.(\d+)`)
)

// TestWorkflowGoVersionTracksTheModuleDirective keeps the six hand-written
// toolchain pins in the workflows on the same minor version as go.mod.
//
// # Why these are hand-written at all
//
// The obvious spelling is `go-version-file: go.mod`, and this repository used
// it until it caused an outage. go.mod declares a *patch-level* minimum —
// `go 1.25.8`, inherited from gocloud.dev via gocloud-ext and not something
// astrogo asks for — so setup-go demanded that exact patch, could not use the
// runner's preinstalled toolchain, and downloaded one before every job on
// every run. On PR #151 that download hung for 25 minutes without a single
// test running. A minor-version spec resolves to whatever 1.25.x the runner
// already has. See #109 for why the patch level cannot simply be lowered: the
// API gocloud-ext's drivers implement only exists after gocloud.dev's newest
// tag, so the pseudo-version that carries the directive is load-bearing.
//
// # What this guards
//
// Decoupling the two is safe in the direction that matters — Go refuses to
// build below go.mod's directive and fetches the toolchain it needs, so a
// mismatch degrades to a download rather than to a wrong result. What it is
// not is *visible*: the day go.mod moves to 1.26.x, all six pins silently
// reintroduce the exact download #109 exists to remove, and CI stays green
// while doing it. Nothing about a slow job says which line to change.
//
// So this fails instead, and names the lines.
func TestWorkflowGoVersionTracksTheModuleDirective(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	modBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	m := goDirective.FindStringSubmatch(string(modBytes))
	if m == nil {
		t.Fatal("go.mod has no `go` directive this guard can read; if its spelling changed, so must goDirective")
	}

	wantMajor, wantMinor := m[1], m[2]
	want := wantMajor + "." + wantMinor

	found := 0

	for _, wf := range workflows {
		path := filepath.Join(root, wf)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", wf, err)

			continue
		}

		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			pin := workflowGoVersion.FindStringSubmatch(line)
			if pin == nil {
				continue
			}

			found++

			if got := pin[1] + "." + pin[2]; got != want {
				t.Errorf("%s:%d pins Go %s, but go.mod declares %s.\n"+
					"  These are deliberately separate (see this test's doc comment and #109), but they "+
					"must stay on the same minor version: a stale pin makes every CI job download a "+
					"toolchain again, silently, which is the outage that separated them in the first place.",
					wf, i+1, got, want)
			}
		}
	}

	// A pin that stops matching is worse than a mismatched one: the guard
	// keeps passing while guarding nothing. Seven is the count today — it went
	// from six when the Validation workflow split its network tier into its own
	// job (#123) — and the eighth should arrive with this number updated.
	const wantPins = 7

	if found != wantPins {
		t.Errorf("found %d Go version pins across %v, want %d.\n"+
			"  Either a workflow step was added or removed — update this count — or the "+
			"pins are no longer written in a form workflowGoVersion recognises, in which "+
			"case this test has been passing without checking anything.", found, workflows, wantPins)
	}
}
