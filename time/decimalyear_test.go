package time_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestDecimalYearIsTheFractionOfTheYearElapsed is #697. DecimalYear was
// year + (month − 0.5 + dayFraction)/12: it dropped the day of the month and
// added the day's fraction as if it were the month's, so within a month it
// ran backward with the date. It is now the fraction of the year elapsed.
func TestDecimalYearIsTheFractionOfTheYearElapsed(t *testing.T) {
	t.Parallel()

	at := func(y int, m time.Month, d, h, mi int) float64 {
		return time.Date(y, m, d, h, mi, 0, 0, time.LocationUTC).DecimalYear()
	}

	// The issue's pair: the later instant read the smaller year.
	if early, late := at(2026, time.March, 1, 23, 59), at(2026, time.March, 31, 0, 0); early >= late {
		t.Errorf("2026-03-01 23:59 reads %.6f, 2026-03-31 00:00 reads %.6f: the year runs backward", early, late)
	}

	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"the start of a year", at(2025, time.January, 1, 0, 0), 2025},
		{"noon on 2 July of a common year", at(2025, time.July, 2, 12, 0), 2025 + 182.5/365},
		{"1 March of a leap year, after 29 February", at(2024, time.March, 1, 0, 0), 2024 + 60.0/366},
		{"the last minute of a leap year", at(2024, time.December, 31, 23, 59), 2024 + (365+1439.0/1440)/366},
		{"1900, which is not leap", at(1900, time.March, 1, 0, 0), 1900 + 59.0/365},
		{"2000, which is", at(2000, time.March, 1, 0, 0), 2000 + 60.0/366},
		{"astronomical year 0, a leap year", at(0, time.December, 31, 0, 0), 365.0 / 366},
	} {
		testutil.AssertNear(t, c.name, c.got, c.want, 1e-9)
	}

	// Strictly increasing through a year in six-hour steps, across every
	// month boundary, where the old formula jumped back by most of a month.
	prev := at(2023, time.December, 31, 18, 0)

	for day := range 366 * 4 {
		cur := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.LocationUTC).
			Add(time.FromGoDuration(time.Duration(day) * 6 * time.Hour)).DecimalYear()
		if cur <= prev {
			t.Fatalf("step %d: DecimalYear %.9f does not advance from %.9f", day, cur, prev)
		}

		prev = cur
	}
}
