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

// Where the difference against GAMBONS actually lives.
//
// TestAgainstGAMBONS reports one number. This takes it apart: the geometry the
// zenith is looking through, each component above and below the atmosphere, the
// hard upper bound on what the missing scattered-in term could recover, and how
// much of the answer rests on the passband standing in for Johnson V.
//
// It asserts almost nothing. Its output is the error budget, and a budget whose
// entries are asserted is a budget nobody will update when the physics changes.
func TestGAMBONSGapDecomposition(t *testing.T) {
	testutil.RequireReachable(t, "irsa.ipac.caltech.edu:443")
	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")
	testutil.RequireReachable(t, "github.com:443")

	ctx, cancel := context.WithTimeout(context.Background(), 15*gotime.Minute)
	defer cancel()

	remote.EnableDownloads(32<<20, remote.GaiaStarMap)

	grid := skybrightness.DefaultOpticalGrid()
	band := johnsonVTophat()
	scene := gambonsScene(t)
	zenith := coord.NewAltAz(angle.Deg(89.9), angle.Deg(0))

	// ── where the zenith is pointing ────────────────────────────────────────
	cc := coord.NewContext(astrotime.FromGo(scene.Time), scene.Observer,
		scene.Atmosphere.Refraction())

	icrs, err := cc.AltAzToICRS(zenith)
	if err != nil {
		t.Fatalf("AltAzToICRS: %v", err)
	}

	gal := coord.ICRSToGalactic(icrs)
	ecl := coord.ICRSToEcliptic(icrs, astrotime.FromGo(scene.Time))

	t.Logf("zenith: RA %.2f Dec %+.2f | galactic l %.2f b %+.2f | ecliptic lon %.2f lat %+.2f",
		icrs.RA().Degrees(), icrs.Dec().Degrees(),
		gal.L().Degrees(), gal.B().Degrees(),
		ecl.Lon().Degrees(), ecl.Lat().Degrees())

	// ── the atmosphere the light crosses ────────────────────────────────────
	airmass, err := atmosphere.Airmass(zenith.Alt())
	if err != nil {
		t.Fatalf("Airmass: %v", err)
	}

	pressure, _ := scene.Atmosphere.Surface()
	aerosol := scene.Atmosphere.Aerosol()

	const vNM = 550

	rayleigh, err := atmosphere.RayleighOpticalDepth(vNM, float64(pressure))
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	tauAero := aerosol.TauAt(vNM)
	slant := (rayleigh + unit.OpticalDepth(tauAero)) * unit.OpticalDepth(airmass)
	trans := float64(atmosphere.Transmission(slant))

	t.Logf("at 550 nm: tau_rayleigh %.4f, tau_aerosol %.4f, airmass %.4f, T %.4f",
		float64(rayleigh), tauAero, airmass, trans)
	t.Logf("extinction is %.3f mag; that is the hard ceiling on what returning the "+
		"scattered-out light could recover", -2.5*math.Log10(trans))

	// ── the star map, above the atmosphere ──────────────────────────────────
	skyMap, err := starlight.Open(ctx)
	if err != nil {
		t.Skipf("star map: %v", err)
	}

	stars, err := skyMap.Band("V")
	if err != nil {
		t.Fatalf("Band: %v", err)
	}

	islExtra, err := stars.RadianceAt(icrs.RA(), icrs.Dec())
	if err != nil {
		t.Fatalf("RadianceAt: %v", err)
	}

	// Cross-check the interface lookup against the raw pixel, so a frame or
	// index mistake in the adapter cannot hide behind a plausible number.
	hpx := skyMap.Grid()
	pixel := hpx.PixelOf(icrs.RA(), icrs.Dec())

	raw, err := skyMap.Pixel("V", pixel)
	if err != nil {
		t.Fatalf("Pixel(%d): %v", pixel, err)
	}

	t.Logf("star map at the zenith: %.6e W m^-2 sr^-1 nm^-1 = %.3f mag arcsec^-2 "+
		"above the atmosphere", islExtra, toMag(islExtra))
	t.Logf("  raw pixel %d holds %.6e; interface and pixel agree: %t",
		pixel, raw, raw == islExtra)

	// What the neighbourhood looks like, so a single odd pixel is visible as
	// one rather than mistaken for the region.
	var lo, hi, sum float64

	lo = math.Inf(1)

	const span = 40

	count := 0

	for p := pixel - span; p <= pixel+span; p++ {
		if p < 0 || p >= hpx.NumPixels() {
			continue
		}

		v, err := skyMap.Pixel("V", p)
		if err != nil || v <= 0 {
			continue
		}

		lo = math.Min(lo, v)
		hi = math.Max(hi, v)
		sum += v
		count++
	}

	t.Logf("  %d neighbouring pixels run %.3f to %.3f mag arcsec^-2, mean %.3f",
		count, toMag(hi), toMag(lo), toMag(sum/float64(count)))

	// ── the dust the DGL rests on ───────────────────────────────────────────
	dustMap, err := dust.Fetch(ctx, nil, dust.Direction{L: gal.L(), B: gal.B()})
	if err != nil {
		t.Skipf("IRSA: %v", err)
	}

	i100, err := dustMap.IntensityAt(gal.L(), gal.B())
	if err != nil {
		t.Fatalf("IntensityAt: %v", err)
	}

	t.Logf("100 micron intensity toward the zenith: %.3f MJy/sr", i100)

	// ── build the model ─────────────────────────────────────────────────────
	isl, err := skybrightness.NewIntegratedStarlight(stars, solarLikeShape(grid), grid, band)
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	dgl, err := skybrightness.NewDiffuseGalacticLight(dustMap, stars, band)
	if err != nil {
		t.Fatalf("NewDiffuseGalacticLight: %v", err)
	}

	glow, err := airglow.NewAirglow(ctx, airglow.Spec{
		Observatory:  airglow.Paranal,
		SolarFluxSFU: gambonsSolarSF,
		MinNM:        float64(grid.At(0)) - 1,
		MaxNM:        float64(grid.At(grid.Len()-1)) + 1,
		StepNM:       0.1,
	}, grid, 87_000)
	if err != nil {
		t.Skipf("SkyCalc: %v", err)
	}

	model, err := skybrightness.NewModel("decompose",
		isl, dgl, skybrightness.NewZodiacalLight(), glow,
		skybrightness.NewExtragalacticBackground())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	est, err := model.Estimate(ctx,
		skybrightness.Query{Scene: scene, Direction: zenith, Grid: grid})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	// ── each component, and what it would be unattenuated ───────────────────
	t.Log("component, as evaluated and with the atmosphere removed:")

	var total, totalNoExt float64

	for _, id := range est.ComponentIDs() {
		spectrum, ok := est.Component(id)
		if !ok {
			continue
		}

		flux := bandFlux(t, spectrum, grid, band)
		if flux <= 0 {
			continue
		}

		total += flux

		// Airglow is emitted inside the atmosphere and is deliberately not
		// attenuated, so dividing it by T would be inventing a correction.
		noExt := flux
		if id != skybrightness.AirglowContinuum {
			noExt = flux / trans
		}

		totalNoExt += noExt

		t.Logf("  %-18s %6.2f  ->  %6.2f without extinction", id, toMag(flux), toMag(noExt))
	}

	t.Logf("total %.2f, and %.2f if every attenuated term were fully restored",
		toMag(total), toMag(totalNoExt))
	t.Logf("GAMBONS with airglow is %.2f, so the ceiling leaves %+.2f mag unexplained",
		gambonsZenithWithAirglow, toMag(totalNoExt)-gambonsZenithWithAirglow)

	// ── how much rests on the passband ──────────────────────────────────────
	t.Log("sensitivity to the band standing in for Johnson V:")

	for _, c := range []struct {
		name   string
		lo, hi unit.WavelengthNM
	}{
		{"500-600 (used)", 500, 600},
		{"505-595 (V half-power)", 505, 595},
		{"480-620 (wider)", 480, 620},
	} {
		p := band
		p.WavelengthNM = []unit.WavelengthNM{c.lo - 1, c.lo, c.hi, c.hi + 1}
		p.Response = []float64{0, 1, 1, 0}

		mag, err := magnitude.SurfaceBrightness(est.SpectralRadiance(), grid, p, magnitude.Vega, 0.5)
		if err != nil {
			t.Fatalf("SurfaceBrightness(%s): %v", c.name, err)
		}

		t.Logf("  %-24s %6.2f mag arcsec^-2", c.name, mag)
	}
}

// toMag converts a band-averaged radiance to mag arcsec^-2 through Johnson V's
// Vega zero point.
func toMag(radiance float64) float64 {
	const (
		vZeroFlux      = 3.63e-11
		arcsec2PerSter = 4.254517e10
	)

	if radiance <= 0 {
		return math.NaN()
	}

	return -2.5 * math.Log10(radiance/(vZeroFlux*arcsec2PerSter))
}
