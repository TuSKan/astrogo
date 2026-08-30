package airglow

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// The almanac's own "no data" sentinel becomes this package's "unset".
//
// # Why the translation matters
//
// SkyCalc says it has no published monthly average by returning -1, which is
// the ordinary answer for the current month and anything after it: the figure
// is a monthly mean from Natural Resources Canada and does not exist until
// the month is over. Passed straight through, it would reach a request as a
// solar flux of minus one — a value the service would reject, or worse
// accept. Zero is what [Spec.SolarFluxSFU] already treats as unset, so
// translating to it leaves the request at SkyCalc's own default and the two
// compose without the caller checking.
func TestAlmanacTranslatesTheNoDataSentinel(t *testing.T) {
	t.Parallel()

	var res almanacResponse

	res.Output.Observation.SeasonFlag = 5
	res.Output.Observation.TimeFlag = 1
	res.Output.Sun.AveFlux = -1

	got, err := res.almanac()
	if err != nil {
		t.Fatalf("almanac: %v", err)
	}

	if got.SolarFluxSFU != 0 {
		t.Errorf("a -1 became %g, want 0 — the sentinel must not reach a request",
			got.SolarFluxSFU)
	}

	if got.Season != 5 || got.TimeOfNight != 1 {
		t.Errorf("season/time came back %d/%d, want 5/1", got.Season, got.TimeOfNight)
	}
}

// A flux that is neither the sentinel nor a measurement is refused.
//
// Distinguished from the -1 case deliberately: -1 means "not published yet"
// and is expected, while -20 or a NaN means the response is not one this
// package understands, and passing either on as a solar flux would put a
// number nobody can defend into a spectrum.
func TestAlmanacRefusesAnImpossibleFlux(t *testing.T) {
	t.Parallel()

	for _, flux := range []float64{-20, -0.5, math.NaN(), math.Inf(1)} {
		var res almanacResponse

		res.Output.Sun.AveFlux = flux

		if _, err := res.almanac(); !errors.Is(err, ErrService) {
			t.Errorf("a flux of %g was accepted: %v", flux, err)
		}
	}
}

// Season and time of night are checked against the range the request
// parameters accept.
//
// The almanac and the spectrum request are two calls to one service, and
// nothing guarantees the first returns something the second will take. A flag
// outside range would otherwise travel as far as the request builder, which
// would refuse it with an error about the caller's Spec rather than about the
// almanac that produced it.
func TestAlmanacChecksTheFlagRanges(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name          string
		season, night int
	}{
		{"season high", 7, 1},
		{"season low", -1, 1},
		{"night high", 3, 4},
		{"night low", 3, -1},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			var res almanacResponse

			res.Output.Observation.SeasonFlag = c.season
			res.Output.Observation.TimeFlag = c.night
			res.Output.Sun.AveFlux = 100

			_, err := res.almanac()
			if !errors.Is(err, ErrService) {
				t.Errorf("season %d, night %d was accepted: %v", c.season, c.night, err)
			}
		})
	}
}

// An observatory the service does not have is refused before the call.
func TestAlmanacAtRefusesAnUnknownObservatory(t *testing.T) {
	t.Parallel()

	_, err := AlmanacAt(t.Context(), timeFixture(), "lapalma")
	if !errors.Is(err, ErrSpec) {
		t.Errorf("got %v, want ErrSpec", err)
	}

	if err != nil && !strings.Contains(err.Error(), "lapalma") {
		t.Errorf("the refusal does not name the observatory: %v", err)
	}
}

// timeFixture is any instant; the refusal under test happens before the date
// is used.
func timeFixture() time.GoTime {
	return time.GoDate(2020, time.June, 15, 2, 0, 0, 0, time.LocationUTC)
}
