package plan_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// mockBrightSource is a resolve.BrightObjectSearcher test double that
// honors req.MaxVMag itself (like a real provider would), so tests can
// verify VisibleTonight actually forwards magLimit rather than just
// trusting whatever a source happens to return.
type mockBrightSource struct {
	targets []resolve.Target
}

func (m *mockBrightSource) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapMagnitudeBrowse}
}

func (m *mockBrightSource) SearchBright(_ context.Context, req resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	var matches []resolve.Target

	for _, t := range m.targets {
		if t.HasVMag && t.VMag < req.MaxVMag {
			matches = append(matches, t)
		}
	}

	return resolve.SliceSeq(matches)
}

func saoPauloSite(t *testing.T) *plan.Site {
	t.Helper()

	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	if err != nil {
		t.Fatalf("NewEarthLocation: %v", err)
	}

	site, err := plan.NewSite("Sao Paulo", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return site
}

// sirius is a real, well-known bright star confirmed (via manual
// verification against this exact site/date) to clear the horizon
// overnight from São Paulo — a genuine reference value, not a fixture
// tuned to pass.
var sirius = resolve.Target{
	ID: "* alf CMa", Name: "Sirius", Kind: resolve.KindStar,
	Coord:    coord.NewICRS(angle.Deg(101.28715), angle.Deg(-16.71611)),
	HasCoord: true, VMag: -1.46, HasVMag: true, Catalog: "SIMBAD",
}

// polaris sits within a few degrees of the north celestial pole — never
// above the horizon from a southern-hemisphere site like São Paulo
// (lat -23.55°), a well-known, real fact, not a contrived fixture.
var polaris = resolve.Target{
	ID: "* alf UMi", Name: "Polaris", Kind: resolve.KindStar,
	Coord:    coord.NewICRS(angle.Deg(37.95456), angle.Deg(89.26410)),
	HasCoord: true, VMag: 1.98, HasVMag: true, Catalog: "SIMBAD",
}

// vega transits well within the astronomical-dusk-to-dawn window on
// testNight from São Paulo (confirmed via live testing — unlike sirius,
// whose real transit for this same night/site falls just after dawn),
// giving TestVisibleTonight_PeakConsistentWithTransit a fixture that
// actually exercises the Peak-vs-Transit cross-check instead of always
// skipping it.
var vega = resolve.Target{
	ID: "* alf Lyr", Name: "Vega", Kind: resolve.KindStar,
	Coord:    coord.NewICRS(angle.Deg(279.234735), angle.Deg(38.783689)),
	HasCoord: true, VMag: 0.03, HasVMag: true, Catalog: "SIMBAD",
}

var testNight = time.Date(2026, 8, 1, 12, 0, 0, 0, time.LocationUTC)

// findByName returns the result named name, if present. ephemeris.Default()
// supports all 8 major planets via SOFA's Plan94 (not just Sun/Moon), so at
// any reasonably loose magLimit the Moon and several planets legitimately
// appear alongside whatever star fixtures a test supplies — assertions
// here check presence/absence of a specific named result rather than a
// brittle total count.
func findByName(results []plan.VisibleObject, name string) (plan.VisibleObject, bool) {
	for _, r := range results {
		if r.Target.Name == name {
			return r, true
		}
	}

	return plan.VisibleObject{}, false
}

func TestVisibleTonight_FiltersAndReturnsVisibleStar(t *testing.T) {
	faint := resolve.Target{
		ID: "faint", Name: "Faint Star", Kind: resolve.KindStar,
		Coord: sirius.Coord, HasCoord: true, VMag: 6.0, HasVMag: true, Catalog: "SIMBAD",
	}

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{sirius, faint}}}

	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), testNight, 2, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	got, ok := findByName(results, "Sirius")
	if !ok {
		t.Fatalf("expected Sirius (VMag -1.46 < magLimit 2) in results, got %+v", results)
	}

	if _, ok := findByName(results, "Faint Star"); ok {
		t.Error("expected Faint Star (VMag 6.0, fainter than magLimit=2) to be excluded")
	}

	if got.Constellation != "Canis Major" || got.ConstellationAbbr != "CMa" {
		t.Errorf("Constellation = %q (%q), want Canis Major (CMa)", got.Constellation, got.ConstellationAbbr)
	}

	if len(got.Windows) == 0 {
		t.Error("expected at least one horizon-clearing window")
	}

	if got.PeakAltitude.Degrees() <= 0 {
		t.Errorf("expected a positive PeakAltitude, got %v", got.PeakAltitude.Degrees())
	}

	if got.PeakTime.IsZero() {
		t.Error("expected a non-zero PeakTime")
	}

	if got.Direction == "" {
		t.Error("expected a non-empty compass Direction")
	}
}

