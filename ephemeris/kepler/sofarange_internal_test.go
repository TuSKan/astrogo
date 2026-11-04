package kepler

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/logging"
	"github.com/TuSKan/astrogo/time"
)

// Plan94 returns +2 when its solution failed to converge, and the position
// that comes with it is not a solution of Kepler's equation. It was read as
// success, since only a negative status was checked (#1003). At JD 1e12 SOFA
// reports +2 for Mercury through Uranus.
func TestPlan94NonConvergenceIsAnError(t *testing.T) {
	t.Parallel()

	p := New()
	at := time.FromJD(1e12, time.TT)

	for _, id := range []core.ID{
		core.MercuryBarycenter, core.VenusBarycenter, core.MarsBarycenter,
		core.JupiterBarycenter, core.SaturnBarycenter, core.UranusBarycenter,
	} {
		st, err := p.Locate(id, at)
		if !errors.Is(err, ErrSofaFailure) {
			t.Errorf("%v at JD 1e12: position %v, err %v, want ErrSofaFailure", id, st.Pos, err)
		}
	}
}

// A date outside Epv00's or Plan94's fitted span is answered, as SOFA answers
// it, but the first one in a process writes a warning naming the theory: the
// position is worse than inside the span, by a factor SOFA states, and before
// #1003 nothing said so. Once per theory, since a scan over a century would
// otherwise write one per sample.
//
// Not parallel: it swaps the process-wide logger and re-arms the warnings.
func TestADateOutsideASofaSpanWarnsOncePerTheory(t *testing.T) {
	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	rearm := func() { *epv00Span.once, *plan94Span.once = sync.Once{}, sync.Once{} }
	rearm()

	t.Cleanup(func() {
		logging.Set(nil)
		rearm()
	})

	p := New()

	year := func(y float64) time.Time { return time.FromJD(2451545.0+(y-2000)*365.25, time.TT) }

	locate := func(id core.ID, y float64) {
		t.Helper()

		if _, err := p.Locate(id, year(y)); err != nil {
			t.Fatalf("%v in %v: %v", id, y, err)
		}
	}

	warnings := func(theory string) int { return strings.Count(buf.String(), "theory="+theory) }

	// Inside both spans: nothing.
	locate(core.Sun, 2000)
	locate(core.MarsBarycenter, 2000)

	if buf.Len() != 0 {
		t.Fatalf("inside both spans, logged %q", buf.String())
	}

	// 1800 is outside Epv00's 1900-2100 and inside Plan94's 1000-3000.
	locate(core.Sun, 1800)
	locate(core.MarsBarycenter, 1800)

	if got := warnings("Epv00"); got != 1 {
		t.Errorf("after the Sun in 1800: %d Epv00 warnings, want 1; log %q", got, buf.String())
	}

	if got := warnings("Plan94"); got != 0 {
		t.Errorf("after Mars in 1800: %d Plan94 warnings, want 0", got)
	}

	// Outside Epv00's span again, at another date: still one.
	locate(core.Sun, 2200)

	if got := warnings("Epv00"); got != 1 {
		t.Errorf("after the Sun in 2200 as well: %d Epv00 warnings, want still 1", got)
	}

	// 3500 is outside Plan94's span, for two planets: one warning.
	locate(core.MarsBarycenter, 3500)
	locate(core.JupiterBarycenter, 3500)

	if got := warnings("Plan94"); got != 1 {
		t.Errorf("after Mars and Jupiter in 3500: %d Plan94 warnings, want 1; log %q", got, buf.String())
	}
}
