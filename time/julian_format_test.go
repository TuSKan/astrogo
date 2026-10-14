package time_test

import (
	"fmt"
	"testing"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestDateJulianCalFormatsBackToItself: every whole-second Julian-calendar
// date and time must format back to what built it. DateJulianCal summed the
// day number and the time of day into one float64, which near JD 1.7e6 is 20
// µs coarse, and FormatJulian truncated to the second, so an instant a few
// microseconds short printed a second early: 12:30:00 as 12:29:59, for 509
// of these 1,070 (#439).
func TestDateJulianCalFormatsBackToItself(t *testing.T) {
	wrong := 0

	for y := 1; y < 1500; y += 7 {
		for _, hms := range [][3]int{{15, 0, 0}, {0, 0, 0}, {12, 30, 0}, {23, 59, 59}, {6, 0, 1}} {
			want := fmt.Sprintf("%04d-04-03 %02d:%02d:%02d", y, hms[0], hms[1], hms[2])

			if got := time.DateJulianCal(y, 4, 3, hms[0], hms[1], hms[2]).FormatJulian("2006-01-02 15:04:05"); got != want {
				if wrong++; wrong <= 5 {
					t.Errorf("DateJulianCal(%s) formats as %s", want, got)
				}
			}
		}
	}

	if wrong > 0 {
		t.Errorf("%d of 1,070 dates did not format back to themselves", wrong)
	}
}

// TestFormattingRoundsToTheSecond: both formatters round to the nearest
// second, carrying into the next day when that is midnight, rather than
// truncating.
func TestFormattingRoundsToTheSecond(t *testing.T) {
	// 0.4 s short of midnight, in the Julian calendar, and 0.6 s short.
	for _, c := range []struct {
		after float64
		want  string
	}{
		{0.6, "0033-04-03 00:00:00"},
		{0.4, "0033-04-02 23:59:59"},
	} {
		late := time.DateJulianCal(33, 4, 2, 23, 59, 59).Add(unit.Seconds(c.after))
		if got := late.FormatJulian("2006-01-02 15:04:05"); got != c.want {
			t.Errorf("23:59:59 + %v s on 33 April 2 formats as %s, want %s", c.after, got, c.want)
		}
	}

	// A year outside 0–9999 takes Format's own path rather than the standard
	// library's: 12:29:59.7 on 1 March 500 BC, proleptic Gregorian.
	bc := time.Date(-500, 3, 1, 12, 29, 59, 700_000_000, time.LocationUTC)
	if got := bc.Format("2006-01-02 15:04:05"); got != "-0500-03-01 12:30:00" {
		t.Errorf("12:29:59.7 on -500 March 1 formats as %s, want -0500-03-01 12:30:00", got)
	}
}

// TestFormattersReadTheUTCClock holds both formatters to one clock for one
// instant, whichever scale holds it (#766). Format read UTC within years
// 0–9999, through ToGo, and t's own scale outside them; FormatJulian always
// read t's own. A TT instant in 585 BC printed 5h06m after the same instant
// held as UTC, and in 2026 FormatJulian put it 1 min 9 s after Format.
//
// A quarter second past the minute keeps the comparison off a rounding edge:
// the standard library truncates where the manual paths round, and both read
// 12:00:00 there.
func TestFormattersReadTheUTCClock(t *testing.T) {
	t.Parallel()

	const layout = "2006-01-02 15:04:05 MST"

	for _, y := range []int{-584, 1000, 2026, 10001} {
		utc := time.Date(y, 5, 28, 12, 0, 0, 250_000_000, time.LocationUTC)

		for _, held := range []time.Time{utc.TT(), utc.TDB()} {
			if got, want := held.Format(layout), utc.Format(layout); got != want {
				t.Errorf("year %d held as %s: Format = %q, want %q", y, held.Scale(), got, want)
			}

			if got, want := held.FormatJulian(layout), utc.FormatJulian(layout); got != want {
				t.Errorf("year %d held as %s: FormatJulian = %q, want %q", y, held.Scale(), got, want)
			}
		}
	}

	// The two formatters differ in calendar, never in clock: in 2026 the
	// Julian date is thirteen days behind and the time of day is the same.
	tt := time.Date(2026, 5, 28, 12, 0, 0, 250_000_000, time.LocationUTC).TT()
	if g, j := tt.Format("15:04:05"), tt.FormatJulian("15:04:05"); g != j || g != "12:00:00" {
		t.Errorf("TT instant at 12:00:00.25 UTC: Format clock %s, FormatJulian clock %s; want both 12:00:00", g, j)
	}
}
