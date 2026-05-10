package testutil

import (
	"reflect"
	"testing"
)

// ── Generic Assertions ───────────────────────────────────────────────────────

// AssertEqual fails t if got != want using deep equality.
// label is included in the failure message.
func AssertEqual[T any](tb testing.TB, label string, got, want T) {
	tb.Helper()

	if !reflect.DeepEqual(got, want) {
		tb.Errorf("%s: got %v, want %v", label, got, want)
	}
}
