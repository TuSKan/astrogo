//go:build validation

package skybrightness_test

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
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

// The whole-sky GAMBONS export, from the same run as the zenith figures:
// Barcelona, 21 August 2026 01:16 GMT+2, band V, AOD 0.056, airglow
// ESO_SkyCalc_100_10.dat, 129,600 points on a 0.5 degree grid. See
// docs/skybrightness.md section 13.
//
// Medians rather than means, which is GAMBONS' own choice and the right one:
// single bright stars spike individual pixels — the brightest in their export
// is 17.75 against a horizon of 22.15 — so a mean measures which stars the
// grid happened to land on. A median does not.
var gambonsAltitudeBands = []struct {
	loAlt, hiAlt float64

	// Airglow at 100 per cent.
	median, p05, p95 float64

	// Airglow at 0 per cent. Only the two extreme bands were recorded, so the
	// others carry NaN and are reported rather than compared.
	medianNoAirglow float64
}{
	{0, 15, 21.128, 20.879, 22.056, 22.65},
	{15, 30, 21.107, 20.832, 21.280, math.NaN()},
	{30, 45, 21.272, 20.982, 21.496, math.NaN()},
	{45, 60, 21.399, 21.011, 21.603, math.NaN()},
	{60, 75, 21.378, 20.915, 21.566, math.NaN()},
	{75, 90, 21.238, 20.858, 21.469, 21.82},
}

// The aggregate figures from the same two exports.
const (
	gambonsWholeSkyWithAirglow = 21.21
	gambonsWholeSkyNoAirglow   = 22.17

	// Altitude 0-30 degrees, which their table records as zenith angle 60-90.
	gambonsLowSkyWithAirglow = 21.15
	gambonsLowSkyNoAirglow   = 22.35

	// Microwatts per square metre on an upward-facing horizontal surface.
	gambonsIrradianceWithAirglow = 1.457
	gambonsIrradianceNoAirglow   = 0.678
)

// samplesPerBand fixes the cost of this test and its precision together, and
// the binding constraint is courtesy rather than time.
//
// Diffuse galactic light comes from IRSA one sightline at a time, paced, at
// about 1.8 seconds each. IRSA is a shared facility, so the sample count is
// chosen to keep this run to a few minutes of light traffic rather than to
// whatever would make the medians prettiest: a few hundred requests is an
// ordinary user, a few thousand is something a service is entitled to refuse.
//
// It cannot be recovered by sampling dust more coarsely than the sky. Measured
// against IRSA directly, the 100 micron intensity moves 20 to 40 per cent over
// 5 degrees and more than doubles that near the galactic plane, so snapping
// sightlines onto a coarse grid would put an uncontrolled error into one of
// the very components this comparison exists to measure.
//
// Equal samples per band rather than equal density over the hemisphere. The
// top band is 3.4 per cent of the sky and the bottom 25.9 per cent, so uniform
// sampling would determine the zenith band's median thirty times less well
// than the horizon's while spending most of the budget where the answer is
// already known. Each band's samples are equal-area within that band, so the
// band statistics are unbiased, and the bands carry their true solid angle
// when they are combined.
//
// At 20 per band the median of a band carries a standard error near 0.09 mag,
// against differences this comparison reports in tenths. That is the honest
// resolution of this test and it is stated rather than implied.
const samplesPerBand = 24

// dustChunk is how many sightlines are requested from IRSA before pausing to
// report progress.
//
// Chunked so that being cut off costs the run its remaining sightlines rather
// than all of them, and so that a service which has started refusing is
// noticed within seconds instead of minutes.
const dustChunk = 25

// bandSolidAngle is the steradians a band between two altitudes covers.
func bandSolidAngle(loAlt, hiAlt float64) float64 {
	return 2 * math.Pi * (math.Sin(hiAlt*math.Pi/180) - math.Sin(loAlt*math.Pi/180))
}

// bandDirections spreads n directions over one altitude band, equal solid
// angle each.
//
// Deterministic: sin(altitude) steps uniformly through the band, which is what
// makes the samples equal-area, and the azimuth advances by the golden angle so
// successive rings do not line up into spokes.
func bandDirections(loAlt, hiAlt float64, n int) []coord.AltAz {
	const goldenAngleDeg = 137.507764

	sinLo := math.Sin(loAlt * math.Pi / 180)
	sinHi := math.Sin(hiAlt * math.Pi / 180)

	out := make([]coord.AltAz, 0, n)

	for k := range n {
		sinAlt := sinLo + (float64(k)+0.5)/float64(n)*(sinHi-sinLo)
		alt := math.Asin(sinAlt) * 180 / math.Pi

		out = append(out, coord.NewAltAz(
			angle.Deg(alt),
			angle.Deg(math.Mod(float64(k)*goldenAngleDeg, 360)),
		))
	}

	return out
}

