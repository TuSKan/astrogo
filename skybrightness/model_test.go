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
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// constantComponent adds a fixed radiance at every wavelength. It exists
// so the Phase 0 machinery — accumulation, validation, projection — can be
// exercised without any physics, which Phase 0 deliberately does not have.
type constantComponent struct {
	id    skybrightness.ComponentID
	value float64
}

func (c constantComponent) ID() skybrightness.ComponentID { return c.id }

func (c constantComponent) AddRadiance(
	_ context.Context,
	dst skybrightness.SpectralRadiance,
	_ unit.SpectralGrid,
	_ coord.AltAz,
	_ *skybrightness.Scene,
) (skybrightness.Flag, error) {
	for i := range dst {
		dst[i] += c.value
	}

	return 0, nil
}

func (c constantComponent) Provenance() skybrightness.Provenance {
	return skybrightness.Provenance{
		Model:            "constant test component",
		PrimaryReference: "synthetic test fixture, not a physical model",
	}
}

// badComponent emits a value the model must refuse.
type badComponent struct {
	id    skybrightness.ComponentID
	value float64
}

func (b badComponent) ID() skybrightness.ComponentID { return b.id }

func (b badComponent) AddRadiance(
	_ context.Context,
	dst skybrightness.SpectralRadiance,
	_ unit.SpectralGrid,
	_ coord.AltAz,
	_ *skybrightness.Scene,
) (skybrightness.Flag, error) {
	dst[0] = b.value

	return 0, nil
}

func (b badComponent) Provenance() skybrightness.Provenance { return skybrightness.Provenance{} }

func testScene(t *testing.T) *skybrightness.Scene {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635) // Paranal
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   loc,
		Time:       gotime.Date(2026, 8, 14, 3, 0, 0, 0, gotime.UTC),
		Atmosphere: atmosphere.StandardDefault(2635),
	}
}

func zenith() coord.AltAz { return coord.NewAltAz(angle.Deg(90), angle.Zero()) }

// A model with no components is the honest Phase 0 state: zero radiance,
// flagged as such, rather than a plausible-looking dark sky.
func TestEmptyModelIsZeroAndFlagged(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test")
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	est, err := m.Estimate(context.Background(), skybrightness.Query{
		Scene:     testScene(t),
		Direction: zenith(),
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	if !est.Quality.Has(skybrightness.NoComponents) {
		t.Errorf("Quality = %v, want NoComponents", est.Quality)
	}

	for i, v := range est.SpectralRadiance() {
		if v != 0 {
			t.Fatalf("sample %d = %v, want 0 from an empty model", i, v)
		}
	}

	// A zero sky is infinitely faint, not an error and not a NaN.
	sb, err := est.SurfaceBrightness(testBand(), magnitude.AB)
	if err != nil {
		t.Fatalf("SurfaceBrightness: %v", err)
	}

	if !math.IsInf(sb, 1) {
		t.Errorf("SurfaceBrightness of a zero sky = %v, want +Inf", sb)
	}
}

// Components add in linear radiance space. Summing magnitudes instead
// would be a correctness bug, so the additivity is asserted directly.
func TestComponentsSumLinearly(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test",
		constantComponent{id: skybrightness.Starlight, value: 1e-9},
		constantComponent{id: skybrightness.Zodiacal, value: 2e-9},
		constantComponent{id: skybrightness.AirglowContinuum, value: 3e-9},
	)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	est, err := m.Estimate(context.Background(), skybrightness.Query{
		Scene:     testScene(t),
		Direction: zenith(),
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	const want = 6e-9

	for i, v := range est.SpectralRadiance() {
		if math.Abs(v-want) > 1e-21 {
			t.Fatalf("total[%d] = %v, want %v", i, v, want)
		}
	}

	// Each component's own contribution survives separately, which is what
	// makes a breakdown possible.
	for id, want := range map[skybrightness.ComponentID]float64{
		skybrightness.Starlight:        1e-9,
		skybrightness.Zodiacal:         2e-9,
		skybrightness.AirglowContinuum: 3e-9,
	} {
		got, ok := est.Component(id)
		if !ok {
			t.Fatalf("component %q missing from the breakdown", id)
		}

		if math.Abs(got[0]-want) > 1e-21 {
			t.Errorf("component %q = %v, want %v", id, got[0], want)
		}
	}
}

// Two components claiming the same physical contribution would
// double-count it.
func TestDuplicateComponentsRejected(t *testing.T) {
	t.Parallel()

	_, err := skybrightness.NewModel("test",
		constantComponent{id: skybrightness.Starlight, value: 1},
		constantComponent{id: skybrightness.Starlight, value: 2},
	)

	if !errors.Is(err, skybrightness.ErrDuplicateComponent) {
		t.Errorf("NewModel with duplicates = %v, want ErrDuplicateComponent", err)
	}
}

// A negative or non-finite radiance is physically impossible and must be
// caught at the component that produced it, not silently summed away.
func TestRejectsImpossibleRadiance(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		value float64
		want  error
	}{
		{"negative", -1e-9, skybrightness.ErrNegativeRadiance},
		{"NaN", math.NaN(), skybrightness.ErrNonFiniteRadiance},
		{"infinite", math.Inf(1), skybrightness.ErrNonFiniteRadiance},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			m, err := skybrightness.NewModel("test", badComponent{id: skybrightness.Artificial, value: tc.value})
			if err != nil {
				t.Fatalf("NewModel: %v", err)
			}

			_, err = m.Estimate(context.Background(), skybrightness.Query{
				Scene:     testScene(t),
				Direction: zenith(),
			})

			if !errors.Is(err, tc.want) {
				t.Errorf("Estimate = %v, want %v", err, tc.want)
			}

			// The failure must name the offending component.
			if !errors.Is(err, skybrightness.ErrComponentFailed) {
				t.Errorf("Estimate = %v, want it wrapped in ErrComponentFailed", err)
			}
		})
	}
}

