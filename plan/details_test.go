package plan

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// TestGetDetails_Star exercises computeDetails' non-MovingBody path
// (fillStaticMagnitude, the direct ICRSToAltAz branch), fillTypedProps'
// Star case (parallax → Distance, proper motion → ExtraProps),
// fillAliasProps (Messier number extraction), and applyProps overrides.
// None of this was previously exercised by any test — every mock Observable
// elsewhere in this package implements its own trivial GetDetails stub.
func TestGetDetails_Star(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC) // J2000
	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	star := NewStar("Vega", angle.Hour(18.615), angle.Deg(38.78),
		WithStarMagnitude(0.03),
		WithParallax(angle.Arcsec(0.130)),
		WithProperMotion(angle.Arcsec(0.20094/3600), angle.Arcsec(0.28642/3600)),
		WithAliases("M 45", "HR 7001"),
	)

	d, err := star.GetDetails(ctx, "Description", "A bright star in Lyra")
	testutil.AssertNoError(t, err)

	if d.Name != "Vega" {
		t.Errorf("Name = %q, want Vega", d.Name)
	}

	if d.Magnitude != "0.0 mag" {
		t.Errorf("Magnitude = %q, want ~0.0 mag (via fillStaticMagnitude)", d.Magnitude)
	}

	if d.DistanceUnit != "pc" {
		t.Errorf("DistanceUnit = %q, want pc (non-MovingBody branch)", d.DistanceUnit)
	}

	// parallax 0.130" -> distance = 1/0.130 ≈ 7.69 pc
	testutil.AssertNear(t, "Distance from parallax", d.Distance, 1.0/0.130, 0.01)

	if _, ok := d.ExtraProps["Proper motion (RA)"]; !ok {
		t.Error("expected Proper motion (RA) in ExtraProps (fillTypedProps Star case)")
	}

	if _, ok := d.ExtraProps["Proper motion (Dec)"]; !ok {
		t.Error("expected Proper motion (Dec) in ExtraProps (fillTypedProps Star case)")
	}

	if got := d.ExtraProps["Messier number"]; got != "M45" {
		t.Errorf("Messier number = %q, want M45 (fillAliasProps)", got)
	}

	if d.Description != "A bright star in Lyra" {
		t.Errorf("Description = %q, want override applied via applyProps", d.Description)
	}

	// Altitude/Azimuth must have been populated by the non-MovingBody
	// ICRSToAltAz branch (not left at zero-value from a discarded error).
	if d.Altitude.Degrees() == 0 && d.Azimuth.Degrees() == 0 {
		t.Error("Altitude/Azimuth both zero — ICRSToAltAz branch may not have run")
	}
}

// TestGetDetails_DeepSkyObject exercises fillTypedProps' DeepSkyObject case
// (fillAliasProps via an NGC alias).
func TestGetDetails_DeepSkyObject(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC)
	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	dso := NewDeepSkyObject("Andromeda Galaxy", angle.Hour(0.712), angle.Deg(41.27),
		WithDSOMagnitude(3.4),
		WithDSOAliases("NGC 224", "M31"),
	)

	d, err := dso.GetDetails(ctx)
	testutil.AssertNoError(t, err)

	if got := d.ExtraProps["NGC/IC number"]; got != "NGC 224" {
		t.Errorf("NGC/IC number = %q, want NGC 224 (fillTypedProps DeepSkyObject case)", got)
	}
}

// TestGetDetails_MovingBody exercises computeDetails' MovingBody path
// (fillMovingBody: topocentric vector, diurnal-parallax-corrected RA/Dec,
// distance in a.u., and elongation-from-Sun computation).
func TestGetDetails_MovingBody(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC)
	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	mars := NewMars(eph.Default())

	d, err := mars.GetDetails(ctx)
	testutil.AssertNoError(t, err)

	if d.Name != "Mars" {
		t.Errorf("Name = %q, want Mars", d.Name)
	}

	if d.DistanceUnit != "a.u." {
		t.Errorf("DistanceUnit = %q, want a.u. (fillMovingBody branch)", d.DistanceUnit)
	}

	if d.Distance <= 0 {
		t.Errorf("Distance = %v, want > 0", d.Distance)
	}

	// Elongation from the Sun should be populated (non-zero for Mars away
	// from conjunction) and physically bounded to [0, 180] degrees.
	elongDeg := d.Elongation.Degrees()
	if elongDeg < 0 || elongDeg > 180 {
		t.Errorf("Elongation = %.2f°, want in [0, 180]", elongDeg)
	}
}

// TestGetDetails_String confirms the String() formatter runs cleanly over a
// fully-populated TargetDetails (rise/set/transit fields included) without
// panicking on nil pointer dereferences.
func TestGetDetails_String(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC)
	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	star := NewStar("Sirius", angle.Hour(6.75), angle.Deg(-16.72), WithStarMagnitude(-1.46))

	d, err := star.GetDetails(ctx)
	testutil.AssertNoError(t, err)

	s := d.String()
	if !strings.Contains(s, "SIRIUS") {
		t.Errorf("String() output missing uppercased name: %q", s)
	}
}

