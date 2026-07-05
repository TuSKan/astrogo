package skybrightness_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/time"
)

// errComponent is a Component stub that always fails, for error-propagation tests.
type errComponent struct{}

func (errComponent) Radiance(coord.AltAz, *coord.Context) (skybrightness.Nanolambert, error) {
	return 0, errStub
}

var errStub = errors.New("stub failure")

// TestCompositeLinearSum verifies the model sums components in LINEAR flux
// space: two equal floors are 2.5·log₁₀(2) brighter than one, not equal to it.
func TestCompositeLinearSum(t *testing.T) {
	t.Parallel()

	const v = 21.0

	one := skybrightness.NewCompositeModel(skybrightness.NewFloorSQM(v))
	two := skybrightness.NewCompositeModel(
		skybrightness.NewFloorSQM(v),
		skybrightness.NewFloorSQM(v),
	)

	aa := coord.NewAltAz(angle.Deg(45), angle.Deg(0))

	sb1, _ := one.SurfaceBrightness(aa, nil)
	sb2, _ := two.SurfaceBrightness(aa, nil)

	testutil.AssertNear(t, "single floor", float64(sb1), v, 1e-12)
	testutil.AssertNear(t, "doubled floor", float64(sb2), v-2.5*math.Log10(2), 1e-12)
}

// TestCompositeEmpty verifies an empty model is infinitely faint.
func TestCompositeEmpty(t *testing.T) {
	t.Parallel()

	sb, err := skybrightness.NewCompositeModel().SurfaceBrightness(coord.NewAltAz(angle.Deg(45), angle.Zero()), nil)
	if err != nil {
		t.Fatalf("SurfaceBrightness: %v", err)
	}

	if !math.IsInf(float64(sb), 1) {
		t.Errorf("empty model: got %v, want +Inf", sb)
	}
}

// TestCompositeErrorPropagates verifies a component error aborts the sum.
func TestCompositeErrorPropagates(t *testing.T) {
	t.Parallel()

	m := skybrightness.NewCompositeModel(skybrightness.NewFloorSQM(21.0), errComponent{})
	if _, err := m.SurfaceBrightness(coord.NewAltAz(angle.Deg(45), angle.Zero()), nil); !errors.Is(err, errStub) {
		t.Errorf("expected stub error, got %v", err)
	}
}

// TestCompositeBrighterThanFloor verifies adding components only brightens the
// sky (lower magnitude) relative to the floor alone.
func TestCompositeBrighterThanFloor(t *testing.T) {
	t.Parallel()

	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	tm := time.FromJD(2451545.5, time.UTC)
	ctx := coord.NewContext(tm, loc, atmosphere.AtAltitude(0))
	aa := coord.NewAltAz(angle.Deg(60), angle.Deg(120))

	floor := skybrightness.NewFloorSQM(21.9)
	floorOnly := skybrightness.NewCompositeModel(floor)
	full := skybrightness.NewCompositeModel(
		floor,
		skybrightness.NewMoonlight(),
		skybrightness.NewZodiacalLight(nil),
		skybrightness.NewAirglow(),
	)

	sbFloor, _ := floorOnly.SurfaceBrightness(aa, ctx)

	sbFull, err := full.SurfaceBrightness(aa, ctx)
	if err != nil {
		t.Fatalf("full model: %v", err)
	}

	if !(float64(sbFull) < float64(sbFloor)) {
		t.Errorf("full model %.3f should be brighter (smaller) than floor-only %.3f", sbFull, sbFloor)
	}
}

// BenchmarkCompositeSurfaceBrightness benchmarks the allocation-free linear-sum
// hot path using direction/time-independent components (no ephemeris).
func BenchmarkCompositeSurfaceBrightness(b *testing.B) {
	b.ReportAllocs()

	m := skybrightness.NewCompositeModel(
		skybrightness.NewFloorSQM(21.0),
		skybrightness.NewAirglow(),
	)
	aa := coord.NewAltAz(angle.Deg(45), angle.Deg(0))

	var sink skybrightness.SurfaceBrightnessV
	for range b.N {
		sink, _ = m.SurfaceBrightness(aa, nil)
	}

	_ = sink
}
