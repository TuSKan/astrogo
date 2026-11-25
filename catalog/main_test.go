package catalog_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/internal/leakcheck"
	"github.com/TuSKan/astrogo/internal/remotetest"
)

// catalog fans out across providers (askAll) and across cone-search anchors,
// and both paths join their work before returning. This is what keeps that
// true — and it catches a test that forgets to close an api.Client, whose
// transport goroutines would otherwise outlive the suite.
//
// Under remotetest.Run, as every package whose untagged tests link remote: a
// test that needs the network or a cached file fails instead of downloading
// or reading what this machine holds.
func TestMain(m *testing.M) {
	os.Exit(remotetest.Run(func() int { return leakcheck.RunWithLeakCheck(m) }))
}
