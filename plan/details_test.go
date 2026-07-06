package plan

import (
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

	site, err := NewSite("Test", loc, angle.Zero(), nil)
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

	site, err := NewSite("Test", loc, angle.Zero(), nil)
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

	site, err := NewSite("Test", loc, angle.Zero(), nil)
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

	site, err := NewSite("Test", loc, angle.Zero(), nil)
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
