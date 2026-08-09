package skybrightness_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	astrotime "github.com/TuSKan/astrogo/time"
)

// constComponent is a test-only Component reporting a fixed spectral
// radiance everywhere, tagged with a caller-chosen ComponentID.
type constComponent struct {
	id    skybrightness.ComponentID
	value skybrightness.SpectralRadiance
}

func (c constComponent) ID() skybrightness.ComponentID { return c.id }

func (c constComponent) Algorithm() skybrightness.AlgorithmRef {
	return skybrightness.AlgorithmRef{Name: "constComponent", Version: "test"}
}

func (c constComponent) Eval(_ context.Context, _ skybrightness.EvalInput, out skybrightness.SpectralField) (skybrightness.ComponentReport, error) {
	nDir, nLambda := out.Dims()

	for d := range nDir {
		row := out.Row(d)
		for i := range nLambda {
			row[i] = c.value
		}
	}

	return skybrightness.ComponentReport{
		Uncertainty: skybrightness.ComponentUncertainty{RelSigma: 0.1, Group: skybrightness.GroupNatural},
		Provenance:  skybrightness.ComponentProvenance{Component: c.id},
	}, nil
}

func testEngine(t *testing.T, components ...skybrightness.Component) *skybrightness.CompositeEngine {
	t.Helper()

	eng, err := skybrightness.NewCompositeEngine(skybrightness.CompositeConfig{
		Name:       skybrightness.AlgorithmRef{Name: "test-engine", Version: "1"},
		Components: components,
		Mode:       skybrightness.ModeUserSupplied,
	})
	if err != nil {
		t.Fatalf("NewCompositeEngine: %v", err)
	}

	return eng
}

func testRequest(grid skybrightness.SpectralGrid, dirs []coord.AltAz, materialize bool) skybrightness.Request {
	atm, err := atmosphere.NewBuilder().Build()
	if err != nil {
		panic(err) // fixed, valid inputs
	}

	return skybrightness.Request{
		Astro: coord.NewContext(testTime(), testSite(), testAtmosphere()), Directions: dirs, Grid: grid,
		Mode:       skybrightness.ModeUserSupplied,
		Atmosphere: atm,
		Selection:  skybrightness.ComponentSelection{Materialize: materialize},
	}
}

// TestInvariant_TotalFiniteNonNegative asserts Result.Total is finite and
// >= 0 for every direction/wavelength, including poles and az wrap.
func TestInvariant_TotalFiniteNonNegative(t *testing.T) {
	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.5})
	grid := skybrightness.DefaultOpticalGrid()

	dirs := []coord.AltAz{
		coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
		coord.NewAltAz(angle.Deg(-90), angle.Deg(0)),
		coord.NewAltAz(angle.Deg(10), angle.Deg(0)),
		coord.NewAltAz(angle.Deg(10), angle.Deg(359.999)),
	}

	res, err := eng.Evaluate(context.Background(), testRequest(grid, dirs, true))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !res.Total.MinNonNegative() {
		t.Error("Total is not finite/non-negative everywhere")
	}
}

// TestInvariant_ComponentSumEqualsTotal asserts sum(Components) == Total,
// in linear space, exactly (both computed the same way internally).
func TestInvariant_ComponentSumEqualsTotal(t *testing.T) {
	eng := testEngine(t,
		constComponent{id: skybrightness.Airglow, value: 1.0},
		constComponent{id: skybrightness.MoonScattered, value: 2.0},
		constComponent{id: skybrightness.Artificial, value: 0.5},
	)
	grid := skybrightness.DefaultOpticalGrid()
	dirs := []coord.AltAz{coord.NewAltAz(angle.Deg(45), angle.Deg(90))}

	res, err := eng.Evaluate(context.Background(), testRequest(grid, dirs, true))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	sum := skybrightness.NewSpectralField(1, grid.Len())

	res.Components.Each(func(_ skybrightness.ComponentID, f skybrightness.SpectralField, _ skybrightness.ComponentReport) bool {
		sum.Add(f)
		return true
	})

	for i := range grid.Len() {
		if math.Abs(float64(sum.At(0, i)-res.Total.At(0, i))) > 1e-12 {
			t.Fatalf("sum(Components)[%d] = %v, Total = %v", i, sum.At(0, i), res.Total.At(0, i))
		}
	}
}

