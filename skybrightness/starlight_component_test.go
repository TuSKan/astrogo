package skybrightness_test

import (
	"context"
	"errors"
	"math"
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

// uniformSky is a stand-in starlight map: the same passband-averaged spectral
// radiance in every direction, so what is under test is the attenuation rather
// than the map's own structure.
type uniformSky struct {
	value    float64
	galactic bool
}

func (u uniformSky) RadianceAt(_, _ angle.Angle) (float64, error) { return u.value, nil }
func (u uniformSky) Galactic() bool                               { return u.galactic }

// emptySky covers nothing, which is how a real map behaves outside its
// footprint.
type emptySky struct{}

func (emptySky) RadianceAt(_, _ angle.Angle) (float64, error) { return 0, nil }
func (emptySky) Galactic() bool                               { return true }

// spySky records the coordinates it was asked for.
type spySky struct {
	galactic bool
	lon, lat angle.Angle
}

func (s *spySky) RadianceAt(lon, lat angle.Angle) (float64, error) {
	s.lon, s.lat = lon, lat

	return 1e-9, nil
}

func (s *spySky) Galactic() bool { return s.galactic }

// starlightScene is a clear night at Paranal.
func starlightScene(t *testing.T) *skybrightness.Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.02, 550, 1.3, 0.95, 0.65).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2024, 6, 21, 4, 0, 0, 0, gotime.UTC),
		Atmosphere: atm,
		Ephemeris:  eph.Default(),
	}
}

// solarShape is a stand-in spectral shape for the summed light of stars of
// every type: a 5500 K Planck curve in relative units. Its absolute scale is
// arbitrary, which is the property the normalisation has to remove.
func solarShape(grid unit.SpectralGrid) skybrightness.SpectralRadiance {
	shape := skybrightness.NewSpectralRadiance(grid)
	for i := range shape {
		lambda := float64(grid.At(i)) * 1e-9
		shape[i] = 1 / (math.Pow(lambda, 5) * (math.Exp(0.0143877696/(lambda*5500)) - 1))
	}

	return shape
}

// starlightAt evaluates the component and returns the band average of what it
// added.
func starlightAt(
	t *testing.T,
	isl *skybrightness.IntegratedStarlight,
	grid unit.SpectralGrid,
	scene *skybrightness.Scene,
	alt float64,
) (mean float64, flags skybrightness.Flag) {
	t.Helper()

	dir := coord.NewAltAz(angle.Deg(alt), angle.Deg(0))

	dst := skybrightness.NewSpectralRadiance(grid)

	var err error

	flags, err = isl.AddRadiance(context.Background(), dst, grid, dir, scene)
	if err != nil {
		t.Fatalf("AddRadiance at %g degrees: %v", alt, err)
	}

	mean, err = magnitude.MeanFluxDensity(dst, grid, testBand(), 0.99)
	if err != nil {
		t.Fatalf("MeanFluxDensity: %v", err)
	}

	return mean, flags
}

// The map holds one number per direction: the band-averaged spectral radiance.
// Above the atmosphere, the spectrum the component adds must average back to
// exactly that number over the same band.
//
// That is what makes the scaling a definition rather than an approximation. It
// is checked with the atmosphere switched off, so only the normalisation is
// under test.
func TestIntegratedStarlightReproducesTheMapValue(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	band := testBand()

	const value = 4.2e-9

	isl, err := skybrightness.NewIntegratedStarlight(uniformSky{value: value}, solarShape(grid), grid, band)
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	scene := starlightScene(t)

	// A vacuum: no molecules and no aerosol, so nothing is removed between
	// the map and the observer.
	vacuum, err := atmosphere.NewBuilder().
		Surface(1e-6, 284).
		Aerosol(0, 550, 1.3, 0.95, 0.65).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	scene.Atmosphere = vacuum

	got, _ := starlightAt(t, isl, grid, scene, 89.9)

	if math.Abs(got-value)/value > 1e-5 {
		t.Errorf("band average = %.6e, want %.6e", got, value)
	}
}

