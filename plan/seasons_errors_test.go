package plan

import (
	"errors"
	"fmt"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// failingNearProvider fails its first call for an instant within an hour of
// around, and answers every other.
type failingNearProvider struct {
	eph.Provider

	around time.Time
	failed bool
}

func (p *failingNearProvider) State(id eph.ID, t time.Time) (core.State, error) {
	if !p.failed && t.Sub(p.around).Abs() < unit.Hours(1) {
		p.failed = true

		return core.State{}, errFailingOnce
	}

	st, err := p.Provider.State(id, t)
	if err != nil {
		return core.State{}, fmt.Errorf("failingNearProvider: %w", err)
	}

	return st, nil
}

// TestSeasonsReturnARefinementFailure: Seasons samples the Sun's longitude
// at midnight and refines each crossing, so the only evaluations within an
// hour of the 2026 vernal equinox, 14:46 UTC on March 20, are the
// refinement's. A provider that fails once there must fail the call. Seasons
// skipped the season instead and returned the other three with a nil error,
// which no caller can tell from a year missing its equinox (#453).
func TestSeasonsReturnARefinementFailure(t *testing.T) {
	equinox := time.Date(2026, 3, 20, 14, 46, 0, 0, time.LocationUTC)
	prov := &failingNearProvider{Provider: eph.Default(), around: equinox}

	seasons, err := Seasons(2026, prov)
	if !prov.failed {
		t.Fatal("no evaluation came within an hour of the equinox; the test is not reaching the refinement")
	}

	if !errors.Is(err, errFailingOnce) {
		t.Errorf("Seasons with the provider failing once in the equinox's refinement: %d seasons, err %v; want errFailingOnce",
			len(seasons), err)
	}
}
