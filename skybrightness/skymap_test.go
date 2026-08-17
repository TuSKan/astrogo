package skybrightness_test

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness"
)

// A sky map must cover the hemisphere: summing every sample's solid angle
// gives 2*pi steradians. A ring-weighting bug shows up here immediately.
func TestSkyMapCoversHemisphere(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test", constantComponent{id: skybrightness.Starlight, value: 1e-9})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	points, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: testScene(t)}, 12)
	if err != nil {
		t.Fatalf("SkyMap: %v", err)
	}

	if len(points) == 0 {
		t.Fatal("SkyMap returned no samples")
	}

	var total float64
	for _, p := range points {
		total += p.SolidAngleSR
	}

	if rel := math.Abs(total-2*math.Pi) / (2 * math.Pi); rel > 1e-9 {
		t.Errorf("total solid angle = %v sr, want %v (rel %g)", total, 2*math.Pi, rel)
	}
}

// Every sample must be evaluated at its own direction, not copied from the
// zenith — directional structure is the point of an all-sky map.
func TestSkyMapSamplesDistinctDirections(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test")
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	points, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: testScene(t)}, 6)
	if err != nil {
		t.Fatalf("SkyMap: %v", err)
	}

	seen := make(map[string]struct{}, len(points))

	for _, p := range points {
		key := p.Direction.Alt().String() + "/" + p.Direction.Az().String()
		if _, dup := seen[key]; dup {
			t.Errorf("direction %s sampled twice", key)
		}

		seen[key] = struct{}{}

		// No sample sits exactly on the horizon, where airmass diverges.
		if p.Direction.Alt().Degrees() <= 0 {
			t.Errorf("sample at altitude %v, want strictly above the horizon", p.Direction.Alt().Degrees())
		}
	}
}

func TestSkyMapRejectsZeroRings(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test")
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	if _, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: testScene(t)}, 0); !errors.Is(err, skybrightness.ErrRingCount) {
		t.Errorf("SkyMap(0 rings) = %v, want ErrRingCount", err)
	}
}

// A uniform sky of radiance L gives a horizontal irradiance of pi*L per
// nanometre — the standard Lambertian result. It is the one closed form
// available for the hemispheric integral, so it pins the cos(alt)
// weighting and the solid-angle bookkeeping together.
func TestIntegratedHemisphereUniformSky(t *testing.T) {
	t.Parallel()

	const radiance = 1e-9

	m, err := skybrightness.NewModel("test", constantComponent{id: skybrightness.Starlight, value: radiance})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	points, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: testScene(t), Grid: grid}, 60)
	if err != nil {
		t.Fatalf("SkyMap: %v", err)
	}

	spectrum, err := skybrightness.IntegratedHemisphere(points, grid)
	if err != nil {
		t.Fatalf("IntegratedHemisphere: %v", err)
	}

	want := math.Pi * radiance

	// The ring quadrature is a discretisation of the cos-weighted integral,
	// so it converges to pi*L rather than hitting it exactly; 1% at 60
	// rings is the expected accuracy of this sampling.
	if rel := math.Abs(float64(spectrum[0])-want) / want; rel > 0.01 {
		t.Errorf("hemispheric irradiance = %v per nm, want ~%v (rel %g)", spectrum[0], want, rel)
	}
}

// HorizontalIlluminance is the spectrally integrated form of the same
// quantity, so it must equal the per-nm result times the grid span for a
// flat spectrum.
func TestHorizontalIlluminanceMatchesSpectralIntegral(t *testing.T) {
	t.Parallel()

	const radiance = 1e-9

	m, err := skybrightness.NewModel("test", constantComponent{id: skybrightness.Starlight, value: radiance})
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	grid := skybrightness.DefaultOpticalGrid()

	points, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: testScene(t), Grid: grid}, 60)
	if err != nil {
		t.Fatalf("SkyMap: %v", err)
	}

	got, err := skybrightness.HorizontalIlluminance(points, grid)
	if err != nil {
		t.Fatalf("HorizontalIlluminance: %v", err)
	}

	span := float64(grid.EndNM() - grid.StartNM)
	want := math.Pi * radiance * span

	if rel := math.Abs(float64(got)-want) / want; rel > 0.01 {
		t.Errorf("HorizontalIlluminance = %v W/m^2, want ~%v (rel %g)", float64(got), want, rel)
	}
}

// Mixing grids between a map and its integral would silently produce
// nonsense, so it is refused.
func TestIntegratedHemisphereRejectsGridMismatch(t *testing.T) {
	t.Parallel()

	m, err := skybrightness.NewModel("test")
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	points, err := m.SkyMap(context.Background(), skybrightness.Query{Scene: testScene(t)}, 3)
	if err != nil {
		t.Fatalf("SkyMap: %v", err)
	}

	other := mustGrid(t, 400, 100)

	if _, err := skybrightness.IntegratedHemisphere(points, other); err == nil {
		t.Error("expected a grid-mismatch error")
	}
}
