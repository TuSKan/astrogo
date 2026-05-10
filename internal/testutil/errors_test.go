package testutil_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// ── AssertNoError ─────────────────────────────────────────────────────────────

func TestAssertNoError_nil(t *testing.T) {
	testutil.AssertNoError(t, nil) // must not fail
}

func TestAssertNoError_nonNil(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertNoError(spy, errors.New("oops"))

	if !spy.failed {
		t.Errorf("AssertNoError did not fail for non-nil error")
	}
}

// ── AssertError ───────────────────────────────────────────────────────────────

func TestAssertError_nonNil(t *testing.T) {
	testutil.AssertError(t, errors.New("expected")) // must not fail
}

func TestAssertError_nil(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertError(spy, nil)

	if !spy.failed {
		t.Errorf("AssertError did not fail for nil error")
	}
}

// ── AssertErrorContains ───────────────────────────────────────────────────────

func TestAssertErrorContains_match(t *testing.T) {
	testutil.AssertErrorContains(t, errors.New("connection refused"), "refused")
}

func TestAssertErrorContains_substring_not_found(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertErrorContains(spy, errors.New("timeout"), "refused")

	if !spy.failed {
		t.Errorf("AssertErrorContains did not fail when substring absent")
	}
}

func TestAssertErrorContains_nil_error(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertErrorContains(spy, nil, "anything")

	if !spy.failed {
		t.Errorf("AssertErrorContains did not fail for nil error")
	}
}

// ── AssertErrorIs ─────────────────────────────────────────────────────────────

var errSentinel = errors.New("sentinel")

func TestAssertErrorIs_match(t *testing.T) {
	wrapped := errors.Join(errors.New("context: "), errSentinel)
	testutil.AssertErrorIs(t, wrapped, errSentinel)
}

func TestAssertErrorIs_no_match(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertErrorIs(spy, errors.New("unrelated"), errSentinel)

	if !spy.failed {
		t.Errorf("AssertErrorIs did not fail for non-matching error")
	}
}

func TestAssertErrorIs_nil_error(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertErrorIs(spy, nil, errSentinel)

	if !spy.failed {
		t.Errorf("AssertErrorIs did not fail for nil error")
	}
}

// ── CaseLabel ─────────────────────────────────────────────────────────────────

func TestCaseLabel(t *testing.T) {
	if got := testutil.CaseLabel(0, ""); got != "case[0]" {
		t.Errorf("CaseLabel(0, \"\") = %q, want \"case[0]\"", got)
	}

	if got := testutil.CaseLabel(3, "wraparound"); got != "case[3] (wraparound)" {
		t.Errorf("CaseLabel(3, \"wraparound\") = %q", got)
	}
}

// ── FailCase ──────────────────────────────────────────────────────────────────

func TestFailCase_records_failure(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.FailCase(spy, 2, "my case", "got %d, want %d", 1, 2)

	if !spy.failed {
		t.Errorf("FailCase did not mark test as failed")
	}
}

// ── RunCases ──────────────────────────────────────────────────────────────────

type mathCase struct {
	name string
	a, b float64
	want float64
}

func (c mathCase) Name() string { return c.name }
func (c mathCase) Run(t *testing.T) {
	got := c.a + c.b
	testutil.AssertNear(t, "sum", got, c.want, 1e-14)
}

func TestRunCases(t *testing.T) {
	testutil.RunCases(t, []mathCase{
		{"zero", 0, 0, 0},
		{"positive", 1, 2, 3},
		{"negative", -1, -2, -3},
		{"mixed", 1, -1, 0},
	})
}
