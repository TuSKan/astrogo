package plan_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/plan"
)

func circumpolarTestSite(t *testing.T, latDeg float64) *plan.Site {
	t.Helper()

	site, err := plan.NewSiteEarthLocation("test", latDeg, 0, 0)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	return site
}

// TestIsCircumpolarKnownCases spot-checks real, well-known circumpolar and
// never-rises pairs against a sea-level (no horizon dip) site — Polaris
// (dec ≈ +89.26°) is circumpolar from any northern site, and the same
// declination is permanently below the horizon (never up) from the
// equivalent southern latitude by symmetry.
func TestIsCircumpolarKnownCases(t *testing.T) {
	t.Parallel()

	north40 := circumpolarTestSite(t, 40)
	south40 := circumpolarTestSite(t, -40)
	equator := circumpolarTestSite(t, 0)

	polaris := angle.Deg(89.26)

	if !plan.IsCircumpolar(polaris, north40) {
		t.Error("Polaris should be circumpolar from 40°N")
	}

	if plan.IsNeverUp(polaris, north40) {
		t.Error("Polaris should not be never-up from 40°N")
	}

	if !plan.IsNeverUp(polaris, south40) {
		t.Error("Polaris-declination object should be never-up from 40°S")
	}

	if plan.IsCircumpolar(polaris, south40) {
		t.Error("Polaris-declination object should not be circumpolar from 40°S")
	}

	// At the equator, nothing is circumpolar or never-up except exactly at
	// the poles (dec = ±90°) — every other declination rises and sets.
	if plan.IsCircumpolar(angle.Deg(80), equator) {
		t.Error("dec=80° should not be circumpolar at the equator")
	}

	if plan.IsNeverUp(angle.Deg(-80), equator) {
		t.Error("dec=-80° should not be never-up at the equator")
	}
}

// TestIsCircumpolarMatchesClassicFormula cross-checks IsCircumpolar/IsNeverUp
// against the standard closed-form spherical-astronomy shortcuts, derived
// independently from the same min/max-altitude identities IsCircumpolar
// itself uses (not copied from its implementation):
//
//	circumpolar ⟺ minAlt > 0 ⟺ cos(lat+dec) < 0 ⟺ |lat+dec| > 90
//	never-up    ⟺ maxAlt < 0 ⟺ cos(lat−dec) < 0 ⟺ |lat−dec| > 90
//
// These are genuinely different conditions (dec+lat vs. dec−lat) — the
// issue's own pasted "current workaround" snippet, `|dec+lat| >= 90`, is
// named isCircumpolar and only ever claimed to detect that one case; a
// first draft of this test wrongly reused it as a combined
// circumpolar-or-never-up check and failed against dec near the pole
// opposite the site's hemisphere (e.g. lat=40°N, dec=−89°: |dec+lat|=49,
// under 90, yet the object's whole altitude range is −41° to −39° — always
// down, i.e. never-up — which only the dec−lat form catches). Skips
// combinations landing within a degree of either boundary, where the
// classic formula's non-strict >= and IsCircumpolar/IsNeverUp's strict >/<
// (an object exactly grazing the horizon at one extreme is not "always
// above it") can legitimately disagree.
func TestIsCircumpolarMatchesClassicFormula(t *testing.T) {
	t.Parallel()

	const boundaryGuard = 1.0 // degrees

	for latDeg := -80.0; latDeg <= 80; latDeg += 10 {
		site := circumpolarTestSite(t, latDeg)

		for decDeg := -89.0; decDeg <= 89; decDeg += 7 {
			sum, diff := decDeg+latDeg, latDeg-decDeg
			if math.Abs(math.Abs(sum)-90) < boundaryGuard || math.Abs(math.Abs(diff)-90) < boundaryGuard {
				continue
			}

			wantCircumpolar := math.Abs(sum) > 90
			wantNeverUp := math.Abs(diff) > 90

			gotCircumpolar := plan.IsCircumpolar(angle.Deg(decDeg), site)
			gotNeverUp := plan.IsNeverUp(angle.Deg(decDeg), site)

			if gotCircumpolar && gotNeverUp {
				t.Fatalf("lat=%.0f dec=%.0f: both circumpolar and never-up", latDeg, decDeg)
			}

			if gotCircumpolar != wantCircumpolar {
				t.Errorf("lat=%.0f dec=%.0f: IsCircumpolar = %v, want %v (|dec+lat|=%.1f)",
					latDeg, decDeg, gotCircumpolar, wantCircumpolar, math.Abs(sum))
			}

			if gotNeverUp != wantNeverUp {
				t.Errorf("lat=%.0f dec=%.0f: IsNeverUp = %v, want %v (|lat-dec|=%.1f)",
					latDeg, decDeg, gotNeverUp, wantNeverUp, math.Abs(diff))
			}
		}
	}
}

