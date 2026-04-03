package tools

import "testing"

// TestDummy ensures the test runner registers this package natively,
// bypassing the covdata parsing bug in certain CI containers lacking full toolchains.
func TestDummy(t *testing.T) {
	// No-op
}
