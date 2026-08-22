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
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// cityAt builds a uniform emitter at a bearing and distance from Paranal,
// with a flat spectrum and a mostly unshielded emission function.
func cityAt(tb testing.TB, bearingDeg, distanceKM, radiance float64) *skybrightness.UniformEmitter {
	tb.Helper()

	// Walk out from the observer along a great circle. At these distances a
	// flat-Earth offset in latitude and longitude is accurate enough for a
	// test fixture, and the component measures the real ground distance
	// itself.
	const degPerKM = 1 / 111.195

	bearing := bearingDeg * math.Pi / 180
	lat := -24.6 + distanceKM*degPerKM*math.Cos(bearing)
	lon := -70.4 + distanceKM*degPerKM*math.Sin(bearing)/math.Cos(-24.6*math.Pi/180)

	at, err := coord.NewGeodetic(angle.Deg(lon), angle.Deg(lat), 2000)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	return &skybrightness.UniformEmitter{
		At:           at,
		Name:         "test city",
		WavelengthNM: []unit.WavelengthNM{300, 400, 500, 600, 700, 800, 1100},
		Radiance:     []float64{radiance, radiance, radiance, radiance, radiance, radiance, radiance},
		Emission: skybrightness.UpwardEmission{
			Cosine:             1,
			HorizontalFraction: 0.3,
		},
		Flags: skybrightness.AssumedSourceSpectrum | skybrightness.AssumedEmissionFunction,
	}
}

func artificialScene(t *testing.T) *skybrightness.Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.05, 550, 1.3, 0.9, 0.7).
		BoundaryLayer(1500).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2026, 3, 3, 5, 0, 0, 0, gotime.UTC),
		Atmosphere: atm,
	}
}

// radianceAt evaluates one component in one direction at one wavelength.
func radianceAt(t *testing.T, c skybrightness.Component, scene *skybrightness.Scene, alt, az float64) float64 {
	t.Helper()

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	if _, err := c.AddRadiance(context.Background(), dst, grid,
		coord.NewAltAz(angle.Deg(alt), angle.Deg(az)), scene); err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	return dst[gridIndex(t, grid, 554)]
}

// The claim that matters for a light-pollution model: a city twice as far
// away contributes less skyglow, not more.
//
// This is where the component earns the contract on [AllSkyRadiance]. The
// kernel by itself *grows* with distance, because distance reaches it only
// through the atmospheric parameter t. Getting a falling curve out requires
// the component to apply the transmission e^{-M_S*t} to the emitter's output
// first, which is the step an earlier revision of this package got wrong for
// long enough to withdraw the whole kernel over.
func TestArtificialSkyglowFallsWithDistance(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	prev := math.Inf(1)

	for _, km := range []float64{5, 10, 20, 40, 80} {
		component, err := skybrightness.NewArtificialSkyglow(
			[]skybrightness.GroundEmitter{cityAt(t, 0, km, 1e-3)})
		if err != nil {
			t.Fatalf("NewArtificialSkyglow: %v", err)
		}

		got := radianceAt(t, component, scene, 45, 0)

		if got <= 0 {
			t.Fatalf("a city at %v km contributed %v", km, got)
		}

		if got >= prev {
			t.Errorf("a city at %v km contributes %.4g, more than the %.4g of the nearer one", km, got, prev)
		}

		prev = got
	}
}

