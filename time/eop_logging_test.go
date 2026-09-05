package time

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/logging"
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
	logEOPUnavailable(58849.0)

	out := buf.String()

	if out == "" {
		t.Fatal("the EOP-unavailable warning wrote nothing to the installed logger")
	}

	if !strings.Contains(out, "level=WARN") {
		t.Errorf("the EOP-unavailable message is not at WARN:\n%s\n"+
			"  The default logger drops everything below WARN, so demoting this "+
			"would silence the only notice that accuracy degraded.", out)
	}

	for _, want := range []string{"EOP data unavailable", "mjd=58849"} {
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

	if !logging.Get().Enabled(t.Context(), slog.LevelWarn) {
		t.Error("the default logger drops WARN, so an unconfigured caller would " +
			"never learn that EOP were unavailable")
	}
}
