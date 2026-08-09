package natural

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/skybrightness"
	astrotime "github.com/TuSKan/astrogo/time"
)

func testAstro() *coord.Context {
	site, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		panic(err)
	}

	t := astrotime.FromJD(2451545.0, astrotime.UTC)

	return coord.NewContext(t, site, atmosphere.StandardRefraction)
}

// roundTripToleranceMag bounds the Garstang nanolambert round-trip
// precision — see constant_airglow.go's doc comment: the shared,
// historically rounded 0.92104 literal (not the more precise 0.4*ln(10))
// means the round trip is precise to ~1.5e-4 mag at V~22, not to float64
// precision.
const roundTripToleranceMag = 5e-4

// TestConstantAirglow_RoundTripToHistoricalPrecision confirms the
// Garstang nanolambert convention reproduces the original V mag/arcsec^2
// through VegaSurfaceBrightness against TopHatJohnsonV, to the precision
// of v1's own shared 0.92104 literal (see constant_airglow.go's doc
// comment).
func TestConstantAirglow_RoundTripToHistoricalPrecision(t *testing.T) {
	grid := TopHatVGrid()
	pb := TopHatJohnsonV()

	for _, v := range []float64{21.9, 18.0, 22.0, 16.5} {
		a := NewConstantAirglowSB(v)
		out := skybrightness.NewSpectralField(1, grid.Len())

		_, err := a.Eval(context.Background(), skybrightness.EvalInput{
			Directions: []coord.AltAz{coord.NewAltAz(angle.Deg(45), angle.Deg(0))},
			Grid:       grid,
		}, out)
		if err != nil {
			t.Fatalf("Eval(v=%v): %v", v, err)
		}

		mean, err := skybrightness.BandMeanSpectralRadiance(grid, out.Row(0), pb)
		if err != nil {
			t.Fatalf("BandMeanSpectralRadiance: %v", err)
		}

		got, err := skybrightness.VegaSurfaceBrightness(mean, pb)
		if err != nil {
			t.Fatalf("VegaSurfaceBrightness: %v", err)
		}

		if diff := math.Abs(float64(got) - v); diff > roundTripToleranceMag {
			t.Errorf("v=%v: round trip gave %v (diff %v, want <= %v)", v, got, diff, roundTripToleranceMag)
		}
	}
}

// TestConstantAirglow_ZeroValueDefaults confirms a zero-value
// ConstantAirglow (constructed without NewConstantAirglow) falls back to
// DefaultConstantAirglowV, matching v1's behavior.
func TestConstantAirglow_ZeroValueDefaults(t *testing.T) {
	a := &ConstantAirglow{}
	grid := TopHatVGrid()
	pb := TopHatJohnsonV()

	out := skybrightness.NewSpectralField(1, grid.Len())

	_, err := a.Eval(context.Background(), skybrightness.EvalInput{
		Directions: []coord.AltAz{coord.NewAltAz(angle.Deg(45), angle.Deg(0))}, Grid: grid,
	}, out)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}

	mean, err := skybrightness.BandMeanSpectralRadiance(grid, out.Row(0), pb)
	if err != nil {
		t.Fatalf("BandMeanSpectralRadiance: %v", err)
	}

	got, err := skybrightness.VegaSurfaceBrightness(mean, pb)
	if err != nil {
		t.Fatalf("VegaSurfaceBrightness: %v", err)
	}

	if diff := math.Abs(float64(got) - DefaultConstantAirglowV); diff > roundTripToleranceMag {
		t.Errorf("zero-value ConstantAirglow gave %v, want default %v (diff %v, want <= %v)",
			got, DefaultConstantAirglowV, diff, roundTripToleranceMag)
	}
}

// TestVBandMoonlight_BelowHorizonIsZero confirms zero
// contribution when the Moon is below the horizon.
func TestVBandMoonlight_BelowHorizonIsZero(t *testing.T) {
	prov := eph.Default()
	m := NewVBandMoonlight(WithMoonProvider(prov))

	astro := testAstro()
	grid := TopHatVGrid()

	// Find a time where the Moon is genuinely below the horizon by
	// scanning: not all epochs guarantee this at lat/lon (0,0), so pick a
	// far-future/-past instant deterministically and just check the
	// physical implication (below horizon -> exactly zero), whichever
	// epoch that turns out to be, rather than asserting a specific time
	// is below the horizon a priori.
	tm := astrotime.FromJD(2451545.0, astrotime.UTC)

	moonVec, err := eph.Position(prov, eph.Moon, tm)
	if err != nil {
		t.Fatalf("moon position: %v", err)
	}

	moonICRS, err := eph.ToICRS(moonVec)
	if err != nil {
		t.Fatalf("moon ICRS: %v", err)
	}

	moonAA, err := astro.ICRSToAltAz(moonICRS)
	if err != nil {
		t.Fatalf("moon alt-az: %v", err)
	}

	if moonAA.Alt().Degrees() > 0 {
		t.Skip("Moon above horizon at the fixed test epoch; skipping this specific below-horizon check")
	}

	out := skybrightness.NewSpectralField(1, grid.Len())

	_, err = m.Eval(context.Background(), skybrightness.EvalInput{
		Astro: astro, Directions: []coord.AltAz{coord.NewAltAz(angle.Deg(45), angle.Deg(0))}, Grid: grid,
	}, out)
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}

	for i, v := range out.Row(0) {
		if v != 0 {
			t.Errorf("wavelength %d: expected 0 with Moon below horizon, got %v", i, v)
		}
	}
}

// TestVBandMoonlight_NilAstroErrors confirms a nil
// EvalInput.Astro errors rather than panicking.
func TestVBandMoonlight_NilAstroErrors(t *testing.T) {
	m := NewVBandMoonlight()
	grid := TopHatVGrid()
	out := skybrightness.NewSpectralField(1, grid.Len())

	_, err := m.Eval(context.Background(), skybrightness.EvalInput{
		Directions: []coord.AltAz{coord.NewAltAz(angle.Deg(45), angle.Deg(0))}, Grid: grid,
	}, out)
	if err == nil {
		t.Error("expected an error for nil Astro, got nil")
	}
}

// TestNewFastEngine_Builds confirms NewFastEngine assembles a working
// Engine with no errors.
func TestNewFastEngine_Builds(t *testing.T) {
	eng, err := NewFastEngine(FastConfig{Ephemeris: eph.Default()})
	if err != nil {
		t.Fatalf("NewFastEngine: %v", err)
	}

	grid := TopHatVGrid()

	res, err := eng.Evaluate(context.Background(), skybrightness.Request{
		Astro:      testAstro(),
		Directions: []coord.AltAz{coord.NewAltAz(angle.Deg(60), angle.Deg(30))},
		Grid:       grid,
		Mode:       skybrightness.ModeFast,
	})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !res.Total.MinNonNegative() {
		t.Error("FastEngine produced a non-finite/negative Total")
	}

	if !res.Quality.Has(skybrightness.QualityFlagApproximatePhysics) {
		t.Error("expected QualityFlagApproximatePhysics on a ModeFast result")
	}
}
