package skybrightness_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
)

// TestRequestBuilder_MatchesLiteral proves RequestBuilder produces a
// Request field-for-field identical to an equivalent hand-written literal
// — the acceptance test for "one general construction mechanism", not a
// second, drifting one.
func TestRequestBuilder_MatchesLiteral(t *testing.T) {
	astro := coord.NewContext(testTime(), testSite(), testAtmosphere())
	dirs := []coord.AltAz{coord.NewAltAz(angle.Deg(45), angle.Deg(90))}
	grid := skybrightness.DefaultOpticalGrid()

	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		t.Fatalf("atmosphere.NewBuilder().Build(): %v", err)
	}

	instrument := &skybrightness.Instrument{}
	limMag := skybrightness.NewSchaeferNELM()

	built, err := skybrightness.NewRequestBuilder(astro, dirs, grid).
		Atmosphere(atm).
		Mode(skybrightness.ModeUserSupplied).
		Select(skybrightness.Mask(skybrightness.Airglow), 0).
		Materialize().
		Transmission().
		Derive(skybrightness.DerivePassbands|skybrightness.DeriveAllSkyStats).
		LimitingMag(limMag).
		Instrument(instrument).
		Uncertainty(skybrightness.UncLinearized, 100, 42).
		Fallback(skybrightness.FallbackToFast).
		MaxInputAge(5 * time.Minute).
		Performance(skybrightness.PerformanceOptions{Parallelism: 2, ScatteringOrders: 1}).
		Build()
	if err != nil {
		t.Fatalf("RequestBuilder.Build(): %v", err)
	}

	want := skybrightness.Request{
		Astro: astro, Directions: dirs, Grid: grid,
		Mode:       skybrightness.ModeUserSupplied,
		Atmosphere: atm,
		Selection: skybrightness.ComponentSelection{
			Include: skybrightness.Mask(skybrightness.Airglow), Materialize: true,
		},
		Options: skybrightness.EvaluationOptions{
			ComputeTransmission: true,
			Fallback:            skybrightness.FallbackToFast,
			MaxInputAge:         5 * time.Minute,
			Derived: skybrightness.DerivedOptions{
				Mask:        skybrightness.DerivePassbands | skybrightness.DeriveAllSkyStats | skybrightness.DeriveLimitingMag | skybrightness.DeriveDetectorBackground,
				LimitingMag: limMag,
				Instrument:  instrument,
			},
			Uncertainty: skybrightness.UncertaintyOptions{Mode: skybrightness.UncLinearized, Samples: 100, Seed: 42},
			Performance: skybrightness.PerformanceOptions{Parallelism: 2, ScatteringOrders: 1},
		},
	}

	if built.Mode != want.Mode || built.Options != want.Options || built.Selection != want.Selection {
		t.Fatalf("RequestBuilder output mismatch:\n got  = %+v\n want = %+v", built, want)
	}

	if built.Astro != want.Astro || built.Atmosphere != want.Atmosphere || !reflect.DeepEqual(built.Grid, want.Grid) {
		t.Fatalf("RequestBuilder identity-field mismatch:\n got  = %+v\n want = %+v", built, want)
	}
}

// TestRequestBuilder_BuildValidates proves Build() runs Request.Validate
// internally rather than silently returning an invalid Request.
func TestRequestBuilder_BuildValidates(t *testing.T) {
	_, err := skybrightness.NewRequestBuilder(nil, nil, skybrightness.SpectralGrid{}).Build()
	if err == nil {
		t.Fatal("expected an error for a builder with no Astro/Directions/Grid")
	}
}