// A scene missing its observer, atmosphere or time is refused rather than
// defaulted: an unstated atmosphere is the largest source of silent error
// in a sky prediction.
func TestSceneValidation(t *testing.T) {
	t.Parallel()

	full := testScene(t)

	cases := []struct {
		name  string
		scene *skybrightness.Scene
		want  error
	}{
		{"nil", nil, skybrightness.ErrNoObserver},
		{"no observer", &skybrightness.Scene{Time: full.Time, Atmosphere: full.Atmosphere}, skybrightness.ErrNoObserver},
		{"no atmosphere", &skybrightness.Scene{Observer: full.Observer, Time: full.Time}, skybrightness.ErrNoAtmosphere},
		{"no time", &skybrightness.Scene{Observer: full.Observer, Atmosphere: full.Atmosphere}, skybrightness.ErrNoTime},
	}

	m, err := skybrightness.NewModel("test")
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := m.Estimate(context.Background(), skybrightness.Query{Scene: tc.scene, Direction: zenith()})
			if !errors.Is(err, tc.want) {
				t.Errorf("Estimate = %v, want %v", err, tc.want)
			}
		})
	}
}

// Component selection restricts evaluation without changing the physics of
// what remains.
func TestComponentSelection(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test",
		constantComponent{id: skybrightness.Starlight, value: 1e-9},
		constantComponent{id: skybrightness.Artificial, value: 5e-9},
	)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	est, err := m.Estimate(context.Background(), skybrightness.Query{
		Scene:      testScene(t),
		Direction:  zenith(),
		Components: []skybrightness.ComponentID{skybrightness.Starlight},
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	if got := est.SpectralRadiance()[0]; math.Abs(got-1e-9) > 1e-21 {
		t.Errorf("selected-only total = %v, want 1e-9", got)
	}

	if _, ok := est.Component(skybrightness.Artificial); ok {
		t.Error("a deselected component must not appear in the breakdown")
	}
}

// Evaluation must be deterministic for a fixed scene: two calls produce
// byte-identical spectra. Anything else makes a scientific result
// impossible to reproduce.
func TestEstimateIsDeterministic(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test", constantComponent{id: skybrightness.Starlight, value: 1e-9})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	q := skybrightness.Query{Scene: testScene(t), Direction: zenith()}

	first, err := m.Estimate(context.Background(), q)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	second, err := m.Estimate(context.Background(), q)
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	a, b := first.SpectralRadiance(), second.SpectralRadiance()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs between runs: %v vs %v", i, a[i], b[i])
		}
	}
}

