package satellite_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
)

// The error paths through the wrapper, and the type contract they carry.
//
// Each of these turns an sgp4 error into one of this package's own sentinels,
// and that mapping is the promise a caller matches on: ErrMalformedTLE means
// the text was wrong, ErrPropagation means the model could not answer. A branch
// that wraps the wrong one compiles, passes every accuracy test, and tells the
// caller something false.

// decaying is an element set whose drag term drives it into the ground within
// hours, which is the only ordinary way to make propagation fail.
//
// It is built from the ISS element set with the B* field replaced: "-11606-4"
// becomes " 50000-0", which is +0.5 in the TLE's assumed-exponent notation —
// four thousand times a real drag term. The check digits are recomputed so the
// set is well-formed, because the point is to reach the propagator rather than
// to be refused before it.
func decaying(t *testing.T) (line1, line2 string) {
	t.Helper()

	const (
		base1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		base2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	l1 := base1[:53] + " 50000-0" + base1[61:]

	// Recompute the check digit rather than hand-computing it: this test is
	// about propagation, and a wrong digit here would fail it for the wrong
	// reason.
	l1 = l1[:68] + string(rune('0'+sgp4.Checksum(l1)))

	if err := satellite.ValidateTLE(l1, base2); err != nil {
		t.Fatalf("the constructed element set is not well formed: %v", err)
	}

	return l1, base2
}

// TestPropagationFailureIsTypedAsSuch covers every path that turns an sgp4
// error into ErrPropagation.
//
// A decayed satellite is the case that reaches all of them: the model reports
// it, and the report has to travel out through State, Altitude and the ground
// track without changing kind on the way.
func TestPropagationFailureIsTypedAsSuch(t *testing.T) {
	t.Parallel()

	line1, line2 := decaying(t)

	sat, err := satellite.NewFromTLE("decaying", line1, line2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	epoch := sat.Propagator().Elements().Epoch

	// Far enough past epoch that the model has put it under the surface.
	at := epoch.AddDays(2)

	t.Run("State", func(t *testing.T) {
		t.Parallel()

		if _, err := sat.State(core.ID(0), at); !errors.Is(err, satellite.ErrPropagation) {
			t.Errorf("State returned %v, want one wrapping ErrPropagation", err)
		}
	})

	t.Run("Altitude", func(t *testing.T) {
		t.Parallel()

		if _, err := sat.Altitude(at); !errors.Is(err, satellite.ErrPropagation) {
			t.Errorf("Altitude returned %v, want one wrapping ErrPropagation", err)
		}
	})

	// And the same satellite at its own epoch still answers, so the test above
	// is measuring the failure rather than a satellite that never worked.
	if _, err := sat.State(core.ID(0), epoch); err != nil {
		t.Errorf("at its own epoch the same element set failed too (%v), so the assertions "+
			"above prove nothing about decay", err)
	}
}

// TestNewFromTLERejectsWithTheRightKind separates the two ways construction can
// fail, because they mean different things to a caller: one is a bad feed, the
// other is a model that cannot take the elements.
func TestNewFromTLERejectsWithTheRightKind(t *testing.T) {
	t.Parallel()

	const (
		good1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		good2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	// A mutation with its check digit recomputed, so the line is internally
	// consistent and only the parser can object. This is the case the checksum
	// structurally cannot catch — letters and spaces count for nothing in a
	// modulo-10 sum, so a field replaced with text can still check out — and it
	// is the reason ValidateTLE asks two questions rather than one.
	withValidChecksum := func(line string) string {
		return line[:68] + string(rune('0'+sgp4.Checksum(line)))
	}

	for _, tc := range []struct {
		name         string
		line1, line2 string
	}{
		{"a wrong check digit", good1[:68] + "0", good2},
		{"a field that is not a number", good1, good2[:8] + " XX.XXXX" + good2[16:]},
		{"a mean motion of zero", good1, good2[:52] + " 0.00000000" + good2[63:]},
		{"truncated", good1[:40], good2},
		{
			name:  "text in a numeric field, checksum recomputed to match",
			line1: good1,
			line2: withValidChecksum(good2[:8] + " XX.XXXX" + good2[16:]),
		},
		{
			name:  "an impossible eccentricity, checksum recomputed to match",
			line1: good1,
			line2: withValidChecksum(good2[:52] + " 0.00000000" + good2[63:]),
		},
	} {
		_, err := satellite.NewFromTLE("bad", tc.line1, tc.line2)
		if !errors.Is(err, satellite.ErrMalformedTLE) {
			t.Errorf("%s: NewFromTLE returned %v, want one wrapping ErrMalformedTLE",
				tc.name, err)
		}
	}
}

// TestStateRefusesAnotherBody covers the guard that made a real bug possible.
//
// A *Satellite tracks exactly one object, so any id but the zero value is a
// caller asking the wrong provider — and answering with this satellite's own
// state instead is what let plan.Satellite.ApparentMagnitudeCtx compute a Sun
// position from a satellite.
func TestStateRefusesAnotherBody(t *testing.T) {
	t.Parallel()

	const (
		line1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		line2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	sat, err := satellite.NewFromTLE("ISS", line1, line2)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := sat.State(core.Sun, sat.Propagator().Elements().Epoch); !errors.Is(err, satellite.ErrUnexpectedID) {
		t.Errorf("State(core.Sun) returned %v, want one wrapping ErrUnexpectedID", err)
	}
}

// TestOrbitalPeriodAndCloseOnAZeroValue covers the two trivial members, and the
// guard that only a zero value can reach.
//
// NewFromTLE cannot produce a non-positive mean motion — sgp4.Elements.Validate
// refuses it — so OrbitalPeriod's guard is reachable only through the exported
// zero value, which a caller can construct. It returns 0 rather than dividing
// by it.
func TestOrbitalPeriodAndCloseOnAZeroValue(t *testing.T) {
	t.Parallel()

	var zero satellite.Satellite

	if got := zero.OrbitalPeriod(); got != 0 {
		t.Errorf("a zero Satellite reports an orbital period of %v, want 0", got)
	}

	if err := zero.Close(); err != nil {
		t.Errorf("Close returned %v on a zero value", err)
	}

	const (
		line1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		line2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	sat, err := satellite.NewFromTLE("ISS", line1, line2)
	if err != nil {
		t.Fatal(err)
	}

	// 15.72 revolutions a day is a period near 92 minutes.
	if got := sat.OrbitalPeriod(); math.Abs(got-91.6) > 0.5 {
		t.Errorf("OrbitalPeriod = %v minutes, want about 91.6", got)
	}

	if err := sat.Close(); err != nil {
		t.Errorf("Close returned %v", err)
	}
}
