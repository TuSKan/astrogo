package time

import (
	"bytes"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	stdtime "time"

	"github.com/TuSKan/astrogo/logging"
)

// TestLeapSecondIsItsOwnInstant is #144, and replaces the test that pinned
// the defect.
//
// That test asserted 23:59:60 still landed on the following midnight, and said
// that if it ever failed because the leap second had become representable, the
// warning and the test should be deleted rather than the aliasing restored.
// It did, and they were.
//
// 23:59:60 is now 86400/86401 of the way through 2016-12-31 — iauDtf2d's
// convention — which puts it strictly between 23:59:59 and the next midnight,
// one second from each.
func TestLeapSecondIsItsOwnInstant(t *testing.T) {
	before := Date(2016, 12, 31, 23, 59, 59, 0, stdtime.UTC)
	leap := Date(2016, 12, 31, 23, 59, 60, 0, stdtime.UTC)
	next := Date(2017, 1, 1, 0, 0, 0, 0, stdtime.UTC)

	leap1, leap2 := leap.JDParts()
	next1, next2 := next.JDParts()

	if leap1 == next1 && leap2 == next2 {
		t.Fatal("23:59:60 still aliases the following midnight")
	}

	// The value iauDtf2d gives: 0h on 2016-12-31 is JD 2457753.5, and the
	// fraction is of an 86401-second day.
	want := 2457753.5 + 86400.0/86401.0
	if got := leap.JD(); math.Abs(got-want)*86400 > 1e-6 {
		t.Errorf("JD(2016-12-31 23:59:60) = %.12f, want %.12f", got, want)
	}

	if !before.Before(leap) || !leap.Before(next) {
		t.Error("23:59:60 is not between 23:59:59 and the following midnight")
	}

	// One SI second either side, measured physically.
	for _, c := range []struct {
		name string
		a, b Time
	}{
		{"23:59:59 to 23:59:60", before, leap},
		{"23:59:60 to 00:00:00", leap, next},
	} {
		if got := c.b.Sub(c.a).Seconds(); math.Abs(got-1) > 1e-6 {
			t.Errorf("%s spans %.9f s, want 1", c.name, got)
		}
	}
}

// TestDateReportsASecondThatNeverExisted is the wiring for the warning that
// remains: a second of 60 on a day that gained no leap second names an instant
// UTC never had, and Date, with no error to return, says so.
//
// The real leap second must stay quiet now that it is represented — a warning
// on a correct answer is noise a caller learns to filter, and then misses the
// one that matters.
//
// The sync.Once is reset rather than worked around, because the property under
// test is "the first Date to name such a second reports it". Not parallel, for
// that reason and because it swaps the process-wide logger.
func TestDateReportsASecondThatNeverExisted(t *testing.T) {
	defer logging.Set(nil)

	warnSecondOutOfRangeOnce = sync.Once{}
	defer func() { warnSecondOutOfRangeOnce = sync.Once{} }()

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	Date(2016, 12, 31, 23, 59, 60, 0, stdtime.UTC)

	if buf.String() != "" {
		t.Errorf("Date warned about a real leap second, which it now represents:\n%q", buf.String())
	}

	Date(2016, 6, 30, 23, 59, 60, 0, stdtime.UTC)

	if !strings.Contains(buf.String(), "second out of range") {
		t.Errorf("Date normalised a second that never existed without reporting it; logger saw:\n%q",
			buf.String())
	}

	buf.Reset()

	warnSecondOutOfRangeOnce = sync.Once{}

	Date(2016, 12, 31, 23, 59, 59, 0, stdtime.UTC)

	if buf.String() != "" {
		t.Errorf("Date warned about an ordinary second:\n%q", buf.String())
	}
}

// TestSecondOutOfRangeIsAWarningNotProgress: Date has no error return, so this
// message is the only notice a caller gets that the instant they built is not
// the one they asked for. Demoted to Info it would vanish under the default
// logger and the loss would be silent.
func TestSecondOutOfRangeIsAWarningNotProgress(t *testing.T) {
	// Not parallel: it swaps the process-wide logger.
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Called directly rather than through warnSecondOutOfRange, whose sync.Once
	// would be spent by whichever test ran first.
	logSecondOutOfRange(2016, 6, 30, 23, 59, 60)

	out := buf.String()

	if !strings.Contains(out, "level=WARN") {
		t.Errorf("the message is not at WARN:\n%s\n"+
			"  The default logger drops everything below WARN.", out)
	}

	for _, want := range []string{
		`msg="second out of range, instant normalised into the following minute"`,
		"utc=2016-06-30T23:59:60Z",
		"reason=",
		"remedy=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the message does not carry %q:\n%s", want, out)
		}
	}
}

// TestDateBuildsTheLeapSecondThroughAnyZone: a leap second is inserted at the
// end of a UTC day, so in UTC+13 the 2016 one is 2017-01-01 12:59:60 local. The
// day is decided in UTC, and the instant built must be the same one.
func TestDateBuildsTheLeapSecondThroughAnyZone(t *testing.T) {
	t.Parallel()

	east := stdtime.FixedZone("UTC+13", 13*3600)

	local := Date(2017, stdtime.January, 1, 12, 59, 60, 500_000_000, east)
	utc := Date(2016, stdtime.December, 31, 23, 59, 60, 500_000_000, stdtime.UTC)

	if !local.Equal(utc) {
		t.Errorf("12:59:60.5 in UTC+13 = JD %.12f, 23:59:60.5 UTC = JD %.12f; the same instant",
			local.JD(), utc.JD())
	}
}

// TestSecondSixtyOneIsNotALeapSecond: the leap day's last minute has 61
// seconds, numbered 0 through 60. A 61st second is not a longer leap second but
// an instant that never existed, and gets the warning's treatment rather than a
// representation.
func TestSecondSixtyOneIsNotALeapSecond(t *testing.T) {
	t.Parallel()

	if _, ok := dateInLeapSecond(2016, stdtime.December, 31, 23, 59, 61, 0, stdtime.UTC); ok {
		t.Error("23:59:61 was accepted as a leap second")
	}

	if _, ok := dateInLeapSecond(2016, stdtime.December, 31, 23, 59, 60, 999_999_999, stdtime.UTC); !ok {
		t.Error("23:59:60.999999999 was refused, but it is inside the inserted second")
	}

	if _, ok := dateInLeapSecond(2016, stdtime.June, 30, 23, 59, 60, 0, stdtime.UTC); ok {
		t.Error("23:59:60 on 2016-06-30, which gained no leap second, was accepted")
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