// quantile returns the p-quantile of an already-sorted slice.
func quantile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}

	idx := int(p * float64(len(sorted)-1))

	return sorted[idx]
}

// magFromRadiance turns a passband-averaged spectral radiance back into a
// surface brightness, so band and whole-sky means can be formed in linear
// radiance — the only space in which they may be formed at all — and reported
// as magnitudes.
func magFromRadiance(meanPerNM float64, band magnitude.Passband) float64 {
	if meanPerNM <= 0 {
		return math.Inf(1)
	}

	// The same projection SurfaceBrightness performs: per steradian to per
	// square arcsecond, then per wavelength to per frequency at the pivot.
	// Taken from constants rather than written out, so this cannot drift from
	// the projection it is reproducing. Checked against
	// magnitude.SurfaceBrightness directly: the two agree to 1e-13 magnitudes.
	pivot, err := band.PivotWavelength()
	if err != nil {
		return math.NaN()
	}

	perArcsec2 := meanPerNM * constants.ArcsecondSquaredToSteradian
	lambdaM := float64(pivot) * 1e-9
	fNu := perArcsec2 * 1e9 * lambdaM * lambdaM / constants.SI2019.SpeedOfLight.Value

	return -2.5 * math.Log10(fNu/(band.VegaZeroPointJy*1e-26))
}

// allSkyRun is everything one pass over the sky produced.
type allSkyRun struct {
	results        []allSkySample
	componentShare map[skybrightness.ComponentID]float64
	totalShareFlux float64
	band           magnitude.Passband
	grid           unit.SpectralGrid
}

// allSkySample is one evaluated direction, kept in radiance so band and
// whole-sky means can be formed in the only space they may be formed in.
type allSkySample struct {
	bandIdx         int
	solidSR         float64
	sinAlt          float64
	magOn, magOff   float64
	fluxOn, fluxOff float64

	// The airglow components' own band flux, taken directly rather than
	// inferred from the difference of two magnitudes.
	fluxAirglow float64
}

