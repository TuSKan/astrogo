package skybrightness

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// ErrInvalidSpectralGrid is returned by NewSpectralGrid/UniformSpectralGrid
// for fewer than two points or a non-strictly-increasing wavelength array.
var ErrInvalidSpectralGrid = errors.New("skybrightness: invalid spectral grid")

// GridID is a stable content hash of a SpectralGrid's wavelengths — two
// grids built from the same wavelength array (regardless of how they were
// constructed) have the same GridID. Used to key caches and to appear in
// Provenance.
type GridID [16]byte

// SpectralGrid is the ordered set of wavelengths a spectral evaluation runs
// on, plus the trapezoid quadrature weights precomputed for it. Immutable
// after construction.
type SpectralGrid struct {
	lambda  []unit.WavelengthNM
	weights []float64
	id      GridID
}

// NewSpectralGrid builds a SpectralGrid from a strictly increasing
// wavelength array, copying lambda so the caller's slice can be reused or
// mutated afterward with no effect on the grid.
func NewSpectralGrid(lambda []unit.WavelengthNM) (SpectralGrid, error) {
	if len(lambda) < 2 {
		return SpectralGrid{}, fmt.Errorf("%w: need at least 2 points, got %d", ErrInvalidSpectralGrid, len(lambda))
	}

	cp := make([]unit.WavelengthNM, len(lambda))
	copy(cp, lambda)

	for i := 1; i < len(cp); i++ {
		if cp[i] <= cp[i-1] {
			return SpectralGrid{}, fmt.Errorf("%w: wavelengths must be strictly increasing (index %d: %g <= %g)",
				ErrInvalidSpectralGrid, i, float64(cp[i]), float64(cp[i-1]))
		}
	}

	return SpectralGrid{lambda: cp, weights: trapezoidWeights(cp), id: gridID(cp)}, nil
}

// UniformSpectralGrid builds a SpectralGrid of n evenly spaced points from
// lo to hi inclusive.
func UniformSpectralGrid(lo, hi unit.WavelengthNM, n int) (SpectralGrid, error) {
	if n < 2 {
		return SpectralGrid{}, fmt.Errorf("%w: n must be >= 2, got %d", ErrInvalidSpectralGrid, n)
	}

	if hi <= lo {
		return SpectralGrid{}, fmt.Errorf("%w: hi (%g) must be > lo (%g)", ErrInvalidSpectralGrid, float64(hi), float64(lo))
	}

	lambda := make([]unit.WavelengthNM, n)
	step := (hi - lo) / unit.WavelengthNM(n-1)

	for i := range lambda {
		lambda[i] = lo + unit.WavelengthNM(i)*step
	}

	lambda[n-1] = hi // avoid float accumulation drift on the last point

	return NewSpectralGrid(lambda)
}

// DefaultOpticalGrid is the standard 330-1000 nm, 5 nm grid used by the
// Point convenience API when a caller supplies no grid of its own.
func DefaultOpticalGrid() SpectralGrid {
	g, err := UniformSpectralGrid(330, 1000, 135)
	if err != nil {
		panic("skybrightness: DefaultOpticalGrid: " + err.Error()) // unreachable: fixed, valid inputs
	}

	return g
}

// Len returns the number of wavelength points.
func (g SpectralGrid) Len() int { return len(g.lambda) }

// At returns the i'th wavelength.
func (g SpectralGrid) At(i int) unit.WavelengthNM { return g.lambda[i] }

// Lambda returns a read-only view of the grid's wavelengths. Callers must
// not mutate the returned slice.
func (g SpectralGrid) Lambda() []unit.WavelengthNM { return g.lambda }

// Weights returns the precomputed trapezoid quadrature weights, in
// nanometres, one per wavelength point. Callers must not mutate the
// returned slice.
func (g SpectralGrid) Weights() []float64 { return g.weights }

// ID returns the grid's content-hash identity.
func (g SpectralGrid) ID() GridID { return g.id }