// The azimuthal structure of skyglow, which is the whole reason to have a
// directional model: a scalar "sky quality" number returns the same value
// everywhere and cannot tell an observer where to point.
//
// The profile is NOT monotonic from the city outward, and asserting that it
// were would be testing an assumption. The minimum sits about 90 degrees away
// in azimuth, where the scattering angle passes through a right angle, and
// the sky brightens again toward the anti-city direction. That rise is the
// back-scattering lobe of the Rayleigh phase function, which goes as
// 1.06 + cos^2(Theta) and is therefore equally strong at 20 and 160 degrees;
// what tilts the profile forward on top of it is the Henyey-Greenstein
// aerosol term.
//
// So the testable claims are: the city direction dominates, and the darkest
// sky is at right angles to it rather than opposite it.
func TestArtificialSkyglowIsDirectional(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	component, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 0, 30, 1e-3)}) // due north
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	toward := radianceAt(t, component, scene, 20, 0)   // low, toward the city
	across := radianceAt(t, component, scene, 20, 90)  // low, at right angles
	away := radianceAt(t, component, scene, 20, 180)   // low, away from it
	overhead := radianceAt(t, component, scene, 90, 0) // zenith

	if toward <= across || toward <= away {
		t.Errorf("the city direction does not dominate: toward %.4g, across %.4g, away %.4g",
			toward, across, away)
	}

	if across >= away {
		t.Errorf("the darkest azimuth is %.4g at right angles against %.4g opposite the city; "+
			"the Rayleigh back-scattering lobe is missing", across, away)
	}

	if toward <= overhead {
		t.Errorf("low toward the city (%.4g) is not brighter than the zenith (%.4g)", toward, overhead)
	}

	// The contrast has to be worth acting on, not a rounding difference.
	if ratio := toward / across; ratio < 2 {
		t.Errorf("brightest/darkest azimuth = %.2f; too flat to guide an observer", ratio)
	}
}

// Sources add in linear radiance space. Two identical cities at the same
// distance in the same direction must give exactly twice one of them —
// summing in magnitudes, the classic error, would give something else.
func TestArtificialSkyglowSumsLinearly(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	one, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 45, 25, 1e-3)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	two, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{
		cityAt(t, 45, 25, 1e-3),
		cityAt(t, 45, 25, 1e-3),
	})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	single := radianceAt(t, one, scene, 30, 45)
	double := radianceAt(t, two, scene, 30, 45)

	if rel := math.Abs(double-2*single) / (2 * single); rel > 1e-12 {
		t.Errorf("two identical cities gave %.6g, want twice the %.6g of one", double, single)
	}

	// And a brighter city scales the same way, since Eq. 2 is linear in L_S.
	brighter, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 45, 25, 3e-3)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	if got := radianceAt(t, brighter, scene, 30, 45); math.Abs(got-3*single)/(3*single) > 1e-12 {
		t.Errorf("three times the source radiance gave %.6g, want %.6g", got, 3*single)
	}
}

// Shielding is the lever that actually reduces skyglow, and the model has to
// show it. A fully shielded source — nothing emitted near the horizontal —
// must contribute far less than an unshielded one of identical output.
func TestArtificialSkyglowRespondsToShielding(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	shielded := cityAt(t, 0, 30, 1e-3)
	shielded.Emission = skybrightness.UpwardEmission{Cosine: 3, HorizontalFraction: 0}

	unshielded := cityAt(t, 0, 30, 1e-3)
	unshielded.Emission = skybrightness.UpwardEmission{Cosine: 0, HorizontalFraction: 1}

	dark, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{shielded})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	bright, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{unshielded})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	shieldedGlow := radianceAt(t, dark, scene, 30, 0)
	unshieldedGlow := radianceAt(t, bright, scene, 30, 0)

	if shieldedGlow >= unshieldedGlow {
		t.Errorf("shielded source gives %.4g, not less than the unshielded %.4g",
			shieldedGlow, unshieldedGlow)
	}

	// At zero escape elevation a cosine-only source emits nothing at all
	// along the path that reaches a distant sky.
	if shieldedGlow != 0 {
		t.Errorf("a fully shielded source contributed %.4g at the horizon escape angle", shieldedGlow)
	}
}

