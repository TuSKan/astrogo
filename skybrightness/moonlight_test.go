package skybrightness_test

import (
	"context"
	"errors"
	"math"
	"sync"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	astrotime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// solarSpectrumFixture is a 5772 K blackbody normalised to the solar
// constant, sampled at the ROLO bands.
//
// It is a test fixture, not a solar reference: the package ships no solar
// spectrum on purpose, and this one is good to roughly 20% across the
// optical, which is enough for the order-of-magnitude checks below and not
// enough for anything else.
func solarSpectrumFixture(t *testing.T) []float64 {
	t.Helper()

	const (
		h    = 6.62607015e-34 // J s, SI 2019 exact
		c    = 2.99792458e8   // m/s, SI exact
		kB   = 1.380649e-23   // J/K, SI 2019 exact
		temp = 5772.0         // K, IAU nominal solar effective temperature
		// Solid angle of the Sun's disk at 1 AU, sr.
		omega = 6.794e-5
	)

	planck := func(lambdaM float64) float64 {
		return 2 * h * c * c / math.Pow(lambdaM, 5) / math.Expm1(h*c/(lambdaM*kB*temp))
	}

	bands := magnitude.ROLOBands()
	out := make([]float64, len(bands))

	for i, nm := range bands {
		// Planck gives W m^-2 sr^-1 m^-1; the solid angle turns radiance
		// into irradiance and 1e-9 converts per metre to per nanometre.
		out[i] = planck(float64(nm)*1e-9) * omega * 1e-9
	}

	return out
}

// moonlightScene builds a Paranal scene at t with a clear standard
// atmosphere and the offline SOFA ephemeris.
func moonlightScene(t *testing.T, when gotime.Time) *skybrightness.Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).                   // hPa, K — Paranal
		Aerosol(0.02, 550, 1.3, 0.95, 0.65). // clean high-altitude aerosol
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       when,
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}
}

// nearFullMoonUp scans a lunation for the moment the Moon is closest to full
// while well above the horizon at Paranal, so the absolute check below runs
// in the geometry it is calibrated against. Deterministic and offline.
func nearFullMoonUp(t *testing.T) (gotime.Time, angle.Angle) {
	t.Helper()

	provider := eph.Default()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	start := gotime.Date(2026, 3, 1, 0, 0, 0, 0, gotime.UTC)

	var (
		best      gotime.Time
		bestPhase = angle.Deg(360)
	)

	for step := range 30 * 24 {
		when := start.Add(gotime.Hour * gotime.Duration(step)) //nolint:durationcheck // step is a count of hours, not a duration
		at := astrotime.FromGo(when)

		phase, err := magnitude.PhaseAngle(provider, eph.Moon, at)
		if err != nil {
			t.Fatalf("PhaseAngle: %v", err)
		}

		moon, err := provider.State(eph.Moon, at)
		if err != nil {
			t.Fatalf("moon state: %v", err)
		}

		icrs, err := eph.ToICRS(moon.Pos)
		if err != nil {
			t.Fatalf("ToICRS: %v", err)
		}

		altaz, err := coord.NewContext(at, loc, atmosphere.Refraction{}).ICRSToAltAz(icrs)
		if err != nil {
			t.Fatalf("ICRSToAltAz: %v", err)
		}

		if altaz.Alt().Degrees() > 60 && phase < bestPhase {
			best, bestPhase = when, phase
		}
	}

	if best.IsZero() {
		t.Fatal("found no epoch with the Moon above 60 degrees in a full lunation")
	}

	return best, bestPhase
}

