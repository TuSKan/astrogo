package time

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	stdtime "time"

	"github.com/TuSKan/astrogo/logging"
)

// TestLeapSecondIsStillAliased pins the defect the warning exists to announce.
//
// This is deliberately an assertion that the wrong thing still happens. #144
// weighs three options; this change takes the cheap two (report it, document
// it) and leaves the expensive one — letting the UTC day run to 86401 seconds
// the way iauDtf2d does — undecided. Should someone take it, this test fails,
// and its failure is the signal to delete the warning rather than to restore
// the aliasing.
func TestLeapSecondIsStillAliased(t *testing.T) {
	leap := Date(2016, 12, 31, 23, 59, 60, 0, stdtime.UTC)
	next := Date(2017, 1, 1, 0, 0, 0, 0, stdtime.UTC)

	leap1, leap2 := leap.JDParts()
	next1, next2 := next.JDParts()

	if leap1 != next1 || leap2 != next2 {
		t.Fatalf("23:59:60 no longer aliases the following midnight: (%.1f, %.15f) vs (%.1f, %.15f)\n"+
			"  If that is because the leap second is now representable, #144 is fixed:\n"+
			"  delete the warning in leapsecond_alias.go along with this test.",
			leap1, leap2, next1, next2)
	}
}

// TestDateReportsTheAliasing is the wiring: everything else here exercises
// logLeapSecondAliased directly, so without this a Date that never called it
// would still pass the whole file.
//
// The sync.Once is reset rather than worked around, because the property under
// test is "the first Date to alias a leap second reports it" and any test
// running earlier would otherwise have spent it. Not parallel, for that reason
// and because it swaps the process-wide logger.
func TestDateReportsTheAliasing(t *testing.T) {
	defer logging.Set(nil)

	warnLeapSecondAliasedOnce = sync.Once{}
	defer func() { warnLeapSecondAliasedOnce = sync.Once{} }()

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	Date(2016, 12, 31, 23, 59, 60, 0, stdtime.UTC)

	if !strings.Contains(buf.String(), "leap second not representable") {
		t.Errorf("Date aliased a leap second without reporting it; logger saw:\n%q", buf.String())
	}

	// An ordinary second must stay quiet, or the warning becomes noise a
	// caller learns to filter out.
	buf.Reset()

	warnLeapSecondAliasedOnce = sync.Once{}

	Date(2016, 12, 31, 23, 59, 59, 0, stdtime.UTC)

	if buf.String() != "" {
		t.Errorf("Date warned about an ordinary second:\n%q", buf.String())
	}
}

// TestLeapSecondAliasWarningIsAWarningNotProgress: Date has no error return,
// so this message is the only notice a caller gets that the instant they built
// is one second away from the instant they asked for. Demoted to Info it would
// vanish under the default logger and the loss would be silent again — which
// #144 names as the worst property of the current behaviour.
func TestLeapSecondAliasWarningIsAWarningNotProgress(t *testing.T) {
	// Not parallel: it swaps the process-wide logger.
	defer logging.Set(nil)

	var buf bytes.Buffer

	// Info-and-above, so a demotion to Info is caught by the level assertion
	// below rather than by an empty buffer.
	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Called directly rather than through warnLeapSecondAliased, whose
	// sync.Once would be spent by whichever test ran first.
	logLeapSecondAliased(2016, 12, 31, 23, 59, 60)

	out := buf.String()

	if out == "" {
		t.Fatal("the leap-second warning wrote nothing to the installed logger")
	}

	if !strings.Contains(out, "level=WARN") {
		t.Errorf("the leap-second message is not at WARN:\n%s\n"+
			"  The default logger drops everything below WARN, so demoting this "+
			"would make the aliasing silent again.", out)
	}

	for _, want := range []string{
		`msg="leap second not representable, instant moved to the following midnight"`,
		"utc=2016-12-31T23:59:60Z",
		"delta_at_applied=37",
		"delta_at_correct=36",
		"remedy=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the message does not carry %q:\n%s", want, out)
		}
	}
}

