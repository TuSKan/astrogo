//go:build network

package examples_test

import (
	"context"
	"net"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// TestNetworkExamplesRun runs every example the README says reaches the
// network and requires each of its programs to exit 0. One whose endpoints
// cannot be reached is skipped, as the network tests skip; one that reaches
// them and still fails, fails. Examples 02, 03, 05, 12 and 20 were each found
// broken at the release commit only by running them by hand (#1188).
//
// The programs share one cache, so a kernel two of them use is fetched once.
// ASTROGO_CACHE_DIR names it when set, as the nightly workflow sets it to a
// directory it keeps between runs; otherwise it is a fresh one.
func TestNetworkExamplesRun(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("go not on PATH: %v", err)
	}

	cache := os.Getenv("ASTROGO_CACHE_DIR")
	if cache == "" {
		cache = cacheURL(t.TempDir())
	}

	names := make([]string, 0, len(networkExamples))
	for name := range networkExamples {
		names = append(names, name)
	}

	slices.Sort(names)

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			for _, id := range networkExamples[name] {
				testutil.RequireReachable(t, endpointHost(t, id))
			}

			for _, pkg := range examplePrograms(t, name) {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)

				cmd := exec.CommandContext(ctx, goBin, "run", pkg)

				cmd.Env = append(os.Environ(), "ASTROGO_CACHE_DIR="+cache, "NO_COLOR=1")

				out, err := cmd.CombinedOutput()

				cancel()

				if err != nil {
					t.Errorf("%s failed: %v\n%s", pkg, err, out)

					continue
				}

				t.Logf("%s:\n%s", pkg, out)
			}
		})
	}
}

// endpointHost returns the host and port the endpoint id is served from, as
// testutil.RequireReachable takes them.
func endpointHost(t *testing.T, id remote.EndpointID) string {
	t.Helper()

	ep, ok := remote.Lookup(id)
	if !ok {
		t.Fatalf("endpoint %q is not registered", id)
	}

	u, err := url.Parse(ep.URL)
	if err != nil || u.Hostname() == "" {
		t.Fatalf("endpoint %q has no host in %q: %v", id, ep.URL, err)
	}

	port := u.Port()
	if port == "" {
		port = map[string]string{"http": "80", "https": "443"}[u.Scheme]
	}

	return net.JoinHostPort(u.Hostname(), port)
}