// The escape elevation is the component's own modelling choice, so raising
// it must visibly change the answer — otherwise the option is decorative and
// the doc comment describing it is wrong.
func TestArtificialSkyglowEscapeElevation(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	shielded := cityAt(t, 0, 30, 1e-3)
	shielded.Emission = skybrightness.UpwardEmission{Cosine: 1, HorizontalFraction: 0}

	atHorizon, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{shielded})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	steep, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{shielded},
		skybrightness.WithEscapeElevation(angle.Deg(30)))
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	flat := radianceAt(t, atHorizon, scene, 40, 0)
	raised := radianceAt(t, steep, scene, 40, 0)

	if flat != 0 {
		t.Errorf("a pure cosine source gave %.4g at zero escape elevation, want 0", flat)
	}

	if raised <= 0 {
		t.Errorf("raising the escape elevation to 30 degrees gave %.4g, want a positive contribution", raised)
	}
}

// The emitter's own quality flags have to reach the estimate: an assumed
// source spectrum is the single largest uncertainty in artificial skyglow,
// and a result that hides it is misleading.
func TestArtificialSkyglowPropagatesEmitterFlags(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	component, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 0, 30, 1e-3)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	flags, err := component.AddRadiance(context.Background(), dst, grid,
		coord.NewAltAz(angle.Deg(30), angle.Deg(0)), scene)
	if err != nil {
		t.Fatalf("AddRadiance: %v", err)
	}

	if flags&skybrightness.AssumedSourceSpectrum == 0 {
		t.Error("the emitter's AssumedSourceSpectrum flag did not reach the caller")
	}

	if flags&skybrightness.AssumedEmissionFunction == 0 {
		t.Error("the emitter's AssumedEmissionFunction flag did not reach the caller")
	}
}

// A line of sight below the horizon looks at ground, not sky.
func TestArtificialSkyglowBelowHorizon(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	component, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 0, 30, 1e-3)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	if got := radianceAt(t, component, scene, -5, 0); got != 0 {
		t.Errorf("looking 5 degrees below the horizon gave %v, want 0", got)
	}
}

// The per-scene cache is keyed on the grid as well as the scene, because
// every cached array is grid-length. A stale entry would be a bounds panic
// at best and silently wrong wavelengths at worst.
func TestArtificialSkyglowCacheRespectsGrid(t *testing.T) {
	t.Parallel()

	scene := artificialScene(t)

	component, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 0, 30, 1e-3)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	dir := coord.NewAltAz(angle.Deg(30), angle.Deg(0))

	wide := skybrightness.DefaultOpticalGrid()

	narrow, err := unit.NewSpectralGrid(500, 5, 40) // 500-695 nm
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	at := func(grid unit.SpectralGrid) float64 {
		dst := skybrightness.NewSpectralRadiance(grid)
		if _, err := component.AddRadiance(context.Background(), dst, grid, dir, scene); err != nil {
			t.Fatalf("AddRadiance: %v", err)
		}

		return dst[gridIndex(t, grid, 550)]
	}

	first := at(wide)
	second := at(narrow)
	third := at(wide)

	if first != third {
		t.Errorf("returning to the first grid gave %v, not the original %v", third, first)
	}

	// The same wavelength through the same atmosphere from the same city is
	// the same radiance, whichever grid it was evaluated on.
	if rel := math.Abs(second-first) / first; rel > 1e-9 {
		t.Errorf("550 nm on the narrow grid gave %v against %v on the wide one", second, first)
	}
}

func TestNewArtificialSkyglowRejectsBadInput(t *testing.T) {
	t.Parallel()

	if _, err := skybrightness.NewArtificialSkyglow(nil); !errors.Is(err, skybrightness.ErrNoEmitters) {
		t.Errorf("no emitters: err = %v, want ErrNoEmitters", err)
	}

	locationless := &skybrightness.UniformEmitter{Name: "nowhere"}
	if _, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{locationless}); !errors.Is(err, skybrightness.ErrNoEmitterLocation) {
		t.Errorf("emitter with no location: err = %v, want ErrNoEmitterLocation", err)
	}
}

