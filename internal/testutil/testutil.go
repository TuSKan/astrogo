package testutil

import (
	"reflect"
	"testing"
)

// ── Generic Assertions ───────────────────────────────────────────────────────

// AssertEqual fails t if got != want using deep equality.
// label is included in the failure message.
func AssertEqual[T any](t testing.TB, label string, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s: got %v, want %v", label, got, want)
	}
}
