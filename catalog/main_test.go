package catalog_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/internal/leakcheck"
)

// catalog fans out across providers (askAll) and across cone-search anchors,
// and both paths join their work before returning. This is what keeps that
// true — and it catches a test that forgets to close an api.Client, whose
// transport goroutines would otherwise outlive the suite.
func TestMain(m *testing.M) { os.Exit(leakcheck.RunWithLeakCheck(m)) }
