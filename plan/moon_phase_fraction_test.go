package plan

import (
	"errors"
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestMoonElongationAgainstRealPhaseEvents cross-checks MoonElongation (and
// MoonPhaseFraction, derived from it) against MoonPhases' own independently
// root-found event times, over a real year — the elongation at each event
// must sit at that phase's own defining angle (0°/90°/180°/270°), and the
// fraction at correspondingly 0/0.25/0.5/0.75.
func TestMoonElongationAgainstRealPhaseEvents(t *testing.T) {
	prov := eph.Default()

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.AddDays(365)

	events, err := MoonPhases(start, end, prov)
	if err != nil {
		t.Fatalf("MoonPhases: %v", err)
	}

	if len(events) < 40 {
		t.Fatalf("expected a real year's worth of phase events, got %d", len(events))
	}

	wantElong := map[MoonPhase]float64{
		PhaseNewMoon:      0,
		PhaseFirstQuarter: 90,
		PhaseFullMoon:     180,
		PhaseLastQuarter:  270,
	}

	const tolDeg = 0.5 // FindRoot's own convergence tolerance, not this function's

	for _, ev := range events {
		elong, err := MoonElongation(ev.Time, prov)
		if err != nil {
			t.Fatalf("MoonElongation at %v: %v", ev.Time, err)
		}

		want := wantElong[ev.Phase]

		// Wrap-around: a NewMoon root can land at 359.9° as easily as 0.1°.
		diff := math.Mod(elong.Degrees()-want+540, 360) - 180 // in (-180, 180]
		if math.Abs(diff) > tolDeg {
			t.Errorf("%v at %v: MoonElongation = %.3f°, want ≈%.0f° (diff %.3f°)",
				ev.Phase, ev.Time, elong.Degrees(), want, diff)
		}

		frac, err := MoonPhaseFraction(ev.Time, prov)
		if err != nil {
			t.Fatalf("MoonPhaseFraction at %v: %v", ev.Time, err)
		}

		wantFrac := want / 360.0

		fracDiff := math.Mod(frac-wantFrac+1.5, 1) - 0.5
		if math.Abs(fracDiff) > tolDeg/360.0 {
			t.Errorf("%v at %v: MoonPhaseFraction = %.5f, want ≈%.2f", ev.Phase, ev.Time, frac, wantFrac)
		}
	}
}

// TestMoonPhaseFractionRange verifies MoonPhaseFraction always stays within
// [0, 1) — it is a normalized elongation/360, so it can never reach exactly
// 1.0 (that value is 0.0 of the next cycle) nor go negative.
func TestMoonPhaseFractionRange(t *testing.T) {
	prov := eph.Default()

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for i := range 60 {
		tm := start.AddDays(float64(i))

		frac, err := MoonPhaseFraction(tm, prov)
		if err != nil {
			t.Fatalf("MoonPhaseFraction at %v: %v", tm, err)
		}

		if frac < 0 || frac >= 1 {
			t.Errorf("MoonPhaseFraction(%v) = %v, want [0, 1)", tm, frac)
		}
	}
}

// TestMoonElongationDistinguishesWaxingFromWaning is the concrete case the
// issue is about: MoonIllumination's phaseAngle is symmetric about full and
// cannot tell these two dates apart (both have illumination fraction close
// to the same value), while MoonElongation — and MoonPhaseFraction — can.
func TestMoonElongationDistinguishesWaxingFromWaning(t *testing.T) {
	prov := eph.Default()

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	events, err := MoonPhases(start, start.AddDays(35), prov)
	if err != nil {
		t.Fatalf("MoonPhases: %v", err)
	}

	var fullMoon time.Time

	for _, ev := range events {
		if ev.Phase == PhaseFullMoon {
			fullMoon = ev.Time

			break
		}
	}

	if fullMoon.IsZero() {
		t.Fatal("no full moon found in the test window")
	}

	before := fullMoon.AddDays(-3)
	after := fullMoon.AddDays(3)

	// The concrete symmetry MoonElongation exists to break: MoonIllumination's
	// phaseAngle is close to identical on both sides (both close to full),
	// which is exactly why it can't answer "waxing or waning" on its own.
	_, phaseBefore, err := MoonIllumination(before, prov)
	if err != nil {
		t.Fatalf("MoonIllumination (before): %v", err)
	}

	_, phaseAfter, err := MoonIllumination(after, prov)
	if err != nil {
		t.Fatalf("MoonIllumination (after): %v", err)
	}

	if math.Abs(phaseBefore.Degrees()-phaseAfter.Degrees()) > 5 {
		t.Errorf("phaseAngle should be nearly symmetric 3 days either side of full moon; before=%.1f° after=%.1f°",
			phaseBefore.Degrees(), phaseAfter.Degrees())
	}

	elongBefore, err := MoonElongation(before, prov)
	if err != nil {
		t.Fatalf("MoonElongation (before): %v", err)
	}

	elongAfter, err := MoonElongation(after, prov)
	if err != nil {
		t.Fatalf("MoonElongation (after): %v", err)
	}

	if elongBefore.Degrees() >= 180 {
		t.Errorf("3 days before full moon should be waxing (elongation < 180°), got %.1f°", elongBefore.Degrees())
	}

	if elongAfter.Degrees() <= 180 {
		t.Errorf("3 days after full moon should be waning (elongation > 180°), got %.1f°", elongAfter.Degrees())
	}

	fracB, err := MoonPhaseFraction(before, prov)
	if err != nil {
		t.Fatalf("MoonPhaseFraction (before): %v", err)
	}

	fracA, err := MoonPhaseFraction(after, prov)
	if err != nil {
		t.Fatalf("MoonPhaseFraction (after): %v", err)
	}

	if fracB >= 0.5 {
		t.Errorf("waxing fraction should be < 0.5, got %v", fracB)
	}

	if fracA <= 0.5 {
		t.Errorf("waning fraction should be > 0.5, got %v", fracA)
	}
}

// TestMoonElongationNilProviderDefaultsToDefault verifies MoonElongation's
// nil-provider guard: a nil eph.Provider produces the exact same result as
// passing eph.Default() explicitly, not a panic or a distinct code path.
func TestMoonElongationNilProviderDefaultsToDefault(t *testing.T) {
	tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	withNil, err := MoonElongation(tm, nil)
	if err != nil {
		t.Fatalf("MoonElongation(nil): %v", err)
	}

	withDefault, err := MoonElongation(tm, eph.Default())
	if err != nil {
		t.Fatalf("MoonElongation(eph.Default()): %v", err)
	}

	if withNil.Degrees() != withDefault.Degrees() {
		t.Errorf("MoonElongation(nil) = %v, want exactly MoonElongation(eph.Default()) = %v", withNil, withDefault)
	}
}

var errMoonPhaseTestProvider = errors.New("moon_phase_fraction_test: provider always fails")

// errStateProvider is an eph.Provider whose State always fails -- the
// simplest fixture for proving MoonElongation/MoonPhaseFraction wrap and
// propagate a provider error rather than swallowing it.
type errStateProvider struct{}

func (errStateProvider) State(eph.ID, time.Time) (eph.State, error) {
	return eph.State{}, errMoonPhaseTestProvider
}

func (errStateProvider) Close() error { return nil }

// TestMoonElongationAndMoonPhaseFractionPropagateProviderError covers both
// functions' error paths: MoonElongation wraps moonElongation's own error,
// and MoonPhaseFraction (built on MoonElongation) inherits the same
// failure rather than silently producing a zero fraction.
func TestMoonElongationAndMoonPhaseFractionPropagateProviderError(t *testing.T) {
	tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	if _, err := MoonElongation(tm, errStateProvider{}); !errors.Is(err, errMoonPhaseTestProvider) {
		t.Errorf("MoonElongation: expected errMoonPhaseTestProvider, got %v", err)
	}

	if _, err := MoonPhaseFraction(tm, errStateProvider{}); !errors.Is(err, errMoonPhaseTestProvider) {
		t.Errorf("MoonPhaseFraction: expected errMoonPhaseTestProvider, got %v", err)
	}
}