// Refining the spectral grid must not change how much starlight there is.
//
// This is the regression test for a normalisation that divided by the sum of
// the shape's samples rather than by its passband average. That made the total
// come out right only for whatever sampling the shape happened to have, and
// halved the starlight whenever the grid was refined. The failure is silent —
// every value stays positive and plausible — so nothing but an explicit
// resolution sweep catches it.
func TestIntegratedStarlightIsIndependentOfGridResolution(t *testing.T) {
	t.Parallel()

	const value = 4.2e-9

	scene := starlightScene(t)
	band := testBand()

	// The same 400-700 nm span at 1 nm, 0.5 nm and 0.25 nm sampling.
	means := make([]float64, 0, 3)

	for _, step := range []unit.WavelengthNM{1, 0.5, 0.25} {
		grid, err := unit.NewSpectralGrid(400, step, int(300/float64(step))+1)
		if err != nil {
			t.Fatalf("NewSpectralGrid(%v): %v", step, err)
		}

		isl, err := skybrightness.NewIntegratedStarlight(
			uniformSky{value: value}, solarShape(grid), grid, band)
		if err != nil {
			t.Fatalf("NewIntegratedStarlight(%v): %v", step, err)
		}

		mean, _ := starlightAt(t, isl, grid, scene, 70)
		means = append(means, mean)
	}

	for i, mean := range means[1:] {
		if math.Abs(mean-means[0])/means[0] > 1e-3 {
			t.Errorf("refinement %d gave %.6e, want %.6e within 0.1 per cent",
				i+1, mean, means[0])
		}
	}
}