// Covers reports whether the grid's range fully spans [lo, hi].
func (g SpectralGrid) Covers(lo, hi unit.WavelengthNM) bool {
	if len(g.lambda) == 0 {
		return false
	}

	return g.lambda[0] <= lo && g.lambda[len(g.lambda)-1] >= hi
}

func trapezoidWeights(lambda []unit.WavelengthNM) []float64 {
	n := len(lambda)
	w := make([]float64, n)

	if n == 1 {
		return w
	}

	w[0] = float64(lambda[1]-lambda[0]) / 2
	w[n-1] = float64(lambda[n-1]-lambda[n-2]) / 2

	for i := 1; i < n-1; i++ {
		w[i] = float64(lambda[i+1]-lambda[i-1]) / 2
	}

	return w
}

func gridID(lambda []unit.WavelengthNM) GridID {
	h := sha256.New()
	buf := make([]byte, 8)

	for _, l := range lambda {
		// l is a wavelength in nm, always positive and well within int64
		// range at 1e-6 nm resolution — the int64->uint64 conversion never
		// wraps for any physically meaningful grid.
		binary.LittleEndian.PutUint64(buf, uint64(int64(l*1e6))) //nolint:gosec // wavelengths are always positive, no overflow
		h.Write(buf)
	}

	sum := h.Sum(nil)

	var id GridID

	copy(id[:], sum[:16])

	return id
}

// SpectralField is a flat, direction-major [nDir x nLambda] buffer — the
// ONLY container this package uses for spectral output. It is never
// [][]float64: a single backing array keeps an all-sky evaluation
// allocation-flat regardless of how many directions are requested.
type SpectralField struct {
	nDir, nLambda int
	data          []unit.SpectralRadiance
}

// NewSpectralField allocates a zeroed field of the given dimensions.
func NewSpectralField(nDir, nLambda int) SpectralField {
	return SpectralField{nDir: nDir, nLambda: nLambda, data: make([]unit.SpectralRadiance, nDir*nLambda)}
}

// Dims returns the field's dimensions.
func (f SpectralField) Dims() (nDir, nLambda int) { return f.nDir, f.nLambda }

// Empty reports whether the field has zero elements (e.g. a component that
// was never materialized under ComponentSelection.Materialize == false).
func (f SpectralField) Empty() bool { return len(f.data) == 0 }

// Row returns direction i's spectrum as a mutable, no-copy view into the
// field's backing array.
func (f SpectralField) Row(i int) []unit.SpectralRadiance {
	return f.data[i*f.nLambda : (i+1)*f.nLambda]
}

// At returns the value at direction dir, wavelength index k.
func (f SpectralField) At(dir, k int) unit.SpectralRadiance { return f.data[dir*f.nLambda+k] }

// Clone returns a deep copy.
func (f SpectralField) Clone() SpectralField {
	cp := make([]unit.SpectralRadiance, len(f.data))
	copy(cp, f.data)

	return SpectralField{nDir: f.nDir, nLambda: f.nLambda, data: cp}
}

// Zero resets every element to 0.
func (f *SpectralField) Zero() {
	for i := range f.data {
		f.data[i] = 0
	}
}

// Add adds o into f element-wise. Panics if the dimensions differ.
func (f *SpectralField) Add(o SpectralField) {
	f.AddScaled(o, 1)
}

// AddScaled adds s*o into f element-wise. Panics if the dimensions differ.
func (f *SpectralField) AddScaled(o SpectralField, s float64) {
	if o.nDir != f.nDir || o.nLambda != f.nLambda {
		panic(fmt.Sprintf("skybrightness: SpectralField.AddScaled: dimension mismatch %dx%d vs %dx%d",
			f.nDir, f.nLambda, o.nDir, o.nLambda))
	}

	for i := range f.data {
		f.data[i] += unit.SpectralRadiance(s) * o.data[i]
	}
}

// MinNonNegative reports whether every element is finite and >= 0 — the
// scientific invariant every Result.Total/Components field must satisfy.
func (f SpectralField) MinNonNegative() bool {
	for _, v := range f.data {
		fv := float64(v)
		if math.IsNaN(fv) || math.IsInf(fv, 0) || fv < 0 {
			return false
		}
	}

	return true
}
