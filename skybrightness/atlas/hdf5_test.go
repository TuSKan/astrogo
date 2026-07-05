package atlas

import (
	"math"
	"path/filepath"
	"testing"

	"github.com/scigolib/hdf5"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// writeTestHDF5 creates a single-group HDF5 file with a 2-D float32 dataset at
// "/grids/data" holding the given row-major values, and returns its path.
func writeTestHDF5(t *testing.T, width, height int, data []float32) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "atlas_test.h5")

	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		t.Fatalf("CreateForWrite: %v", err)
	}

	if _, err := fw.CreateGroup("/grids"); err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}

	dw, err := fw.CreateDataset("/grids/data", hdf5.Float32, []uint64{uint64(height), uint64(width)})
	if err != nil {
		t.Fatalf("CreateDataset: %v", err)
	}

	if err := dw.Write(data); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := dw.Close(); err != nil {
		t.Fatalf("Dataset Close: %v", err)
	}

	if err := fw.Close(); err != nil {
		t.Fatalf("File Close: %v", err)
	}

	return path
}

// northUpGT builds a north-up geotransform with square pixels.
func northUpGT(originLon, originLat, px float64) GeoTransform {
	return GeoTransform{A: originLon, B: px, C: 0, D: originLat, E: 0, F: -px}
}

func centerLonLatGT(gt GeoTransform, col, row int) (lon, lat float64) {
	return gt.A + (float64(col)+0.5)*gt.B, gt.D + (float64(row)+0.5)*gt.F
}

// TestLoadHDF5Grid verifies a written HDF5 dataset round-trips into a Grid with
// the correct shape and per-pixel values, readable through NewGridProvider.
func TestLoadHDF5Grid(t *testing.T) {
	t.Parallel()

	const w, h = 4, 3

	data := rampPixels(w, h, 2) // pixel (c,r) = 2 + c + 10r, as mcd/m²

	path := writeTestHDF5(t, w, h, data)
	gt := northUpGT(-10, 40, 0.5)

	grid, err := LoadHDF5Grid(path, "/grids/data", gt)
	if err != nil {
		t.Fatalf("LoadHDF5Grid: %v", err)
	}

	if grid.Width != w || grid.Height != h {
		t.Fatalf("grid dims = %dx%d, want %dx%d", grid.Width, grid.Height, w, h)
	}

	p, err := NewGridProvider(grid)
	if err != nil {
		t.Fatalf("NewGridProvider: %v", err)
	}

	// Pixel (2,1) centre should read its mcd/m² value converted to SQM.
	lon, lat := centerLonLatGT(gt, 2, 1)

	got, err := p.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("ZenithBrightness: %v", err)
	}

	want := skybrightness.SurfaceBrightnessFromMcdM2(float64(data[1*w+2]))
	testutil.AssertNear(t, "grid SQM", float64(got), float64(want), 1e-4)
}

// TestLoadHDF5GridFill verifies a fill value is mapped to no-data.
func TestLoadHDF5GridFill(t *testing.T) {
	t.Parallel()

	const w, h = 2, 2

	// All cells are the fill sentinel ⇒ every sample is no-data.
	data := []float32{-999, -999, -999, -999}

	path := writeTestHDF5(t, w, h, data)
	gt := northUpGT(0, 0, 1.0)

	grid, err := LoadHDF5Grid(path, "/grids/data", gt, WithHDF5Fill(-999))
	if err != nil {
		t.Fatalf("LoadHDF5Grid: %v", err)
	}

	p, _ := NewGridProvider(grid)

	lon, lat := centerLonLatGT(gt, 0, 0)
	if _, err := p.ZenithBrightness(lat, lon); err == nil {
		t.Error("expected no-data error for all-fill grid, got nil")
	}
}

// TestLoadHDF5GridScaleOffset verifies scale/offset is applied to raw samples.
func TestLoadHDF5GridScaleOffset(t *testing.T) {
	t.Parallel()

	const w, h = 2, 2

	data := []float32{10, 20, 30, 40}

	path := writeTestHDF5(t, w, h, data)
	gt := northUpGT(0, 0, 1.0)

	grid, err := LoadHDF5Grid(path, "/grids/data", gt, WithHDF5ScaleOffset(0.1, 1))
	if err != nil {
		t.Fatalf("LoadHDF5Grid: %v", err)
	}

	// Raw 10 ⇒ 10*0.1 + 1 = 2.0 mcd/m².
	if v := grid.Data[0]; math.Abs(v-2.0) > 1e-9 {
		t.Errorf("scaled sample = %g, want 2.0", v)
	}
}

// TestLoadHDF5GridMissing verifies a clear error for an absent dataset path.
func TestLoadHDF5GridMissing(t *testing.T) {
	t.Parallel()

	path := writeTestHDF5(t, 2, 2, []float32{1, 2, 3, 4})

	if _, err := LoadHDF5Grid(path, "/grids/nope", northUpGT(0, 0, 1)); err == nil {
		t.Error("expected ErrHDF5 for missing dataset, got nil")
	}
}

// TestNewVIIRSHDF5Provider verifies the end-to-end HDF5→VIIRS path: a radiance
// granule decoded and converted through the empirical fit, matching the
// per-pixel reference conversion.
func TestNewVIIRSHDF5Provider(t *testing.T) {
	t.Parallel()

	const w, h = 3, 2

	// Radiance values (nW·cm⁻²·sr⁻¹).
	data := []float32{5, 10, 20, 40, 80, 160}

	path := writeTestHDF5(t, w, h, data)
	gt := northUpGT(-46, -23, 0.25)

	p, err := NewVIIRSHDF5Provider(path, "/grids/data", gt, nil)
	if err != nil {
		t.Fatalf("NewVIIRSHDF5Provider: %v", err)
	}

	lon, lat := centerLonLatGT(gt, 1, 0)

	got, err := p.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("ZenithBrightness: %v", err)
	}

	want := radianceToArtificialSB(float64(data[1]), viirsSlope, viirsZeroPoint)
	testutil.AssertNear(t, "VIIRS HDF5 SB", float64(got), float64(want), 1e-4)
}
