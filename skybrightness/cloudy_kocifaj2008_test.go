package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
)

// kocifaj2008Scene is the cloudless atmosphere of Kocifaj (2008), Appl. Opt.
// 47, 792, §3, which runs "in accordance with" Kocifaj (2007), the model
// CloudySkyglow implements. Its parameters, read from the paper:
//
//   - molecular scale height 8 km. This package derives it from the surface
//     temperature, and 273.3 K gives 8.0 km;
//   - aerosol extinction falling at 0.65 per km, a scale height of 1538 m;
//   - aerosol asymmetry 0.90 and single-scattering albedo 0.85;
//   - aerosol optical depth 0.5 at 500 nm, falling as lambda^-1, an Angstrom
//     exponent of 1;
//   - molecular optical depth falling as lambda^-4.09, which the Rayleigh
//     scattering here reproduces at sea-level pressure;
//   - Q = q = 0.15, cloudyCity's Garstang emission.
//
// The paper weights 450-750 nm by photopic efficiency; radianceAt reads the
// 554 nm grid point, near that weighting's peak.
func kocifaj2008Scene(tb testing.TB) *skybrightness.Scene {
	tb.Helper()

	air, err := atmosphere.NewBuilder().
		Surface(1013.25, 273.3).
		Aerosol(0.5, 500, 1.0, 0.85, 0.90).
		AerosolScaleHeight(1538).
		Build()
	if err != nil {
		tb.Fatalf("atmosphere Build: %v", err)
	}

	observer, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2000)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   observer,
		Time:       time.GoDate(2026, 3, 20, 3, 0, 0, 0, time.LocationUTC),
		Atmosphere: air,
	}
}

// TestKocifaj2008SceneIsThePapers holds the scene to the paper's numbers where
// they reach this package as derived quantities: the molecular scale height
// from the temperature, and the aerosol optical depth's wavelength dependence.
func TestKocifaj2008SceneIsThePapers(t *testing.T) {
	t.Parallel()

	scene := kocifaj2008Scene(t)

	_, temperature := scene.Atmosphere.Surface()

	h, err := atmosphere.MolecularScaleHeight(temperature)
	if err != nil {
		t.Fatalf("MolecularScaleHeight: %v", err)
	}

	if math.Abs(h.Meters()-8000) > 10 {
		t.Errorf("molecular scale height %.0f m, the paper's h0 is 8 km", h.Meters())
	}

	aerosol := scene.Atmosphere.Aerosol()

	at500, at1000 := float64(aerosol.TauAt(500)), float64(aerosol.TauAt(1000))
	if math.Abs(at500-0.5) > 1e-12 || math.Abs(at500/at1000-2) > 1e-9 {
		t.Errorf("aerosol optical depth %.4f at 500 nm and %.4f at 1000 nm; the paper's is 0.5, "+
			"falling as lambda^-1", at500, at1000)
	}
}

// k2008Sky is one cloudless sky, sampled.
type k2008Sky struct {
	sourceAz    float64
	radiance    func(z, az float64) float64
	minZ, minAz float64
	minL, maxL  float64
}

// sampleK2008Sky evaluates c over the sky at every combination of zs and azs,
// recording the darkest and the brightest.
func sampleK2008Sky(t *testing.T, c skybrightness.Component, sourceAz float64, zs, azs []float64) k2008Sky {
	t.Helper()

	scene := kocifaj2008Scene(t)
	sky := k2008Sky{
		sourceAz: sourceAz,
		radiance: func(z, az float64) float64 { return radianceAt(t, c, scene, 90-z, az) },
		minL:     math.Inf(1),
	}

	for _, z := range zs {
		for _, az := range azs {
			l := sky.radiance(z, az)
			if l < sky.minL {
				sky.minL, sky.minZ, sky.minAz = l, z, az
			}

			sky.maxL = max(sky.maxL, l)
		}
	}

	return sky
}

// steps returns from, from+step, ..., up to and including to.
func steps(from, to, step float64) []float64 {
	var out []float64
	for v := from; v <= to+1e-9; v += step {
		out = append(out, v)
	}

	return out
}

// azimuthFrom returns the angular difference of az from ref, 0 to 180.
func azimuthFrom(az, ref float64) float64 {
	return math.Abs(math.Remainder(az-ref, 360))
}

