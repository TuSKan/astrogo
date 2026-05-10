package testutil

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// ── Error assertion helpers ──────────────────────────────────────────────────

// AssertNoError fails t if err != nil.
// Use this when an operation must succeed.
func AssertNoError(t testing.TB, err error) {
	t.Helper()

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// AssertError fails t if err == nil.
// Use this when an operation must fail (any error is acceptable).
func AssertError(t testing.TB, err error) {
	t.Helper()

	if err == nil {
		t.Errorf("expected an error but got nil")
	}
}

// AssertErrorContains fails t if err is nil or if err.Error() does not
// contain substr.
func AssertErrorContains(t testing.TB, err error, substr string) {
	t.Helper()

	if err == nil {
		t.Errorf("expected error containing %q but got nil", substr)
		return
	}

	if !strings.Contains(err.Error(), substr) {
		t.Errorf("error %q does not contain %q", err.Error(), substr)
	}
}

// AssertErrorIs fails t if !errors.Is(err, target).
// Use this when the exact sentinel error matters.
func AssertErrorIs(t testing.TB, err, target error) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Errorf("got error %v; want errors.Is match for %v", err, target)
	}
}

// ── Table-driven test diagnostics ───────────────────────────────────────────
// These helpers make failure messages in table-driven tests unambiguous:
// every failure includes the row index and, when provided, the case name.

// CaseLabel returns a formatted label for a table test case.
// If name is non-empty the result is "case[i] (name)", otherwise "case[i]".
func CaseLabel(i int, name string) string {
	if name != "" {
		return fmt.Sprintf("case[%d] (%s)", i, name)
	}

	return fmt.Sprintf("case[%d]", i)
}

// FailCase marks t as failed with a diagnostic that identifies the table row.
// It uses t.Errorf so the test continues running remaining cases.
// i is the zero-based row index; name is optional.
func FailCase(t testing.TB, i int, name, format string, args ...any) {
	t.Helper()

	label := CaseLabel(i, name)
	msg := fmt.Sprintf(format, args...)
	t.Errorf("%s: %s", label, msg)
}

// RunCases iterates over a slice of test cases using t.Run subtests.
// Each case must provide a Name() string and a Run(*testing.T) method.
// This keeps the caller free from the boilerplate of range + t.Run.
//
//	type myCase struct { name string; ... }
//	func (c myCase) Name() string       { return c.name }
//	func (c myCase) Run(t *testing.T)   { ... }
//	testutil.RunCases(t, cases)
func RunCases[C interface {
	Name() string
	Run(*testing.T)
}](t *testing.T, cases []C) {
	t.Helper()

	for _, c := range cases {
		t.Run(c.Name(), c.Run)
	}
}
