package docsguard_test

import (
	"os/exec"
	"strings"
	"testing"
)

// eopImport is the one edge from epoch to remote: epoch loads Earth
// orientation through remote (#1264).
var eopImport = [2]string{"github.com/TuSKan/astrogo/epoch/internal/iers", "github.com/TuSKan/astrogo/remote"}

// TestEphemerisDoesNotLinkTheStorageClient keeps the split #112 made.
//
// # What it is guarding
//
// Asking SOFA where Mars is used to cost 13.9 MB and 424 packages, of which
// about 12 MB and 131 were an object-storage client the answer never touches:
// ephemeris imported ephemeris/jpl, which imports remote, which imports
// gocloud.dev/blob, which pulls 64 packages of gRPC for an error-code enum and
// 34 of OpenTelemetry because it instruments unconditionally.
//
// The kernel half now registers itself, so a build says whether it wants that.
// The measurement afterwards was 4.7 MB and 224 packages with all four groups
// at zero.
//
// # Why a dependency check rather than a size check
//
// A binary's size moves with the toolchain, the platform and the linker's mood;
// its import graph does not. Byte thresholds either drift into meaninglessness
// or fail on somebody's machine for a reason nobody can act on. "Does this
// package reach remote" has one answer, the same everywhere, and it is the
// thing that actually decides the size.
//
// # What it forbids, and why that changed
//
// It used to name gocloud.dev, gRPC, OpenTelemetry and protobuf: the four
// groups that arrived through the storage layer. gocloud.dev is gone from the
// module, so all four are now permanently absent from every package's graph and
// a check for them could never fail again. A test that cannot fail is worse
// than no test, because it reads as cover.
//
// What survives is the structural rule: ephemeris must not reach
// astrogo/remote, but by one edge. epoch loads Earth orientation through
// remote (#1264), so every program converting a time scale links it, at a
// measured 2.98 MB and 96 packages without it against 6.08 MB and 217 with it,
// nearly all of the difference Go's own HTTP stack. That edge is a decision.
// Any other path from ephemeris to remote is the import #112 removed, and
// stays a defect whatever remote is built on next. The former four are kept
// alongside it as named regressions, so a future dependency that reintroduces
// them is caught by name rather than only by the import that brought them.
//
// A regression here is one accidental import away and would be invisible:
// everything keeps working, the binary is simply megabytes bigger.
func TestEphemerisDoesNotLinkTheStorageClient(t *testing.T) {
	const root = "github.com/TuSKan/astrogo/ephemeris"

	// The whole transitive graph of the root package, as the toolchain sees it.
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps",
		"-f", "{{.ImportPath}}\t{{join .Imports \",\"}}", root).Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}

	imports := map[string][]string{}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		path, list, _ := strings.Cut(strings.TrimSpace(line), "\t")
		if list != "" {
			imports[path] = strings.Split(list, ",")
		} else {
			imports[path] = nil
		}
	}

	if len(imports) < 50 {
		t.Fatalf("go list reported only %d packages; the command is not doing what this "+
			"test assumes", len(imports))
	}

	// Everything ephemeris reaches without following eopImport.
	reached := map[string]bool{root: true}
	queue := []string{root}

	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]

		for _, imp := range imports[next] {
			if reached[imp] || [2]string{next, imp} == eopImport {
				continue
			}

			reached[imp] = true
			queue = append(queue, imp)
		}
	}

	// The first entry is the rule; the rest are the heavyweights that used to
	// arrive through it, kept so a reintroduction is named rather than merely
	// implied. None has anything to do with computing an ephemeris.
	forbidden := map[string]string{
		"github.com/TuSKan/astrogo/remote": "astrogo's I/O boundary; the kernel half registers itself instead",
		"gocloud.dev":                      "the object-storage client this module no longer uses at all",
		"google.golang.org/grpc":           "gRPC, which gocloud.dev/gcerrors pulled for an error-code enum",
		"go.opentelemetry.io":              "OpenTelemetry, which gocloud.dev/blob instrumented unconditionally",
		"google.golang.org/protobuf":       "protobuf, via gRPC",
	}

	counts := map[string]int{}

	for d := range reached {
		for prefix := range forbidden {
			if strings.HasPrefix(d, prefix) {
				counts[prefix]++
			}
		}
	}

	for prefix, why := range forbidden {
		if counts[prefix] > 0 {
			t.Errorf("ephemeris reaches %d packages under %s other than through epoch's "+
				"Earth-orientation loader — %s.\n"+
				"  Something in the ephemeris tree imported remote again. The kernel-backed\n"+
				"  sources reach it through the backend ephemeris/jpl registers, so nothing\n"+
				"  in the root package should name that package. See #112.",
				counts[prefix], prefix, why)
		}
	}

	t.Logf("%d packages reached without epoch's loader, checked against %d forbidden prefixes",
		len(reached), len(forbidden))
}
