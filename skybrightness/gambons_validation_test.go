//go:build validation

package skybrightness_test

import (
	"context"
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
	"github.com/TuSKan/astrogo/skybrightness/dataset/dust"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
	astrotime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// The GAMBONS scene, reproduced exactly. See docs/skybrightness.md §13.
const (
	gambonsLatDeg  = 41.38
	gambonsLonDeg  = 2.11
	gambonsElevM   = 0
	gambonsAOD550  = 0.056
	gambonsSolarSF = 100 // ESO_SkyCalc_100_10.dat is msolflux = 100

	// Their published zenith figures, 0-5 degrees of zenith angle.
	gambonsZenithWithAirglow = 21.13
	gambonsZenithNoAirglow   = 21.74
)

// gambonsEpoch is 21 August 2026 01:16 GMT+2, which is what the run recorded.
func gambonsEpoch() gotime.Time {
	return gotime.Date(2026, 8, 20, 23, 16, 0, 0, gotime.UTC)
}

// johnsonVTophat approximates the Johnson V response.
//
// This module ships no V curve, and inventing a detailed one would be the thing
// it refuses to do. A tophat over 500-600 nm is what the rest of this package's
// tests use, and it is close to V's real 505-595 nm half-power span. It is an
// approximation and it is the largest one in this comparison: the published map
// is a V-band average and reading it against a slightly different band shifts
// the answer by a few hundredths of a magnitude.
func johnsonVTophat() magnitude.Passband {
	return magnitude.Passband{
		Name:         "Johnson V (tophat approximation)",
		WavelengthNM: []unit.WavelengthNM{499, 500, 600, 601},
		Response:     []float64{0, 1, 1, 0},
		Detector:     magnitude.PhotonCounting,

		// Bessell, Castelli & Plez (1998). It cross-checks against the
		// 3.63e-11 W m^-2 nm^-1 the star map's own zero point uses: 3636 Jy
		// at V's 545 nm effective wavelength is 3.67e-11 in those units, one
		// per cent away, which is the difference between their effective
		// wavelength and the 550 nm round number.
		VegaZeroPointJy: 3636,

		Reference: "tophat over V's half-power span, Vega zero point from " +
			"Bessell, Castelli & Plez (1998); see this test's own caveat",
	}
}

// solarLikeShape is the starlight spectral shape.
//
// Integrated starlight is the summed light of stars of every type, so no single
// blackbody is right and the component makes the caller choose rather than
// guessing. A 5500 K Planck function is the conventional stand-in and is what
// the rest of this package's tests use. The component renormalises it so its
// passband average is one, so the choice affects the spectrum's colour, not the
// V-band value the map already fixes.
func solarLikeShape(grid unit.SpectralGrid) skybrightness.SpectralRadiance {
	// [skybrightness.BlackbodyShape] rather than Planck's law written out
	// again. This helper used to carry its own copy, with the second radiation
	// constant inlined as a literal; the package now owns the physics and the
	// constants come from the module's own set.
	shape, err := skybrightness.BlackbodyShape(grid, solarLikeTemperatureK)
	if err != nil {
		panic("skybrightness test: blackbody shape: " + err.Error())
	}

	return shape
}

// solarLikeTemperatureK is the conventional stand-in for the integrated light
// of the sky.
const solarLikeTemperatureK = 5500

// The end-to-end comparison against GAMBONS.
//
// Every other check in this repository is either internal or checks one link.
// This runs the whole chain — the published star map, dust from IRSA, zodiacal
// light, airglow from ESO SkyCalc, the extragalactic background, and
// atmospheric transport — against an independent model of the same sky, in the
// same band, for the same site, epoch and atmosphere.
//
// It compares at the zenith rather than all-sky because diffuse galactic light
// is fetched per direction from IRSA at one request each: a whole sky would be
// tens of thousands of requests to a shared service to answer a question the
// zenith already answers.
//
// The bound is deliberately loose. Two independent implementations of a
// six-term radiative model agreeing to a few tenths of a magnitude is a
// meaningful result; agreeing to a hundredth would mean one had been tuned to
// the other. What this is built to catch is the class of error this module has
// already shipped once — a factor, a sign, a unit — which lands whole
// magnitudes away, not tenths.
func TestAgainstGAMBONS(t *testing.T) {
	testutil.RequireReachable(t, "irsa.ipac.caltech.edu:443")
	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")
	testutil.RequireReachable(t, "github.com:443")

	ctx, cancel := context.WithTimeout(context.Background(), 15*gotime.Minute)
	defer cancel()

	remote.EnableDownloads(32<<20, remote.GaiaStarMap)

	grid := skybrightness.DefaultOpticalGrid()
	band := johnsonVTophat()

	scene := gambonsScene(t)

	// The published integrated-starlight map.
	skyMap, err := starlight.Open(ctx)
	if err != nil {
		t.Skipf("could not fetch the published star map: %v", err)
	}

	stars, err := skyMap.Band("V")
	if err != nil {
		t.Fatalf("Band: %v", err)
	}

	isl, err := skybrightness.NewIntegratedStarlight(stars, solarLikeShape(grid), grid, band)
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	// Diffuse galactic light needs the 100 micron intensity along every
	// sightline the cap average will use, not just the zenith.
	dustMap, err := dust.Fetch(ctx, nil, capDustDirections(t, scene, gambonsCapDeg, gambonsCapSamples)...)
	if err != nil {
		t.Skipf("IRSA did not answer: %v", err)
	}

	dgl, err := skybrightness.NewDiffuseGalacticLight(dustMap, stars, band)
	if err != nil {
		t.Fatalf("NewDiffuseGalacticLight: %v", err)
	}

	// Airglow at the solar flux GAMBONS' own reference spectrum was built at.
	glow, err := airglow.NewAirglow(ctx, airglow.Spec{
		Observatory:  airglow.Paranal,
		SolarFluxSFU: gambonsSolarSF,
		MinNM:        float64(grid.At(0)) - 1,
		MaxNM:        float64(grid.At(grid.Len()-1)) + 1,
		StepNM:       0.1,
	}, grid, 87_000)
	if err != nil {
		t.Skipf("SkyCalc did not answer: %v", err)
	}

	zodiacal := skybrightness.NewZodiacalLight()
	egb := skybrightness.NewExtragalacticBackground()

	// Two models: the whole natural sky, and the same without airglow, which
	// is the pairing the GAMBONS runs provide.
	withAirglow, err := skybrightness.NewModel("gambons-comparison", isl, dgl, zodiacal, glow, egb)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	noAirglow, err := skybrightness.NewModel("gambons-comparison-no-airglow", isl, dgl, zodiacal, egb)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// GAMBONS reports "0-5 degrees" of zenith angle, which is an average over
	// that cap and not a point at the zenith. Comparing a single 13.7 arcmin
	// HEALPix pixel against it is not like for like: the pixels around this
	// zenith span 20.5 to 23.3 mag arcsec^-2, so a point sample and a cap mean
	// can differ by several tenths for reasons that have nothing to do with
	// either model.
	directions := zenithCap(gambonsCapDeg, gambonsCapSamples)

	got := map[string]float64{
		"with airglow": capMagnitude(ctx, t, withAirglow, scene, grid, band, directions),
		"no airglow":   capMagnitude(ctx, t, noAirglow, scene, grid, band, directions),
	}

	t.Logf("GAMBONS zenith: %.2f with airglow, %.2f without",
		gambonsZenithWithAirglow, gambonsZenithNoAirglow)
	t.Logf("astrogo zenith: %.2f with airglow, %.2f without",
		got["with airglow"], got["no airglow"])

	for _, c := range []struct {
		name string
		want float64
	}{
		{"with airglow", gambonsZenithWithAirglow},
		{"no airglow", gambonsZenithNoAirglow},
	} {
		diff := got[c.name] - c.want
		t.Logf("  %-13s astrogo %.2f vs GAMBONS %.2f, %+.2f mag", c.name, got[c.name], c.want, diff)

		if math.Abs(diff) > 1.0 {
			t.Errorf("%s: %.2f against GAMBONS' %.2f is %+.2f mag apart; two independent "+
				"models of the same sky should not disagree by a factor of two and a half",
				c.name, got[c.name], c.want, diff)
		}
	}

	// Where our total comes from, so a disagreement is attributable to a
	// component rather than left as one number.
	est, err := withAirglow.Estimate(ctx,
		skybrightness.Query{Scene: scene, Direction: directions[0], Grid: grid})
	if err != nil {
		t.Fatalf("Estimate for the breakdown: %v", err)
	}

	totalFlux := bandFlux(t, est.SpectralRadiance(), grid, band)

	t.Log("component breakdown at the zenith:")

	for _, id := range est.ComponentIDs() {
		spectrum, ok := est.Component(id)
		if !ok {
			continue
		}

		flux := bandFlux(t, spectrum, grid, band)
		if flux <= 0 {
			t.Logf("  %-18s (no contribution)", id)

			continue
		}

		mag, err := magnitude.SurfaceBrightness(spectrum, grid, band, magnitude.Vega, 0.5)
		if err != nil {
			t.Fatalf("SurfaceBrightness(%s): %v", id, err)
		}

		t.Logf("  %-18s %6.2f mag arcsec^-2  %5.1f%% of the total", id, mag, 100*flux/totalFlux)
	}

	// The airglow term itself, which both models isolate the same way.
	ourAirglow := got["no airglow"] - got["with airglow"]
	theirAirglow := gambonsZenithNoAirglow - gambonsZenithWithAirglow

	t.Logf("airglow contributes %+.2f mag here against GAMBONS' %+.2f", ourAirglow, theirAirglow)

	if math.Abs(ourAirglow-theirAirglow) > 0.6 {
		t.Errorf("airglow moves the zenith by %.2f mag here and %.2f in GAMBONS; "+
			"both drive it from the same ESO spectrum, so this is the scaling",
			ourAirglow, theirAirglow)
	}
}

// gambonsScene builds the scene the GAMBONS run used.
func gambonsScene(t *testing.T) *skybrightness.Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(gambonsLonDeg), angle.Deg(gambonsLatDeg), gambonsElevM)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	// Sea-level pressure and a late-summer surface temperature. GAMBONS took
	// relative humidity and a "Continental Clean" aerosol type; the Angstrom
	// exponent, single-scattering albedo and asymmetry below are that type's
	// conventional values rather than numbers GAMBONS published, which is an
	// approximation this comparison carries.
	// The transfer factor belongs to the atmosphere, so the preset reports it
	// and the scene sets it. Taking it from the preset rather than writing 0.5
	// here is what keeps the two from drifting apart.
	kappa, err := skybrightness.GAMBONSWeb.DiffuseKappa()
	if err != nil {
		t.Fatalf("DiffuseKappa: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(1013, 293).
		Aerosol(gambonsAOD550, 550, 1.3, 0.95, 0.65).
		BoundaryLayer(1000).
		DiffuseScattering(kappa).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       gambonsEpoch(),
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}
}

