package skybrightness

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/time"
)

// paranalMoonScene is Paranal on 2026 March 1 at hh:mm UTC, through air that
// does not refract, so the Moon's altitude is the geometric one Skyfield's
// altaz() gives with no refraction asked for.
func paranalMoonScene(t *testing.T, hh, mm int) *Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	air, err := atmosphere.NewBuilder().Refraction(atmosphere.RefractionNone{}, 0, 0.55).Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	return &Scene{
		Observer:   loc,
		Time:       time.GoDate(2026, 3, 1, hh, mm, 0, 0, time.LocationUTC),
		Atmosphere: air,
		Ephemeris:  eph.Default(),
	}
}

// flatMoonlight is a ScatteredMoonlight on a flat solar spectrum, enough to
// resolve its geometry.
func flatMoonlight(t *testing.T) *ScatteredMoonlight {
	t.Helper()

	solar := make([]float64, len(magnitude.ROLOBands()))
	for i := range solar {
		solar[i] = 1
	}

	m, err := NewScatteredMoonlight(solar)
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	return m
}

// TestMoonlightPlacesTheMoonWhereTheSiteSeesIt: ScatteredMoonlight placed the
// Moon from the Earth's center, as a star is placed, which put it up to its
// horizontal parallax, about 0.95°, above where the site sees it (#646).
// Skyfield 1.55 over DE440s, from the same site with no refraction, puts the
// Moon at altitude 9.42713° and azimuth 296.52343° at 07:46 UTC, where this
// agrees within 0.002°. The geocentric direction put it at 10.391°, and
// charged its beam 5.38 airmasses where the site's is 5.90.
func TestMoonlightPlacesTheMoonWhereTheSiteSeesIt(t *testing.T) {
	t.Parallel()

	geom, err := flatMoonlight(t).computeGeometry(paranalMoonScene(t, 7, 46))
	if err != nil {
		t.Fatalf("computeGeometry: %v", err)
	}

	alt, az := geom.direction.Alt().Degrees(), geom.direction.Az().Degrees()
	t.Logf("Moon at altitude %.5f°, azimuth %.5f°", alt, az)

	if math.Abs(alt-9.42713) > 0.01 || math.Abs(az-296.52343) > 0.01 {
		t.Errorf("Moon at (%.5f°, %.5f°), want Skyfield's (9.42713°, 296.52343°) within 0.01°", alt, az)
	}

	want, err := atmosphere.Airmass(angle.Deg(9.42713))
	if err != nil {
		t.Fatalf("Airmass: %v", err)
	}

	if math.Abs(geom.airmass-want) > 0.01 {
		t.Errorf("Moon's airmass %.4f, want %.4f, the site's", geom.airmass, want)
	}
}

// TestMoonJustBelowTheSitesHorizonLightsNothing: at 08:35 UTC the Moon is
// 0.34° below Paranal's horizon (Skyfield), though its geocentric direction is
// still 0.64° above it. Placed from the Earth's center, it lit the sky.
func TestMoonJustBelowTheSitesHorizonLightsNothing(t *testing.T) {
	t.Parallel()

	scene := paranalMoonScene(t, 8, 35)

	// The premise: placed as a star, from the Earth's center, it is up.
	at := time.FromGo(scene.Time)

	state, err := scene.Ephemeris.State(eph.Moon, at)
	if err != nil {
		t.Fatalf("State: %v", err)
	}

	icrs, err := eph.ToICRS(state.Pos)
	if err != nil {
		t.Fatalf("ToICRS: %v", err)
	}

	geocentric, err := coord.NewContext(at, scene.Observer, scene.Atmosphere.Refraction()).ICRSToAltAz(icrs)
	if err != nil {
		t.Fatalf("ICRSToAltAz: %v", err)
	}

	if geocentric.Alt().Degrees() <= 0 {
		t.Fatalf("geocentric Moon at %.3f°, the premise of this test", geocentric.Alt().Degrees())
	}

	m := flatMoonlight(t)

	geom, err := m.computeGeometry(scene)
	if err != nil {
		t.Fatalf("computeGeometry: %v", err)
	}

	if geom.aboveHorizon {
		t.Errorf("Moon at %.3f° counted above the horizon", geom.direction.Alt().Degrees())
	}

	grid := DefaultOpticalGrid()
	dst := NewSpectralRadiance(grid)

	if _, err := m.AddRadiance(context.Background(), dst, grid, coord.NewAltAz(angle.Deg(30), angle.Deg(290)), scene); err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	for i, v := range dst {
		if v != 0 {
			t.Fatalf("a Moon below the site's horizon lit band %d: %g", i, v)
		}
	}
}