// TestInvariant_PassbandIntegrationIsLinear asserts
// I(a*s1 + b*s2) == a*I(s1) + b*I(s2).
func TestInvariant_PassbandIntegrationIsLinear(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid()
	pb := skybrightness.Gaussian("test.g", 550, 100)

	s1 := make([]skybrightness.SpectralRadiance, grid.Len())
	s2 := make([]skybrightness.SpectralRadiance, grid.Len())

	for i := range s1 {
		lam := float64(grid.At(i))
		s1[i] = skybrightness.SpectralRadiance(1e-9 * math.Exp(-((lam-500)*(lam-500))/5000))
		s2[i] = skybrightness.SpectralRadiance(2e-9 * math.Exp(-((lam-600)*(lam-600))/8000))
	}

	const a, b = 2.0, 3.0

	combo := make([]skybrightness.SpectralRadiance, grid.Len())
	for i := range combo {
		combo[i] = skybrightness.SpectralRadiance(a*float64(s1[i]) + b*float64(s2[i]))
	}

	i1, err := skybrightness.IntegrateRadiance(grid, s1, pb)
	if err != nil {
		t.Fatalf("IntegrateRadiance(s1): %v", err)
	}

	i2, err := skybrightness.IntegrateRadiance(grid, s2, pb)
	if err != nil {
		t.Fatalf("IntegrateRadiance(s2): %v", err)
	}

	iCombo, err := skybrightness.IntegrateRadiance(grid, combo, pb)
	if err != nil {
		t.Fatalf("IntegrateRadiance(combo): %v", err)
	}

	want := a*float64(i1) + b*float64(i2)
	if diff := math.Abs((float64(iCombo) - want) / want); diff > 1e-10 {
		t.Errorf("I(a*s1+b*s2) = %v, want %v (rel diff %v)", iCombo, want, diff)
	}
}

// TestInvariant_ABRoundTrip asserts ABSurfaceBrightness/ABToBandMean
// round-trip to << 0.01 mag (the mandate's numerical target).
func TestInvariant_ABRoundTrip(t *testing.T) {
	pivot := skybrightness.WavelengthNM(551)

	for _, mean := range []skybrightness.SpectralRadiance{1e-9, 1e-12, 1e-15, 5e-10} {
		ab := skybrightness.ABSurfaceBrightness(mean, pivot)
		back := skybrightness.ABToBandMean(ab, pivot)

		if diff := math.Abs(float64(back-mean) / float64(mean)); diff > 1e-10 {
			t.Errorf("round-trip mean=%v: got back %v (rel diff %v)", mean, back, diff)
		}

		abBack := skybrightness.ABSurfaceBrightness(back, pivot)
		if diff := math.Abs(float64(abBack - ab)); diff > 1e-9 {
			t.Errorf("round-trip AB mag: %v vs %v (diff %v mag)", ab, abBack, diff)
		}
	}
}

// TestInvariant_NoAzimuthWrapDiscontinuity asserts L(az=359.99) and
// L(az=0.01) are close for a component with no inherent azimuthal
// structure.
func TestInvariant_NoAzimuthWrapDiscontinuity(t *testing.T) {
	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})
	grid := skybrightness.DefaultOpticalGrid()

	dirs := []coord.AltAz{
		coord.NewAltAz(angle.Deg(45), angle.Deg(359.99)),
		coord.NewAltAz(angle.Deg(45), angle.Deg(0.01)),
	}

	res, err := eng.Evaluate(context.Background(), testRequest(grid, dirs, true))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	for i := range grid.Len() {
		if diff := math.Abs(float64(res.Total.At(0, i) - res.Total.At(1, i))); diff > 1e-12 {
			t.Errorf("azimuth-wrap discontinuity at wavelength %d: %v vs %v", i, res.Total.At(0, i), res.Total.At(1, i))
		}
	}
}

// TestInvariant_MaterializeMatchesTotal asserts Materialize:false produces
// a bit-identical Total to Materialize:true.
func TestInvariant_MaterializeMatchesTotal(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid()
	dirs := []coord.AltAz{coord.NewAltAz(angle.Deg(60), angle.Deg(30))}

	newComponents := func() []skybrightness.Component {
		return []skybrightness.Component{
			constComponent{id: skybrightness.Airglow, value: 1.0},
			constComponent{id: skybrightness.MoonScattered, value: 3.0},
		}
	}

	resTrue, err := testEngine(t, newComponents()...).Evaluate(context.Background(), testRequest(grid, dirs, true))
	if err != nil {
		t.Fatalf("Evaluate (materialize=true): %v", err)
	}

	resFalse, err := testEngine(t, newComponents()...).Evaluate(context.Background(), testRequest(grid, dirs, false))
	if err != nil {
		t.Fatalf("Evaluate (materialize=false): %v", err)
	}

	for i := range grid.Len() {
		if resTrue.Total.At(0, i) != resFalse.Total.At(0, i) {
			t.Fatalf("Total differs at wavelength %d: materialize=true %v, materialize=false %v",
				i, resTrue.Total.At(0, i), resFalse.Total.At(0, i))
		}
	}

	if f, ok := resFalse.Components.Field(skybrightness.Airglow); ok && !f.Empty() {
		t.Error("Materialize:false should not populate a per-component SpectralField")
	}
}

