//go:build network

package jpl_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// TestHorizonsFailuresAreClassified checks that every way these fetchers report
// Horizons failing reaches a skip, and that astrogo asking for the wrong thing
// does not.
//
// horizonsStatusError is matched by testutil through an interface nothing here
// names, so there is no compile-time link: spell the method HttpStatus and this
// package still builds, still passes offline, and fails on JPL's next
// maintenance window instead of skipping. The table is what notices.
func TestHorizonsFailuresAreClassified(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		wantSkip bool
	}{
		{"503, overloaded", horizonsStatusError(http.StatusServiceUnavailable), true},
		{"429, rate limited", horizonsStatusError(http.StatusTooManyRequests), true},
		{"the HTML error page under a 200", errHorizonsUnavailable, true},
		{"the page wrapped by a caller", fmt.Errorf("vectors: %w", errHorizonsUnavailable), true},

		// astrogo's own mistakes, which these tests exist to catch.
		{"404, a query for something not there", horizonsStatusError(http.StatusNotFound), false},
		{"400, a malformed query", horizonsStatusError(http.StatusBadRequest), false},
		{"an answer with no ephemeris in it", errNoEphemerisData, false},
	} {
		var skipped bool

		t.Run(tc.name, func(t *testing.T) {
			// Read in a defer because Skipf ends the subtest through
			// runtime.Goexit, so nothing after the call would run.
			defer func() { skipped = t.Skipped() }()

			skipIfHorizonsDown(t, tc.err)
		})

		if skipped != tc.wantSkip {
			t.Errorf("%s: skipped = %v, want %v", tc.name, skipped, tc.wantSkip)
		}
	}
}

// TestAStatusMeaningDowntimeIsTheOldSentinelToo pins the compatibility the
// Is method exists for: callers that test errors.Is(err, errHorizonsUnavailable)
// — loadCases, which records NOT VERIFIED, and the observer pipeline test —
// must see a 503 as downtime exactly as they did when it reached them as an
// HTML body, and must not see a 404 that way.
func TestAStatusMeaningDowntimeIsTheOldSentinelToo(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		code int
		want bool
	}{
		{http.StatusServiceUnavailable, true},
		{http.StatusBadGateway, true},
		{http.StatusTooManyRequests, true},
		{http.StatusRequestTimeout, true},
		{http.StatusForbidden, true},
		{http.StatusNotFound, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
	} {
		wrapped := fmt.Errorf("fetching: %w", horizonsStatusError(tc.code))

		if got := errors.Is(wrapped, errHorizonsUnavailable); got != tc.want {
			t.Errorf("http %d: errors.Is(errHorizonsUnavailable) = %v, want %v", tc.code, got, tc.want)
		}
	}
}