// TestIsCircumpolarNeitherForOrdinaryObject verifies the common case: a
// mid-declination object at a mid-latitude site is neither circumpolar nor
// never-up — it has a real rise and set.
func TestIsCircumpolarNeitherForOrdinaryObject(t *testing.T) {
	t.Parallel()

	site := circumpolarTestSite(t, 34) // Mauna Kea-ish latitude
	orion := angle.Deg(-5.9)           // Betelgeuse's declination, roughly

	if plan.IsCircumpolar(orion, site) {
		t.Error("Betelgeuse-declination object should not be circumpolar from 34°N")
	}

	if plan.IsNeverUp(orion, site) {
		t.Error("Betelgeuse-declination object should not be never-up from 34°N")
	}
}

// TestIsCircumpolarUsesTheAlmanacHorizon: refraction lifts an object, so
// the horizon a star rises and sets on sits 34′ below the geometric one, and
// the circumpolar zone is wider than the geometry alone gives (#568). At 60°N
// a declination of 29.6° dips just under the geometric horizon at lower
// culmination: circumpolar on the almanac horizon, the default, and not on
// the geometric one.
func TestIsCircumpolarUsesTheAlmanacHorizon(t *testing.T) {
	t.Parallel()

	site := circumpolarTestSite(t, 60)
	dec := angle.Deg(29.6)

	if plan.IsCircumpolar(dec, site, plan.WithHorizonAltitude(angle.Zero())) {
		t.Fatalf("test fixture assumption broken: dec=%v should dip below the geometric horizon", dec)
	}

	if !plan.IsCircumpolar(dec, site) {
		t.Error("dec 29.6° at 60°N stays above the almanac horizon and should be circumpolar by default")
	}

	// And it narrows the never-up zone: dec −30.4° culminates at −0.4°,
	// under the geometric horizon but above the almanac one.
	south := angle.Deg(-30.4)

	if !plan.IsNeverUp(south, site, plan.WithHorizonAltitude(angle.Zero())) {
		t.Fatalf("test fixture assumption broken: dec=%v should stay under the geometric horizon", south)
	}

	if plan.IsNeverUp(south, site) {
		t.Error("dec −30.4° at 60°N clears the almanac horizon and should not be never-up by default")
	}
}

// TestIsCircumpolarWithHorizonAltitude verifies a caller-supplied minimum
// altitude overrides the site's own horizon entirely: a normally-circumpolar
// object stops being circumpolar against a high enough obstruction.
func TestIsCircumpolarWithHorizonAltitude(t *testing.T) {
	t.Parallel()

	site := circumpolarTestSite(t, 70)
	polaris := angle.Deg(89.26)

	if !plan.IsCircumpolar(polaris, site) {
		t.Fatal("Polaris should be circumpolar from 70°N by default")
	}

	// Polaris's minimum altitude from 70°N is dec-(90-lat) = 89.26-20 =
	// 69.26° — an obstruction higher than that defeats its circumpolarity.
	if plan.IsCircumpolar(polaris, site, plan.WithHorizonAltitude(angle.Deg(75))) {
		t.Error("a 75° horizon obstruction should defeat Polaris's circumpolarity from 70°N (min altitude there is 69.26°)")
	}
}

// TestIsCircumpolarPoles verifies the degenerate declination ±90° case: the
// celestial pole itself is circumpolar (or never-up) from literally
// everywhere except the equator, where its altitude is exactly the
// latitude (0°) at every hour angle — neither strictly above nor below a
// zero threshold.
func TestIsCircumpolarPoles(t *testing.T) {
	t.Parallel()

	northPole := angle.Deg(90)

	for _, latDeg := range []float64{-89, -45, -1, 1, 45, 89} {
		site := circumpolarTestSite(t, latDeg)

		wantCircumpolar := latDeg > 0
		wantNeverUp := latDeg < 0

		if got := plan.IsCircumpolar(northPole, site); got != wantCircumpolar {
			t.Errorf("lat=%.0f: IsCircumpolar(northPole) = %v, want %v", latDeg, got, wantCircumpolar)
		}

		if got := plan.IsNeverUp(northPole, site); got != wantNeverUp {
			t.Errorf("lat=%.0f: IsNeverUp(northPole) = %v, want %v", latDeg, got, wantNeverUp)
		}
	}
}