// bandFlux is the passband-averaged radiance, which is what shares are taken
// over. Magnitudes are logarithmic and must never be summed or differenced for
// this purpose.
func bandFlux(
	t *testing.T,
	spectrum skybrightness.SpectralRadiance,
	grid unit.SpectralGrid,
	band magnitude.Passband,
) float64 {
	t.Helper()

	mean, err := magnitude.MeanFluxDensity(spectrum, grid, band, 0.5)
	if err != nil {
		t.Fatalf("MeanFluxDensity: %v", err)
	}

	return mean
}

// The cap GAMBONS' first row averages over, and how finely it is sampled here.
const (
	gambonsCapDeg     = 5.0
	gambonsCapSamples = 64
)

// zenithCap returns directions spread over the cap within capDeg of the zenith,
// equal solid angle per sample so a plain mean of their radiances is the
// solid-angle average.
//
// Deterministic rather than random: cos(zenith angle) steps uniformly and the
// azimuth advances by the golden angle, which spreads the samples without
// needing a seed a test would have to pin.
func zenithCap(capDeg float64, samples int) []coord.AltAz {
	const goldenAngleDeg = 137.507764

	cosCap := math.Cos(capDeg * math.Pi / 180)
	out := make([]coord.AltAz, 0, samples)

	for k := range samples {
		cosZ := 1 - (float64(k)+0.5)/float64(samples)*(1-cosCap)
		zenithAngle := math.Acos(cosZ) * 180 / math.Pi

		out = append(out, coord.NewAltAz(
			angle.Deg(90-zenithAngle),
			angle.Deg(math.Mod(float64(k)*goldenAngleDeg, 360)),
		))
	}

	return out
}

