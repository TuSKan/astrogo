package time

import (
	"bytes"
	"log/slog"
	"math"
	"strings"
	"sync"
	"testing"
	stdtime "time"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/logging"
)

// builtinRecord reads gofa's own ΔAT table back out as registry entries.
//
// Derived rather than transcribed: RegisterLeapSeconds refuses a table that
// contradicts the built-in record, so a hand-copied one would have to be kept
// in step with a gofa upgrade by hand, and the failure would look like a
// registry bug rather than a stale fixture. The scan is the same one
// builtinLastStep uses.
func builtinRecord(t *testing.T) []LeapSecond {
	t.Helper()

	const (
		firstYear = 1972
		lastYear  = 2100
	)

	var (
		out      []LeapSecond
		previous float64
	)

	for y := firstYear; y <= lastYear; y++ {
		for _, m := range [2]int{1, 7} {
			got, status := gofaext.Dat(y, m, 1, 0)
			if status < 0 {
				continue
			}

			if len(out) == 0 || math.Abs(got-previous) > 1e-9 {
				out = append(out, LeapSecond{Year: y, Month: m, Day: 1, DeltaAT: got})
			}

			previous = got
		}
	}

	if len(out) == 0 {
		t.Fatal("gofa reported no ΔAT steps; the scan is wrong")
	}

	return out
}

// withNegativeLeapSecondAt2030 registers the published record extended by a
// hypothetical negative leap second at the end of 2029 — ΔAT dropping by one,
// which has never been announced and which Levine, Tavella & Milton (2023)
// project for about 2030.
//
// Whether one is ever announced for that date is beside the point. What is
// exercised is that an announced one would be recognised.
func withNegativeLeapSecondAt2030(t *testing.T) {
	t.Helper()

	record := builtinRecord(t)
	last := record[len(record)-1]

	if err := RegisterLeapSeconds(append(record,
		LeapSecond{Year: 2030, Month: 1, Day: 1, DeltaAT: last.DeltaAT - 1},
	), "test: hypothetical negative leap second"); err != nil {
		t.Fatalf("RegisterLeapSeconds: %v", err)
	}

	t.Cleanup(ResetLeapSeconds)
}

// TestNegativeLeapSecondEndsDay: the classifier has to tell a removed second
// from an inserted one and from an ordinary day, and fifty years of history
// contains only the middle case.
func TestNegativeLeapSecondEndsDay(t *testing.T) {
	withNegativeLeapSecondAt2030(t)

	tests := []struct {
		y, m, d int
		want    bool
		why     string
	}{
		{2029, 12, 31, true, "the hypothetical negative leap second"},
		{2016, 12, 31, false, "a positive leap second is not a removed one"},
		{2029, 12, 30, false, "the day before the step, not the step"},
		{2030, 1, 1, false, "the day after"},
		{2029, 6, 30, false, "a mid-year boundary with no step"},
		{1965, 12, 31, false, "pre-1972 ΔAT drifts by microseconds, which is not a leap second"},
	}

	for _, tt := range tests {
		if got := negativeLeapSecondEndsDay(tt.y, tt.m, tt.d); got != tt.want {
			t.Errorf("negativeLeapSecondEndsDay(%d-%02d-%02d) = %v, want %v (%s)",
				tt.y, tt.m, tt.d, got, tt.want, tt.why)
		}
	}
}

// TestPositiveAndNegativeClassifiersDoNotOverlap: a day carries at most one
// kind of leap second, and reporting an inserted one as removed would send the
// caller looking for the opposite defect.
func TestPositiveAndNegativeClassifiersDoNotOverlap(t *testing.T) {
	withNegativeLeapSecondAt2030(t)

	for _, d := range []struct {
		y, m, day          int
		positive, negative bool
	}{
		{2016, 12, 31, true, false},
		{2029, 12, 31, false, true},
		{2020, 12, 31, false, false},
	} {
		gotP := leapSecondEndsDay(d.y, d.m, d.day)
		gotN := negativeLeapSecondEndsDay(d.y, d.m, d.day)

		if gotP != d.positive || gotN != d.negative {
			t.Errorf("%d-%02d-%02d: inserted = %v (want %v), removed = %v (want %v)",
				d.y, d.m, d.day, gotP, d.positive, gotN, d.negative)
		}
	}
}