// TestInvariant_ProvenanceDigestStable asserts identical inputs produce an
// identical Provenance digest.
func TestInvariant_ProvenanceDigestStable(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid()
	dirs := []coord.AltAz{coord.NewAltAz(angle.Deg(60), angle.Deg(30))}

	digests := make([][32]byte, 0, 3)

	for range 3 {
		eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})

		res, err := eng.Evaluate(context.Background(), testRequest(grid, dirs, true))
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}

		// EvaluatedAt varies run to run; zero it (not just Truncate(0),
		// which only strips the monotonic reading and leaves the wall
		// clock value — a real, different timestamp each call) before
		// hashing, so this test isolates determinism of everything else
		// in Provenance.
		res.Provenance.EvaluatedAt = time.Time{}
		digests = append(digests, res.Provenance.Digest())
	}

	for i, d := range digests {
		if d != digests[0] {
			t.Errorf("digest %d = %x, want %x (identical inputs must produce an identical digest)", i, d, digests[0])
		}
	}

	// Confirm Digest() is at least deterministic for a fixed Provenance
	// value (marshal twice, compare) — the EvaluatedAt timestamp varying
	// across calls is expected and is Provenance's own honest behavior,
	// not a bug; a caller wanting reproducible digests holds the
	// Provenance value fixed, which this loop's per-call re-marshal proves
	// is stable.
	grid2 := skybrightness.DefaultOpticalGrid()
	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})

	res, err := eng.Evaluate(context.Background(), testRequest(grid2, dirs, true))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	d1 := res.Provenance.Digest()
	d2 := res.Provenance.Digest()

	if d1 != d2 {
		t.Error("Digest() is not stable across repeated calls on the same Provenance value")
	}
}

// TestInvariant_ComponentSelectionExcludeIsExact asserts excluding a
// component removes exactly its contribution from Total.
func TestInvariant_ComponentSelectionExcludeIsExact(t *testing.T) {
	grid := skybrightness.DefaultOpticalGrid()
	dirs := []coord.AltAz{coord.NewAltAz(angle.Deg(60), angle.Deg(30))}

	full, err := testEngine(t,
		constComponent{id: skybrightness.Airglow, value: 1.0},
		constComponent{id: skybrightness.MoonScattered, value: 2.0},
	).Evaluate(context.Background(), testRequest(grid, dirs, true))
	if err != nil {
		t.Fatalf("Evaluate (full): %v", err)
	}

	req := testRequest(grid, dirs, true)
	req.Selection.Exclude = skybrightness.Mask(skybrightness.MoonScattered)

	partial, err := testEngine(t,
		constComponent{id: skybrightness.Airglow, value: 1.0},
		constComponent{id: skybrightness.MoonScattered, value: 2.0},
	).Evaluate(context.Background(), req)
	if err != nil {
		t.Fatalf("Evaluate (partial): %v", err)
	}

	if partial.Components.Has(skybrightness.MoonScattered) {
		t.Error("excluded component still present in Result.Components")
	}

	for i := range grid.Len() {
		want := full.Total.At(0, i) - 2.0
		if diff := math.Abs(float64(partial.Total.At(0, i) - want)); diff > 1e-12 {
			t.Errorf("wavelength %d: Total after exclude = %v, want %v", i, partial.Total.At(0, i), want)
		}
	}
}

// TestInvariant_ConcurrentEvaluateIsRaceFree confirms many goroutines
// calling Evaluate on one shared Engine concurrently produce identical
// results (run this test file with -race).
func TestInvariant_ConcurrentEvaluateIsRaceFree(t *testing.T) {
	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})
	grid := skybrightness.DefaultOpticalGrid()
	dirs := []coord.AltAz{coord.NewAltAz(angle.Deg(60), angle.Deg(30))}

	const n = 64

	results := make([]skybrightness.Result, n)
	errs := make([]error, n)

	done := make(chan int, n)
	for i := range n {
		go func(i int) {
			results[i], errs[i] = eng.Evaluate(context.Background(), testRequest(grid, dirs, true))
			done <- i
		}(i)
	}

	for range n {
		<-done
	}

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: Evaluate: %v", i, err)
		}
	}

	for i := 1; i < n; i++ {
		if results[i].Total.At(0, 0) != results[0].Total.At(0, 0) {
			t.Errorf("goroutine %d produced a different Total than goroutine 0", i)
		}
	}
}

// TestInvariant_HighLatitudePolarDirectionsFinite exercises directions at
// and near the poles (alt=+/-90) to confirm no NaN/Inf/panic arises.
func TestInvariant_HighLatitudePolarDirectionsFinite(t *testing.T) {
	eng := testEngine(t, constComponent{id: skybrightness.Airglow, value: 1.0})
	grid := skybrightness.DefaultOpticalGrid()

	dirs := []coord.AltAz{
		coord.NewAltAz(angle.Deg(90), angle.Deg(0)),
		coord.NewAltAz(angle.Deg(89.9999), angle.Deg(180)),
		coord.NewAltAz(angle.Deg(-89.9999), angle.Deg(270)),
	}

	res, err := eng.Evaluate(context.Background(), testRequest(grid, dirs, true))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if !res.Total.MinNonNegative() {
		t.Error("polar directions produced non-finite/negative Total")
	}
}

func testTime() astrotime.Time { return astrotime.FromJD(2451545.0, astrotime.UTC) }

func testSite() *coord.Geodetic {
	site, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		panic(err) // fixed, valid inputs
	}

	return site
}

func testAtmosphere() atmosphere.Refraction { return atmosphere.StandardRefraction }