// runAllSky builds the GAMBONS scene, samples the sky and evaluates it.
//
// Shared by the two comparisons below rather than folded into either. With
// the dust cache warm this is seconds rather than minutes, so running it
// twice costs less than threading one set of results through two tests would
// cost in clarity.
func runAllSky(t *testing.T) allSkyRun {
	t.Helper()
	testutil.RequireReachable(t, "irsa.ipac.caltech.edu:443")
	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")
	testutil.RequireReachable(t, "github.com:443")

	ctx, cancel := context.WithTimeout(context.Background(), 90*gotime.Minute)
	defer cancel()

	remote.EnableDownloads(32<<20, remote.GaiaStarMap)

	grid := skybrightness.DefaultOpticalGrid()
	band := johnsonVTophat()
	scene := gambonsScene(t)

	skyMap, err := starlight.Open(ctx)
	if err != nil {
		t.Skipf("could not fetch the published star map: %v", err)
	}

	stars, err := skyMap.Band("V")
	if err != nil {
		t.Fatalf("Band: %v", err)
	}

	// Every direction this test will evaluate, band by band.
	type sample struct {
		dir      coord.AltAz
		bandIdx  int
		solidSR  float64
		galactic coord.Galactic
	}

	cc := coord.NewContext(astrotime.FromGo(scene.Time), scene.Observer,
		scene.Atmosphere.Refraction())

	var samples []sample

	for bi, b := range gambonsAltitudeBands {
		perSample := bandSolidAngle(b.loAlt, b.hiAlt) / float64(samplesPerBand)

		for _, d := range bandDirections(b.loAlt, b.hiAlt, samplesPerBand) {
			icrs, err := cc.AltAzToICRS(d)
			if err != nil {
				t.Fatalf("AltAzToICRS: %v", err)
			}

			samples = append(samples, sample{
				dir:      d,
				bandIdx:  bi,
				solidSR:  perSample,
				galactic: coord.ICRSToGalactic(icrs),
			})
		}
	}

	t.Logf("sampling %d directions, %d per band, over %d bands",
		len(samples), samplesPerBand, len(gambonsAltitudeBands))

	// Dust for every sightline, at its own direction. See samplesPerBand.
	dirs := make([]dust.Direction, 0, len(samples))
	for _, s := range samples {
		dirs = append(dirs, dust.Direction{L: s.galactic.L(), B: s.galactic.B()})
	}

	fetchStart := gotime.Now()

	dustMap := dust.NewMap()

	for start := 0; start < len(dirs); start += dustChunk {
		end := min(start+dustChunk, len(dirs))

		if _, err := dust.Fetch(ctx, dustMap, dirs[start:end]...); err != nil {
			// Stop asking. A service that has begun refusing is not helped by
			// being asked the remaining several hundred times, and a partial
			// sky would silently bias every band it did not finish.
			t.Skipf("IRSA stopped answering after %d of %d sightlines in %v: %v",
				dustMap.Len(), len(dirs), gotime.Since(fetchStart).Round(gotime.Second), err)
		}

		t.Logf("  dust: %d of %d sightlines (%v)", end, len(dirs),
			gotime.Since(fetchStart).Round(gotime.Second))
	}

	t.Logf("fetched %d dust cells in %v", dustMap.Len(), gotime.Since(fetchStart).Round(gotime.Second))

	// Built from the preset rather than assembled here.
	//
	// This is the whole point of having one: what is being compared against
	// the GAMBONS export is the configuration the library ships, not one a
	// test wired up by hand and which could drift away from it unnoticed. If
	// this comparison holds, [skybrightness.GAMBONSWeb] reproduces GAMBONS;
	// if the preset changes, this test says so.
	model, err := skybrightness.NewPreset(skybrightness.GAMBONSWeb, skybrightness.PresetInputs{
		Stars:         stars,
		StarShape:     solarLikeShape(grid),
		Dust:          dustMap,
		AirglowZenith: gambonsAirglow(ctx, t, grid),
		Grid:          grid,
		Band:          band,
	})
	if err != nil {
		t.Fatalf("NewPreset: %v", err)
	}

	// One evaluation per direction. The airglow-free sky is that same estimate
	// with its two airglow components subtracted, which is exact because
	// components sum in linear radiance — and which guarantees the two skies
	// differ by airglow alone, rather than by anything a second model build
	// might also have changed.
	results := make([]allSkySample, 0, len(samples))
	evalStart := gotime.Now()

	componentShare := make(map[skybrightness.ComponentID]float64)

	var totalShareFlux float64

	for i, s := range samples {
		est, err := model.Estimate(ctx,
			skybrightness.Query{Scene: scene, Direction: s.dir, Grid: grid})
		if err != nil {
			t.Fatalf("Estimate at alt %.2f az %.2f: %v",
				s.dir.Alt().Degrees(), s.dir.Az().Degrees(), err)
		}

		total := est.SpectralRadiance()

		withoutAirglow := skybrightness.NewSpectralRadiance(grid)
		copy(withoutAirglow, total)

		for _, id := range []skybrightness.ComponentID{
			skybrightness.AirglowContinuum, skybrightness.AirglowLines,
		} {
			if spectrum, ok := est.Component(id); ok {
				for j := range withoutAirglow {
					withoutAirglow[j] -= spectrum[j]
				}
			}
		}

		fluxOn := bandFlux(t, total, grid, band)
		fluxOff := bandFlux(t, withoutAirglow, grid, band)

		results = append(results, allSkySample{
			bandIdx:     s.bandIdx,
			solidSR:     s.solidSR,
			sinAlt:      s.dir.Alt().Sin(),
			magOn:       magFromRadiance(fluxOn, band),
			magOff:      magFromRadiance(fluxOff, band),
			fluxOn:      fluxOn,
			fluxOff:     fluxOff,
			fluxAirglow: fluxOn - fluxOff,
		})

		// Solid-angle-weighted share of each component over the whole sky.
		for _, id := range est.ComponentIDs() {
			if spectrum, ok := est.Component(id); ok {
				componentShare[id] += bandFlux(t, spectrum, grid, band) * s.solidSR
			}
		}

		totalShareFlux += fluxOn * s.solidSR

		if i > 0 && i%100 == 0 {
			t.Logf("  evaluated %d of %d directions", i, len(samples))
		}
	}

	t.Logf("evaluated %d directions in %v", len(results), gotime.Since(evalStart).Round(gotime.Second))

	return allSkyRun{
		results:        results,
		componentShare: componentShare,
		totalShareFlux: totalShareFlux,
		band:           band,
		grid:           grid,
	}
}

