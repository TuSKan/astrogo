//go:build validation

package skybrightness_test

import (
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// The Žilina configuration of Kocifaj, Falchi & Kundracik (2025) §Results.
//
// Every value here is stated in that paper: a town of about 80,000 with an
// urban radius of 2.5 to 3.5 km, aerosol optical depth 0.1 and 0.3, Ångström
// exponent 1.3, single-scattering albedo 0.90, asymmetry 0.65, cloud base
// 2 km, cloud fractions to 0.9, the 500-600 nm band, and observers from over
// the town out to 18.5 km.
const (
	zilinaUrbanRadiusKM = 3.0
	zilinaCloudBaseM    = 2000
	zilinaAOD           = 0.1
	zilinaMaxCover      = 0.9
	zilinaFarObserverKM = 18.5
)

// zilinaCity spreads a town's light over a ring at the urban radius, centred
// at a given distance and bearing from the observer.
//
// A ring rather than a point because the paper models a town of finite
// extent, and because a point source directly beneath the observer is the one
// geometry Eq. 27 cannot express: the 1/h^2 in the scattering integral is
// cancelled by cos^2(z0_h) only while the source is somewhere else, and at
// zero horizontal separation that cancellation fails. Eight emitters is
// enough for the ring to behave as an extended source rather than as a point
// at the zenith.
func zilinaCity(tb testing.TB, centreKM float64) []skybrightness.GroundEmitter {
	tb.Helper()

	const emitters = 8

	out := make([]skybrightness.GroundEmitter, 0, emitters)

	for i := range emitters {
		// Half-integer steps so that no emitter lands on the axis through the
		// observer: at a town centre one urban radius away that would put one
		// of them at zero horizontal separation, which is the single geometry
		// the scattering integral cannot express.
		phi := 2 * math.Pi * (float64(i) + 0.5) / emitters

		// The emitter's offset from the town centre, in a frame whose x axis
		// points from the observer to that centre.
		dx := zilinaUrbanRadiusKM * math.Cos(phi)
		dy := zilinaUrbanRadiusKM * math.Sin(phi)

		distance := math.Hypot(centreKM+dx, dy)
		bearing := math.Atan2(dy, centreKM+dx) * 180 / math.Pi

		// The town's total output split evenly over the ring.
		out = append(out, cloudyCity(tb, bearing, distance))
	}

	return out
}

// zilinaScene builds the atmosphere the paper runs, clear or clouded.
func zilinaScene(tb testing.TB, cover float64) *skybrightness.Scene {
	tb.Helper()

	b := atmosphere.NewBuilder().
		Surface(1013, 288).
		Aerosol(zilinaAOD, 500, 1.3, 0.90, 0.65).
		AerosolScaleHeight(1538)

	if cover > 0 {
		b = b.AddCloud(atmosphere.CloudLayer{
			Fraction: unit.CloudFraction(cover),
			BaseAlt:  zilinaCloudBaseM,
			TopAlt:   zilinaCloudBaseM + 1000,
			// Not stated for these runs. Kocifaj (2007) adopts 0.4 and 0.7
			// against a published average of 0.46, and this takes the middle
			// of that range rather than the flattering end.
			Albedo:       0.55,
			OpticalDepth: 20,
		})
	}

	air, err := b.Build()
	if err != nil {
		tb.Fatalf("atmosphere Build: %v", err)
	}

	observer, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2000)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   observer,
		Time:       gotime.Date(2026, 3, 20, 3, 0, 0, 0, gotime.UTC),
		Atmosphere: air,
	}
}

// zilinaGrid is the 500-600 nm band the paper reports in.
func zilinaGrid(tb testing.TB) unit.SpectralGrid {
	tb.Helper()

	g, err := unit.NewSpectralGrid(500, 5, 21)
	if err != nil {
		tb.Fatalf("NewSpectralGrid: %v", err)
	}

	return g
}

