package jpl_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestMain grants download consent for the whole package's default test
// suite. These tests construct real jpl.Provider values against the
// planetary de440s kernel and, for two small-body tests, live Horizons SPK
// generation — a network/cache dependency that predates remote's
// consent-gating (kernels landing in the shared user cache dir made it
// invisible before). Rather than silently break every test in this file,
// grant the same access explicitly here; TODO: replace with committed
// offline SPK fixtures and move the network-dependent cases to
// //go:build network per CLAUDE.md's test-tag convention.
func TestMain(m *testing.M) {
	remote.EnableDownloads(remote.NAIFSPK, 0)
	remote.EnableDownloads(remote.NAIFLSK, 0)
	remote.EnableDownloads(remote.JPLHorizons, 0)

	os.Exit(m.Run())
}