// A Model is shared across a scheduler's goroutines, so concurrent
// independent evaluation must be safe. Run under -race this is the real
// assertion.
func TestConcurrentEstimatesAreSafe(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test",
		constantComponent{id: skybrightness.Starlight, value: 1e-9},
		constantComponent{id: skybrightness.Zodiacal, value: 2e-9},
	)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	scene := testScene(t)

	var wg sync.WaitGroup

	for i := range 16 {
		wg.Add(1)

		go func(alt float64) {
			defer wg.Done()

			est, err := m.Estimate(context.Background(), skybrightness.Query{
				Scene:     scene,
				Direction: coord.NewAltAz(angle.Deg(alt), angle.Deg(alt*3)),
			})
			if err != nil {
				t.Errorf("Estimate(alt=%v): %v", alt, err)

				return
			}

			if got := est.SpectralRadiance()[0]; math.Abs(got-3e-9) > 1e-21 {
				t.Errorf("Estimate(alt=%v) = %v, want 3e-9", alt, got)
			}
		}(float64(i*5) + 5)
	}

	wg.Wait()
}

// Directions that historically break coordinate handling must produce a
// finite, non-negative sky rather than a NaN.
func TestNumericalStabilityAcrossGeometry(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test", constantComponent{id: skybrightness.Starlight, value: 1e-9})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	sites := []struct {
		name              string
		lon, lat, heightM float64
	}{
		{"north pole", 0, 90, 0},
		{"south pole", 0, -90, 0},
		{"equator", 0, 0, 0},
		{"date line", 180, 0, 0},
		{"just west of date line", -179.999, 0, 0},
		{"sea level", 10, 50, 0},
		{"very high", -70.4, -24.6, 5600},
	}

	directions := []struct {
		name    string
		alt, az float64
	}{
		{"zenith", 90, 0},
		{"high", 60, 45},
		{"30 degrees", 30, 120},
		{"15 degrees", 15, 200},
		{"near horizon", 1, 300},
		{"horizon", 0, 359.999},
	}

	for _, s := range sites {
		for _, d := range directions {
			loc, err := coord.NewGeodetic(angle.Deg(s.lon), angle.Deg(s.lat), s.heightM)
			if err != nil {
				t.Fatalf("NewGeodetic(%s): %v", s.name, err)
			}

			est, err := m.Estimate(context.Background(), skybrightness.Query{
				Scene: &skybrightness.Scene{
					Observer:   loc,
					Time:       gotime.Date(2026, 8, 14, 3, 0, 0, 0, gotime.UTC),
					Atmosphere: atmosphere.StandardDefault(s.heightM),
				},
				Direction: coord.NewAltAz(angle.Deg(d.alt), angle.Deg(d.az)),
			})
			if err != nil {
				t.Fatalf("Estimate(%s, %s): %v", s.name, d.name, err)
			}

			for i, v := range est.SpectralRadiance() {
				if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
					t.Fatalf("%s/%s sample %d = %v, want finite and non-negative", s.name, d.name, i, v)
				}
			}
		}
	}
}

// testBand is a synthetic response used to exercise projection, not a
// published curve.
func testBand() magnitude.Passband {
	return magnitude.Passband{
		Name:         "test",
		WavelengthNM: []unit.WavelengthNM{499, 500, 600, 601},
		Response:     []float64{0, 1, 1, 0},
		Detector:     magnitude.PhotonCounting,
		Reference:    "synthetic test fixture",
	}
}

// mustGrid builds a 1 nm grid for tests.
func mustGrid(t *testing.T, start unit.WavelengthNM, n int) unit.SpectralGrid {
	t.Helper()

	g, err := unit.NewSpectralGrid(start, 1, n)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	return g
}