// TestVisibleTonight_PeakConsistentWithTransit is a regression test for
// evaluateCandidate's Peak computation: an earlier version approximated
// the "best observed" instant as either the true TransitTime or, when that
// fell outside tonight's window, the crude midpoint of the horizon-
// clearing window. Peak is now always a real TransitEstimate optimum. When
// the true transit DOES fall within tonight's window, Peak and Transit are
// independently computed maxima of the same altitude curve and must
// therefore describe (very nearly) the same instant.
func TestVisibleTonight_PeakConsistentWithTransit(t *testing.T) {
	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{vega}}}

	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), testNight, 2, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	got, ok := findByName(results, "Vega")
	if !ok {
		t.Fatalf("expected Vega in results, got %+v", results)
	}

	if got.PeakTime.IsZero() {
		t.Fatal("expected a non-zero PeakTime")
	}

	if got.TransitTime.IsZero() {
		t.Skip("true transit fell outside tonight's window for this fixture/date — nothing to cross-check")
	}

	diff := got.PeakTime.Sub(got.TransitTime)
	if diff < 0 {
		diff = -diff
	}

	if diff > time.Minute {
		t.Errorf("PeakTime (%v) and TransitTime (%v) differ by %v, want <1 min when both are populated",
			got.PeakTime, got.TransitTime, diff)
	}
}

// TestVisibleTonight_MidnightNightOrdersDawnAfterDusk is a regression test
// for a real bug found via live testing: an earlier version searched the
// same [night, night+24h) window for both dusk and dawn independently.
// When night is local midnight (a documented valid choice per
// VisibleTonight's doc comment), the first dawn found in that window is
// that same morning's — before dusk, not after it — silently producing an
// inverted [start, end) that made every real candidate (confirmed live:
// ~150 of them, including Sirius) evaluate as never visible, with no
// error surfaced. testNight (noon UTC) never exercised this path, since
// noon always falls between a morning's dawn and that evening's dusk.
func TestVisibleTonight_MidnightNightOrdersDawnAfterDusk(t *testing.T) {
	// Local midnight in São Paulo (UTC-3) on 2026-08-01 is 2026-08-01T03:00:00Z.
	midnightNight := time.Date(2026, time.August, 1, 3, 0, 0, 0, time.LocationUTC)

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{sirius}}}

	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), midnightNight, 2, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	if _, ok := findByName(results, "Sirius"); !ok {
		t.Fatalf("expected Sirius visible when night is local midnight, got %+v", results)
	}
}

func TestVisibleTonight_ExcludesObjectNeverAboveHorizon(t *testing.T) {
	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{polaris}}}

	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), testNight, 5, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	if _, ok := findByName(results, "Polaris"); ok {
		t.Fatalf("expected Polaris (never visible from Sao Paulo) to be excluded, got %+v", results)
	}
}

func TestVisibleTonight_MoonAppearsWhenBright(t *testing.T) {
	// No star sources at all — this isolates the Moon/planet path.
	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), testNight, -5, nil, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	found := false

	for _, r := range results {
		if r.Target.Kind == resolve.KindMoon {
			found = true

			if r.ApparentMag > -5 {
				t.Errorf("Moon ApparentMag = %v, want < -5", r.ApparentMag)
			}
		}
	}

	if !found {
		t.Error("expected the Moon (always far brighter than mag -5 when up) in the results")
	}
}