func TestArtificialSkyglowProvenance(t *testing.T) {
	t.Parallel()

	component, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 0, 30, 1e-3)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	p := component.Provenance()

	if p.PrimaryReference == "" || p.ValidityDomain == "" {
		t.Error("provenance is missing its primary reference or validity domain")
	}

	if len(p.KnownApproximations) == 0 {
		t.Error("provenance records no approximations, but M_S and the escape elevation are both choices")
	}

	if component.ID() != skybrightness.Artificial {
		t.Errorf("ID = %q, want %q", component.ID(), skybrightness.Artificial)
	}
}

func BenchmarkArtificialSkyglow(b *testing.B) {
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		b.Fatal(err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0.05, 550, 1.3, 0.9, 0.7).
		BoundaryLayer(1500).
		Build()
	if err != nil {
		b.Fatal(err)
	}

	city, err := coord.NewGeodetic(angle.Deg(-70.1), angle.Deg(-24.4), 2000)
	if err != nil {
		b.Fatal(err)
	}

	emitter := &skybrightness.UniformEmitter{
		At:           city,
		Name:         "benchmark city",
		WavelengthNM: []unit.WavelengthNM{300, 600, 1100},
		Radiance:     []float64{1e-3, 1e-3, 1e-3},
		Emission:     skybrightness.UpwardEmission{Cosine: 1, HorizontalFraction: 0.3},
	}

	component, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{emitter})
	if err != nil {
		b.Fatal(err)
	}

	scene := &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2026, 3, 3, 5, 0, 0, 0, gotime.UTC),
		Atmosphere: atm,
	}

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)
	dir := coord.NewAltAz(angle.Deg(30), angle.Deg(10))

	// Warm the per-scene cache, which is what a sky map amortises.
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

// The band-independent Rayleigh phase function is hoisted out of the
// per-band loop rather than going through atmosphere.CombinedPhaseFunction
// each time. That is a performance change, so it needs a test that the
// weighting still matches — a subtly wrong combination would keep every
// ratio in the tests above intact while shifting the spectrum.
//
// With the aerosol removed the combined phase function is pure Rayleigh, and
// two directions at the same altitude share an identical airmass and kernel.
// Their radiance ratio must then equal the Rayleigh phase-function ratio
// exactly, which pins the weighting with nothing else in the way.
func TestArtificialSkyglowPhaseWeighting(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(743, 284).
		Aerosol(0, 550, 1.3, 0.9, 0.7). // molecular atmosphere only
		BoundaryLayer(1500).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	scene := &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2026, 3, 3, 5, 0, 0, 0, gotime.UTC),
		Atmosphere: atm,
	}

	component, err := skybrightness.NewArtificialSkyglow(
		[]skybrightness.GroundEmitter{cityAt(t, 0, 30, 1e-3)})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	const alt = 25.0

	source := coord.NewAltAz(0, 0) // the city, at the horizon due north

	ratioAt := func(azA, azB float64) (got, want float64) {
		a := coord.NewAltAz(angle.Deg(alt), angle.Deg(azA))
		b := coord.NewAltAz(angle.Deg(alt), angle.Deg(azB))

		thetaA := math.Acos(source.ToUnitVector().Dot(a.ToUnitVector()))
		thetaB := math.Acos(source.ToUnitVector().Dot(b.ToUnitVector()))

		want = atmosphere.RayleighPhaseFunction(thetaA, atmosphere.RayleighDepolarisation) /
			atmosphere.RayleighPhaseFunction(thetaB, atmosphere.RayleighDepolarisation)

		got = radianceAt(t, component, scene, alt, azA) / radianceAt(t, component, scene, alt, azB)

		return got, want
	}

	for _, pair := range [][2]float64{{0, 90}, {45, 135}, {30, 180}} {
		got, want := ratioAt(pair[0], pair[1])

		if rel := math.Abs(got-want) / want; rel > 1e-12 {
			t.Errorf("azimuths %v and %v: radiance ratio %.10g, want the Rayleigh phase ratio %.10g",
				pair[0], pair[1], got, want)
		}
	}
}
