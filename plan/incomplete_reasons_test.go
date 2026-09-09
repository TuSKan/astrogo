package plan_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
)

// Stand-ins for failures this test did not cause: a kernel that would not
// parse, and a query that failed outright rather than coming back short.
var (
	errCorruptKernel  = errors.New("moon kernel \"plu060\": spk: bad file record")
	errSiteUnresolved = errors.New("plan: site could not be resolved")
)

// flattenReasons returns the individual skip reasons under an ErrIncomplete.
//
// skips.err joins one error per dropped candidate, so errors.Is over the whole
// thing answers "was anything denied", not "was everything denied" — and only
// the second question separates a limit a caller set for itself from a real
// failure sitting alongside it. TestVisibleTonight_PlanetaryMoons caps
// downloads on purpose and needs exactly that distinction (#244).
//
// ErrIncomplete is dropped rather than returned as a reason: skips.err builds
// its result as fmt.Errorf("%w: %w", ErrIncomplete, joined), so the sentinel
// is a sibling of the reasons in the tree rather than one of them. Returning
// it would make every caller reject its own expected incompleteness.
//
// This lives in an untagged file so it is exercised by the ordinary test run
// rather than only under -tags=network, where the one caller is.
func flattenReasons(err error) []error {
	// A structural walk, so errors.As on the interface rather than on a
	// concrete type: nothing here looks for a particular error, only for the
	// shape that has children.
	var multi interface{ Unwrap() []error }
	if !errors.As(err, &multi) {
		if errors.Is(err, plan.ErrIncomplete) {
			return nil
		}

		return []error{err}
	}

	children := multi.Unwrap()
	out := make([]error, 0, len(children))

	for _, child := range children {
		out = append(out, flattenReasons(child)...)
	}

	return out
}

// incompleteLike rebuilds the error shape plan's unexported skips.err
// produces: the sentinel and the joined reasons as siblings.
//
// The coupling is deliberate and is this test's one weakness — if skips.err
// changes shape, this keeps passing while flattenReasons stops matching
// reality. It is called out here so the next reader checks both together.
func incompleteLike(reasons ...error) error {
	return fmt.Errorf("%w: %w", plan.ErrIncomplete, errors.Join(reasons...))
}

// TestFlattenReasonsSeparatesTheSentinelFromTheReasons is the test that makes
// the helper worth trusting.
//
// Its whole job is to answer "was *everything* skipped for the reason I
// caused", and the two ways it can be wrong are both silent: returning
// ErrIncomplete as a reason makes a caller reject its own expected
// incompleteness, and returning nothing makes the check vacuous — it would
// accept any failure at all.
func TestFlattenReasonsSeparatesTheSentinelFromTheReasons(t *testing.T) {
	t.Parallel()

	denied := fmt.Errorf("moon kernel %q: %w", "sat441", remote.ErrDownloadDenied)
	deniedToo := fmt.Errorf("moon kernel %q: %w", "jup365", remote.ErrDownloadDenied)
	corrupt := errCorruptKernel

	for _, tc := range []struct {
		name       string
		err        error
		wantCount  int
		allDenials bool
	}{
		{"one denial", incompleteLike(denied), 1, true},
		{"several denials", incompleteLike(denied, deniedToo), 2, true},
		{"a denial and a real failure", incompleteLike(denied, corrupt), 2, false},
		{"only a real failure", incompleteLike(corrupt), 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := flattenReasons(tc.err)
			if len(got) != tc.wantCount {
				t.Fatalf("flattenReasons returned %d reasons, want %d: %v",
					len(got), tc.wantCount, got)
			}

			all := true

			for _, r := range got {
				if errors.Is(r, plan.ErrIncomplete) {
					t.Errorf("ErrIncomplete came back as a reason; a caller checking its "+
						"own expected incompleteness would reject it: %v", r)
				}

				if !errors.Is(r, remote.ErrDownloadDenied) {
					all = false
				}
			}

			if all != tc.allDenials {
				t.Errorf("every reason a download denial = %v, want %v (%v)",
					all, tc.allDenials, got)
			}
		})
	}
}

// TestFlattenReasonsOnAPlainError covers the shape a caller gets when the
// query failed outright rather than coming back short.
func TestFlattenReasonsOnAPlainError(t *testing.T) {
	t.Parallel()

	plain := errSiteUnresolved

	got := flattenReasons(plain)
	if len(got) != 1 || !errors.Is(got[0], plain) {
		t.Errorf("flattenReasons(plain) = %v, want the error itself", got)
	}
}