// Extinction removes light, and removes more of it through more air. This is
// the directly attenuated term alone, so it is monotonic in altitude; the
// scattered term that would partly refill it is documented as absent.
func TestIntegratedStarlightDimsTowardTheHorizon(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := starlightScene(t)

	isl, err := skybrightness.NewIntegratedStarlight(
		uniformSky{value: 4.2e-9}, solarShape(grid), grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	var previous float64

	for _, alt := range []float64{80, 60, 40, 20, 10} {
		mean, flags := starlightAt(t, isl, grid, scene, alt)

		if previous != 0 && mean >= previous {
			t.Errorf("altitude %g gave %.4e, not below %.4e", alt, mean, previous)
		}

		// Below 30 degrees the missing scattered term matters enough to say so.
		wantFlag := alt < 30
		if got := flags&skybrightness.ExtrapolatedModel != 0; got != wantFlag {
			t.Errorf("altitude %g: ExtrapolatedModel = %v, want %v", alt, got, wantFlag)
		}

		previous = mean
	}
}

// Extinction is steepest at the blue end, so what arrives is redder than what
// left. Reddening is the signature of attenuating per wavelength rather than
// scaling a band-integrated number by one grey factor.
func TestIntegratedStarlightReddens(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()

	isl, err := skybrightness.NewIntegratedStarlight(
		uniformSky{value: 4.2e-9}, solarShape(grid), grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	dir := coord.NewAltAz(angle.Deg(30), angle.Deg(0))

	dst := skybrightness.NewSpectralRadiance(grid)
	if _, err := isl.AddRadiance(context.Background(), dst, grid, dir, starlightScene(t)); err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	shape := solarShape(grid)

	// 400 nm and 700 nm on the default grid, which starts at 330 nm.
	blue, red := 70, 370

	before := shape[blue] / shape[red]
	after := dst[blue] / dst[red]

	if after >= before {
		t.Errorf("blue over red went from %.4f to %.4f; extinction must redden it", before, after)
	}
}

// A direction the map does not cover is missing data, not a dark sightline.
func TestIntegratedStarlightFlagsMissingCoverage(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()

	isl, err := skybrightness.NewIntegratedStarlight(emptySky{}, solarShape(grid), grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	dir := coord.NewAltAz(angle.Deg(60), angle.Deg(0))

	dst := skybrightness.NewSpectralRadiance(grid)

	flags, err := isl.AddRadiance(context.Background(), dst, grid, dir, starlightScene(t))
	if err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	if flags&skybrightness.UnknownCloud == 0 {
		t.Errorf("flags = %v, want UnknownCloud for an uncovered direction", flags)
	}

	for i, v := range dst {
		if v != 0 {
			t.Fatalf("uncovered direction wrote %v at index %d", v, i)
		}
	}
}

// A galactic map and an ICRS map must each be read in their own frame. Reading
// one as the other rotates the Milky Way across the sky and still returns
// plausible numbers everywhere, so the only way to catch it is to make the map
// report which coordinates it was asked for.
func TestIntegratedStarlightHonoursTheMapFrame(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	scene := starlightScene(t)

	dir := coord.NewAltAz(angle.Deg(50), angle.Deg(45))

	asked := make(map[bool][2]angle.Angle, 2)

	for _, galactic := range []bool{false, true} {
		spy := &spySky{galactic: galactic}

		isl, err := skybrightness.NewIntegratedStarlight(spy, solarShape(grid), grid, testBand())
		if err != nil {
			t.Fatalf("NewIntegratedStarlight: %v", err)
		}

		if _, err := isl.AddRadiance(
			context.Background(), skybrightness.NewSpectralRadiance(grid), grid, dir, scene,
		); err != nil {
			t.Fatalf("AddRadiance: %v", err)
		}

		asked[galactic] = [2]angle.Angle{spy.lon, spy.lat}
	}

	if asked[false] == asked[true] {
		t.Fatalf("both frames were sampled at %v; the galactic rotation was not applied", asked[true])
	}

	frame := coord.NewContext(
		astrotime.FromGo(scene.Time), scene.Observer, scene.Atmosphere.Refraction())

	icrs, err := frame.AltAzToICRS(dir)
	if err != nil {
		t.Fatalf("AltAzToICRS: %v", err)
	}

	if d := math.Abs((asked[false][1] - icrs.Dec()).Degrees()); d > 1e-9 {
		t.Errorf("ICRS map was asked for declination %v, want %v", asked[false][1], icrs.Dec())
	}

	want := coord.ICRSToGalactic(icrs)
	if d := math.Abs((asked[true][1] - want.B()).Degrees()); d > 1e-9 {
		t.Errorf("galactic map was asked for latitude %v, want %v", asked[true][1], want.B())
	}
}

func TestIntegratedStarlightValidates(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()
	shape := solarShape(grid)
	sky := uniformSky{value: 1e-9}

	if _, err := skybrightness.NewIntegratedStarlight(
		nil, shape, grid, testBand()); !errors.Is(err, skybrightness.ErrNoStarMap) {
		t.Errorf("no map: err = %v, want ErrNoStarMap", err)
	}

	if _, err := skybrightness.NewIntegratedStarlight(
		sky, shape[:10], grid, testBand()); !errors.Is(err, skybrightness.ErrNoStarMap) {
		t.Errorf("short shape: err = %v, want ErrNoStarMap", err)
	}

	negative := solarShape(grid)
	negative[0] = -1

	if _, err := skybrightness.NewIntegratedStarlight(
		sky, negative, grid, testBand()); !errors.Is(err, skybrightness.ErrNoStarMap) {
		t.Errorf("negative shape: err = %v, want ErrNoStarMap", err)
	}

	// A grid that does not span the band cannot average the shape over it,
	// and guessing the missing part is exactly what would go unnoticed.
	narrow := mustGrid(t, 500, 40)
	if _, err := skybrightness.NewIntegratedStarlight(
		sky, solarShape(narrow), narrow, testBand()); err == nil {
		t.Error("a grid covering part of the band must be rejected")
	}

	// Evaluating on a grid the component was not built for is a mismatch, not
	// something to resample silently.
	isl, err := skybrightness.NewIntegratedStarlight(sky, shape, grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	other := mustGrid(t, 400, 301)

	dir := coord.NewAltAz(angle.Deg(60), angle.Deg(0))

	if _, err := isl.AddRadiance(
		context.Background(), skybrightness.NewSpectralRadiance(other), other, dir, starlightScene(t),
	); !errors.Is(err, skybrightness.ErrNoStarMap) {
		t.Errorf("grid mismatch: err = %v, want ErrNoStarMap", err)
	}
}

// Nothing below the horizon contributes.
func TestIntegratedStarlightIsZeroBelowTheHorizon(t *testing.T) {
	t.Parallel()

	grid := skybrightness.DefaultOpticalGrid()

	isl, err := skybrightness.NewIntegratedStarlight(
		uniformSky{value: 4.2e-9}, solarShape(grid), grid, testBand())
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	dir := coord.NewAltAz(angle.Deg(-5), angle.Deg(0))

	dst := skybrightness.NewSpectralRadiance(grid)
	if _, err := isl.AddRadiance(context.Background(), dst, grid, dir, starlightScene(t)); err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	for i, v := range dst {
		if v != 0 {
			t.Fatalf("below the horizon wrote %v at index %d", v, i)
		}
	}
}
