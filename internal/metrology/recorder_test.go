package metrology_test

import (
	"fmt"
	"strings"
)

// recorder is a [metrology.TB] that remembers what was done to it instead of
// failing a real test.
//
// It exists because the interesting behaviour of this package is what it does
// on failure: which contract violations it reports, how it describes them,
// that a non-finite sample is an error rather than a silent drop, and that a
// suite which could not run is skipped rather than passed. None of that can
// be exercised through a real *testing.T without writing tests that are
// supposed to fail.
type recorder struct {
	logs    []string
	errors  []string
	skipped bool
	skip    string
	helpers int
}

func (r *recorder) Helper() { r.helpers++ }

func (r *recorder) Log(args ...any) { r.logs = append(r.logs, fmt.Sprint(args...)) }

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Skipf(format string, args ...any) {
	r.skipped = true
	r.skip = fmt.Sprintf(format, args...)
}

// output is every log and error concatenated, for substring assertions about
// what a reader of the test output would actually see.
func (r *recorder) output() string {
	return strings.Join(append(append([]string{}, r.logs...), r.errors...), "\n")
}
