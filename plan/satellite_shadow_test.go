package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// skyfieldShadowTransition is one instant at which Skyfield 1.55's
// is_sunlit changed for the ISS element set issLine1/issLine2, found by
// find_discrete to 0.1 ms with the Sun from de440s:
//
//	sat = EarthSatellite(issLine1, issLine2, 'ISS', ts)
//	f = lambda t: sat.at(t).is_sunlit(eph); f.step_days = 1/1440
//	find_discrete(ts.utc(2026,4,19,12), ts.utc(2026,4,20,12), f, epsilon=1e-4/86400)
//
// sunlit is the state the satellite enters.
type skyfieldShadowTransition struct {
	day, hour, minute, second int
	millisecond               float64
	sunlit                    bool
}

var skyfieldISSShadow = []skyfieldShadowTransition{
	{19, 12, 54, 30, 705.589, false},
	{19, 13, 30, 21, 927.958, true},
	{19, 14, 27, 28, 832.927, false},
	{19, 15, 3, 20, 643.344, true},
	{19, 16, 0, 26, 951.373, false},
	{19, 16, 36, 19, 332.658, true},
	{19, 17, 33, 25, 61.170, false},
	{19, 18, 9, 17, 996.263, true},
	{19, 19, 6, 23, 162.437, false},
	{19, 19, 42, 16, 634.281, true},
	{19, 20, 39, 21, 255.416, false},
	{19, 21, 15, 15, 246.911, true},
	{19, 22, 12, 19, 340.267, false},
	{19, 22, 48, 13, 834.274, true},
	{19, 23, 45, 17, 417.233, false},
	{20, 0, 21, 12, 396.653, true},
	{20, 1, 18, 15, 486.435, false},
	{20, 1, 54, 10, 934.168, true},
	{20, 2, 51, 13, 548.152, false},
	{20, 3, 27, 9, 446.939, true},
	{20, 4, 24, 11, 602.548, false},
	{20, 5, 0, 7, 935.249, true},
	{20, 5, 57, 9, 649.781, false},
	{20, 6, 33, 6, 399.218, true},
	{20, 7, 30, 7, 690.095, false},
	{20, 8, 6, 4, 839.046, true},
	{20, 9, 3, 5, 723.610, false},
	{20, 9, 39, 3, 254.896, true},
	{20, 10, 36, 3, 750.606, false},
	{20, 11, 12, 1, 646.928, true},
}

// TestSatelliteShadowAgreesWithSkyfield finds every shadow entry and exit of
// the ISS over a day and holds each to Skyfield's. Until #562 astrogo had no
// shadow test at all, and gave the ISS a magnitude of −3.9 on a pass spent
// entirely in Earth's shadow.
func TestSatelliteShadowAgreesWithSkyfield(t *testing.T) {
	t.Parallel()

	prov, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	sun := eph.Default()

	eclipsed := func(tm time.Time) bool {
		satSt, err := prov.State(eph.ID(0), tm)
		if err != nil {
			t.Fatalf("satellite state at %v: %v", tm, err)
		}

		sunSt, err := sun.State(eph.Sun, tm)
		if err != nil {
			t.Fatalf("sun state at %v: %v", tm, err)
		}

		return satelliteEclipsed(satSt.Pos, sunSt.Pos)
	}

	type transition struct {
		at     time.Time
		sunlit bool
	}

	start := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)
	end := time.Date(2026, 4, 20, 12, 0, 0, 0, time.LocationUTC)
	step := unit.Seconds(60)

	var found []transition

	prev, prevIn := start, eclipsed(start)

	for tm := start.Add(step); !tm.After(end); tm = tm.Add(step) {
		in := eclipsed(tm)
		if in == prevIn {
			prev = tm

			continue
		}

		lo, hi := prev, tm
		for hi.Sub(lo).Seconds() > 1e-4 {
			mid := lo.Add(hi.Sub(lo) / 2)
			if eclipsed(mid) == prevIn {
				lo = mid
			} else {
				hi = mid
			}
		}

		found = append(found, transition{at: hi, sunlit: !in})
		prev, prevIn = tm, in
	}

	if len(found) != len(skyfieldISSShadow) {
		t.Fatalf("found %d shadow transitions, Skyfield finds %d", len(found), len(skyfieldISSShadow))
	}

	worst := 0.0

	for i, want := range skyfieldISSShadow {
		wantAt := time.Date(2026, 4, want.day, want.hour, want.minute, want.second,
			int(math.Round(want.millisecond*1e6)), time.LocationUTC)

		if found[i].sunlit != want.sunlit {
			t.Errorf("transition %d at %v: entering sunlit=%v, Skyfield says %v",
				i, wantAt, found[i].sunlit, want.sunlit)
		}

		// Measured within 0.35 ms: the same SGP4, a Sun a few arcseconds
		// apart, and a radius 0.4 m larger than Skyfield's ERAD. 10 ms is
		// still small against the 36 minutes of a pass in Earth's shadow.
		diff := math.Abs(found[i].at.Sub(wantAt).Seconds())
		worst = math.Max(worst, diff)

		if diff > 0.01 {
			t.Errorf("transition %d: %v, Skyfield %v, %.3f s apart", i, found[i].at, wantAt, diff)
		}
	}

	t.Logf("worst disagreement with Skyfield: %.4f s over %d transitions", worst, len(found))
}