// TestCloudlessSkyHasKocifaj2008sShape runs Kocifaj (2008) Fig. 1's case, one
// source 2 km away at azimuth 250 degrees under a cloudless sky, and checks the
// shape §3.1 describes:
//
//   - luminance decreases rapidly with angular distance from the source;
//   - its minimum lies at a medium or smaller zenith angle on the side opposite
//     the source, and does not move much with the observer's distance;
//   - past the minimum it grows slightly toward the opposite horizon;
//   - it varies over one to two orders of magnitude.
//
// # What the last one can and cannot check
//
// The range depends on how near the horizon the sky is sampled toward the
// source, and the paper does not say. Sampled every degree in zenith angle,
// brightest over darkest is 19x out to 60 degrees, 63x to 70, 148x to 75,
// 2500x to 85 and 34000x to 89, because a line of sight that low passes a few
// hundred meters over the source. The paper's "one to two orders" is that range
// out to 60-75 degrees. So this requires at least one order, and states the
// range rather than holding it to a prose figure it cannot be compared with.
//
// The figures are contour plots in relative units for this case, so the shape
// is what can be checked, not values read off them.
func TestCloudlessSkyHasKocifaj2008sShape(t *testing.T) {
	t.Parallel()

	const sourceAz = 250

	antiAz := math.Mod(sourceAz+180, 360)

	near, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{cloudyCity(t, sourceAz, 2)})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	sky := sampleK2008Sky(t, near, sourceAz, steps(0, 85, 5), steps(0, 350, 10))

	t.Logf("darkest at azimuth %.0f, zenith angle %.0f; brightest/darkest %.0fx out to 85 degrees",
		sky.minAz, sky.minZ, sky.maxL/sky.minL)

	if sky.maxL/sky.minL < 10 {
		t.Errorf("the cloudless sky varies only %.1fx; the paper reports one to two orders of magnitude",
			sky.maxL/sky.minL)
	}

	// Decreasing with angular distance from the source: along each almucantar,
	// from the source's azimuth round to the opposite one.
	for _, z := range []float64{30, 60, 85} {
		prev := math.Inf(1)

		for d := 0.0; d <= 180; d += 10 {
			for _, side := range []float64{1, -1} {
				l := sky.radiance(z, sourceAz+side*d)
				if l > prev*(1+1e-9) {
					t.Errorf("at zenith angle %.0f the sky brightens from %.4g to %.4g moving %+.0f degrees "+
						"in azimuth from the source", z, prev, l, side*d)
				}
			}

			prev = min(sky.radiance(z, sourceAz+d), sky.radiance(z, sourceAz-d))
		}
	}

	// The minimum: opposite the source, at a medium or smaller zenith angle.
	if d := azimuthFrom(sky.minAz, antiAz); d > 45 {
		t.Errorf("the minimum is at azimuth %.0f, %.0f degrees from the side opposite the source",
			sky.minAz, d)
	}

	if sky.minZ > 45 {
		t.Errorf("the minimum is at zenith angle %.0f; the paper puts it at a medium or smaller one", sky.minZ)
	}

	// Past it, a slight rise toward the opposite horizon: above the minimum,
	// but by less than the order of magnitude the sky spans overall.
	horizon := sky.radiance(85, antiAz)
	if rise := horizon / sky.minL; rise <= 1 || rise >= 10 {
		t.Errorf("toward the opposite horizon the sky is %.2fx its minimum; the paper describes a "+
			"slight rise", rise)
	}

	// Not moving much with distance: the same scan, near the opposite
	// meridian, for sources further out.
	for _, km := range []float64{5, 10} {
		far, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{cloudyCity(t, sourceAz, km)})
		if err != nil {
			t.Fatalf("NewCloudySkyglow: %v", err)
		}

		farSky := sampleK2008Sky(t, far, sourceAz, steps(0, 85, 5), steps(antiAz-30, antiAz+30, 10))

		t.Logf("source at %.0f km: darkest at azimuth %.0f, zenith angle %.0f", km, farSky.minAz, farSky.minZ)

		if math.Abs(farSky.minZ-sky.minZ) > 10 {
			t.Errorf("with the source at %.0f km the minimum moves to zenith angle %.0f, from %.0f at 2 km",
				km, farSky.minZ, sky.minZ)
		}
	}
}
