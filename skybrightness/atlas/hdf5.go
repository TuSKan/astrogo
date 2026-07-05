package atlas

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/scigolib/hdf5"

	"github.com/TuSKan/astrogo/skybrightness"
)

// ErrHDF5 is returned for HDF5 read problems (dataset not found, unparseable
// shape, dimension/length mismatch).
var ErrHDF5 = errors.New("atlas: HDF5")

// hdf5DimsRe extracts the row×col shape from the scigolib/hdf5 Dataset.Info()
// string, e.g. "Dataset: float (size=4 bytes), 2D array [3 x 4], contiguous...".
var hdf5DimsRe = regexp.MustCompile(`2D array \[(\d+) x (\d+)\]`)

// hdf5Config holds optional HDF5-loader settings.
type hdf5Config struct {
	fill   *float64
	scale  float64
	offset float64
}

// HDF5Option configures [LoadHDF5Grid].
type HDF5Option func(*hdf5Config)

// WithHDF5Fill marks a raw sample value as no-data (mapped to NaN, which the
// grid samplers skip). Apply the product's documented fill value (e.g. 65535
// for a uint16 VNP46A4 composite). The fill is matched on the RAW value, before
// any scale/offset.
func WithHDF5Fill(raw float64) HDF5Option {
	return func(c *hdf5Config) { c.fill = &raw }
}

// WithHDF5ScaleOffset applies value = raw·scale + offset to every non-fill
// sample (e.g. a scaled-integer product's scale_factor / add_offset). Defaults
// are scale=1, offset=0.
func WithHDF5ScaleOffset(scale, offset float64) HDF5Option {
	return func(c *hdf5Config) { c.scale, c.offset = scale, offset }
}

// LoadHDF5Grid reads a single named 2-D dataset from an HDF5 file into an
// in-memory [Grid], using the pure-Go scigolib/hdf5 reader (no CGO). dataset is
// the full HDF5 path (e.g.
// "/HDFEOS/GRIDS/VNP_Grid_DNB/Data Fields/AllAngle_Composite_Snow_Free"); gt is
// the affine geotransform for the grid (NASA tile products carry their extent in
// global attributes — read them and build the GeoTransform, or supply a known
// per-tile transform). The caller supplies the file; nothing is downloaded.
//
// The whole dataset is read into memory, so use a per-tile granule (a 10°×10°
// VNP46A4/VJ146A4 tile is ~46 MB at 15″), not the global mosaic. Sample values
// are interpreted by the chosen provider: [NewGridProvider] treats them as
// artificial brightness (mcd/m²); [NewVIIRSGridProvider] treats them as VIIRS
// radiance (nW·cm⁻²·sr⁻¹).
func LoadHDF5Grid(path, dataset string, gt GeoTransform, opts ...HDF5Option) (*Grid, error) {
	cfg := hdf5Config{scale: 1}
	for _, opt := range opts {
		opt(&cfg)
	}

	f, err := hdf5.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open %q: %w", ErrHDF5, path, err)
	}
	defer func() { _ = f.Close() }()

	var ds *hdf5.Dataset

	f.Walk(func(p string, obj hdf5.Object) {
		if d, ok := obj.(*hdf5.Dataset); ok && p == dataset {
			ds = d
		}
	})

	if ds == nil {
		return nil, fmt.Errorf("%w: dataset %q not found", ErrHDF5, dataset)
	}

	width, height, err := hdf5Dims(ds)
	if err != nil {
		return nil, err
	}

	vals, err := ds.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: read %q: %w", ErrHDF5, dataset, err)
	}

	if len(vals) != width*height {
		return nil, fmt.Errorf("%w: %q has %d samples, expected %d×%d=%d",
			ErrHDF5, dataset, len(vals), width, height, width*height)
	}

	applyFillScale(vals, cfg)

	return &Grid{
		Width:     width,
		Height:    height,
		Data:      vals,
		HasNoData: cfg.fill != nil,
		NoData:    math.NaN(),
		GT:        gt,
	}, nil
}

// hdf5Dims parses the dataset's row×col shape from its Info() string.
func hdf5Dims(ds *hdf5.Dataset) (width, height int, err error) {
	info, err := ds.Info()
	if err != nil {
		return 0, 0, fmt.Errorf("%w: info: %w", ErrHDF5, err)
	}

	m := hdf5DimsRe.FindStringSubmatch(info)
	if m == nil {
		return 0, 0, fmt.Errorf("%w: not a 2-D dataset: %q", ErrHDF5, info)
	}

	rows, _ := strconv.Atoi(m[1])
	cols, _ := strconv.Atoi(m[2])

	if rows <= 0 || cols <= 0 {
		return 0, 0, fmt.Errorf("%w: bad shape [%d x %d]", ErrHDF5, rows, cols)
	}

	return cols, rows, nil
}

// applyFillScale maps fill values to NaN (skipped as no-data) and applies the
// linear scale/offset to the remaining samples, in place.
func applyFillScale(vals []float64, cfg hdf5Config) {
	for i, v := range vals {
		if cfg.fill != nil && v == *cfg.fill {
			vals[i] = math.NaN()

			continue
		}

		vals[i] = v*cfg.scale + cfg.offset
	}
}

// NewVIIRSHDF5Provider loads a VIIRS radiance dataset from an HDF5 granule (e.g.
// NASA Black Marble VNP46A4 / VJ146A4) and returns a [skybrightness.SQMProvider]
// applying the empirical radiance→SQM fit (see [NewVIIRSProvider] for the
// fidelity caveats and coefficient citation). It combines [LoadHDF5Grid] and
// [NewVIIRSGridProvider]; HDF5 options configure the read (fill/scale) and VIIRS
// options the conversion coefficients.
func NewVIIRSHDF5Provider(path, dataset string, gt GeoTransform, hdf5Opts []HDF5Option, viirsOpts ...VIIRSOption) (skybrightness.SQMProvider, error) {
	grid, err := LoadHDF5Grid(path, dataset, gt, hdf5Opts...)
	if err != nil {
		return nil, err
	}

	return NewVIIRSGridProvider(grid, viirsOpts...)
}
