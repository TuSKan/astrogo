package plan

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
)

func skyTestFixture(t *testing.T) (*Site, time.Time, *coord.Context) {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Test", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	tm := time.FromJD(2451545.0, time.UTC) // target RA 18.69h is near zenith at Greenwich
	ctx := coord.NewContext(tm, loc, site.Refraction())

	return site, tm, ctx
}

// The constants/helpers below reproduce, minimally and test-locally, the
// exact Garstang nanolambert<->V-magnitude round-trip
// skybrightness/natural/garstang_units.go uses for ConstantAirglow/
// VBandMoonlight (see that file's doc comment for the
// derivation) — this package cannot import skybrightness/natural (only
// core skybrightness; skybrightness/importgraph_test.go's
// TestPlanImportsOnlyCoreSkybrightness enforces this), so a fixed-SQM
// test double is built directly against the Component/Engine interfaces
// instead.
const (
	testGarstangScale = 34.08
	testGarstangExp   = 20.7233
)

func testVegaZeroPoint() skybrightness.SpectralRadiance {
	return skybrightness.SpectralRadiance(testGarstangScale * math.Exp(testGarstangExp))
}

func testPassband() *skybrightness.Passband {
	return &skybrightness.Passband{
		ID: "test.V", System: skybrightness.SystemVega, Detector: skybrightness.EnergyIntegrating,
		Wavelength: []skybrightness.WavelengthNM{470, 700}, Response: []float64{1, 1},
		VegaZP: &skybrightness.VegaZeroPoint{MeanFlambda: testVegaZeroPoint()},
	}
}

// fixedSQMComponent is a test-only skybrightness.Component reporting a
// flat spectrum whose value is chosen so that, through testPassband(),
// VegaSurfaceBrightness reproduces sqm exactly — the direct successor to
// v1's skybrightness.AsModel(skybrightness.NewFloorSQM(sqm)) test fixture.
type fixedSQMComponent struct{ sqm float64 }

func (fixedSQMComponent) ID() skybrightness.ComponentID { return skybrightness.Artificial }

func (fixedSQMComponent) Algorithm() skybrightness.AlgorithmRef {
	return skybrightness.AlgorithmRef{Name: "fixedSQMComponent", Version: "test"}
}

func (f fixedSQMComponent) Eval(_ context.Context, in skybrightness.EvalInput, out skybrightness.SpectralField) (skybrightness.ComponentReport, error) {
	nl := skybrightness.SpectralRadiance(testGarstangScale * math.Exp(testGarstangExp-0.92104*f.sqm))

	for d := range in.Directions {
		row := out.Row(d)
		for i := range row {
			row[i] = nl
		}
	}

	return skybrightness.ComponentReport{
		Provenance: skybrightness.ComponentProvenance{Component: skybrightness.Artificial, Algorithm: f.Algorithm()},
	}, nil
}

func fixedSQMEngine(t *testing.T, sqm float64) skybrightness.Engine {
	t.Helper()

	eng, err := skybrightness.NewCompositeEngine(skybrightness.CompositeConfig{
		Name:       skybrightness.AlgorithmRef{Name: "fixedSQMEngine", Version: "test"},
		Components: []skybrightness.Component{fixedSQMComponent{sqm: sqm}},
		Mode:       skybrightness.ModeUserSupplied,
	})
	if err != nil {
		t.Fatalf("NewCompositeEngine: %v", err)
	}

	return eng
}

// TestLimitingMagBooleanGate verifies the hard-cutoff mode passes under a
// dark sky and fails under a bright sky for the same faint target.
func TestLimitingMagBooleanGate(t *testing.T) {
	high := NewStar("High", angle.Hour(18.69), angle.Deg(0))
	site, tm, ctx := skyTestFixture(t)

	conv := skybrightness.NewSchaeferNELM()
	need6 := func(Observable) float64 { return 6.0 }

	dark := LimitingMagnitudeConstraint{
		Engine: fixedSQMEngine(t, 22.0), Passband: testPassband(),
		Conversion: conv, Required: need6, Boolean: true,
	}
	bright := LimitingMagnitudeConstraint{
		Engine: fixedSQMEngine(t, 18.0), Passband: testPassband(),
		Conversion: conv, Required: need6, Boolean: true,
	}

	resDark, err := dark.CheckCtx(high, tm, site, ctx)
	if err != nil {
		t.Fatalf("dark CheckCtx: %v", err)
	}

	if !resDark.Pass {
		t.Errorf("expected PASS under dark sky (limMag=%.2f >= 6.0)", resDark.Value)
	}

	resBright, err := bright.CheckCtx(high, tm, site, ctx)
	if err != nil {
		t.Fatalf("bright CheckCtx: %v", err)
	}

	if resBright.Pass {
		t.Errorf("expected FAIL under bright sky (limMag=%.2f < 6.0)", resBright.Value)
	}
}