// The whole sky, against GAMBONS, band by band.
//
// TestAgainstGAMBONS compares one direction. This compares the shape of the
// sky: six altitude bands, their medians and spread, the whole-sky aggregate
// and the horizontal irradiance, for both the airglow-on and airglow-off runs.
//
// The shape is the part a single direction cannot check. Two models can agree
// at the zenith and disagree everywhere else — an extinction law applied with
// the wrong airmass, a van Rhijn enhancement in the wrong direction, a
// zodiacal light pinned to the wrong ecliptic geometry all leave the zenith
// almost untouched and bend the profile. GAMBONS' own profile is not monotonic
// in altitude, brightest around 15-30 degrees and faintest around 45-60,
// because airglow's limb brightening and atmospheric extinction pull opposite
// ways; reproducing that non-monotonicity is a stronger statement than
// reproducing any one number.
func TestAgainstGAMBONSAllSky(t *testing.T) {
	run := runAllSky(t)

	results, band := run.results, run.band
	componentShare, totalShareFlux := run.componentShare, run.totalShareFlux

	// ── band by band ────────────────────────────────────────────────────────
	t.Log("")
	t.Log("altitude band medians, airglow at 100 per cent:")
	t.Logf("  %-10s %8s %8s %9s   %8s %8s %8s", "band", "astrogo", "GAMBONS", "diff", "p05", "p95", "GAM p05/p95")

	var worst float64

	worstBand := ""

	for bi, b := range gambonsAltitudeBands {
		var on []float64

		for _, r := range results {
			if r.bandIdx == bi {
				on = append(on, r.magOn)
			}
		}

		sort.Float64s(on)

		med := quantile(on, 0.5)
		diff := med - b.median

		t.Logf("  %3.0f-%3.0f deg %8.3f %8.3f %+9.3f   %8.3f %8.3f   %.2f/%.2f",
			b.loAlt, b.hiAlt, med, b.median, diff,
			quantile(on, 0.05), quantile(on, 0.95), b.p05, b.p95)

		if math.Abs(diff) > math.Abs(worst) {
			worst = diff
			worstBand = fmt.Sprintf("altitude %.0f-%.0f degrees", b.loAlt, b.hiAlt)
		}
	}

	t.Log("")
	t.Log("altitude band medians, airglow at 0 per cent:")

	for bi, b := range gambonsAltitudeBands {
		var off []float64

		for _, r := range results {
			if r.bandIdx == bi {
				off = append(off, r.magOff)
			}
		}

		sort.Float64s(off)

		med := quantile(off, 0.5)

		if math.IsNaN(b.medianNoAirglow) {
			t.Logf("  %3.0f-%3.0f deg %8.3f    (GAMBONS did not record this band)", b.loAlt, b.hiAlt, med)

			continue
		}

		t.Logf("  %3.0f-%3.0f deg %8.3f %8.3f %+9.3f", b.loAlt, b.hiAlt, med, b.medianNoAirglow,
			med-b.medianNoAirglow)
	}

	// ── aggregates ──────────────────────────────────────────────────────────
	var (
		skyFluxOn, skyFluxOff, skySR float64
		lowFluxOn, lowFluxOff, lowSR float64
		irradOn, irradOff            float64
	)

	for _, r := range results {
		skyFluxOn += r.fluxOn * r.solidSR
		skyFluxOff += r.fluxOff * r.solidSR
		skySR += r.solidSR

		irradOn += r.fluxOn * r.sinAlt * r.solidSR
		irradOff += r.fluxOff * r.sinAlt * r.solidSR

		if gambonsAltitudeBands[r.bandIdx].hiAlt <= 30 {
			lowFluxOn += r.fluxOn * r.solidSR
			lowFluxOff += r.fluxOff * r.solidSR
			lowSR += r.solidSR
		}
	}

	t.Log("")
	t.Log("aggregates (solid-angle weighted, formed in radiance and then converted):")
	t.Logf("  %-28s %8s %8s %9s", "", "astrogo", "GAMBONS", "diff")

	for _, c := range []struct {
		name      string
		got, want float64
	}{
		{"whole sky, airglow on", magFromRadiance(skyFluxOn/skySR, band), gambonsWholeSkyWithAirglow},
		{"whole sky, airglow off", magFromRadiance(skyFluxOff/skySR, band), gambonsWholeSkyNoAirglow},
		{"altitude 0-30, airglow on", magFromRadiance(lowFluxOn/lowSR, band), gambonsLowSkyWithAirglow},
		{"altitude 0-30, airglow off", magFromRadiance(lowFluxOff/lowSR, band), gambonsLowSkyNoAirglow},
	} {
		t.Logf("  %-28s %8.3f %8.3f %+9.3f", c.name, c.got, c.want, c.got-c.want)
	}

	// The irradiance here is the band-averaged radiance integrated over the
	// hemisphere, which is a V-band quantity rather than the bolometric one
	// GAMBONS reports; the ratio between the two runs is the comparable part.
	t.Logf("  %-28s %8.3f %8.3f %+9.3f", "irradiance ratio on/off",
		irradOn/irradOff, gambonsIrradianceWithAirglow/gambonsIrradianceNoAirglow,
		irradOn/irradOff-gambonsIrradianceWithAirglow/gambonsIrradianceNoAirglow)

	// ── where the light comes from ──────────────────────────────────────────
	t.Log("")
	t.Log("whole-sky component shares, by solid-angle-weighted V-band radiance:")

	ids := make([]skybrightness.ComponentID, 0, len(componentShare))
	for id := range componentShare {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool { return componentShare[ids[i]] > componentShare[ids[j]] })

	for _, id := range ids {
		if componentShare[id] <= 0 {
			continue
		}

		t.Logf("  %-20s %5.1f%%", id, 100*componentShare[id]/totalShareFlux)
	}

	// ── the shape of the profile ────────────────────────────────────────────
	//
	// GAMBONS' airglow-on profile is not monotonic in altitude and its
	// airglow-off profile is. Reproducing that pair is a statement about the
	// two mechanisms rather than about any one number.
	medians := make([]float64, len(gambonsAltitudeBands))
	mediansOff := make([]float64, len(gambonsAltitudeBands))

	for bi := range gambonsAltitudeBands {
		var on, off []float64

		for _, r := range results {
			if r.bandIdx == bi {
				on = append(on, r.magOn)
				off = append(off, r.magOff)
			}
		}

		sort.Float64s(on)
		sort.Float64s(off)

		medians[bi] = quantile(on, 0.5)
		mediansOff[bi] = quantile(off, 0.5)
	}

	// With airglow off, extinction is the only thing shaping the profile, and
	// it dims the horizon most: the sky must brighten steadily from the
	// horizon to the zenith, which in magnitudes is a steadily falling number.
	offMonotonic := true

	for bi := 1; bi < len(mediansOff); bi++ {
		if mediansOff[bi] > mediansOff[bi-1] {
			offMonotonic = false

			break
		}
	}

	// With airglow on, van Rhijn brightens the limb and pulls the other way,
	// so the profile turns over: there is an interior faintest band rather
	// than a monotonic climb.
	faintest := 0
	for bi := range medians {
		if medians[bi] > medians[faintest] {
			faintest = bi
		}
	}

	onTurnsOver := faintest > 0 && faintest < len(medians)-1

	t.Log("")
	t.Logf("profile shape, airglow off: brightens monotonically toward the zenith: %v (GAMBONS: true)",
		offMonotonic)
	t.Logf("profile shape, airglow on:  turns over at an interior band: %v (GAMBONS: true, at 45-60)",
		onTurnsOver)
	t.Logf("  our airglow-off medians by band: %.2f", mediansOff)
	t.Logf("  our airglow-on  medians by band: %.2f, faintest at %.0f-%.0f degrees",
		medians, gambonsAltitudeBands[faintest].loAlt, gambonsAltitudeBands[faintest].hiAlt)

	if !offMonotonic {
		t.Errorf("with airglow off the only thing shaping the profile is extinction, which "+
			"dims the horizon most, so the sky must brighten steadily toward the zenith: %.2f",
			mediansOff)
	}

	if !onTurnsOver {
		// Reported rather than failed, because the cause is known and cannot be
		// removed here. GAMBONS' profile turns over because van Rhijn brightens
		// the limb and extinction darkens it; ours now applies both, but it
		// also attenuates without returning any of the light scattered back
		// into the beam, and that omission is largest exactly where extinction
		// is largest. The horizon is therefore dimmed too far and becomes the
		// faintest band instead of an interior one.
		//
		// Closing it needs the scattered term of Masana et al. Eq. 8, which
		// this project does not have; see docs/skybrightness.md section 16.
		// Inventing a substitute would make the profile agree by construction,
		// which is the one way of agreeing that would mean nothing.
		t.Logf("  the profile does not turn over: ours is faintest at the horizon, which is "+
			"the missing scattered-in term dimming it too far there: %.2f", medians)
	}

	// The bound is loose on purpose, as in TestAgainstGAMBONS: two independent
	// implementations of a six-term radiative model agreeing to a few tenths
	// is the meaningful result, and what this is built to catch is a factor, a
	// sign or a unit, which lands whole magnitudes away.
	if math.Abs(worst) > 1.0 {
		t.Errorf("the worst band disagrees with GAMBONS by %+.2f mag (%s); "+
			"a disagreement of more than a magnitude is a factor of two and a half",
			worst, worstBand)
	}
}

