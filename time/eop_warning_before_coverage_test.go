package time

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/logging"
)

// TestEOPWarningBeforeTheBulletin: for an epoch the IERS bulletin cannot
// cover, the warning advised a download, and stated the bound leap seconds
// kept |UT1-UTC| inside, even for AD 33 (#1141). Before 1960 there was no UTC
// to differ from UT1, so the notice says what is used instead and that there
// is nothing to fetch; from 1960 to the bulletin's first day, 1973-01-02, the
// warning stays but no download is offered, and before 1972 the leap-second
// bound is not claimed.
func TestEOPWarningBeforeTheBulletin(t *testing.T) {
	// Not parallel: it swaps the process-wide logger.
	defer logging.Set(nil)

	ad33 := Date(33, April, 3, 15, 0, 0, 0, LocationUTC).MJD()

	for _, tc := range []struct {
		name      string
		mjd       float64
		want, not []string
	}{
		{
			"AD 33, before UTC existed", ad33,
			[]string{
				"no Earth orientation before 1960",
				"Espenak & Meeus (2006)",
				"nothing to fetch",
				"RegisterModel(ZeroModel{})",
			},
			[]string{"remote/eop", "EnableDownloads", "0.9 s", "leap seconds"},
		},
		{
			"1965, steered UTC", Date(1965, June, 1, 0, 0, 0, 0, LocationUTC).MJD(),
			[]string{"EOP unavailable", "steered toward UT2", "begins on 1973-01-02"},
			[]string{"EnableDownloads", "0.9 s"},
		},
		{
			"1972, leap seconds but no bulletin yet", Date(1972, June, 1, 0, 0, 0, 0, LocationUTC).MJD(),
			[]string{"EOP unavailable", "up to 0.9 s", "begins on 1973-01-02"},
			[]string{"EnableDownloads"},
		},
		{
			"2020, inside the bulletin's years", Date(2020, January, 1, 0, 0, 0, 0, LocationUTC).MJD(),
			[]string{"EOP unavailable", "up to 0.9 s", "github.com/TuSKan/astrogo/remote/eop", "EnableDownloads"},
			[]string{"begins on 1973-01-02", "before 1960"},
		},
	} {
		var buf bytes.Buffer

		logging.Set(slog.New(slog.NewTextHandler(&buf, nil)))
		logEOPUnavailable(tc.mjd, errNothingLoaded)

		out := buf.String()
		if !strings.Contains(out, "level=WARN") {
			t.Errorf("%s: not a warning:\n%s", tc.name, out)
		}

		for _, w := range tc.want {
			if !strings.Contains(out, w) {
				t.Errorf("%s (MJD %.1f): the notice lacks %q:\n%s", tc.name, tc.mjd, w, out)
			}
		}

		for _, n := range tc.not {
			if strings.Contains(out, n) {
				t.Errorf("%s (MJD %.1f): the notice says %q, which does not apply:\n%s", tc.name, tc.mjd, n, out)
			}
		}
	}
}

// TestBulletinBoundariesAreTheirDates holds the hand-written MJDs to the
// dates they name.
func TestBulletinBoundariesAreTheirDates(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		mjd  float64
		date Time
	}{
		{"mjd1960", mjd1960, Date(1960, January, 1, 0, 0, 0, 0, LocationUTC)},
		{"mjd1972", mjd1972, Date(1972, January, 1, 0, 0, 0, 0, LocationUTC)},
		{"bulletinFirstMJD", bulletinFirstMJD, Date(1973, January, 2, 0, 0, 0, 0, LocationUTC)},
	} {
		if got := c.date.MJD(); got != c.mjd {
			t.Errorf("%s is %v, but its date is MJD %v", c.name, c.mjd, got)
		}
	}
}
