//go:build validation

package jpl_test

import (
	"context"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/time"
)

// sofaEpoch is one fixed instant in the comparison, named so a failing
// subtest identifies itself.
//
// Fixed, because a validation result whose epoch is "now" cannot be
// reproduced: this test previously used time.NowUTC() for one of its three
// points, so every run compared a different instant and any failure would
// have been unreproducible by the person reading the log. It also meant the
// contract below was re-rolled against a fresh epoch on every run — see
// sofaContract for why that mattered more than it looks.
//
// All three epochs sit inside both reference routines' documented validity
// windows: Epv00 quotes its accuracy over 1900-2100, Moon98 over 1950-2100.
type sofaEpoch struct {
	name string
	t    time.Time
}

func sofaEpochs() []sofaEpoch {
	return []sofaEpoch{
		{"J2000", time.FromJD(2451545.0, time.TDB)},
		{"2010-06-21", time.Date(2010, 6, 21, 0, 0, 0, 0, time.LocationUTC)},
		{"2026-01-01", time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)},
	}
}

// sofaContract is the agreement bound for one body, in AU, together with the
// reason it has the value it has.
//
// # Why these numbers changed, and why the old ones were unsound
//
// This comparison is DE440 against an analytical series, so the bound that
// means anything is the *series'* own published accuracy — exceeding it says
// astrogo mis-evaluates the series or the kernel, while sitting inside it
// says nothing is wrong. gofa documents both routines directly:
//
//   - Epv00 (the Sun path), compared with JPL DE405 over 1900-2100:
//     heliocentric position error RMS 3.7 km, max 11.2 km — 7.49e-08 AU.
//   - Moon98, compared with ELP/MPP02 over 1950-2100: RMS 6.1 km,
//     worst case 31.7 km — 2.12e-07 AU.
//
// The previous tolerances were 1e-6 AU for the Sun and 1e-7 AU for the Moon,
// and both were wrong in opposite directions. 1e-6 AU is 149.6 km, thirteen
// times Epv00's published worst case, so the Sun bound could not have caught
// a real regression. 1e-7 AU is 15.0 km — **less than half** Moon98's
// published worst case of 31.7 km, so the Moon bound demanded the two agree
// twice as closely as SOFA documents its own routine to be accurate. It
// passed only because the sampled epochs happened to land in a favourable
// part of the lunar residual, and one of those epochs was time.NowUTC(),
// so that luck was re-rolled on every single run.
//
// The bounds below are the published worst cases doubled. The factor is not
// margin-for-comfort: the quoted figures are against DE405 and ELP/MPP02
// respectively, while this test compares against DE440, and the difference
// between those reference ephemerides is itself nonzero. Doubling covers it
// and keeps the bound a genuine scientific limit rather than a transcription
// of whatever this repository last measured.
type sofaContract struct {
	maxAU     float64
	rationale string
}

func contractFor(bid eph.ID) sofaContract {
	if bid == eph.Moon {
		return sofaContract{
			maxAU:     4.0e-7,
			rationale: "2x Moon98's published worst case against ELP/MPP02 (31.7 km, 1950-2100)",
		}
	}

	return sofaContract{
		maxAU:     1.5e-7,
		rationale: "2x Epv00's published max heliocentric error against DE405 (11.2 km, 1900-2100)",
	}
}

func runSOFATest(t *testing.T, bid eph.ID) {
	t.Helper()

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440")
	if err != nil {
		t.Skipf("skipping SOFA comparison: JPL provider failed: %v", err)
	}

	defer func() { _ = p.Close() }()

	sofa := eph.Default()
	contract := contractFor(bid)

	var worst float64

	for _, e := range sofaEpochs() {
		t.Run(bid.String()+"_"+e.name, func(t *testing.T) {
			jplState, err := p.State(bid, e.t)
			if err != nil {
				t.Fatalf("JPL State() failed at %s: %v", e.name, err)
			}

			sofaState, err := sofa.State(bid, e.t)
			if err != nil {
				t.Fatalf("SOFA State() failed at %s: %v", e.name, err)
			}

			posDiff := jplState.Pos.Sub(sofaState.Pos).Norm()
			if posDiff > worst {
				worst = posDiff
			}

			// The measured value is reported unconditionally, not only on
			// failure. A test that asserts a bound and prints nothing leaves
			// nobody able to say whether the bound is close to the truth or
			// an order of magnitude away from it — which is exactly how the
			// two unsound tolerances above survived.
			t.Logf("%s @ %s: |DE440 - SOFA| = %.4e AU (%.2f km)",
				bid, e.name, posDiff, posDiff*kmPerAU)

			if posDiff > contract.maxAU {
				t.Errorf("%s @ %s: %.4e AU (%.2f km) exceeds the contract %.4e AU (%.2f km)\n  contract rationale: %s",
					bid, e.name, posDiff, posDiff*kmPerAU,
					contract.maxAU, contract.maxAU*kmPerAU, contract.rationale)
			}
		})
	}

	t.Logf("%s: measured max %.4e AU (%.2f km) against contract %.4e AU (%.2f km) — %s",
		bid, worst, worst*kmPerAU, contract.maxAU, contract.maxAU*kmPerAU, contract.rationale)
}

func TestJPLStateAgainstSOFASun(t *testing.T) {
	runSOFATest(t, eph.Sun)
}

func TestJPLStateAgainstSOFAMoon(t *testing.T) {
	runSOFATest(t, eph.Moon)
}