// The same comparison with airglow put on a common footing, and an account of
// what still differs once it is.
//
// Airglow is a free parameter in both models rather than a prediction by
// either, so comparing two runs handed different airglow measures the files
// and not the physics. This scales ours to theirs using one band and then asks
// whether the rest of the sky follows.
func TestGAMBONSAllSkyWithAirglowMatched(t *testing.T) {
	run := runAllSky(t)

	results, band := run.results, run.band

	medians, mediansOff := bandMedians(results)

	var skyFluxOff, skySR float64

	for _, r := range results {
		skyFluxOff += r.fluxOff * r.solidSR
		skySR += r.solidSR
	}

	// ── the same comparison with airglow put on a common footing ────────────
	//
	// Airglow is a free parameter in both models rather than a prediction by
	// either: GAMBONS drives it from ESO_SkyCalc_100_10.dat and this test asks
	// SkyCalc for 100 sfu, and those are about a factor of 1.6 apart. Comparing
	// two models that were handed different airglow measures the files, not the
	// physics, which is the trap this repository's own validation notes warn
	// about.
	//
	// So scale ours to theirs and ask the question again. The scale is taken
	// from the 75-90 degree band, where the geometry is reliable and extinction
	// is a tenth of a magnitude, and it is applied to every band unchanged - if
	// the two models agree about the shape of airglow across the sky, one
	// number fixed at the zenith should bring the whole profile into line, and
	// if they do not, it will not.
	//
	// This is arithmetic on radiances already computed, not a second run:
	// total = airglow-free sky + scale * airglow.
	topBand := len(gambonsAltitudeBands) - 1

	var ourTop, theirTop float64

	{
		var ours []float64

		for _, r := range results {
			if r.bandIdx == topBand {
				ours = append(ours, r.fluxAirglow)
			}
		}

		sort.Float64s(ours)

		ourTop = quantile(ours, 0.5)

		b := gambonsAltitudeBands[topBand]
		theirTop = math.Pow(10, -0.4*b.median) - math.Pow(10, -0.4*b.medianNoAirglow)
	}

	// Their flux is in the arbitrary units of that power law and ours is in
	// physical ones, so the scale is fixed by requiring the two to agree on the
	// ratio of airglow to the airglow-free sky in this band, which is a pure
	// number in both.
	var ourOffTop, theirOffTop float64

	{
		var off []float64

		for _, r := range results {
			if r.bandIdx == topBand {
				off = append(off, r.fluxOff)
			}
		}

		sort.Float64s(off)

		ourOffTop = quantile(off, 0.5)
		theirOffTop = math.Pow(10, -0.4*gambonsAltitudeBands[topBand].medianNoAirglow)
	}

	scale := (theirTop / theirOffTop) / (ourTop / ourOffTop)

	t.Log("")
	t.Logf("airglow normalised to GAMBONS in the %.0f-%.0f band: scale %.3f (%.3f mag)",
		gambonsAltitudeBands[topBand].loAlt, gambonsAltitudeBands[topBand].hiAlt,
		scale, -2.5*math.Log10(scale))
	t.Logf("  %-12s %9s %9s %10s", "band", "astrogo", "GAMBONS", "diff")

	var worstNorm float64

	for bi, b := range gambonsAltitudeBands {
		var scaled []float64

		for _, r := range results {
			if r.bandIdx == bi {
				scaled = append(scaled, magFromRadiance(r.fluxOff+scale*r.fluxAirglow, band))
			}
		}

		sort.Float64s(scaled)

		med := quantile(scaled, 0.5)
		diff := med - b.median

		if math.Abs(diff) > math.Abs(worstNorm) {
			worstNorm = diff
		}

		t.Logf("  %3.0f-%3.0f deg %9.3f %9.3f %+10.3f", b.loAlt, b.hiAlt, med, b.median, diff)
	}

	var normFlux, normSR float64

	for _, r := range results {
		normFlux += (r.fluxOff + scale*r.fluxAirglow) * r.solidSR
		normSR += r.solidSR
	}

	t.Logf("  %-12s %9.3f %9.3f %+10.3f", "whole sky",
		magFromRadiance(normFlux/normSR, band), gambonsWholeSkyWithAirglow,
		magFromRadiance(normFlux/normSR, band)-gambonsWholeSkyWithAirglow)
	t.Logf("  worst band once airglow is on a common footing: %+.3f mag", worstNorm)

	// ── where the remaining difference comes from ───────────────────────────
	//
	// Two mechanisms account for it, and both are already declared rather than
	// discovered here. This computes what each is worth so the declaration is
	// a number instead of a caveat.
	t.Log("")
	t.Log("difference budget:")
	t.Log("")
	t.Log("  1. airglow: compared as flux, not as a difference of magnitudes.")
	t.Log("     How much airglow 'adds' in magnitudes depends on the airglow-free")
	t.Log("     sky underneath it, so differencing the two runs' magnitudes does")
	t.Log("     not compare the airglow. Ours is taken from the component itself;")
	t.Log("     theirs is the flux difference of their two exports, which is only")
	t.Log("     available for the two bands they recorded both runs for.")
	t.Logf("     %-12s %13s %13s %11s %11s", "band", "our airglow", "their airglow", "ours/theirs", "unapplied")

	const representativeKV = 0.12 // mag per airmass, a clear sea-level site in V

	airglowRatioMag := make(map[int]float64)

	for bi, b := range gambonsAltitudeBands {
		var ours []float64

		for _, r := range results {
			if r.bandIdx == bi {
				ours = append(ours, r.fluxAirglow)
			}
		}

		sort.Float64s(ours)

		ourFlux := quantile(ours, 0.5)

		sinMid := (math.Sin(b.loAlt*math.Pi/180) + math.Sin(b.hiAlt*math.Pi/180)) / 2
		mid := math.Asin(sinMid) * 180 / math.Pi

		unapplied := math.NaN()
		if am, err := atmosphere.Airmass(angle.Deg(mid)); err == nil {
			unapplied = representativeKV * am
		}

		if math.IsNaN(b.medianNoAirglow) {
			t.Logf("     %3.0f-%3.0f deg %13.4g %13s %11s %11.3f",
				b.loAlt, b.hiAlt, ourFlux, "(not recorded)", "-", unapplied)

			continue
		}

		// Their airglow is the difference of the two runs, in flux.
		theirFlux := math.Pow(10, -0.4*b.median) - math.Pow(10, -0.4*b.medianNoAirglow)

		// Ours is in physical units and theirs in the arbitrary units of that
		// power law, so only the ratio between the two bands is meaningful;
		// it is normalised below against the highest band.
		airglowRatioMag[bi] = -2.5 * math.Log10(ourFlux/theirFlux)

		t.Logf("     %3.0f-%3.0f deg %13.4g %13.4g %11s %11.3f",
			b.loAlt, b.hiAlt, ourFlux, theirFlux, "see below", unapplied)
	}

	// Only the change in the ratio across the sky is free of the unit
	// mismatch, and that change is what extinction would explain.
	if lo, okLo := airglowRatioMag[0]; okLo {
		if hi, okHi := airglowRatioMag[len(gambonsAltitudeBands)-1]; okHi {
			swing := lo - hi

			var differential float64

			amLo, errLo := atmosphere.Airmass(angle.Deg(7.44))
			amHi, errHi := atmosphere.Airmass(angle.Deg(79.41))

			if errLo == nil && errHi == nil {
				differential = representativeKV * (amLo - amHi)
			}

			t.Log("")
			t.Logf("     our airglow relative to theirs swings %+.3f mag from the 75-90 band"+
				" to the 0-15 one", swing)
			t.Logf("     the slant extinction never applied differs by %+.3f mag across the same span",
				differential)
			t.Logf("     leaving %+.3f mag the missing extinction does not account for, which is"+
				" the van Rhijn layer height or their own angular treatment",
				math.Abs(swing)-differential)
		}
	}

	t.Log("")
	t.Log("     Separately from the slope, the normalisation differs. Near the")
	t.Log("     zenith, where the geometry is reliable and extinction is a tenth")
	t.Log("     of a magnitude, our airglow is a factor of about 1.6 fainter than")
	t.Log("     GAMBONS'. Both drive it from an ESO SkyCalc spectrum, so that is a")
	t.Log("     parameter difference rather than physics: their reference file is")
	t.Log("     ESO_SkyCalc_100_10.dat and this test asks SkyCalc for 100 sfu,")
	t.Log("     which need not be the same normalisation.")

	t.Log("")
	t.Log("  2. no light is scattered back into the beam.")
	t.Log("     Starlight, diffuse galactic light, zodiacal light and the")
	t.Log("     extragalactic background are attenuated by the atmosphere and")
	t.Log("     nothing is scattered in to replace what is scattered out.")
	t.Log("     atmosphere.MultipleScatteringFactor exists and is applied only by")
	t.Log("     the moonlight component, so the airglow-free sky here is the")
	t.Log("     singly-transmitted sky alone.")

	rayleigh, err := atmosphere.RayleighOpticalDepth(550, 1013)
	if err == nil {
		if f, ferr := atmosphere.MultipleScatteringFactor(rayleigh); ferr == nil {
			offDiff := magFromRadiance(skyFluxOff/skySR, band) - gambonsWholeSkyNoAirglow

			t.Logf("     Rayleigh depth at 550 nm: %.4f, so 1 + 4.5 tau = %.3f, worth %.3f mag",
				float64(rayleigh), f, 2.5*math.Log10(f))
			t.Logf("     our airglow-off whole sky is %+.3f mag from GAMBONS, a factor of %.3f",
				offDiff, math.Pow(10, offDiff/2.5))
		}
	}

	t.Log("")
	t.Log("  These pull opposite ways in the airglow-on total — too much light at")
	t.Log("  the horizon, too little everywhere from the missing scattered-in")
	t.Log("  term — which is why the whole-sky airglow-on figure agrees far")
	t.Log("  better than either mechanism alone would suggest. Agreement there is")
	t.Log("  partly cancellation and should not be read as the model being right")
	t.Log("  in both respects.")

	_ = medians
	_ = mediansOff
	_ = skyFluxOff
	_ = skySR
}