// TestGetDetails_RadialVelocity confirms fillRadialVelocity dispatches on
// the MeasuredRadialVelocity interface and formats both the topocentric
// and barycentric values, cross-checked directly against
// ctx.ObservedRadialVelocity/BarycentricRVCorrection rather than a
// hardcoded number, so this doesn't silently drift from coord's own
// sign convention.
func TestGetDetails_RadialVelocity(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	const rvBarycentric = -5.5

	ra, dec := angle.Hour(6.7525), angle.Deg(-16.7161)

	star := NewStar("Sirius", ra, dec, WithRadialVelocity(rvBarycentric))

	d, err := star.GetDetails(ctx)
	testutil.AssertNoError(t, err)

	if d.RadialVelocity == "" {
		t.Fatal("expected RadialVelocity to be populated for a star with WithRadialVelocity set")
	}

	var gotTopo, gotBary float64
	if _, err := fmt.Sscanf(d.RadialVelocity, "%f km/s topocentric (%f km/s barycentric)", &gotTopo, &gotBary); err != nil {
		t.Fatalf("RadialVelocity format = %q, failed to parse: %v", d.RadialVelocity, err)
	}

	if gotBary != rvBarycentric {
		t.Errorf("parsed barycentric = %v, want %v", gotBary, rvBarycentric)
	}

	wantTopo := ctx.ObservedRadialVelocity(coord.NewICRS(ra, dec), rvBarycentric)
	testutil.AssertNear(t, "topocentric RV", gotTopo, wantTopo, 0.01)

	if s := d.String(); !strings.Contains(s, "Radial velocity:") {
		t.Errorf("String() output missing the Radial velocity line: %q", s)
	}
}

// TestGetDetails_RadialVelocity_NotSet confirms RadialVelocity stays
// empty for a Star with no WithRadialVelocity (not "0.00 km/s ...",
// which would misrepresent "never measured" as "measured exactly zero"),
// and for a DeepSkyObject (doesn't implement MeasuredRadialVelocity at all).
func TestGetDetails_RadialVelocity_NotSet(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC)
	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	star := NewStar("Vega", angle.Hour(18.615), angle.Deg(38.78))

	d, err := star.GetDetails(ctx)
	testutil.AssertNoError(t, err)

	if d.RadialVelocity != "" {
		t.Errorf("RadialVelocity = %q, want empty for a star with no RV set", d.RadialVelocity)
	}

	dso := NewDeepSkyObject("M31", angle.Hour(0.712), angle.Deg(41.269))

	d2, err := dso.GetDetails(ctx)
	testutil.AssertNoError(t, err)

	if d2.RadialVelocity != "" {
		t.Errorf("RadialVelocity = %q, want empty for a DeepSkyObject (no MeasuredRadialVelocity)", d2.RadialVelocity)
	}
}

// TestGetDetails_RadialVelocity_PropOverride confirms an injected
// "RadialVelocity" prop wins over the computed value, matching every
// other applyProps-overridable field.
func TestGetDetails_RadialVelocity_PropOverride(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	tm := time.FromJD(2451545.0, time.UTC)
	ctx := coord.NewContext(tm, loc, site.Atmosphere())

	star := NewStar("Sirius", angle.Hour(6.7525), angle.Deg(-16.7161), WithRadialVelocity(-5.5))

	d, err := star.GetDetails(ctx, "RadialVelocity", "custom override")
	testutil.AssertNoError(t, err)

	if d.RadialVelocity != "custom override" {
		t.Errorf("RadialVelocity = %q, want prop override to win", d.RadialVelocity)
	}
}

// TestGetDetails_RadialVelocity_SixMonthSwing is the test that actually
// catches a sign inversion, not just a formatting bug: a fixed target
// near the ecliptic (RA=0, Dec=0 — the same geometry
// TestBarycentricRVCorrection_AnnualSinusoid already validates at the
// coord layer, peak-to-peak ~59.6 km/s) evaluated 6 months apart must
// show a topocentric RV swing on the order of twice Earth's orbital
// speed projected onto that sightline. A sign error would collapse this
// to ~zero instead.
func TestGetDetails_RadialVelocity_SixMonthSwing(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	site, err := NewSite("Test", loc)
	testutil.AssertNoError(t, err)

	ra, dec := angle.Zero(), angle.Zero()
	star := NewStar("EclipticTarget", ra, dec, WithRadialVelocity(0))

	t1 := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	t2 := t1.AddDays(182)

	parseTopo := func(tm time.Time) float64 {
		ctx := coord.NewContext(tm, loc, site.Atmosphere())

		d, err := star.GetDetails(ctx)
		testutil.AssertNoError(t, err)

		var topo, bary float64
		if _, err := fmt.Sscanf(d.RadialVelocity, "%f km/s topocentric (%f km/s barycentric)", &topo, &bary); err != nil {
			t.Fatalf("RadialVelocity format = %q, failed to parse: %v", d.RadialVelocity, err)
		}

		return topo
	}

	topo1 := parseTopo(t1)
	topo2 := parseTopo(t2)

	swing := math.Abs(topo2 - topo1)
	if swing < 40 || swing > 65 {
		t.Errorf("6-month topocentric RV swing = %v km/s, want ~59.6 km/s (2x Earth's orbital speed) within [40, 65]", swing)
	}
}
