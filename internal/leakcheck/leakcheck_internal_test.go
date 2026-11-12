package leakcheck

import (
	"errors"
	"strings"
	"testing"
)

// TestLeakCheckConsultsTheProbe covers the mutation that matters most: a guard
// that never asks whether anything leaked.
//
// It is invisible from outside — the package passes, which is exactly what it
// would do if nothing had leaked — so nothing but this would notice.
func TestLeakCheckConsultsTheProbe(t *testing.T) {
	t.Parallel()

	probeErr := errors.New("leakcheck_test: no profile") //nolint:err113 // a stand-in for ErrProfileUnavailable

	for _, tc := range []struct {
		name       string
		suiteCode  int
		says       string
		saysErr    error
		wantCode   int
		wantProbed bool
	}{
		{"a clean suite with a leak fails", 0, "goroutineleak profile: total 1", nil, 1, true},
		{"a clean suite with no leak passes", 0, "", nil, 0, true},
		{"a failing suite is not masked", 1, "goroutineleak profile: total 1", nil, 1, false},
		{"an unavailable profile fails rather than passing", 0, "", probeErr, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			probed := false
			probe := func() (string, error) {
				probed = true

				return tc.says, tc.saysErr
			}

			if got := leakCheck(tc.suiteCode, probe); got != tc.wantCode {
				t.Errorf("leakCheck = %d, want %d", got, tc.wantCode)
			}

			if probed != tc.wantProbed {
				t.Errorf("probe consulted = %v, want %v — a guard that does not ask "+
					"cannot answer", probed, tc.wantProbed)
			}
		})
	}
}

// TestLeakCountReadsTheRuntimesOwnTotal pins the parse.
//
// The count comes off the profile's header rather than from counting stack
// lines, because one leaked goroutine contributes several frames — counting
// them would report a single leak as four and, worse, would report a
// *formatting* change as a leak.
func TestLeakCountReadsTheRuntimesOwnTotal(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		profile string
		want    int
		wantOK  bool
	}{
		{"none", "goroutineleak profile: total 0\n", 0, true},
		{"one", "goroutineleak profile: total 1\n1 @ 0x1 0x2\n#\t0x1\tpkg.fn+0x18\tfile.go:14\n", 1, true},
		{"several", "goroutineleak profile: total 12\n", 12, true},
		// None of these says how many leaked, so none may read as zero (#1100).
		{"empty input", "", 0, false},
		{"a header this code does not recognize", "something else entirely\n", 0, false},
		{"a total that is not a number", "goroutineleak profile: total many\n", 0, false},
		{"a total with words after it", "goroutineleak profile: total 3 goroutines\n", 0, false},
		{"a negative total", "goroutineleak profile: total -1\n", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got, ok := leakCount(tc.profile); got != tc.want || ok != tc.wantOK {
				t.Errorf("leakCount = %d, %v; want %d, %v", got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestLeakedReportIsEmptyOnlyWhenNothingLeaked pins the answer a clean run
// gives.
//
// The runtime writes a header whether or not anything leaked, so "the profile
// is non-empty" is not the same question as "something leaked" — reading it
// that way would fail every guarded package on every run. Leaked cannot show
// this by itself, because this process has deliberately leaked goroutines by
// the time anything can ask it twice.
func TestLeakedReportIsEmptyOnlyWhenNothingLeaked(t *testing.T) {
	t.Parallel()

	const leaked = "goroutineleak profile: total 1\n1 @ 0x1\n#\t0x1\tpkg.fn+0x18\tfile.go:14\n"

	for _, tc := range []struct {
		name    string
		profile string
		want    string
	}{
		{"a clean run still has a header", "goroutineleak profile: total 0\n", ""},
		{"a leak is reported verbatim", leaked, leaked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := leakedReport(tc.profile)
			if err != nil || got != tc.want {
				t.Errorf("leakedReport(%q) = %q, %v; want %q, nil", tc.profile, got, err, tc.want)
			}
		})
	}
}

// TestLeakedReportRefusesAHeaderItCannotRead covers the guard switching itself
// off without a sound. A profile whose header does not say how many goroutines
// leaked is "nothing looked", the same answer as no profile at all, and it
// must not read as "nothing leaked". Until #1100 it did. A change to the
// runtime's wording would have passed every guarded package from then on.
func TestLeakedReportRefusesAHeaderItCannotRead(t *testing.T) {
	t.Parallel()

	for _, profile := range []string{
		"",
		"something else entirely\n",
		"goroutineleak profile: 3 leaked\n1 @ 0x1\n",
		"goroutineleak profile: total many\n",
	} {
		got, err := leakedReport(profile)
		if !errors.Is(err, ErrProfileUnavailable) {
			t.Errorf("leakedReport(%q) = %q, %v; want ErrProfileUnavailable", profile, got, err)
		}
	}
}

// TestLeakedNamesTheProfileItNeeds covers the failure that must not look like
// success: no profile at all.
//
// "Nothing leaked" and "nothing looked" are different answers and only one is
// good news. A toolchain without the profile has to say so, and the sentinel
// is what lets a caller tell them apart.
func TestLeakedNamesTheProfileItNeeds(t *testing.T) {
	t.Parallel()

	if !strings.Contains(ErrProfileUnavailable.Error(), "goroutine-leak profile") {
		t.Errorf("ErrProfileUnavailable = %q, want it to name what is missing",
			ErrProfileUnavailable)
	}

	// The profile this package depends on must exist on the toolchain the
	// repository builds with — if it stops existing, that is a finding rather
	// than a quietly disabled guard.
	if _, err := Leaked(); errors.Is(err, ErrProfileUnavailable) {
		t.Fatalf("this toolchain has no readable %q profile, so the guard installed in "+
			"every package that starts goroutines is checking nothing: %v", profileName, err)
	}
}