// The check that matters: a near-full Moon high in a clear sky must produce a
// V-band sky brightness near 18 mag/arcsec^2.
//
// That number comes from neither Kieffer & Stone nor Winkler — it is the
// long-established brightness of a full-moon night at a dark site, and every
// link in the chain has to be right at once to land on it: ROLO's
// reflectance, the solid-angle-over-pi conversion, both inverse squares, the
// Rayleigh optical depth, the phase function, and the single-scattering path
// integral. A factor-of-ten error anywhere shows up here as two magnitudes.
//
// The tolerance is wide on purpose. The solar spectrum is a blackbody
// fixture, the passband is a top hat, the multiple-scattering correction is a
// broadband fit from another site, and the airglow and starlight that also
// contribute are not in this component at all. Two magnitudes is the honest
// band.
func TestScatteredMoonlightFullMoonSkyBrightness(t *testing.T) {
	t.Parallel()

	when, phase := nearFullMoonUp(t)
	scene := moonlightScene(t, when)

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	// Look at the zenith. The Moon is high but not at the zenith, so this is
	// a real scattering angle rather than a line of sight through the Moon.
	dir := coord.NewAltAz(angle.Deg(90), angle.Deg(0))

	flags, err := component.AddRadiance(context.Background(), dst, grid, dir, scene)
	if err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	if flags&skybrightness.ApproximateMultipleScattering == 0 {
		t.Error("the component did not flag its multiple-scattering treatment as approximate")
	}

	// Radiance at 554 nm, the grid point nearest the 553.8 nm ROLO
	// band, which is itself the band closest to Johnson V.
	idx := gridIndex(t, grid, 554)

	radiance := dst[idx] // W m^-2 sr^-1 nm^-1

	if radiance <= 0 {
		t.Fatalf("a near-full Moon at %.1f degrees phase gave %v radiance", phase.Degrees(), radiance)
	}

	// To mag/arcsec^2 against the Johnson V zero point, 3.63e-9 erg s^-1
	// cm^-2 A^-1 for a V = 0 star (Bessell 1979).
	const (
		vZeroPointCGS      = 3.63e-9
		arcsecSqPerSr      = 4.25451702961522e10
		wattPerNmToCGSPerA = 1e2 // W m^-2 nm^-1 -> erg s^-1 cm^-2 A^-1
	)

	perArcsec := radiance * wattPerNmToCGSPerA / arcsecSqPerSr
	mag := -2.5 * math.Log10(perArcsec/vZeroPointCGS)

	t.Logf("phase %.2f deg, V sky brightness %.2f mag/arcsec^2 (%.3g W m^-2 sr^-1 nm^-1)",
		phase.Degrees(), mag, radiance)

	if mag < 16.5 || mag > 20.5 {
		t.Errorf("full-moon V sky brightness = %.2f mag/arcsec^2, want roughly 18", mag)
	}
}

// Away from the Moon the sky is darker. The Rayleigh phase function is
// forward- and back-peaked and the path geometry changes too, so this is the
// combined directional behaviour rather than a single term.
func TestScatteredMoonlightDarkensAwayFromTheMoon(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	scene := moonlightScene(t, when)

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	idx := gridIndex(t, grid, 554)

	at := func(alt, az float64) float64 {
		dir := coord.NewAltAz(angle.Deg(alt), angle.Deg(az))

		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
			t.Fatalf("AddRadiance: %v", err)
		}

		return dst[idx]
	}

	// The Moon's own direction, and the opposite side of the sky at the
	// same altitude so the airmass is identical and only the scattering
	// angle differs.
	moonDir := moonDirection(t, scene)

	near := at(moonDir.Alt().Degrees(), moonDir.Az().Degrees()+20)
	far := at(moonDir.Alt().Degrees(), moonDir.Az().Degrees()+120)

	if near <= far {
		t.Errorf("20 degrees from the Moon gives %.4g, not more than the %.4g at 120 degrees", near, far)
	}
}

// A Moon below the horizon contributes exactly nothing, and says so rather
// than silently returning zero with no explanation.
func TestScatteredMoonlightBelowHorizon(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)

	// Half a day later the near-full Moon is on the other side of the Earth.
	scene := moonlightScene(t, when.Add(12*gotime.Hour))

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	if moonDirection(t, scene).Alt() > 0 {
		t.Skip("the Moon is still up half a day later at this epoch")
	}

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	dir := coord.NewAltAz(angle.Deg(90), angle.Deg(0))

	if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	for i, v := range dst {
		if v != 0 {
			t.Fatalf("band %d is %v with the Moon below the horizon, want 0", i, v)
		}
	}
}

