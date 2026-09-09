package time_test

import (
	"errors"
	"testing"

	gotime "time"

	"github.com/TuSKan/astrogo/time"
)

// The re-exports used to be `var Parse = time.Parse` — an alias, which cannot
// be wrong. Rewriting them as functions to stop anyone reassigning them
// (see #113) means each now has a hand-written signature and a hand-written
// forwarding call, and a transposition compiles.
//
// GoDate is the one that would actually bite: eight parameters, five of them
// `int`, so swapping day and hour type-checks and silently moves every
// timestamp. These tests exist for that, not for coverage of a one-line body.

// TestGoDateForwardsItsArgumentsInOrder pins the parameter order against the
// standard library's own.
func TestGoDateForwardsItsArgumentsInOrder(t *testing.T) {
	t.Parallel()

	// Every field distinct, so any transposition changes the result.
	got := time.GoDate(2026, time.March, 4, 5, 6, 7, 8, time.LocationUTC)
	want := gotime.Date(2026, gotime.March, 4, 5, 6, 7, 8, gotime.UTC)

	if !got.Equal(want) {
		t.Errorf("GoDate = %v, want %v — the arguments are not forwarded in order", got, want)
	}

	// Spelled out as well, because Equal would also pass if both sides were
	// wrong in the same way.
	if y, mo, d := got.Date(); y != 2026 || mo != gotime.March || d != 4 {
		t.Errorf("date = %d-%v-%d, want 2026-March-4", y, mo, d)
	}

	if h, mi, s := got.Clock(); h != 5 || mi != 6 || s != 7 {
		t.Errorf("clock = %d:%d:%d, want 5:6:7", h, mi, s)
	}

	if ns := got.Nanosecond(); ns != 8 {
		t.Errorf("nanosecond = %d, want 8", ns)
	}
}

// TestUnixForwardsSecondsThenNanoseconds catches the other order-sensitive
// pair, where both parameters are int64 and swapping them type-checks.
func TestUnixForwardsSecondsThenNanoseconds(t *testing.T) {
	t.Parallel()

	got := time.Unix(1_700_000_000, 123)
	if want := gotime.Unix(1_700_000_000, 123); !got.Equal(want) {
		t.Errorf("Unix = %v, want %v", got, want)
	}

	if sec := got.Unix(); sec != 1_700_000_000 {
		t.Errorf("Unix().Unix() = %d, want 1700000000 — seconds and nanoseconds are swapped", sec)
	}
}

// TestParseRoundTripsAndReportsFailure covers both outcomes, and confirms the
// error is still the standard library's own type: the wrapper carries a scoped
// //nolint:wrapcheck precisely so that a caller's type assertion keeps working.
func TestParseRoundTripsAndReportsFailure(t *testing.T) {
	t.Parallel()

	got, err := time.Parse(time.RFC3339, "2026-03-04T05:06:07Z")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if want := gotime.Date(2026, gotime.March, 4, 5, 6, 7, 0, gotime.UTC); !got.Equal(want) {
		t.Errorf("Parse = %v, want %v", got, want)
	}

	_, err = time.Parse(time.RFC3339, "not a time")
	if err == nil {
		t.Fatal("Parse accepted a value that is not a time")
	}

	if _, ok := errors.AsType[*gotime.ParseError](err); !ok {
		t.Errorf("Parse returned %T; a caller can no longer type-assert *time.ParseError.\n"+
			"  That is what the //nolint:wrapcheck on this re-export exists to preserve.", err)
	}
}

// TestParseInLocationUsesTheGivenZone checks the third argument is actually
// used, which a signature alone does not.
func TestParseInLocationUsesTheGivenZone(t *testing.T) {
	t.Parallel()

	// +05:30, chosen because it is not a whole number of hours, so a dropped
	// or defaulted location is unmistakable.
	loc := time.FixedZone("test+0530", 5*3600+30*60)

	got, err := time.ParseInLocation(time.DateTime, "2026-03-04 05:06:07", loc)
	if err != nil {
		t.Fatalf("ParseInLocation: %v", err)
	}

	if _, offset := got.Zone(); offset != 5*3600+30*60 {
		t.Errorf("zone offset = %d s, want 19800 — the location argument was ignored", offset)
	}

	if utc := got.UTC(); utc.Hour() != 23 || utc.Minute() != 36 {
		t.Errorf("in UTC the instant is %02d:%02d, want 23:36 the previous day", utc.Hour(), utc.Minute())
	}
}

// TestLoadLocationAndMustLocation covers the pair, including MustLocation's
// panic, which is its whole reason to exist over LoadLocation.
func TestLoadLocationAndMustLocation(t *testing.T) {
	t.Parallel()

	// UTC resolves without the tzdata files, so this runs anywhere.
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatalf("LoadLocation(UTC): %v", err)
	}

	if loc.String() != "UTC" {
		t.Errorf("LoadLocation(UTC) = %q, want UTC", loc)
	}

	if _, err := time.LoadLocation("Not/AZone"); err == nil {
		t.Error("LoadLocation accepted a zone that does not exist")
	}

	if got := time.MustLocation("UTC"); got.String() != "UTC" {
		t.Errorf("MustLocation(UTC) = %q, want UTC", got)
	}

	defer func() {
		if recover() == nil {
			t.Error("MustLocation did not panic on a zone that does not exist, which is the only " +
				"thing distinguishing it from LoadLocation")
		}
	}()

	_ = time.MustLocation("Not/AZone")
}

// TestDurationHelpersDelegate covers Since, Until, Sleep, After, NewTimer and
// NewTicker together. Each is a one-line forward, so the useful assertion is
// the direction of the sign and that the channel actually fires.
func TestDurationHelpersDelegate(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-time.Hour)
	if d := time.Since(past); d < time.Hour {
		t.Errorf("Since(an hour ago) = %v, want at least an hour — the sign is inverted", d)
	}

	future := time.Now().Add(time.Hour)
	if d := time.Until(future); d <= 0 || d > time.Hour {
		t.Errorf("Until(an hour hence) = %v, want a positive duration under an hour", d)
	}

	// Real but negligible: enough to prove the call reaches the standard
	// library, short enough not to slow the suite.
	start := time.Now()

	time.Sleep(time.Millisecond)

	if elapsed := time.Since(start); elapsed <= 0 {
		t.Errorf("Sleep returned after %v; time did not advance", elapsed)
	}

	select {
	case <-time.After(50 * time.Millisecond):
	case <-time.After(5 * time.Second):
		t.Error("After did not fire within 5s")
	}

	timer := time.NewTimer(time.Millisecond)
	defer timer.Stop()

	select {
	case <-timer.C:
	case <-time.After(5 * time.Second):
		t.Error("NewTimer did not fire within 5s")
	}

	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	select {
	case <-ticker.C:
	case <-time.After(5 * time.Second):
		t.Error("NewTicker did not fire within 5s")
	}
}

// TestLayoutConstantsAreUntyped is the assertion that the layouts became
// consts rather than merely stopped being reassignable.
//
// An untyped constant converts to any string type; a `var` of type string does
// not. Passing them to a defined string type is the check, and it is a compile
// -time one — if the layouts revert to var, this file stops building.
func TestLayoutConstantsAreUntyped(t *testing.T) {
	t.Parallel()

	type layout string

	for _, l := range []layout{
		time.RFC1123, time.RFC3339, time.RFC3339Nano,
		time.DateOnly, time.DateTime, time.TimeOnly,
	} {
		if l == "" {
			t.Error("a layout constant is empty")
		}
	}
}
