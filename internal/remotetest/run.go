package remotetest

import (
	"errors"
	"fmt"
	"os"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// Run runs a package's tests, run (m.Run, or a wrapper around it), and returns
// their exit code. In a build with none of the test tiers' tags it first turns
// offline mode on and points the cache at an empty directory, removed
// afterward, so the untagged tests see what a fresh machine with no network
// sees, on every machine: one that needs the network fails instead of
// downloading, and one that needs a cached file fails instead of reading
// whatever this machine happens to hold. A build tagged integration, network
// or validation runs the tests as they are, with the network and the shared
// cache.
//
//	func TestMain(m *testing.M) { os.Exit(remotetest.Run(m.Run)) }
//
// Offline mode refuses only endpoints that reach the network, so a test that
// serves its own fixture through remote.SetURL, from a file:// directory or an
// httptest server, still reaches it.
func Run(run func() int) int {
	if tiered {
		return run()
	}

	dir, err := os.MkdirTemp("", "astrogo-untagged-cache-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "remotetest: create the empty cache: %v\n", err)

		return 1
	}

	cache, err := testutil.DirURL(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "remotetest: %v\n", errors.Join(err, os.RemoveAll(dir)))

		return 1
	}

	remote.SetOffline(true)
	remote.SetDataDir(cache)

	code := run()

	if err := os.RemoveAll(dir); err != nil {
		fmt.Fprintf(os.Stderr, "remotetest: remove the cache %s: %v\n", dir, err)
	}

	return code
}