// Accumulation, not assignment: a component adds to whatever is already in
// dst, because the model sums several of them into one buffer.
func TestScatteredMoonlightAccumulates(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	scene := moonlightScene(t, when)

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	dir := coord.NewAltAz(angle.Deg(70), angle.Deg(30))

	once := skybrightness.NewSpectralRadiance(grid)
	if _, err := component.AddRadiance(context.Background(), once, grid, dir, scene); err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	twice := skybrightness.NewSpectralRadiance(grid)
	for range 2 {
		if _, err := component.AddRadiance(context.Background(), twice, grid, dir, scene); err != nil {
			t.Fatalf("AddRadiance: %v", err)
		}
	}

	idx := gridIndex(t, grid, 554)
	if rel := math.Abs(twice[idx]-2*once[idx]) / (2 * once[idx]); rel > 1e-12 {
		t.Errorf("two calls gave %v, want twice the %v of one", twice[idx], once[idx])
	}
}

// The scene cache must not leak across scenes: a second scene at a different
// time has to recompute the geometry rather than reuse the first one's.
func TestScatteredMoonlightCacheRespectsScene(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	dir := coord.NewAltAz(angle.Deg(80), angle.Deg(0))

	idx := gridIndex(t, grid, 554)

	at := func(scene *skybrightness.Scene) float64 {
		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
			t.Fatalf("AddRadiance: %v", err)
		}

		return dst[idx]
	}

	full := at(moonlightScene(t, when))
	later := at(moonlightScene(t, when.Add(6*gotime.Hour)))
	backAgain := at(moonlightScene(t, when))

	if full == later {
		t.Error("six hours later gave an identical radiance; the cache ignored the scene time")
	}

	if full != backAgain {
		t.Errorf("returning to the first scene gave %v, not the original %v", backAgain, full)
	}
}

// The component advertises that it is safe for concurrent use with distinct
// destination buffers, which a sky map relies on. Both the geometry cache and
// the pooled scratch buffers are shared state, so the claim needs a test —
// run this under -race for it to mean anything.
func TestScatteredMoonlightConcurrent(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	scene := moonlightScene(t, when)

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()
	idx := gridIndex(t, grid, 554)

	const directions = 64

	results := make([]float64, directions)

	var wg sync.WaitGroup

	for i := range directions {
		wg.Go(func() {
			dir := coord.NewAltAz(angle.Deg(20+float64(i%7)*10), angle.Deg(float64(i)*5))

			dst := skybrightness.NewSpectralRadiance(grid)
			if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
				t.Errorf("AddRadiance: %v", err)

				return
			}

			results[i] = dst[idx]
		})
	}

	wg.Wait()

	// Re-run the same directions serially: concurrency must not change a
	// single answer, which a shared scratch buffer being reused mid-flight
	// certainly would.
	for i := range directions {
		dir := coord.NewAltAz(angle.Deg(20+float64(i%7)*10), angle.Deg(float64(i)*5))

		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
			t.Fatalf("AddRadiance: %v", err)
		}

		if dst[idx] != results[i] {
			t.Fatalf("direction %d: concurrent %v, serial %v", i, results[i], dst[idx])
		}
	}
}

func TestNewScatteredMoonlightRejectsBadSpectrum(t *testing.T) {
	t.Parallel()

	if _, err := skybrightness.NewScatteredMoonlight(nil); !errors.Is(err, skybrightness.ErrNoSolarSpectrum) {
		t.Errorf("nil spectrum: err = %v, want ErrNoSolarSpectrum", err)
	}

	if _, err := skybrightness.NewScatteredMoonlight(make([]float64, 10)); !errors.Is(err, skybrightness.ErrNoSolarSpectrum) {
		t.Errorf("short spectrum: err = %v, want ErrNoSolarSpectrum", err)
	}

	bad := solarSpectrumFixture(t)
	bad[5] = -1

	if _, err := skybrightness.NewScatteredMoonlight(bad); !errors.Is(err, skybrightness.ErrNoSolarSpectrum) {
		t.Errorf("negative value: err = %v, want ErrNoSolarSpectrum", err)
	}
}