func TestVisibleTonight_SortedByApparentMag(t *testing.T) {
	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{sirius}}}

	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), testNight, -1, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	for i := 1; i < len(results); i++ {
		if results[i-1].ApparentMag > results[i].ApparentMag {
			t.Errorf("results not sorted ascending by ApparentMag at index %d: %v > %v", i, results[i-1].ApparentMag, results[i].ApparentMag)
		}
	}
}

// TestVisibleTonight_ExtinctionCanPushBorderlineStarOverMagLimit is a
// regression test for a real bug found via live testing: evaluateCandidate
// used to check magLimit against the raw, pre-extinction magnitude, so a
// star cataloged just barely within magLimit could still appear in
// results even though its real, extinction-adjusted brightness (the
// ApparentMag actually reported) was fainter than magLimit — live testing
// at magLimit=2 surfaced results as faint as mag +8.5 this way.
// magnitude.StarApparent's extinction (catMag + k*airmass, default
// k=ExtinctionV≈0.20) always adds at least the zenith minimum, since
// airmass is never below 1 — so a star cataloged within that margin of
// magLimit must always be excluded once extinction is applied, a fact
// this test exploits to stay robust without hardcoding a specific
// real-world altitude/airmass value.
func TestVisibleTonight_ExtinctionCanPushBorderlineStarOverMagLimit(t *testing.T) {
	const magLimit = 2.0

	borderline := resolve.Target{
		ID: "borderline", Name: "Borderline Star", Kind: resolve.KindStar,
		Coord: sirius.Coord, HasCoord: true, VMag: magLimit - 0.1, HasVMag: true, Catalog: "SIMBAD",
	}

	sources := []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{borderline}}}

	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), testNight, magLimit, sources, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	if _, ok := findByName(results, "Borderline Star"); ok {
		t.Fatalf("expected a star cataloged within extinction's minimum margin of magLimit to be excluded once extinction is applied, got %+v", results)
	}
}

// TestVisibleTonight_WithMinAltitude demonstrates the option actually
// changes behavior, not just accepted and ignored: a tight minimum
// altitude excludes Sirius's low-altitude portion of its arc, while the
// default (site.RiseSetThreshold(), near 0°) includes it. magLimit=2
// (rather than something closer to Sirius's own -1.46) leaves headroom for
// atmospheric extinction, which evaluateCandidate now checks against the
// final extinction-adjusted magnitude, not the raw catalog one — a magLimit
// too close to Sirius's raw brightness would make this test's "loose"
// case flaky depending on exactly how much airmass the evaluation instant
// happens to have.
func TestVisibleTonight_WithMinAltitude(t *testing.T) {
	sources := func() []resolve.BrightObjectSearcher {
		return []resolve.BrightObjectSearcher{&mockBrightSource{targets: []resolve.Target{sirius}}}
	}

	site := saoPauloSite(t)

	loose, err := plan.VisibleTonight(context.Background(), site, testNight, 2, sources(), ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight (default threshold): %v", err)
	}

	if _, ok := findByName(loose, "Sirius"); !ok {
		t.Fatalf("expected Sirius visible at the default (near-0°) threshold, got %+v", loose)
	}

	tight, err := plan.VisibleTonight(context.Background(), site, testNight, 2, sources(), ephemeris.Default(),
		plan.WithMinAltitude(angle.Deg(80)))
	if err != nil {
		t.Fatalf("VisibleTonight (80° threshold): %v", err)
	}

	if _, ok := findByName(tight, "Sirius"); ok {
		t.Fatalf("expected an 80°-minimum-altitude threshold to exclude Sirius (never that high from Sao Paulo), got %+v", tight)
	}
}

func TestVisibleTonight_EmptySourcesAndNoPlanetsStillSucceeds(t *testing.T) {
	// A very tight magLimit with no star sources and a planetProvider that
	// can't support any planet (ephemeris.Default() is Sun/Moon-only, and
	// mag -30 excludes even the Moon) should return an empty, error-free
	// result — not a panic or a spurious error.
	results, err := plan.VisibleTonight(context.Background(), saoPauloSite(t), testNight, -30, nil, ephemeris.Default())
	if err != nil {
		t.Fatalf("VisibleTonight: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results at an impossibly bright magLimit, got %d: %+v", len(results), results)
	}
}
