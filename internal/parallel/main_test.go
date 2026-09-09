package parallel_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/internal/leakcheck"
)

// This package's whole purpose is starting goroutines, so it is the first
// place a leak would appear. See leakcheck.RunWithLeakCheck.
func TestMain(m *testing.M) { os.Exit(leakcheck.RunWithLeakCheck(m)) }
