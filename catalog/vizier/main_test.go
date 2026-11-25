package vizier

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/internal/remotetest"
)

// TestMain runs this package's untagged tests offline against an empty
// cache, so a test that needs the network or a cached file fails instead of
// downloading or reading what this machine holds; see remotetest.Run.
func TestMain(m *testing.M) { os.Exit(remotetest.Run(m.Run)) }
