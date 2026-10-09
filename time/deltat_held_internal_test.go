package time

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/logging"
	"github.com/TuSKan/astrogo/time/internal/iers"
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

// tinyEndBulletin covers MJD 60000–60400 with a DUT1 that ends at 7e-18 s,
// what an FMA-fused 0.2 − 0.0005·400 gives on arm64, and an EOP that fails
// for its own last day when failLast is set.
type tinyEndBulletin struct{ failLast bool }

func (b tinyEndBulletin) EOP(mjd float64) (EOP, error) {
	if mjd < 60000 || mjd > 60400 || (b.failLast && mjd == 60400) {
		return EOP{}, iers.ErrOutOfRange
	}

	return EOP{DUT1: 7e-18}, nil
}

func (tinyEndBulletin) Coverage() (mjdMin, mjdMax float64) { return 60000, 60400 }

// TestHeldDUT1IsTheBulletinsOwn: with no leap second between, the held DUT1
// is the bulletin's last value exactly. It used to be end.DUT1 + ΔAT − ΔAT,
// which rounds a DUT1 this small away against 37 s; that failed on macOS
// alone, where FMA left the test bulletin's last DUT1 at 7e-18 rather than 0.
// And a bulletin that cannot answer for its own last day holds nothing: the
// lookup's out-of-range error stands.
func TestHeldDUT1IsTheBulletinsOwn(t *testing.T) {
	// Not parallel: the EOP model is process-wide.
	RegisterModel(tinyEndBulletin{})
	t.Cleanup(ResetEOP)

	eop, report, ok := heldPastBulletin(65000)
	if !ok || eop.DUT1 != 7e-18 {
		t.Errorf("held DUT1 = %g s (ok %v), want the bulletin's last 7e-18 s exactly", eop.DUT1, ok)
	}

	if msg := report.Error(); !strings.Contains(msg, "ΔT held at") || !strings.Contains(msg, "MJD 60400.0") {
		t.Errorf("the held report reads %q, want the held ΔT and the bulletin's last day", msg)
	}

	RegisterModel(tinyEndBulletin{failLast: true})

	if _, _, ok := heldPastBulletin(65000); ok {
		t.Error("a bulletin that cannot answer for its last day was held anyway")
	}
}
