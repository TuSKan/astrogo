package skybrightness_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// A NaN or infinite coordinate failed every comparison in the table's
// bracketing, which landed on its first cell, 0 degrees from the Sun, with a NaN
// weight. At low latitude that cell is blank, so the answer was the
// solar-vicinity error; elsewhere it was a NaN brightness with no error at all
// (#998). Both must now be ErrZodiacalGeometry, with no NaN returned.
func TestZodiacalBrightnessRefusesANonFiniteGeometry(t *testing.T) {
	t.Parallel()

	nan, inf := math.NaN(), math.Inf(1)
	grid := skybrightness.DefaultOpticalGrid()

	for _, g := range []struct {
		name     string
		lon, lat float64
	}{
		{"longitude NaN at latitude 80", nan, 80},
		{"longitude NaN at latitude 0", nan, 0},
		{"latitude NaN at longitude 90", 90, nan},
		{"latitude NaN at longitude 180", 180, nan},
		{"longitude +Inf", inf, 30},
		{"latitude -Inf", 90, -inf},
	} {
		geom := skybrightness.ZodiacalGeometry{
			DifferentialLongitude: angle.Deg(g.lon),
			EclipticLatitude:      angle.Deg(g.lat),
			SunDistance:           unit.AU(1),
		}

		v, err := skybrightness.ZodiacalBrightnessAt(geom)
		if !errors.Is(err, skybrightness.ErrZodiacalGeometry) || math.IsNaN(v) {
			t.Errorf("ZodiacalBrightnessAt, %s: (%v, %v), want ErrZodiacalGeometry and no NaN", g.name, v, err)
		}

		if _, err := skybrightness.ZodiacalRadiance(skybrightness.NewSpectralRadiance(grid), grid, geom); !errors.Is(err, skybrightness.ErrZodiacalGeometry) {
			t.Errorf("ZodiacalRadiance, %s: err %v, want ErrZodiacalGeometry", g.name, err)
		}
	}

	// The control: a finite direction outside the solar vicinity is answered.
	v, err := skybrightness.ZodiacalBrightnessAt(skybrightness.ZodiacalGeometry{
		DifferentialLongitude: angle.Deg(90), EclipticLatitude: angle.Deg(30), SunDistance: unit.AU(1),
	})
	if err != nil || !(v > 0) {
		t.Errorf("ZodiacalBrightnessAt(90, 30) = (%v, %v), want a positive brightness", v, err)
	}
}

// ZodiacalLight reported every ErrZodiacalGeometry as the solar-vicinity gap:
// zero radiance, ExtrapolatedModel, no error. So a NaN or infinite direction
// passed for one beside the Sun, and an estimate quietly lost its brightest
// natural term (#998). It must now refuse such a direction.
func TestZodiacalLightRefusesANonFiniteDirection(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	z := skybrightness.NewZodiacalLight()

	nan, inf := math.NaN(), math.Inf(1)

	for name, dir := range map[string]coord.AltAz{
		"altitude NaN":  coord.NewAltAz(angle.Deg(nan), angle.Deg(90)),
		"azimuth NaN":   coord.NewAltAz(angle.Deg(45), angle.Deg(nan)),
		"altitude +Inf": coord.NewAltAz(angle.Deg(inf), angle.Deg(90)),
		"azimuth +Inf":  coord.NewAltAz(angle.Deg(45), angle.Deg(inf)),
	} {
		c, err := z.AddRadiance(context.Background(), skybrightness.NewSpectralRadiance(grid), grid, dir, scene)
		if err == nil {
			t.Errorf("%s: no error, flags %v; want the direction refused, not reported as beside the Sun", name, c.Flags)
		}
	}
}

// The one geometry error ZodiacalLight may still answer with no contribution
// is the gap it is documented to have: a direction inside the solar vicinity
// the table leaves blank. Looked at straight on, the Sun is 0 degrees from
// itself.
func TestZodiacalLightBesideTheSunIsNoContribution(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := auditScene(t)
	// Local noon at the audit site, so the Sun is above the horizon.
	scene.Time = time.GoDate(2026, 3, 20, 17, 0, 0, 0, time.LocationUTC)

	at := time.FromGo(scene.Time)
	ctx := coord.NewContext(at, scene.Observer, scene.Atmosphere.Refraction())

	sun, err := eph.At(scene.Ephemeris, at).Apparent(eph.Sun, eph.Site(ctx))
	if err != nil {
		t.Fatalf("Apparent Sun: %v", err)
	}

	dir := ctx.TopocentricToObserved(sun.Pos)
	if dir.Alt().Degrees() <= 0 {
		t.Fatalf("the Sun is at altitude %v, want it up for this test", dir.Alt())
	}

	dst := skybrightness.NewSpectralRadiance(grid)

	c, err := skybrightness.NewZodiacalLight().AddRadiance(context.Background(), dst, grid, dir, scene)
	if err != nil {
		t.Fatalf("looking at the Sun: %v, want no contribution and no error", err)
	}

	if c.Flags&skybrightness.ExtrapolatedModel == 0 {
		t.Errorf("looking at the Sun: flags %v, want ExtrapolatedModel", c.Flags)
	}

	for i, v := range dst {
		if v != 0 {
			t.Fatalf("looking at the Sun: bin %d holds %g, want no contribution", i, v)
		}
	}
}