// capDustDirections carries the cap's sightlines to galactic coordinates, which
// is what the dust provider is indexed by.
func capDustDirections(t *testing.T, scene *skybrightness.Scene, capDeg float64, samples int) []dust.Direction {
	t.Helper()

	cc := coord.NewContext(astrotime.FromGo(scene.Time), scene.Observer,
		scene.Atmosphere.Refraction())

	dirs := zenithCap(capDeg, samples)
	out := make([]dust.Direction, 0, len(dirs))

	for _, d := range dirs {
		icrs, err := cc.AltAzToICRS(d)
		if err != nil {
			t.Fatalf("AltAzToICRS: %v", err)
		}

		gal := coord.ICRSToGalactic(icrs)
		out = append(out, dust.Direction{L: gal.L(), B: gal.B()})
	}

	return out
}

// capMagnitude averages a model over a set of directions and converts once.
//
// The radiances are averaged, never the magnitudes. A mean of magnitudes is the
// geometric mean of the radiances, which is not what an instrument or a model
// reports over a solid angle, and it is systematically fainter than the truth
// wherever the sky is structured - which is exactly where the difference would
// matter.
func capMagnitude(
	ctx context.Context,
	t *testing.T,
	model *skybrightness.Model,
	scene *skybrightness.Scene,
	grid unit.SpectralGrid,
	band magnitude.Passband,
	directions []coord.AltAz,
) float64 {
	t.Helper()

	var sum float64

	for _, dir := range directions {
		est, err := model.Estimate(ctx,
			skybrightness.Query{Scene: scene, Direction: dir, Grid: grid})
		if err != nil {
			t.Fatalf("Estimate(%v): %v", dir, err)
		}

		sum += bandFlux(t, est.SpectralRadiance(), grid, band)
	}

	return toMag(sum / float64(len(directions)))
}
