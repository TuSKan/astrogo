package docsguard_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// untaggedNetworkSuites are the packages whose untagged tests link remote and
// still reach the network, each with the reason.
var untaggedNetworkSuites = map[string]string{
	"remote": "the package under test: its tests cannot import a helper that imports it, " +
		"and turn offline mode on themselves where a test needs it",
	"ephemeris/jpl": "reads DE440s, the kernel NAIF publishes, downloaded untagged by design; " +
		"ephemeris/jpl/main_test.go says why",
	"ephemeris/jpl/lsk": "reads naif0012.tls, the leap-second kernel NAIF publishes, as ephemeris/jpl reads DE440s",
	"ephemeris/jpl/spk": "reads DE440s, as ephemeris/jpl does",
}

// TestUntaggedTestsRunOffline holds every package whose untagged tests link
// remote to a TestMain calling remotetest.Run.
//
// # Why
//
// The untagged suite is the one CLAUDE.md calls fast, deterministic and
// offline, and nothing made it so: downloads are allowed by default, so a test
// that needed a file fetched it, and one that found a file in this machine's
// cache used it. The first is slow and fails with the upstream. The second is
// worse, because it passes: skybrightness's golden tables passed on a machine
// holding the IERS bulletin and failed on every CI runner, which had none.
//
// remotetest.Run turns offline mode on and the cache to an empty directory for
// the untagged build, so a test that needs either fails where it is written,
// on every machine. Offline mode refuses only endpoints that reach the
// network, so a test serving its own fixture through remote.SetURL is
// unaffected.
//
// # What counts
//
// A package whose untagged test binary has remote among its dependencies,
// found with one go list -test, since only such a binary can reach the
// network through astrogo. Its TestMain must be in an untagged file, or the
// untagged build does not compile it. untaggedNetworkSuites lists the
// exceptions, and an exception that no longer links remote, or that has
// started calling remotetest.Run, is reported too, so the list cannot outlive
// its reasons.
func TestUntaggedTestsRunOffline(t *testing.T) {
	t.Parallel()

	root := repoRoot(t)

	// Comma-joined, since a dependency recompiled for a test is listed with a
	// space before its bracketed variant.
	cmd := exec.CommandContext(t.Context(), "go", "list", "-test",
		"-f", "{{.ImportPath}}|{{.Dir}}|{{join .Deps \",\"}}", "./...")
	cmd.Dir = root

	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -test: %v", err)
	}

	dirs := map[string]string{} // package → its directory
	linking := map[string]bool{}

	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}

		ip, dir, deps := parts[0], parts[1], parts[2]

		if !strings.Contains(ip, " ") && !strings.HasSuffix(ip, ".test") {
			dirs[ip] = dir

			continue
		}

		pkg, ok := strings.CutSuffix(ip, ".test")
		if !ok {
			continue
		}

		for dep := range strings.SplitSeq(deps, ",") {
			if name, _, _ := strings.Cut(dep, " "); name == remotePkg {
				linking[pkg] = true

				break
			}
		}
	}

	if len(linking) < 10 {
		t.Fatalf("only %d test binaries link remote; go list is not describing the module", len(linking))
	}

	for _, pkg := range slices.Sorted(maps.Keys(linking)) {
		rel := strings.TrimPrefix(strings.TrimPrefix(pkg, "github.com/TuSKan/astrogo"), "/")

		offline := runsOffline(t, dirs[pkg])

		if reason, exempt := untaggedNetworkSuites[rel]; exempt {
			if offline {
				t.Errorf("%s calls remotetest.Run but is listed in untaggedNetworkSuites (%s); "+
					"drop it from the list", rel, reason)
			}

			continue
		}

		if !offline {
			t.Errorf("%s: its untagged tests link remote and no untagged TestMain calls remotetest.Run.\n"+
				"  Add main_test.go with func TestMain(m *testing.M) { os.Exit(remotetest.Run(m.Run)) }, "+
				"so a test needing the network or a cached file fails here as it does on CI. "+
				"A suite that must reach the network belongs behind the network tag.", rel)
		}
	}

	for _, rel := range slices.Sorted(maps.Keys(untaggedNetworkSuites)) {
		if !linking["github.com/TuSKan/astrogo/"+rel] {
			t.Errorf("untaggedNetworkSuites lists %s, whose untagged tests no longer link remote; "+
				"drop it from the list", rel)
		}
	}

	t.Logf("%d untagged test binaries link remote, %d of them listed as network suites",
		len(linking), len(untaggedNetworkSuites))
}

// runsOffline reports whether an untagged test file in dir declares a
// TestMain that calls remotetest.Run.
func runsOffline(t *testing.T, dir string) bool {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}

		src := string(data)
		if strings.HasPrefix(src, "//go:build") {
			continue
		}

		if body, ok := testMainBody(src); ok && strings.Contains(body, "remotetest.Run(") {
			return true
		}
	}

	return false
}
