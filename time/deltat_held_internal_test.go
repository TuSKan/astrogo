package time

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/logging"
)

// TestDeltaTHeldIsItsOwnNotice: past the bulletin's end DUT1 is held, not
// zero, so the notice says that rather than "EOP unavailable", with the held
// value and its uncertainty. It has its own Once: sharing one with the
// unavailable warning, whichever case came first would have silenced the
// other for the rest of the process.
func TestDeltaTHeldIsItsOwnNotice(t *testing.T) {
	// Not parallel: it swaps the process-wide logger and the two Onces.
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	warnEOPUnavailableOnce, warnDeltaTHeldOnce = sync.Once{}, sync.Once{}

	held := &deltaTHeldError{lastMJD: 61400, deltaT: 69.123, sigma: 1.6}

	warnEOPUnavailable(65000, held)
	warnEOPUnavailable(30000, errNothingLoaded)

	out := buf.String()

	for _, want := range []string{
		`msg="EOP bulletin ends before this epoch, holding ΔT at its last value"`,
		"bulletin_last_mjd=61400",
		`delta_t="69.123 s, TT − UT1 on the bulletin's last day"`,
		`delta_t_sigma="1.6 s here, Huber (2000) from the bulletin's end"`,
		"polar_motion=zero",
		`msg="EOP unavailable, using zero DUT1 and polar motion"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the notices are missing %s:\n%s", want, out)
		}
	}

	if n := strings.Count(out, "level=WARN"); n != 2 {
		t.Errorf("%d warnings written, want the two, one of each:\n%s", n, out)
	}
}