// TestSecondRemovedWarningIsAWarningNotProgress mirrors the positive case:
// Date has no error return, so this is the only notice a caller gets that the
// instant they built is one UTC never labelled.
func TestSecondRemovedWarningIsAWarningNotProgress(t *testing.T) {
	// Not parallel: it swaps the process-wide logger and the leap-second table.
	defer logging.Set(nil)

	withNegativeLeapSecondAt2030(t)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	// Called directly rather than through warnSecondRemoved, whose sync.Once
	// would be spent by whichever test ran first.
	logSecondRemoved(2029, 12, 31, 23, 59)

	out := buf.String()

	if !strings.Contains(out, "level=WARN") {
		t.Errorf("the removed-second message is not at WARN:\n%s\n"+
			"  The default logger drops everything below WARN, so demoting this "+
			"would make the loss silent.", out)
	}

	for _, want := range []string{
		`msg="second removed by a negative leap second, instant did not occur"`,
		"utc=2029-12-31T23:59:59Z",
		"delta_at_before=37",
		"delta_at_after=36",
		"remedy=",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the message does not carry %q:\n%s", want, out)
		}
	}
}

// TestDateReportsARemovedSecond is the wiring, and the two negatives beside it
// are what keep the warning from becoming noise: 23:59:59 is an ordinary
// instant on every day in history so far, and the check must stay silent on
// all of them.
func TestDateReportsARemovedSecond(t *testing.T) {
	defer logging.Set(nil)

	withNegativeLeapSecondAt2030(t)

	warnSecondRemovedOnce = sync.Once{}
	defer func() { warnSecondRemovedOnce = sync.Once{} }()

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	Date(2029, 12, 31, 23, 59, 59, 0, stdtime.UTC)

	if !strings.Contains(buf.String(), "second removed by a negative leap second") {
		t.Errorf("Date built a second that never occurred without reporting it; logger saw:\n%q", buf.String())
	}

	for _, quiet := range []struct {
		name               string
		y, mo, d, h, mi, s int
	}{
		{"the same second on an ordinary day", 2029, 12, 30, 23, 59, 59},
		{"the second before the removed one", 2029, 12, 31, 23, 59, 58},
		{"23:59:59 on a positive leap-second day", 2016, 12, 31, 23, 59, 59},
	} {
		buf.Reset()

		warnSecondRemovedOnce = sync.Once{}

		Date(quiet.y, stdtime.Month(quiet.mo), quiet.d, quiet.h, quiet.mi, quiet.s, 0, stdtime.UTC)

		if buf.String() != "" {
			t.Errorf("%s: Date warned about an ordinary instant:\n%q", quiet.name, buf.String())
		}
	}
}

// TestRemovedSecondWarningReportsTheUTCDate: the removal happens at the end of
// a UTC day, so a caller in another zone writes it with a different date and a
// different hour — and the check has to run on the UTC components or it looks
// at the wrong day entirely.
func TestRemovedSecondWarningReportsTheUTCDate(t *testing.T) {
	defer logging.Set(nil)

	withNegativeLeapSecondAt2030(t)

	warnSecondRemovedOnce = sync.Once{}
	defer func() { warnSecondRemovedOnce = sync.Once{} }()

	// UTC+13: 2029-12-31 23:59:59 UTC is 2030-01-01 12:59:59 there.
	east := stdtime.FixedZone("UTC+13", 13*3600)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	Date(2030, stdtime.January, 1, 12, 59, 59, 0, east)

	out := buf.String()

	if !strings.Contains(out, "second removed by a negative leap second") {
		t.Fatalf("a removed second written in UTC+13 was not recognised; logger saw:\n%q", out)
	}

	if !strings.Contains(out, "utc=2029-12-31T23:59:59Z") {
		t.Errorf("the warning reports the local components rather than the UTC ones:\n%s", out)
	}
}