// TestSatelliteEclipsedGeometry checks satelliteEclipsed on positions whose
// answer needs no reference: Earth's shadow is a cylinder of Earth's radius
// pointing away from the Sun, and nothing on the Sun's side of Earth is in it.
func TestSatelliteEclipsedGeometry(t *testing.T) {
	t.Parallel()

	radius := earthEquatorialRadiusKm / auKm
	sun := vector.Vec3{X: 1}

	cases := []struct {
		name string
		sat  vector.Vec3
		want bool
	}{
		{"between Earth and the Sun", vector.Vec3{X: 2 * radius}, false},
		{"directly behind Earth", vector.Vec3{X: -2 * radius}, true},
		{"far behind Earth, inside the cylinder", vector.Vec3{X: -50 * radius, Y: 0.99 * radius}, true},
		{"behind Earth, just outside the cylinder", vector.Vec3{X: -2 * radius, Z: 1.01 * radius}, false},
		{"beside Earth, over the terminator", vector.Vec3{Y: 1.01 * radius}, false},
		{"beside Earth, just behind the terminator", vector.Vec3{X: -0.2 * radius, Y: 0.99 * radius}, true},
		{"inside Earth", vector.Vec3{X: 0.5 * radius}, true},
	}

	for _, c := range cases {
		if got := satelliteEclipsed(c.sat, sun); got != c.want {
			t.Errorf("%s: eclipsed = %v, want %v", c.name, got, c.want)
		}
	}
}

// issOverSaoPaulo is the ISS with a standard magnitude, the observing site
// from #562, and a context at the given instant.
func issOverSaoPaulo(t *testing.T, tm time.Time) (*Satellite, *Site, *coord.Context) {
	t.Helper()

	prov, err := satellite.NewFromTLE("ISS (ZARYA)", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	site, err := NewSiteEarthLocation("São Paulo", -23.55, -46.63, 760)
	if err != nil {
		t.Fatalf("NewSiteEarthLocation: %v", err)
	}

	sat := NewSatellite("ISS", eph.ID(0), prov, WithStdMag(-1.3, magnitude.ConventionMcCants))

	return sat, site, coord.NewContext(tm, site.Location(), site.Refraction())
}

// TestEclipsedSatelliteHasNoMagnitude is #562's reproduction. The ISS passes
// at 79° over São Paulo at 03:07 UTC entirely in Earth's shadow, between
// Skyfield's entry at 02:51:13 and exit at 03:27:09, and was given −3.89.
// The sunlit pass at 17:17 the day before keeps its magnitude.
func TestEclipsedSatelliteHasNoMagnitude(t *testing.T) {
	t.Parallel()

	dark := time.Date(2026, 4, 20, 3, 7, 0, 0, time.LocationUTC)
	sat, _, ctx := issOverSaoPaulo(t, dark)

	m, err := sat.ApparentMagnitudeCtx(dark, ctx)
	if !errors.Is(err, ErrSatelliteEclipsed) {
		t.Errorf("ApparentMagnitudeCtx in Earth's shadow = %.2f, %v; want ErrSatelliteEclipsed", m, err)
	}

	d, err := sat.GetDetails(ctx, DetailOverrides{})
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Magnitude != "" {
		t.Errorf("details in Earth's shadow give magnitude %q, want none", d.Magnitude)
	}

	lit := time.Date(2026, 4, 19, 17, 17, 0, 0, time.LocationUTC)
	sat, _, ctx = issOverSaoPaulo(t, lit)

	m, err = sat.ApparentMagnitudeCtx(lit, ctx)
	if err != nil {
		t.Fatalf("ApparentMagnitudeCtx in sunlight: %v", err)
	}

	if m < -8 || m > 15 {
		t.Errorf("ApparentMagnitudeCtx in sunlight = %.2f, out of plausible range", m)
	}
}

// TestLimitingMagnitudeRejectsAnEclipsedSatellite holds the constraint to the
// difference between a target with no photometry, which passes, and a
// satellite in Earth's shadow, which is measured and dark: no sky is deep
// enough for it, however dark.
func TestLimitingMagnitudeRejectsAnEclipsedSatellite(t *testing.T) {
	t.Parallel()

	dark := time.Date(2026, 4, 20, 3, 7, 0, 0, time.LocationUTC)
	sat, site, ctx := issOverSaoPaulo(t, dark)

	hard := LimitingMagnitudeConstraint{Sky: fixedDepth(30), Boolean: true}

	got, err := hard.CheckCtx(sat, dark, site, ctx)
	if err != nil {
		t.Fatalf("CheckCtx: %v", err)
	}

	if got.Pass {
		t.Errorf("a satellite in Earth's shadow passed a magnitude cutoff under a 30 mag sky: %v", got)
	}

	soft := LimitingMagnitudeConstraint{Sky: fixedDepth(30)}

	score, err := soft.ScoreMultiplier(sat, ctx)
	if err != nil {
		t.Fatalf("ScoreMultiplier: %v", err)
	}

	if score != 0 {
		t.Errorf("score for a satellite in Earth's shadow is %.4f, want 0", score)
	}
}
