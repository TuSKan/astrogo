// Package remotetest holds test helpers that need package remote, which
// internal/testutil cannot import: remote's own tests import testutil.
package remotetest

import (
	"errors"
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// ErrRefused is the reason the policy [RefuseDownloads] installs gives for
// every download it refuses.
var ErrRefused = errors.New("remotetest: download refused by the test's policy")

// RefuseDownloads installs a download policy on [remote.Default] that refuses
// every download from the endpoints in ids, or from every endpoint when ids is
// empty, and puts back the configuration it found when tb ends.
//
// Downloads are allowed by default, so a test of the refusal path asks for it.
func RefuseDownloads(tb testing.TB, ids ...remote.EndpointID) {
	tb.Helper()

	tb.Cleanup(remote.Capture().Restore)

	remote.SetPolicy(func(ep remote.Endpoint, _ int64) error {
		if len(ids) == 0 || slices.Contains(ids, ep.ID) {
			return ErrRefused
		}

		return nil
	})
}