func TestScatteredMoonlightNeedsEphemeris(t *testing.T) {
	t.Parallel()

	when, _ := nearFullMoonUp(t)
	scene := moonlightScene(t, when)
	scene.Ephemeris = nil

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	dir := coord.NewAltAz(angle.Deg(90), angle.Deg(0))

	_, err = component.AddRadiance(context.Background(), skybrightness.NewSpectralRadiance(grid), grid, dir, scene)
	if !errors.Is(err, skybrightness.ErrNoEphemeris) {
		t.Errorf("err = %v, want ErrNoEphemeris", err)
	}
}

// The component must declare what it approximated. A provenance record that
// omits the single-scattering limitation would misrepresent the result.
func TestScatteredMoonlightProvenance(t *testing.T) {
	t.Parallel()

	component, err := skybrightness.NewScatteredMoonlight(solarSpectrumFixture(t))
	if err != nil {
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	p := component.Provenance()

	if p.PrimaryReference == "" || p.ValidityDomain == "" {
		t.Error("provenance is missing its primary reference or validity domain")
	}

	var mentionsSingleScattering bool

	for _, a := range p.KnownApproximations {
		if len(a) > 0 && (a[0] == 'S') {
			mentionsSingleScattering = true
		}
	}

	if !mentionsSingleScattering {
		t.Error("provenance does not record the single-scattering approximation")
	}

	if component.ID() != skybrightness.Moonlight {
		t.Errorf("ID = %q, want %q", component.ID(), skybrightness.Moonlight)
	}
}

// moonDirection resolves the Moon's alt/az for a scene, mirroring what the
// component does internally so a test can aim at or away from it.
func moonDirection(t *testing.T, scene *skybrightness.Scene) coord.AltAz {
	t.Helper()

	at := astrotime.FromGo(scene.Time)

	moon, err := scene.Ephemeris.State(eph.Moon, at)
	if err != nil {
		t.Fatalf("moon state: %v", err)
	}

	icrs, err := eph.ToICRS(moon.Pos)
	if err != nil {
		t.Fatalf("ToICRS: %v", err)
	}

	altaz, err := coord.NewContext(at, scene.Observer, scene.Atmosphere.Refraction()).ICRSToAltAz(icrs)
	if err != nil {
		t.Fatalf("ICRSToAltAz: %v", err)
	}

	return altaz
}

func BenchmarkScatteredMoonlight(b *testing.B) {
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		b.Fatal(err)
	}

	atm, err := atmosphere.NewBuilder().Surface(743, 284).Build()
	if err != nil {
		b.Fatal(err)
	}

	scene := &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2026, 3, 3, 5, 0, 0, 0, gotime.UTC),
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}

	solar := make([]float64, len(magnitude.ROLOBands()))
	for i := range solar {
		solar[i] = 1.5
	}

	component, err := skybrightness.NewScatteredMoonlight(solar)
	if err != nil {
		b.Fatal(err)
	}

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	dir := coord.NewAltAz(angle.Deg(60), angle.Deg(45))

	// Warm the per-scene geometry cache, which is what a sky map amortises.
	if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
			b.Fatal(err)
		}
	}
}

// gridIndex returns the index of a wavelength on a uniform grid.
func gridIndex(t *testing.T, grid unit.SpectralGrid, nm unit.WavelengthNM) int {
	t.Helper()

	for i := range grid.Len() {
		if grid.At(i) == nm {
			return i
		}
	}

	t.Fatalf("%v nm is not on the grid %v", nm, grid)

	return 0
}
