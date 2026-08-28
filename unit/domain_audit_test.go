package unit_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

// Resample must refuse a source curve that is not in increasing wavelength
// order rather than interpolating through it.
//
// The cursor walks forward on that assumption, so unsorted input fails
// quietly: a curve tabulated from the red end resamples to all zeros, and a
// single transposed pair resamples to a curve that is positive, smooth and
// wrong by a factor of five at the peak. Neither announces itself downstream —
// a throughput of zero and a throughput of one fifth both produce a magnitude.
func TestResampleRefusesUnsortedSource(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 50, 5)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	values := []float64{0.0, 0.2, 1.0, 0.2, 0.0}
	dst := make([]float64, grid.Len())

	for _, c := range []struct {
		name string
		src  []unit.WavelengthNM
	}{
		{"descending", []unit.WavelengthNM{600, 550, 500, 450, 400}},
		{"one transposed pair", []unit.WavelengthNM{400, 500, 450, 550, 600}},
		{"duplicated sample", []unit.WavelengthNM{400, 450, 450, 550, 600}},
		{"last two swapped", []unit.WavelengthNM{400, 450, 500, 600, 550}},
	} {
		if err := grid.Resample(dst, c.src, values, 0); !errors.Is(err, unit.ErrSourceNotIncreasing) {
			t.Errorf("%s source: err = %v, want ErrSourceNotIncreasing", c.name, err)
		}
	}

	// The correctly ordered curve still resamples, and onto its own sample
	// points it reproduces itself exactly.
	src := []unit.WavelengthNM{400, 450, 500, 550, 600}
	if err := grid.Resample(dst, src, values, 0); err != nil {
		t.Fatalf("Resample on an increasing source: %v", err)
	}

	for i, want := range values {
		if math.Abs(dst[i]-want) > 1e-12 {
			t.Errorf("resampling onto the source's own points gave dst[%d] = %g, want %g", i, dst[i], want)
		}
	}
}

// Resampling must interpolate between samples and fill outside them, never
// extrapolate a response curve past the data that defines it.
func TestResampleInterpolatesAndFills(t *testing.T) {
	t.Parallel()

	// A grid reaching past both ends of the source.
	grid, err := unit.NewSpectralGrid(300, 100, 6) // 300..800
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	src := []unit.WavelengthNM{400, 600}
	values := []float64{0, 1}
	dst := make([]float64, grid.Len())

	const fill = -7

	if err := grid.Resample(dst, src, values, fill); err != nil {
		t.Fatalf("Resample: %v", err)
	}

	want := []float64{fill, 0, 0.5, 1, fill, fill}
	for i, w := range want {
		if math.Abs(dst[i]-w) > 1e-12 {
			t.Errorf("at %g nm dst[%d] = %g, want %g", float64(grid.At(i)), i, dst[i], w)
		}
	}

	// Interpolation must stay within the bracketing samples: a response curve
	// that overshoots its own data is a filter transmitting more than it was
	// measured to.
	fine, err := unit.NewSpectralGrid(400, 5, 41) // 400..600
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	out := make([]float64, fine.Len())
	if err := fine.Resample(out, src, values, 0); err != nil {
		t.Fatalf("Resample: %v", err)
	}

	for i, v := range out {
		if v < 0 || v > 1 {
			t.Errorf("at %g nm the interpolant is %g, outside the samples [0, 1]", float64(fine.At(i)), v)
		}
	}
}

// Integration is a quadrature, so it has to be exact for the shapes trapezoid
// integration is exact for, and it has to reject a mismatched value count
// rather than integrating whatever it was handed.
func TestIntegrateIsExactWhereItShouldBe(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 1, 201) // 400..600 nm
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	span := float64(grid.EndNM() - grid.StartNM)

	// A constant integrates to value * span.
	constant := make([]float64, grid.Len())
	for i := range constant {
		constant[i] = 3
	}

	got, err := grid.Integrate(constant)
	if err != nil {
		t.Fatalf("Integrate: %v", err)
	}

	if want := 3 * span; math.Abs(got-want) > 1e-9 {
		t.Errorf("a constant 3 over %g nm integrated to %g, want %g", span, got, want)
	}

	// A straight line is exact under the trapezoid rule.
	line := make([]float64, grid.Len())
	for i := range line {
		line[i] = float64(grid.At(i))
	}

	got, err = grid.Integrate(line)
	if err != nil {
		t.Fatalf("Integrate: %v", err)
	}

	lo, hi := float64(grid.StartNM), float64(grid.EndNM())
	if want := (hi*hi - lo*lo) / 2; math.Abs(got-want)/want > 1e-12 {
		t.Errorf("a linear ramp integrated to %.9g, want %.9g", got, want)
	}

	// Zero integrates to zero, not to a residue of the endpoint weighting.
	if got, err := grid.Integrate(make([]float64, grid.Len())); err != nil || got != 0 {
		t.Errorf("integrating zeros gave %v (err %v), want exactly 0", got, err)
	}

	// A mismatched count is refused.
	for _, n := range []int{0, 1, grid.Len() - 1, grid.Len() + 1} {
		if _, err := grid.Integrate(make([]float64, n)); !errors.Is(err, unit.ErrGridMismatch) {
			t.Errorf("integrating %d values against a %d-sample grid: err = %v, want ErrGridMismatch",
				n, grid.Len(), err)
		}
	}
}