// TestSecondSixtyOnAnOrdinaryDayIsNotCalledALeapSecond: 2016-06-30 has no leap
// second at its end, so 23:59:60 there is not an instant this type cannot hold
// — it is an instant that never happened. Reporting the two the same way would
// tell a caller their timestamp is a known limitation of the library when in
// fact their data is wrong.
func TestSecondSixtyOnAnOrdinaryDayIsNotCalledALeapSecond(t *testing.T) {
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	logLeapSecondAliased(2016, 6, 30, 23, 59, 60)

	out := buf.String()

	if !strings.Contains(out, "second out of range") {
		t.Errorf("23:59:60 on a day with no leap second was reported as a leap second:\n%s", out)
	}

	if strings.Contains(out, "delta_at_applied") {
		t.Errorf("a non-existent second was given a ΔAT comparison, which implies it is real:\n%s", out)
	}
}

// TestLeapSecondEndsDay checks the classifier against the published record
// directly, since everything above depends on it telling the two cases apart.
func TestLeapSecondEndsDay(t *testing.T) {
	tests := []struct {
		y, m, d int
		want    bool
		why     string
	}{
		{2016, 12, 31, true, "the most recent leap second"},
		{2015, 6, 30, true, "a mid-year insertion"},
		{2012, 6, 30, true, "a mid-year insertion"},
		{2016, 6, 30, false, "June 2016 carried no leap second"},
		{2016, 12, 30, false, "the day before the insertion, not the insertion"},
		{2017, 1, 1, false, "the day after the step"},
		{2020, 12, 31, false, "a year-end with no insertion"},
		{1960, 12, 31, false, "before leap seconds existed at all"},
	}

	for _, tt := range tests {
		if got := leapSecondEndsDay(tt.y, tt.m, tt.d); got != tt.want {
			t.Errorf("leapSecondEndsDay(%d-%02d-%02d) = %v, want %v (%s)",
				tt.y, tt.m, tt.d, got, tt.want, tt.why)
		}
	}
}

// TestNextDayCarries covers the case leapSecondEndsDay actually needs: leap
// seconds are only ever inserted at the end of a month, so every real call
// crosses a month boundary and most cross a year boundary too.
func TestNextDayCarries(t *testing.T) {
	tests := []struct {
		y, m, d    int
		wy, wm, wd int
	}{
		{2016, 12, 31, 2017, 1, 1},
		{2015, 6, 30, 2015, 7, 1},
		{2016, 2, 29, 2016, 3, 1}, // leap year
		{1900, 2, 28, 1900, 3, 1}, // not a leap year in the Gregorian calendar
		{2000, 2, 29, 2000, 3, 1}, // but 2000 is
	}

	for _, tt := range tests {
		y, m, d, fd := nextDay(tt.y, tt.m, tt.d)
		if y != tt.wy || m != tt.wm || d != tt.wd || fd != 0.0 {
			t.Errorf("nextDay(%d-%02d-%02d) = %d-%02d-%02d+%v, want %d-%02d-%02d+0",
				tt.y, tt.m, tt.d, y, m, d, fd, tt.wy, tt.wm, tt.wd)
		}
	}
}

// TestAliasWarningReportsTheUTCDate: a caller in a non-UTC zone writes the
// 2016-12-31 leap second as a local time on a different date, and classifying
// on the components as given would call the real leap second non-existent.
func TestAliasWarningReportsTheUTCDate(t *testing.T) {
	// UTC+13: 2016-12-31 23:59:60 UTC is 2017-01-01 12:59:60 there.
	east := stdtime.FixedZone("UTC+13", 13*3600)

	y, m, d, hh, mm := utcComponents(2017, stdtime.January, 1, 12, 59, east)

	if y != 2016 || m != stdtime.December || d != 31 || hh != 23 || mm != 59 {
		t.Fatalf("utcComponents(2017-01-01 12:59 UTC+13) = %d-%02d-%02d %02d:%02d, want 2016-12-31 23:59",
			y, m, d, hh, mm)
	}

	if !leapSecondEndsDay(y, int(m), d) {
		t.Error("the 2016-12-31 leap second was not recognised through a non-UTC zone")
	}
}
