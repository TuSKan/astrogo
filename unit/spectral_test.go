package unit_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

func mustGrid(t *testing.T, start, step unit.WavelengthNM, n int) unit.SpectralGrid {
	t.Helper()

	g, err := unit.NewSpectralGrid(start, step, n)
	if err != nil {
		t.Fatalf("NewSpectralGrid(%v, %v, %d): %v", start, step, n, err)
	}

	return g
}

func TestSpectralGridValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		start unit.WavelengthNM
		step  unit.WavelengthNM
		n     int
		want  error
	}{
		{"ok", 400, 1, 100, nil},
		{"zero start", 0, 1, 10, unit.ErrGridStart},
		{"negative start", -1, 1, 10, unit.ErrGridStart},
		{"zero step", 400, 0, 10, unit.ErrGridStep},
		{"negative step", 400, -1, 10, unit.ErrGridStep},
		{"single sample", 400, 1, 1, unit.ErrGridLength},
		{"no samples", 400, 1, 0, unit.ErrGridLength},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := unit.NewSpectralGrid(tc.start, tc.step, tc.n)
			if !errors.Is(err, tc.want) {
				t.Errorf("NewSpectralGrid = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSpectralGridAxis(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 2.5, 5)

	if got := g.Len(); got != 5 {
		t.Errorf("Len = %d, want 5", got)
	}

	if got := g.At(0); got != 400 {
		t.Errorf("At(0) = %v, want 400", got)
	}

	if got := g.EndNM(); got != 410 {
		t.Errorf("EndNM = %v, want 410", got)
	}

	if !g.Contains(405) || g.Contains(399) || g.Contains(411) {
		t.Error("Contains disagrees with the grid span")
	}

	if !g.Equal(mustGrid(t, 400, 2.5, 5)) {
		t.Error("Equal should hold for identical axes")
	}

	if g.Equal(mustGrid(t, 400, 2.5, 6)) {
		t.Error("Equal should fail for a different sample count")
	}
}

// Integrating a constant over the grid must return exactly value x span —
// the simplest closed form the trapezoid rule reproduces without error.
func TestSpectralGridIntegrateConstant(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 1, 201) // 400..600 nm

	values := make([]float64, g.Len())
	for i := range values {
		values[i] = 3
	}

	got, err := g.Integrate(values)
	if err != nil {
		t.Fatalf("Integrate: %v", err)
	}

	const want = 3 * 200.0 // 3 per nm over a 200 nm span

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("Integrate(constant) = %v, want %v", got, want)
	}
}

// A linear ramp is also exact under the trapezoid rule, which distinguishes
// a correct implementation from one that mishandles the half-weight
// endpoints.
func TestSpectralGridIntegrateLinear(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 1, 201)

	values := make([]float64, g.Len())
	for i := range values {
		values[i] = float64(g.At(i)) // f(lambda) = lambda
	}

	got, err := g.Integrate(values)
	if err != nil {
		t.Fatalf("Integrate: %v", err)
	}

	// int_400^600 lambda dlambda = (600^2 - 400^2)/2
	const want = (600.0*600.0 - 400.0*400.0) / 2

	if rel := math.Abs(got-want) / want; rel > 1e-12 {
		t.Errorf("Integrate(linear) = %v, want %v (rel %g)", got, want, rel)
	}
}

func TestSpectralGridIntegrateMismatch(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 1, 10)

	if _, err := g.Integrate(make([]float64, 9)); !errors.Is(err, unit.ErrGridMismatch) {
		t.Errorf("Integrate with wrong length = %v, want ErrGridMismatch", err)
	}
}

func TestSpectralGridResample(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 400, 10, 11) // 400..500

	// A ramp from 0 at 400 nm to 1 at 500 nm, tabulated only at its ends,
	// so every interior grid point exercises interpolation.
	src := []unit.WavelengthNM{400, 500}
	vals := []float64{0, 1}

	dst := make([]float64, g.Len())
	if err := g.Resample(dst, src, vals, math.NaN()); err != nil {
		t.Fatalf("Resample: %v", err)
	}

	for i, got := range dst {
		want := float64(i) / 10
		if math.Abs(got-want) > 1e-12 {
			t.Errorf("Resample[%d] = %v, want %v", i, got, want)
		}
	}
}

// Out-of-range samples must take the caller's fill value, so a filter with
// no measured response outside its band reads as zero rather than as an
// extrapolation.
func TestSpectralGridResampleFill(t *testing.T) {
	t.Parallel()

	g := mustGrid(t, 300, 100, 5) // 300..700

	dst := make([]float64, g.Len())
	if err := g.Resample(dst, []unit.WavelengthNM{400, 500}, []float64{1, 1}, 0); err != nil {
		t.Fatalf("Resample: %v", err)
	}

	want := []float64{0, 1, 1, 0, 0} // 300 below, 400/500 inside, 600/700 above
	for i := range want {
		if dst[i] != want[i] {
			t.Errorf("Resample[%d] (%v nm) = %v, want %v", i, g.At(i), dst[i], want[i])
		}
	}
}

// Transmission and optical depth must round-trip, including their two
// documented saturation cases.
func TestTransmissionOpticalDepthRoundTrip(t *testing.T) {
	t.Parallel()

	for _, tr := range []unit.Transmission{0.01, 0.25, 0.5, 0.9, 0.999} {
		back := tr.ToOpticalDepth().ToTransmission()
		if math.Abs(float64(back-tr)) > 1e-12 {
			t.Errorf("round trip of %v = %v", tr, back)
		}
	}

	if got := unit.Transmission(0).ToOpticalDepth(); !math.IsInf(float64(got), 1) {
		t.Errorf("opaque transmission -> %v, want +Inf optical depth", got)
	}

	if got := unit.Transmission(1).ToOpticalDepth(); got != 0 {
		t.Errorf("unit transmission -> %v, want 0 optical depth", got)
	}
}
