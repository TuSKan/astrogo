package plan

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
)

// TestSolarSystemCandidatesIncludePlutoByItsMagnitude: Pluto, at V ≈ 14.5 in
// 2026, is a candidate at a telescope's limit of 16 and not at 14. Until #407
// it was a candidate at no limit at all, because its magnitude returned an
// error and the error was dropped.
func TestSolarSystemCandidatesIncludePlutoByItsMagnitude(t *testing.T) {
	at := time.Date(2026, 9, 23, 0, 0, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		magLimit float64
		want     bool
	}{
		{16, true},
		{14, false},
	} {
		var dropped skips

		candidates := gatherSolarSystemCandidates(eph.Default(), at, c.magLimit, &dropped)
		if err := dropped.err(); err != nil {
			t.Fatalf("magLimit %v: %v", c.magLimit, err)
		}

		var pluto *visibleCandidate

		for i := range candidates {
			if candidates[i].target.Name == "Pluto" {
				pluto = &candidates[i]
			}
		}

		if got := pluto != nil; got != c.want {
			t.Errorf("magLimit %v: Pluto a candidate: %v, want %v", c.magLimit, got, c.want)

			continue
		}

		if pluto != nil && pluto.target.Kind != resolve.KindDwarfPlanet {
			t.Errorf("Pluto's kind is %v, want KindDwarfPlanet", pluto.target.Kind)
		}
	}
}

var errNoPluto = errors.New("plutolessProvider: no Pluto")

// plutolessProvider answers for everything but Pluto.
type plutolessProvider struct{ eph.Provider }

func (p plutolessProvider) State(id eph.ID, t time.Time) (core.State, error) {
	if id == eph.Pluto {
		return core.State{}, errNoPluto
	}

	st, err := p.Provider.State(id, t)
	if err != nil {
		return core.State{}, fmt.Errorf("plutolessProvider: %w", err)
	}

	return st, nil
}

// TestSolarSystemCandidatesReportABodyTheyCannotEvaluate: a planet whose
// magnitude cannot be computed is reported through ErrIncomplete, by name,
// not left out as though it were too faint. The two are different answers to
// a caller, and until #407 they looked the same.
func TestSolarSystemCandidatesReportABodyTheyCannotEvaluate(t *testing.T) {
	at := time.Date(2026, 9, 23, 0, 0, 0, 0, time.LocationUTC)

	var dropped skips

	candidates := gatherSolarSystemCandidates(plutolessProvider{eph.Default()}, at, 16, &dropped)

	err := dropped.err()
	if !errors.Is(err, ErrIncomplete) || !errors.Is(err, errNoPluto) || !strings.Contains(err.Error(), "Pluto") {
		t.Fatalf("dropped.err() = %v, want ErrIncomplete naming Pluto and wrapping the provider's error", err)
	}

	// Everything else is still there: the failure costs Pluto, not the sky.
	for _, name := range []string{"Moon", "Jupiter", "Saturn", "Neptune"} {
		found := false

		for _, c := range candidates {
			if c.target.Name == name {
				found = true
			}
		}

		if !found {
			t.Errorf("%s is missing from the candidates; one body's failure must not cost the others", name)
		}
	}
}

// TestMoonCandidatesReportAMoonTheyCannotEvaluate is the planetary moons' half
// of the same rule: a kernel's provider that cannot answer costs its moons and
// names them, rather than leaving them out as though they were too faint.
func TestMoonCandidatesReportAMoonTheyCannotEvaluate(t *testing.T) {
	var marsMoons []moonSpec

	for _, m := range moonSpecs {
		if m.parent == eph.Mars {
			marsMoons = append(marsMoons, m)
		}
	}

	if len(marsMoons) == 0 {
		t.Fatal("precondition: no Mars moons in moonSpecs")
	}

	var dropped skips

	at := time.Date(2026, 9, 23, 0, 0, 0, 0, time.LocationUTC)
	if got := moonCandidates(failingProvider{}, marsMoons, at, 30, &dropped); len(got) != 0 {
		t.Errorf("%d candidates from a provider that answers nothing, want 0", len(got))
	}

	err := dropped.err()
	if !errors.Is(err, ErrIncomplete) || !errors.Is(err, errFailingProvider) {
		t.Fatalf("dropped.err() = %v, want ErrIncomplete wrapping the provider's error", err)
	}

	for _, m := range marsMoons {
		if !strings.Contains(err.Error(), m.name) {
			t.Errorf("%s is not named in %v", m.name, err)
		}
	}
}