// TestLimitingMagSoftMonotonic verifies the soft-mode merit increases
// monotonically as the sky darkens.
func TestLimitingMagSoftMonotonic(t *testing.T) {
	high := NewStar("High", angle.Hour(18.69), angle.Deg(0))
	site, tm, ctx := skyTestFixture(t)

	conv := skybrightness.NewSchaeferNELM()
	prev := -1.0

	for _, sqm := range []float64{16, 18, 20, 22} {
		c := LimitingMagnitudeConstraint{
			Engine: fixedSQMEngine(t, sqm), Passband: testPassband(),
			Conversion: conv,
			Required:   func(Observable) float64 { return 5.0 },
		}

		merit, err := c.ScoreMultiplier(high, tm, site, ctx)
		if err != nil {
			t.Fatalf("ScoreMultiplier(SQM=%.0f): %v", sqm, err)
		}

		if merit < prev {
			t.Errorf("merit not monotonic: SQM=%.0f gives %.4f < previous %.4f", sqm, merit, prev)
		}

		if merit < 0 || merit > 1 {
			t.Errorf("merit %.4f out of [0,1] at SQM=%.0f", merit, sqm)
		}

		prev = merit
	}
}

// TestLimitingMagBelowHorizon verifies a below-horizon target fails the
// hard gate and earns zero soft merit.
func TestLimitingMagBelowHorizon(t *testing.T) {
	low := NewStar("Low", angle.Hour(6.69), angle.Deg(0)) // anti-zenith (~nadir) at the fixture epoch
	site, tm, ctx := skyTestFixture(t)

	conv := skybrightness.NewSchaeferNELM()
	engine := fixedSQMEngine(t, 22.0)

	gate := LimitingMagnitudeConstraint{
		Engine: engine, Passband: testPassband(), Conversion: conv,
		Required: func(Observable) float64 { return 6.0 }, Boolean: true,
	}

	res, err := gate.CheckCtx(low, tm, site, ctx)
	if err != nil {
		t.Fatalf("CheckCtx: %v", err)
	}

	if res.Pass {
		t.Error("expected FAIL for below-horizon target")
	}

	soft := gate
	soft.Boolean = false

	merit, err := soft.ScoreMultiplier(low, tm, site, ctx)
	if err != nil {
		t.Fatalf("ScoreMultiplier: %v", err)
	}

	if merit > 1e-6 {
		t.Errorf("expected ~0 merit below horizon, got %.6f", merit)
	}
}

// TestScoreObservableSky verifies the sky merit multiplies the base score
// and demotes a target more under a bright sky than a dark one.
func TestScoreObservableSky(t *testing.T) {
	high := NewStar("High", angle.Hour(18.69), angle.Deg(0))
	site, tm, ctx := skyTestFixture(t)

	conv := skybrightness.NewSchaeferNELM()
	need := func(Observable) float64 { return 5.0 }

	base, err := ScoreObservable(high, tm, site, nil, ctx)
	if err != nil {
		t.Fatalf("ScoreObservable: %v", err)
	}

	if base <= 0 {
		t.Fatalf("expected positive base score, got %.3f", base)
	}

	dark := LimitingMagnitudeConstraint{Engine: fixedSQMEngine(t, 22.0), Passband: testPassband(), Conversion: conv, Required: need}
	bright := LimitingMagnitudeConstraint{Engine: fixedSQMEngine(t, 18.0), Passband: testPassband(), Conversion: conv, Required: need}

	scoreDark, err := ScoreObservableSky(high, tm, site, nil, ctx, dark)
	if err != nil {
		t.Fatalf("ScoreObservableSky dark: %v", err)
	}

	scoreBright, err := ScoreObservableSky(high, tm, site, nil, ctx, bright)
	if err != nil {
		t.Fatalf("ScoreObservableSky bright: %v", err)
	}

	if scoreDark > base+1e-9 {
		t.Errorf("dark sky score %.3f should not exceed base %.3f", scoreDark, base)
	}

	if !(scoreBright < scoreDark) {
		t.Errorf("bright-sky score %.3f should be below dark-sky score %.3f", scoreBright, scoreDark)
	}
}
