package skybrightness_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/natural"
)

// fixedTransmission is a TransmissionModel test double returning a
// constant transmission at every wavelength, so Point's
// ComputeTransmission path has something deterministic to reproduce.
type fixedTransmission struct{ value skybrightness.Transmission }

func (fixedTransmission) Algorithm() skybrightness.AlgorithmRef {
	return skybrightness.AlgorithmRef{Name: "fixedTransmission", Version: "test"}
}

func (f fixedTransmission) LineOfSight(_ coord.AltAz, _ *atmosphere.Atmosphere, g skybrightness.SpectralGrid, out []skybrightness.Transmission) error {
	for i := range out {
		out[i] = f.value
	}

	_ = g

	return nil
}

func testEngineWithTransmission(t *testing.T, tr skybrightness.TransmissionModel, components ...skybrightness.Component) *skybrightness.CompositeEngine {
	t.Helper()

	eng, err := skybrightness.NewCompositeEngine(skybrightness.CompositeConfig{
		Name:         skybrightness.AlgorithmRef{Name: "test-engine-transmission", Version: "1"},
		Components:   components,
		Transmission: tr,
		Mode:         skybrightness.ModeUserSupplied,
	})
	if err != nil {
		t.Fatalf("NewCompositeEngine: %v", err)
	}

	return eng
}

// TestPoint_ComputeTransmission proves Point's new ComputeTransmission
// field reproduces exactly what Engine.Evaluate produces — the concrete
// fix for the gap that forced examples/18_sky_brightness back onto
// Evaluate directly (see docs/skybrightness.md §15).
func TestPoint_ComputeTransmission(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid()
	dir := coord.NewAltAz(angle.Deg(45), angle.Deg(90))
	astro := coord.NewContext(testTime(), testSite(), testAtmosphere())

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("atmosphere.NewBuilder().Build(): %v", err)
	}

	eng := testEngineWithTransmission(t, fixedTransmission{value: 0.5}, constComponent{id: skybrightness.Airglow, value: 1.0})

	req, err := skybrightness.NewRequestBuilder(astro, []coord.AltAz{dir}, grid).
		Atmosphere(atm).
		Mode(skybrightness.ModeUserSupplied).
		Transmission().
		Build()
	if err != nil {
		t.Fatalf("RequestBuilder.Build(): %v", err)
	}

	want, err := eng.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	got, err := skybrightness.Point(context.Background(), eng, skybrightness.PointQuery{
		Astro: astro, Direction: dir, Mode: skybrightness.ModeUserSupplied, Atmosphere: atm, Grid: grid,
		ComputeTransmission: true,
	})
	if err != nil {
		t.Fatalf("Point: %v", err)
	}

	if len(got.Transmission) != len(want.Transmission) {
		t.Fatalf("Point transmission length = %d, Evaluate = %d", len(got.Transmission), len(want.Transmission))
	}

	for i := range got.Transmission {
		if got.Transmission[i] != want.Transmission[i] {
			t.Errorf("Transmission[%d] = %v, want %v (from Evaluate)", i, got.Transmission[i], want.Transmission[i])
		}
	}
}

// TestPoint_LimitingMag proves Point's new LimitingMag field reproduces
// exactly what Engine.Evaluate + DeriveLimitingMag produces.
func TestPoint_LimitingMag(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid()
	dir := coord.NewAltAz(angle.Deg(45), angle.Deg(90))
	astro := coord.NewContext(testTime(), testSite(), testAtmosphere())
	johnsonV := natural.TopHatJohnsonV()
	limMag := skybrightness.NewSchaeferNELM()

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("atmosphere.NewBuilder().Build(): %v", err)
	}

	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})

	req, err := skybrightness.NewRequestBuilder(astro, []coord.AltAz{dir}, grid).
		Atmosphere(atm).
		Mode(skybrightness.ModeUserSupplied).
		Passbands(johnsonV).
		LimitingMag(limMag).
		Build()
	if err != nil {
		t.Fatalf("RequestBuilder.Build(): %v", err)
	}

	want, err := eng.Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if len(want.Derived.LimitingMagnitude) == 0 {
		t.Fatal("Evaluate produced no LimitingMagnitude — fixture is broken")
	}

	got, err := skybrightness.Point(context.Background(), eng, skybrightness.PointQuery{
		Astro: astro, Direction: dir, Passband: johnsonV, Mode: skybrightness.ModeUserSupplied,
		Atmosphere: atm, Grid: grid, LimitingMag: limMag,
	})
	if err != nil {
		t.Fatalf("Point: %v", err)
	}

	if !got.HasLimitingMag {
		t.Fatal("Point: HasLimitingMag = false, want true")
	}

	if got.LimitingMagnitude != want.Derived.LimitingMagnitude[0] {
		t.Errorf("Point LimitingMagnitude = %v, want %v (from Evaluate)", got.LimitingMagnitude, want.Derived.LimitingMagnitude[0])
	}
}

// TestPoint_NoLimitingMagRequested proves HasLimitingMag stays false, not
// a fabricated zero, when PointQuery.LimitingMag was never set.
func TestPoint_NoLimitingMagRequested(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid()
	dir := coord.NewAltAz(angle.Deg(45), angle.Deg(90))
	astro := coord.NewContext(testTime(), testSite(), testAtmosphere())

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("atmosphere.NewBuilder().Build(): %v", err)
	}

	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})

	got, err := skybrightness.Point(context.Background(), eng, skybrightness.PointQuery{
		Astro: astro, Direction: dir, Mode: skybrightness.ModeUserSupplied, Atmosphere: atm, Grid: grid,
	})
	if err != nil {
		t.Fatalf("Point: %v", err)
	}

	if got.HasLimitingMag {
		t.Error("HasLimitingMag = true, want false when PointQuery.LimitingMag was never set")
	}
}

// TestPoint_IntegrationErrorSurfaces proves a failed IntegrateRadiance now
// returns an error from Point instead of silently yielding a zero
// radiance — the fix for the previously-swallowed error.
func TestPoint_IntegrationErrorSurfaces(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid() // 330-1000nm
	dir := coord.NewAltAz(angle.Deg(45), angle.Deg(90))
	astro := coord.NewContext(testTime(), testSite(), testAtmosphere())

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("atmosphere.NewBuilder().Build(): %v", err)
	}

	// A passband entirely outside the grid's wavelength range: zero
	// coverage, guaranteed to fail resampleResponse's coverage check.
	badPassband := &skybrightness.Passband{
		ID:         "bad-passband",
		Wavelength: []skybrightness.WavelengthNM{50, 100},
		Response:   []float64{1, 1},
	}

	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})

	_, err = skybrightness.Point(context.Background(), eng, skybrightness.PointQuery{
		Astro: astro, Direction: dir, Passband: badPassband, Mode: skybrightness.ModeUserSupplied,
		Atmosphere: atm, Grid: grid,
	})
	if err == nil {
		t.Fatal("expected an error for a passband entirely outside the grid's range, got nil")
	}
}
