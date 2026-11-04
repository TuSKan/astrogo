package time_test

import (
	"bytes"
	"log/slog"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/logging"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// A non-finite time is no time, not degraded accuracy, so it does not warn
// that EOP data is unavailable, and it does not spend the once-per-process
// warning a real degradation needs (#988). Before, a program whose first EOP
// lookup came at a NaN time logged "EOP unavailable" for it, and then nothing
// when EOP data really was missing.
func TestNonFiniteTimeDoesNotSpendTheEOPWarning(t *testing.T) {
	// Not parallel: the EOP model, the logger and the warning's Once are all
	// process-wide.

	// Nothing loaded and nothing to load, as TestNothingLoadedWarnsAndChosenZeroDoesNot
	// sets it up, so a finite time degrades and must warn.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	time.ResetEOP()
	t.Cleanup(time.ResetEOP)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { logging.Set(nil) })

	time.ResetEOPWarning()

	// Every degrading path, at NaN and at both infinities.
	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		_ = time.FromJD(jd, time.UTC).EOP()
		_, _ = time.FromJD(jd, time.UTC).UT1()
		_ = time.FromJD(jd, time.UT1).UTC()
	}

	if out := buf.String(); strings.Contains(out, "EOP unavailable") {
		t.Errorf("a non-finite time warned that EOP data is unavailable:\n%q", out)
	}

	// The real notice is still there to give. The buffer is emptied first, so
	// this is checked on its own: left as it was, the line a non-finite time
	// wrongly logged above would satisfy it even with the Once spent.
	buf.Reset()

	_ = time.FromJD(2461318.5, time.UTC).EOP()

	if out := buf.String(); !strings.Contains(out, "EOP unavailable") {
		t.Errorf("a finite time with nothing loaded did not warn after the non-finite lookups; "+
			"they spent the once-per-process warning:\n%q", out)
	}
}
