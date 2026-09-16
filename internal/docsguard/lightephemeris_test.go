package docsguard_test

import (
	"os/exec"
	"strings"
	"testing"
)

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
// What survives the library change is the structural rule: ephemeris must not
// reach astrogo/remote at all. That is what #112 was about — the heavy packages
// were the symptom, the import was the cause — and it stays true whatever
// remote is built on next. The former four are kept alongside it as named
// regressions, so a future dependency that reintroduces them is caught by name
// rather than only by the import that brought them.
//
// A regression here is one accidental import away and would be invisible:
// everything keeps working, the binary is simply megabytes bigger.
func TestEphemerisDoesNotLinkTheStorageClient(t *testing.T) {
	// The whole transitive graph of the root package, as the toolchain sees it.
	out, err := exec.CommandContext(t.Context(),
		"go", "list", "-deps", "github.com/TuSKan/astrogo/ephemeris").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}

	deps := strings.Split(string(out), "\n")

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

	for _, d := range deps {
		for prefix := range forbidden {
			if strings.HasPrefix(strings.TrimSpace(d), prefix) {
				counts[prefix]++
			}
		}
	}

	for prefix, why := range forbidden {
		if counts[prefix] > 0 {
			t.Errorf("ephemeris reaches %d packages under %s — %s.\n"+
				"  Something in the ephemeris tree imported remote again. The kernel-backed\n"+
				"  sources reach it through the backend ephemeris/jpl registers, so nothing\n"+
				"  in the root package should name that package. See #112.",
				counts[prefix], prefix, why)
		}
	}

	if len(deps) < 50 {
		t.Fatalf("go list reported only %d dependencies; the command is not doing what this "+
			"test assumes", len(deps))
	}

	t.Logf("%d transitive packages, none under %d forbidden prefixes", len(deps)-1, len(forbidden))
}