// The amplification and screening the Žilina runs report, measured across
// distance.
//
// # What this establishes, and what it does not
//
// Kocifaj, Falchi & Kundracik (2025) report both signs at their town: cloud
// amplifying zenith radiance more than fifteenfold above it, and screening
// outside it. This reproduces both, and the transition between them is
// monotonic in distance, which is the qualitative claim the design note calls
// this component's acceptance criterion.
//
// It does not reproduce their crossover distance. Measured here it is near
// 45 km, while their observers reach 18.5 km and already see screening. Two
// inputs they do not state would move it and are not knowable from the text:
// the cloud albedo, which scales the reflection term directly, and the town's
// upward emission split. Garstang's q — the share radiated straight up rather
// than reflected off the ground — controls emission near the horizontal,
// which is exactly the light that reaches a distant cloud, so a
// better-shielded town screens sooner. This test therefore asserts the signs
// and the ordering, and records the crossover rather than requiring one.
//
// The magnitudes are bounded loosely for the same reason. A factor of two
// against a paper whose source inventory, cloud albedo and shielding are all
// unavailable is not evidence either way; a wrong sign or an order of
// magnitude is.
func TestZilinaAmplificationAcrossDistance(t *testing.T) {
	t.Parallel()

	grid := zilinaGrid(t)

	// Their own observer distances, extended far enough to find the sign
	// change.
	distances := []float64{0, 6, 10.5, 18.5, 30, 45, 60, 120}

	ratios := make([]float64, 0, len(distances))

	for _, km := range distances {
		comp := mustCloudy(t, zilinaCity(t, km))

		clearSky := zenithRadiance(t, comp, zilinaScene(t, 0), grid)
		if clearSky <= 0 {
			t.Fatalf("at %.1f km the clear sky is %.4g", km, clearSky)
		}

		ratio := zenithRadiance(t, comp, zilinaScene(t, zilinaMaxCover), grid) / clearSky
		ratios = append(ratios, ratio)

		t.Logf("town centre %6.1f km: zenith radiance x%7.3f at CF = %.1f", km, ratio, zilinaMaxCover)
	}

	// Over the town: amplification, and of the order the paper reports rather
	// than a few per cent.
	if ratios[0] < 15 {
		t.Errorf("over the town the cloud gives x%.2f; the paper reports more than "+
			"fifteenfold", ratios[0])
	}

	if ratios[0] > 1000 {
		t.Errorf("over the town the cloud gives x%.2f, two orders above the paper's "+
			"fifteenfold", ratios[0])
	}

	// Monotonic: every step away from the town weakens the cloud's effect,
	// because the deck overhead is lit at an ever more grazing angle while
	// what it blocks from above changes far less.
	for i := 1; i < len(ratios); i++ {
		if ratios[i] > ratios[i-1] {
			t.Errorf("the amplification rises from x%.3f at %.1f km to x%.3f at %.1f km; "+
				"it must fall with distance", ratios[i-1], distances[i-1], ratios[i], distances[i])
		}
	}

	// And screening somewhere: the far end must drop below one, or the model
	// only ever brightens and the sign reversal is absent.
	last := ratios[len(ratios)-1]
	if last >= 1 {
		t.Errorf("at %.1f km the cloud still gives x%.3f; screening never appears, which "+
			"is the half of the behaviour a cloud multiplier cannot produce",
			distances[len(distances)-1], last)
	}

	for i, r := range ratios {
		if r < 1 {
			t.Logf("crossover between %.1f and %.1f km", distances[i-1], distances[i])

			break
		}
	}
}

// Horizontal illuminance rises with cloud over the town.
//
// The paper reports ground-level irradiance amplified more than fourfold and
// photopic horizontal illuminance up to seventeenfold. This checks the
// direction and the order of magnitude over a whole hemisphere rather than one
// direction, which is a different integral over the same model and so a
// different way for a mistake to show.
func TestZilinaGroundIlluminanceRisesUnderCloud(t *testing.T) {
	t.Parallel()

	grid := zilinaGrid(t)

	comp, err := skybrightness.NewCloudySkyglow(zilinaCity(t, 0))
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	model, err := skybrightness.NewModel("zilina", comp)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	illuminance := func(cover float64) float64 {
		t.Helper()

		// Twelve rings is a few hundred directions, which is ample for an
		// integral over a smooth hemisphere and keeps this test in seconds.
		points, err := model.SkyMap(t.Context(), skybrightness.Query{
			Scene:    zilinaScene(t, cover),
			Grid:     grid,
			Fidelity: skybrightness.Standard,
		}, 12)
		if err != nil {
			t.Fatalf("SkyMap: %v", err)
		}

		got, err := skybrightness.HorizontalIlluminance(points, grid)
		if err != nil {
			t.Fatalf("HorizontalIlluminance: %v", err)
		}

		return float64(got)
	}

	clearSky := illuminance(0)
	clouded := illuminance(zilinaMaxCover)

	if clearSky <= 0 {
		t.Fatalf("the clear sky gives %.4g", clearSky)
	}

	ratio := clouded / clearSky

	t.Logf("over the town: horizontal illuminance x%.2f at CF = %.1f", ratio, zilinaMaxCover)

	if ratio <= 1 {
		t.Errorf("cloud over the town gives x%.3f of the clear-sky illuminance; the paper "+
			"reports more than fourfold", ratio)
	}

	if ratio > 200 {
		t.Errorf("illuminance amplification is x%.2f, far above the seventeenfold the "+
			"paper reports even in photopic units", ratio)
	}
}

// zenithRadiance evaluates one component at the zenith and integrates the band.
func zenithRadiance(
	tb testing.TB, c skybrightness.Component, scene *skybrightness.Scene, grid unit.SpectralGrid,
) float64 {
	tb.Helper()

	dst := skybrightness.NewSpectralRadiance(grid)

	if _, err := c.AddRadiance(tb.Context(), dst, grid,
		coord.NewAltAz(angle.Deg(90), angle.Deg(0)), scene); err != nil {
		tb.Fatalf("AddRadiance: %v", err)
	}

	var sum float64
	for _, v := range dst {
		sum += float64(v)
	}

	return sum
}

// mustCloudy builds the component or fails the test.
func mustCloudy(tb testing.TB, emitters []skybrightness.GroundEmitter) *skybrightness.CloudySkyglow {
	tb.Helper()

	c, err := skybrightness.NewCloudySkyglow(emitters)
	if err != nil {
		tb.Fatalf("NewCloudySkyglow: %v", err)
	}

	return c
}
