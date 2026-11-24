package epoch_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/epoch"
)

// bulletin returns n consecutive finals2000A records starting at MJD start,
// each finalsRecord's values under its own date. The parser slices by column,
// so the date and MJD are written at the widths the bulletin uses.
func bulletin(start, n int) string {
	var b strings.Builder

	for i := range n {
		mjd := start + i
		y, m, d := mjdDate(mjd)
		fmt.Fprintf(&b, "%2d%2d%2d %8.2f%s\n", y%100, m, d, float64(mjd), finalsRecord[15:])
	}

	return b.String()
}

// mjdDate is the calendar date of an MJD, for the record's date columns.
func mjdDate(mjd int) (y, m, d int) {
	t := epoch.FromJD(float64(mjd)+2400000.5, epoch.UTC)

	return t.Year(), int(t.Month()), t.Day()
}

// stubLoader is an EOPLoader whose two steps answer what a test says, and
// count how often they were asked.
type stubLoader struct {
	fetch, cached func(context.Context) (epoch.EOPData, error)
	calls         atomic.Int32
}

func (l *stubLoader) Fetch(ctx context.Context) (epoch.EOPData, error) {
	l.calls.Add(1)

	return l.fetch(ctx)
}

func (l *stubLoader) Cached(ctx context.Context) (epoch.EOPData, error) {
	l.calls.Add(1)

	return l.cached(ctx)
}

// serve is a loader step answering raw, or nothing if raw is empty.
func serve(raw string) func(context.Context) (epoch.EOPData, error) {
	return func(context.Context) (epoch.EOPData, error) {
		if raw == "" {
			return epoch.EOPData{}, epoch.ErrNoEOPData
		}

		return epoch.EOPData{Raw: []byte(raw)}, nil
	}
}

// errIERSDown is a loader's network step failing.
var errIERSDown = errors.New("the IERS is down")

// startOver puts the process-wide EOP state back as a test found it.
func startOver(t *testing.T) {
	t.Helper()

	epoch.RegisterEOPLoader(nil)
	epoch.ResetEOP()

	t.Cleanup(func() {
		epoch.RegisterEOPLoader(nil)
		epoch.ResetEOP()
	})
}

// TestLoadEOPWithNoLoaderSaysSo: with no loader registered, LoadEOP is
// ErrNoEOPLoader, where the lazy load says nothing and answers zeros (#1187).
func TestLoadEOPWithNoLoaderSaysSo(t *testing.T) {
	startOver(t)

	if err := epoch.LoadEOP(t.Context()); !errors.Is(err, epoch.ErrNoEOPLoader) {
		t.Errorf("LoadEOP with no loader = %v, want ErrNoEOPLoader", err)
	}
}

// TestLoadEOPFetchesThenReadsTheCache: LoadEOP takes the network step first
// and the cached bulletin when the network gives none that parses, records
// which in EOPSource, and when neither gives one returns both errors and
// leaves the model as it was.
func TestLoadEOPFetchesThenReadsTheCache(t *testing.T) {
	good := bulletin(61041, 30)

	for _, c := range []struct {
		name          string
		fetch, cached func(context.Context) (epoch.EOPData, error)
		wantSource    string
		wantErrs      []error
	}{
		{"the network answers", serve(good), serve(good), "network", nil},
		{"no consent, a bulletin cached", serve(""), serve(good), "cache", nil},
		{"the network sends junk", serve("<html>not a bulletin</html>"), serve(good), "cache", nil},
		{"neither", func(context.Context) (epoch.EOPData, error) { return epoch.EOPData{}, errIERSDown }, serve(""), "zero",
			[]error{errIERSDown, epoch.ErrNoEOPData}},
	} {
		t.Run(c.name, func(t *testing.T) {
			startOver(t)
			epoch.RegisterEOPLoader(&stubLoader{fetch: c.fetch, cached: c.cached})

			err := epoch.LoadEOP(t.Context())

			switch {
			case c.wantErrs == nil && err != nil:
				t.Errorf("LoadEOP = %v, want nil", err)
			case c.wantErrs != nil:
				for _, want := range c.wantErrs {
					if !errors.Is(err, want) {
						t.Errorf("LoadEOP = %v, want it to carry %v", err, want)
					}
				}
			}

			if got := epoch.EOPSource(); got != c.wantSource {
				t.Errorf("EOPSource after LoadEOP = %q, want %q", got, c.wantSource)
			}
		})
	}
}

// TestLoadEOPIsTheModelFromThenOn: the bulletin LoadEOP loads replaces even
// an explicit RegisterModel(ZeroModel{}), answers inside its coverage, and is
// not replaced by the lazy load, which asks the loader nothing more for an
// epoch it does not cover.
func TestLoadEOPIsTheModelFromThenOn(t *testing.T) {
	startOver(t)
	epoch.RegisterModel(epoch.ZeroModel{})

	// No cooldown, so the lazy load would ask the loader again for an epoch
	// past the bulletin; only the loaded model being the caller's choice
	// stops it. With the default cooldown the throttle LoadEOP leaves behind
	// would stop it as well, and the check below would not tell them apart.
	epoch.SetRetryCooldown(0)
	t.Cleanup(func() { epoch.SetRetryCooldown(5 * time.Minute) })

	l := &stubLoader{fetch: serve(bulletin(61041, 30)), cached: serve("")}
	epoch.RegisterEOPLoader(l)

	if err := epoch.LoadEOP(t.Context()); err != nil {
		t.Fatalf("LoadEOP: %v", err)
	}

	lo, hi, ok := epoch.Coverage()
	if !ok || lo > 61041 || hi < 61070 {
		t.Fatalf("Coverage after LoadEOP = [%v, %v] %v, want the bulletin's [61041, 61070]", lo, hi, ok)
	}

	if dut1 := epoch.FromJD(61050.5+2400000.5, epoch.UTC).EOP().DUT1; dut1 == 0 {
		t.Errorf("DUT1 inside the bulletin = 0, want the bulletin's")
	}

	asked := l.calls.Load()

	_ = epoch.FromJD(61500+2400000.5, epoch.UTC).EOP() // past the bulletin

	if more := l.calls.Load() - asked; more != 0 {
		t.Errorf("an epoch past the loaded bulletin asked the loader %d more times, want none", more)
	}

	if got := epoch.EOPSource(); got != "network" {
		t.Errorf("EOPSource = %q, want network", got)
	}
}

// TestLoadEOPHonorsItsContext: the context reaches the loader, so a canceled
// load is the cancellation, not a silent fall to zero.
func TestLoadEOPHonorsItsContext(t *testing.T) {
	startOver(t)

	ctxErr := func(ctx context.Context) (epoch.EOPData, error) { return epoch.EOPData{}, ctx.Err() }
	epoch.RegisterEOPLoader(&stubLoader{fetch: ctxErr, cached: ctxErr})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := epoch.LoadEOP(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("LoadEOP with a canceled context = %v, want context.Canceled", err)
	}
}
