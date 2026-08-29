package metrology_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

// ambientOutDir is the output directory the operator asked for, captured
// before TestMain takes it away.
//
// Package-level initialisation runs before TestMain, so this sees the real
// value; TestRenderAccuracyReport reads it rather than the environment, which
// by then is deliberately empty.
var ambientOutDir = os.Getenv(metrology.OutDirEnv)

// TestMain isolates this package's own tests from that directory.
//
// The suites here are fixtures — "test.suite", "x", "ephemeris.example" —
// and [metrology.Suite.Report] writes a result document whenever
// [metrology.OutDirEnv] names a directory. Anyone running the real suites
// with that variable set, which is the documented way to collect them, was
// therefore also collecting four fixtures, and they appeared in the generated
// accuracy table beside the genuine rows.
//
// Unset here rather than in each test, because the failure mode is a test
// that forgets to: it passes, writes nothing anyone notices, and quietly
// contributes a row to a published document. Tests that need the variable set
// use t.Setenv, which restores the unset state afterwards.
func TestMain(m *testing.M) {
	if err := os.Unsetenv(metrology.OutDirEnv); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}
