//go:build network

package examples_test

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// TestNetworkExamplesRun runs every example the README says reaches the
// network and requires each of its programs to exit 0. Examples 02, 03, 05,
// 12 and 20 were each found broken at the release commit only by running
// them by hand (#1188).
//
// An outage is not a failure, as for the network tests. An example whose
// endpoint hosts cannot be reached is skipped. A program that fails while a
// service it uses rejects a control request that cannot be wrong, as SIMBAD
// answered every request with a 404 page through the CDS outage of
// 2026-11-13, is reported as not verified rather than failed; the example is
// skipped if nothing else in it failed.
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

			var unverified []string

			for _, pkg := range examplePrograms(t, name) {
				ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)

				cmd := exec.CommandContext(ctx, goBin, "run", pkg)

				cmd.Env = append(os.Environ(), "ASTROGO_CACHE_DIR="+cache, "NO_COLOR=1")

				out, err := cmd.CombinedOutput()

				cancel()

				if err == nil {
					t.Logf("%s:\n%s", pkg, out)

					continue
				}

				derr := degraded(t, name)
				if derr == nil {
					t.Errorf("%s failed: %v\n%s", pkg, err, out)

					continue
				}

				// A service it uses rejected a request that cannot be wrong, so
				// this failure is not evidence against the example.
				unverified = append(unverified, pkg)
				t.Logf("%s failed, not verified: %v (%v)\n%s", pkg, derr, err, out)
			}

			if len(unverified) > 0 && !t.Failed() {
				t.Skipf("not verified, a service they use is degraded: %v", unverified)
			}
		})
	}
}

// degraded returns why a service the example name uses is degraded: the
// error of a control request it rejected, for each endpoint that has one.
// It returns nil when every such service answers its control, so a failure is
// the example's.
func degraded(t *testing.T, name string) error {
	t.Helper()

	for _, id := range networkExamples[name] {
		control, ok := tapControls[id]
		if !ok {
			continue
		}

		syncURL, err := remote.URL(id)
		if err != nil {
			t.Fatalf("endpoint %q: %v", id, err)
		}

		// The mirrors remote asks while the endpoint's own host is not
		// serving, so the control asks the host that answered the example.
		ep, _ := remote.Lookup(id)

		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		err = testutil.TAPControl(ctx, syncURL, control.from, control.column, ep.Mirrors...)()

		cancel()

		if err != nil {
			return fmt.Errorf("%s rejected a control request: %w", id, err)
		}
	}

	return nil
}

// tapControls names, for each TAP service an example uses, a table and column
// it has served for decades, for testutil.TAPControl.
var tapControls = map[remote.EndpointID]struct{ from, column string }{
	remote.SIMBAD: {"basic", "main_id"},
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