// A grid that is not a grid must be refused at construction rather than
// producing a wavelength axis that runs backwards or stands still.
func TestGridConstructionRefusesNonsense(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name        string
		start, step unit.WavelengthNM
		n           int
	}{
		{"zero step", 400, 0, 10},
		{"negative step", 400, -1, 10},
		{"NaN step", 400, unit.WavelengthNM(math.NaN()), 10},
		{"infinite step", 400, unit.WavelengthNM(math.Inf(1)), 10},
		{"zero start", 0, 1, 10},
		{"negative start", -400, 1, 10},
		{"NaN start", unit.WavelengthNM(math.NaN()), 1, 10},
		{"single sample", 400, 1, 1},
		{"no samples", 400, 1, 0},
		{"negative count", 400, 1, -5},
	} {
		if _, err := unit.NewSpectralGrid(c.start, c.step, c.n); err == nil {
			t.Errorf("%s was accepted as a spectral grid", c.name)
		}
	}

	// A valid grid's axis must increase and its endpoints must agree with the
	// samples they name.
	grid, err := unit.NewSpectralGrid(350.5, 2.5, 100)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	if got := float64(grid.At(0)); math.Abs(got-350.5) > 1e-12 {
		t.Errorf("sample 0 is at %g nm, want the start 350.5", got)
	}

	if got, want := float64(grid.EndNM()), 350.5+2.5*99; math.Abs(got-want) > 1e-9 {
		t.Errorf("EndNM = %g, want %g", got, want)
	}

	for i := 1; i < grid.Len(); i++ {
		if grid.At(i) <= grid.At(i-1) {
			t.Fatalf("the axis does not increase at sample %d", i)
		}
	}

	// Contains must agree with the axis it describes, at both closed ends.
	for _, c := range []struct {
		lambda unit.WavelengthNM
		want   bool
	}{
		{grid.StartNM, true},
		{grid.EndNM(), true},
		{grid.StartNM - 0.001, false},
		{grid.EndNM() + 0.001, false},
	} {
		if got := grid.Contains(c.lambda); got != c.want {
			t.Errorf("Contains(%g) = %v, want %v", float64(c.lambda), got, c.want)
		}
	}
}

// Transmission and optical depth are two spellings of one quantity, so
// converting between them must be reversible and must handle the two ends
// where the logarithm does not.
func TestTransmissionOpticalDepthEndpoints(t *testing.T) {
	t.Parallel()

	for _, tr := range []float64{1e-12, 1e-6, 0.01, 0.1, 0.5, 0.9, 0.999999} {
		tau := unit.Transmission(tr).ToOpticalDepth()

		if v := float64(tau); v <= 0 || math.IsInf(v, 0) || math.IsNaN(v) {
			t.Errorf("T = %g gave optical depth %v", tr, v)

			continue
		}

		if back := float64(tau.ToTransmission()); math.Abs(back-tr)/tr > 1e-12 {
			t.Errorf("T = %g -> tau = %g -> T = %g", tr, float64(tau), back)
		}
	}

	// Opaque and clear are the two ends, and both must be exact.
	if got := unit.Transmission(0).ToOpticalDepth(); !math.IsInf(float64(got), 1) {
		t.Errorf("an opaque medium gave optical depth %v, want +Inf", float64(got))
	}

	if got := unit.Transmission(1).ToOpticalDepth(); got != 0 {
		t.Errorf("a clear medium gave optical depth %v, want exactly 0", float64(got))
	}

	if got := unit.OpticalDepth(math.Inf(1)).ToTransmission(); got != 0 {
		t.Errorf("infinite optical depth gave transmission %v, want exactly 0", float64(got))
	}

	if got := unit.OpticalDepth(0).ToTransmission(); got != 1 {
		t.Errorf("zero optical depth gave transmission %v, want exactly 1", float64(got))
	}

	// Neither direction may leave the physical range, whatever it is handed.
	for _, tau := range []float64{-100, -1, 0, 1e-12, 1, 100, 1e6, math.Inf(1)} {
		if got := float64(unit.OpticalDepth(tau).ToTransmission()); got < 0 || got > 1 {
			t.Errorf("optical depth %v gave transmission %v, which is not a fraction", tau, got)
		}
	}

	for _, tr := range []float64{-1, 0, 0.5, 1, 2, 100} {
		if got := float64(unit.Transmission(tr).ToOpticalDepth()); got < 0 {
			t.Errorf("transmission %v gave a negative optical depth %v", tr, got)
		}
	}

	// More optical depth is never more transmission.
	previous := 2.0

	for _, tau := range []float64{0, 0.1, 0.5, 1, 2, 5, 10, 50} {
		got := float64(unit.OpticalDepth(tau).ToTransmission())
		if got > previous {
			t.Errorf("transmission rose from %g to %g as the depth increased to %g", previous, got, tau)
		}

		previous = got
	}
}
