package time

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/logging"
)

// TestEOPWarningIsAWarningNotProgress pins the level, which is the whole
// reason the default logger is not a discard handler.
//
// #108 proposed discarding everything by default — right for the two
// progress lines, wrong for this one. [Time.EOP] has no error return and the
// UT1<->UTC path has already decided to degrade rather than fail, so this
// message is the only notice a caller gets that DUT1 and polar motion are zero
// and topocentric accuracy has dropped to about an arcsecond.
//
// If it were ever demoted to Info it would vanish under the default logger,
// and the degradation would become exactly the silent kind the warning exists
// to prevent. Nothing else would fail.
func TestEOPWarningIsAWarningNotProgress(t *testing.T) {
	// Not parallel: it swaps the process-wide logger.
	defer logging.Set(nil)

	var buf bytes.Buffer

	// Info-and-above, so a demotion to Info would still be captured here and
	// the level assertion below is what catches it — not an empty buffer.
	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Called directly rather than through warnEOPUnavailable, whose sync.Once
	// would be spent by whichever test ran first.
	logEOPUnavailable(58849.0, errNothingLoaded)

	out := buf.String()

	if out == "" {
		t.Fatal("the EOP-unavailable warning wrote nothing to the installed logger")
	}

	if !strings.Contains(out, "level=WARN") {
		t.Errorf("the EOP-unavailable message is not at WARN:\n%s\n"+
			"  The default logger drops everything below WARN, so demoting this "+
			"would silence the only notice that accuracy degraded.", out)
	}

	// The message is short and fixed; the detail is in attributes. Both are
	// checked, because a message that carried the detail in prose would still
	// pass a check for the message alone.
	for _, want := range []string{
		`msg="EOP unavailable, using zero DUT1 and polar motion"`,
		"mjd=58849",
		// Which step failed, so a caller can tell the remedies apart (#518).
		`cause="time: no EOP bulletin loaded"`,
		// The rotation the stated UT1 error amounts to, 0.9 s at 15.04″/s,
		// not the "~1 arcsec" this said until #689, which is what it costs
		// only in a year when |DUT1| happens to be small.
		`topocentric_error="up to 13.5 arcsec`,
		"remedy=",
		// The remedy has to name the import: without remote/eop no loader is
		// registered, and EnableDownloads alone consents to a fetch nothing
		// will make.
		"github.com/TuSKan/astrogo/remote/eop",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the message does not carry %q:\n%s", want, out)
		}
	}
}

// TestEOPWarningReachesTheDefaultLogger is the same property from the other
// side: with nothing configured, the warning is still emitted.
//
// The check is on the default logger's own level gate rather than on captured
// output, because the default writes to stderr by design and a test should not
// be asserting against the process's stderr.
func TestEOPWarningReachesTheDefaultLogger(t *testing.T) {
	defer logging.Set(nil)

	logging.Set(nil)

	if !logging.Logger().Enabled(t.Context(), slog.LevelWarn) {
		t.Error("the default logger drops WARN, so an unconfigured caller would " +
			"never learn that EOP were unavailable")
	}
}

// TestEOPWarningSaysWhenLeapSecondsStopBoundingTheError: the 0.9 s the warning
// states is the bound leap seconds kept |UT1−UTC| inside, and UTC is continuous
// from 2027-05-20 (#879). Before that day the warning states the bound; from it
// on, nothing holds UT1−UTC to 0.9 s and the warning must not say so.
func TestEOPWarningSaysWhenLeapSecondsStopBoundingTheError(t *testing.T) {
	// Not parallel: it swaps the process-wide logger.
	defer logging.Set(nil)

	for _, tc := range []struct {
		name      string
		mjd       float64
		want, not string
	}{
		{"the day before continuous UTC", continuousUTC - 1, `ut1_error="up to 0.9 s`, "3600 s"},
		{"the day it begins", continuousUTC, "3600 s", "up to 0.9 s"},
		{"a decade after", continuousUTC + 3653, "3600 s", "13.5 arcsec"},
	} {
		var buf bytes.Buffer

		logging.Set(slog.New(slog.NewTextHandler(&buf, nil)))
		logEOPUnavailable(tc.mjd, errNothingLoaded)

		if out := buf.String(); !strings.Contains(out, tc.want) || strings.Contains(out, tc.not) {
			t.Errorf("%s (MJD %v): the warning should say %q and not %q:\n%s", tc.name, tc.mjd, tc.want, tc.not, out)
		}
	}
}

// TestContinuousUTCIsTheDateTheCGPMSet holds the hand-written MJD to the date
// Resolution 5 of the 28th CGPM (2026) sets: 20 May 2027.
func TestContinuousUTCIsTheDateTheCGPMSet(t *testing.T) {
	t.Parallel()

	if got := Date(2027, May, 20, 0, 0, 0, 0, LocationUTC).MJD(); got != continuousUTC {
		t.Errorf("2027-05-20 is MJD %v, but continuousUTC is %v", got, float64(continuousUTC))
	}
}