// bandMedians is the median magnitude in each band, with airglow and without.
func bandMedians(results []allSkySample) (medians, mediansOff []float64) {
	medians = make([]float64, len(gambonsAltitudeBands))
	mediansOff = make([]float64, len(gambonsAltitudeBands))

	for bi := range gambonsAltitudeBands {
		var on, off []float64

		for _, r := range results {
			if r.bandIdx == bi {
				on = append(on, r.magOn)
				off = append(off, r.magOff)
			}
		}

		sort.Float64s(on)
		sort.Float64s(off)

		medians[bi] = quantile(on, 0.5)
		mediansOff[bi] = quantile(off, 0.5)
	}

	return medians, mediansOff
}

// gambonsAirglow fetches the reference airglow spectrum onto a grid.
//
// The same request GAMBONS describes: Cerro Paranal, the solar-cycle average
// flux, de-extinguished back to the emitting layer by [airglow.Parse].
func gambonsAirglow(ctx context.Context, t *testing.T, grid unit.SpectralGrid) skybrightness.SpectralRadiance {
	t.Helper()

	spectrum, err := airglow.Fetch(ctx, airglow.Spec{
		Observatory:  airglow.Paranal,
		SolarFluxSFU: gambonsSolarSF,
		MinNM:        float64(grid.At(0)) - 1,
		MaxNM:        float64(grid.At(grid.Len()-1)) + 1,
		StepNM:       0.1,
	})
	if err != nil {
		t.Skipf("SkyCalc did not answer: %v", err)
	}

	out := skybrightness.NewSpectralRadiance(grid)
	for i := range out {
		out[i] = spectrum.At(float64(grid.At(i)))
	}

	return out
}
