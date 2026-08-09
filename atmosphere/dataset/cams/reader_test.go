package cams

import (
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/scigolib/hdf5"
	gofs "github.com/ungerik/go-fs"
)

// fillValue mirrors the real CAMS/NetCDF default double fill value
// (NC_FILL_DOUBLE), confirmed live against the real files this package
// was built against (see doc.go).
const fillValue = 9.969209968386687e+36

// synthFixture builds a tiny, synthetic CAMS-shaped NetCDF-4/HDF5 file —
// never a real downloaded .nc file, per this package's test policy — with
// two data variables sharing one set of dimension-scale datasets:
//
//   - aermr01: float64, axes (time=1, level=2, latitude=3, longitude=4),
//     exercising the "has level" ReadPlane/At path and fill value
//     substitution (one deliberately fill-valued cell).
//   - lnsp: float64, axes (time=1, latitude=3, longitude=4) — no level
//     axis — exercising the ErrNoLevelDimension path.
//
// Both use plain contiguous layout, not chunked: scigolib/hdf5 v0.14.0's
// write path for a dataset that is both chunked AND carries attributes
// is broken (confirmed by an isolated spike — a chunked dataset with
// zero attributes round-trips fine; the identical dataset with a
// handful of attributes attached fails on read with "failed to parse
// layout: chunked layout dimension N truncated (32-bit)", meaning
// attribute writes corrupt the already-written Data Layout message's
// on-disk bytes). A separate bug affects WithGZIPCompression's write
// path too (fails with "unsupported filter ID: 0" even with zero other
// attributes). Neither bug affects this package's real read path — the
// real, chunked, gzip-compressed, attribute-bearing CAMS files this
// reader targets were independently confirmed to decode correctly (see
// doc.go and ground_truth_test.go), because this package only ever
// reads files scigolib/hdf5 did not write itself. Plain contiguous
// layout round-trips correctly, including through ReadSlice, and is
// sufficient to validate this package's shape/axis/fill-value/plane-
// extraction logic — the real storage layout's I/O-cost implications
// were already established against production data.
//
// Every data value is level*100 + lat*10 + lon (aermr01) or 1000 + lat*10
// + lon (lnsp), so ReadPlane/At results are checkable by direct formula
// rather than a second fixture-specific table.
func synthFixture(t *testing.T) gofs.File {
	t.Helper()

	path := filepath.Join(t.TempDir(), "synthetic.nc")

	fw, err := hdf5.CreateForWrite(path, hdf5.CreateTruncate)
	if err != nil {
		t.Fatalf("CreateForWrite: %v", err)
	}

	writeDimScale(t, fw, "/longitude", hdf5.Float32, []float32{0, 90, 180, 270}, 0, "degrees_east", "longitude")
	writeDimScale(t, fw, "/latitude", hdf5.Float32, []float32{90, 0, -90}, 1, "degrees_north", "latitude")
	writeLevelDim(t, fw, []int32{1, 2}, 2)
	writeTimeDim(t, fw, []int32{1078200}, 3)

	// aermr01: (time, level, latitude, longitude) = (1,2,3,4).
	aermr, err := fw.CreateDataset("/aermr01", hdf5.Float64, []uint64{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("CreateDataset aermr01: %v", err)
	}

	mustWriteAttr(t, aermr, "_Netcdf4Coordinates", []int32{3, 2, 1, 0})
	mustWriteAttr(t, aermr, "_FillValue", fillValue)
	mustWriteAttr(t, aermr, "missing_value", fillValue)
	mustWriteAttr(t, aermr, "units", "kg kg**-1")
	mustWriteAttr(t, aermr, "long_name", "Sea Salt Aerosol (test)")

	aermrData := make([]float64, 1*2*3*4)

	i := 0

	for level := range 2 {
		for lat := range 3 {
			for lon := range 4 {
				aermrData[i] = float64(level*100 + lat*10 + lon)
				i++
			}
		}
	}
	// Deliberately fill-valued cell: level=1, lat=2, lon=3 (the last element).
	aermrData[len(aermrData)-1] = fillValue

	if err := aermr.Write(aermrData); err != nil {
		t.Fatalf("Write aermr01: %v", err)
	}

	if err := aermr.Close(); err != nil {
		t.Fatalf("Close aermr01 writer: %v", err)
	}

	// lnsp: (time, latitude, longitude) = (1,3,4) — no level axis.
	lnsp, err := fw.CreateDataset("/lnsp", hdf5.Float64, []uint64{1, 3, 4})
	if err != nil {
		t.Fatalf("CreateDataset lnsp: %v", err)
	}

	mustWriteAttr(t, lnsp, "_Netcdf4Coordinates", []int32{3, 1, 0})
	mustWriteAttr(t, lnsp, "_FillValue", fillValue)
	mustWriteAttr(t, lnsp, "units", "~")
	mustWriteAttr(t, lnsp, "long_name", "Logarithm of surface pressure")

	lnspData := make([]float64, 1*3*4)

	i = 0

	for lat := range 3 {
		for lon := range 4 {
			lnspData[i] = float64(1000 + lat*10 + lon)
			i++
		}
	}

	if err := lnsp.Write(lnspData); err != nil {
		t.Fatalf("Write lnsp: %v", err)
	}

	if err := lnsp.Close(); err != nil {
		t.Fatalf("Close lnsp writer: %v", err)
	}

	if err := fw.Close(); err != nil {
		t.Fatalf("Close FileWriter: %v", err)
	}

	return gofs.File(path)
}

func writeDimScale(t *testing.T, fw *hdf5.FileWriter, name string, dtype hdf5.Datatype, data any, dimid int32, units, longName string) {
	t.Helper()

	n := reflectLen(t, data)

	ds, err := fw.CreateDataset(name, dtype, []uint64{uint64(n)})
	if err != nil {
		t.Fatalf("CreateDataset %s: %v", name, err)
	}

	mustWriteAttr(t, ds, "CLASS", "DIMENSION_SCALE")
	mustWriteAttr(t, ds, "NAME", name[1:])
	mustWriteAttr(t, ds, "_Netcdf4Dimid", dimid)

	if units != "" {
		mustWriteAttr(t, ds, "units", units)
	}

	mustWriteAttr(t, ds, "long_name", longName)

	if err := ds.Write(data); err != nil {
		t.Fatalf("Write %s: %v", name, err)
	}

	if err := ds.Close(); err != nil {
		t.Fatalf("Close %s writer: %v", name, err)
	}
}

func writeLevelDim(t *testing.T, fw *hdf5.FileWriter, data []int32, dimid int32) {
	t.Helper()

	ds, err := fw.CreateDataset("/level", hdf5.Int32, []uint64{uint64(len(data))})
	if err != nil {
		t.Fatalf("CreateDataset level: %v", err)
	}

	mustWriteAttr(t, ds, "CLASS", "DIMENSION_SCALE")
	mustWriteAttr(t, ds, "NAME", "level")
	mustWriteAttr(t, ds, "_Netcdf4Dimid", dimid)
	mustWriteAttr(t, ds, "long_name", "model_level_number") // real CAMS files carry no "units" here

	if err := ds.Write(data); err != nil {
		t.Fatalf("Write level: %v", err)
	}

	if err := ds.Close(); err != nil {
		t.Fatalf("Close level writer: %v", err)
	}
}

func writeTimeDim(t *testing.T, fw *hdf5.FileWriter, data []int32, dimid int32) {
	t.Helper()

	ds, err := fw.CreateDataset("/time", hdf5.Int32, []uint64{uint64(len(data))})
	if err != nil {
		t.Fatalf("CreateDataset time: %v", err)
	}

	mustWriteAttr(t, ds, "CLASS", "DIMENSION_SCALE")
	mustWriteAttr(t, ds, "NAME", "time")
	mustWriteAttr(t, ds, "_Netcdf4Dimid", dimid)
	mustWriteAttr(t, ds, "units", "hours since 1900-01-01 00:00:00.0")
	mustWriteAttr(t, ds, "long_name", "time")
	mustWriteAttr(t, ds, "calendar", "gregorian")

	if err := ds.Write(data); err != nil {
		t.Fatalf("Write time: %v", err)
	}

	if err := ds.Close(); err != nil {
		t.Fatalf("Close time writer: %v", err)
	}
}

func mustWriteAttr(t *testing.T, ds *hdf5.DatasetWriter, name string, value any) {
	t.Helper()

	if err := ds.WriteAttribute(name, value); err != nil {
		t.Fatalf("WriteAttribute %s: %v", name, err)
	}
}

func reflectLen(t *testing.T, data any) int {
	t.Helper()

	switch d := data.(type) {
	case []float32:
		return len(d)
	case []int32:
		return len(d)
	default:
		t.Fatalf("reflectLen: unsupported type %T", data)

		return 0
	}
}

func TestOpenAndDims(t *testing.T) {
	f, err := Open(synthFixture(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	dims := f.Dims()
	want := map[string]int{"longitude": 4, "latitude": 3, "level": 2, "time": 1}

	for name, n := range want {
		if dims[name] != n {
			t.Errorf("Dims()[%q] = %d, want %d", name, dims[name], n)
		}
	}
}

func TestVarNotFound(t *testing.T) {
	f, err := Open(synthFixture(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	_, err = f.Var("aermr99")
	if !errors.Is(err, ErrVariableNotFound) {
		t.Errorf("Var(missing) err = %v, want ErrVariableNotFound", err)
	}
}

func TestVarMetadataAndAxisOrder(t *testing.T) {
	f, err := Open(synthFixture(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	aermr, err := f.Var("aermr01")
	if err != nil {
		t.Fatalf("Var(aermr01): %v", err)
	}

	if got := aermr.Units(); got != "kg kg**-1" {
		t.Errorf("aermr01.Units() = %q", got)
	}

	if got := aermr.LongName(); got != "Sea Salt Aerosol (test)" {
		t.Errorf("aermr01.LongName() = %q", got)
	}

	if !aermr.HasLevel() {
		t.Error("aermr01.HasLevel() = false, want true")
	}

	wantAxes := []string{"time", "level", "latitude", "longitude"}
	gotAxes := aermr.AxisNames()

	if len(gotAxes) != len(wantAxes) {
		t.Fatalf("aermr01.AxisNames() = %v, want %v", gotAxes, wantAxes)
	}

	for i, a := range wantAxes {
		if gotAxes[i] != a {
			t.Errorf("aermr01.AxisNames()[%d] = %q, want %q", i, gotAxes[i], a)
		}
	}

	lnsp, err := f.Var("lnsp")
	if err != nil {
		t.Fatalf("Var(lnsp): %v", err)
	}

	if lnsp.HasLevel() {
		t.Error("lnsp.HasLevel() = true, want false")
	}

	wantLnspAxes := []string{"time", "latitude", "longitude"}
	gotLnspAxes := lnsp.AxisNames()

	if len(gotLnspAxes) != len(wantLnspAxes) {
		t.Fatalf("lnsp.AxisNames() = %v, want %v", gotLnspAxes, wantLnspAxes)
	}

	for i, a := range wantLnspAxes {
		if gotLnspAxes[i] != a {
			t.Errorf("lnsp.AxisNames()[%d] = %q, want %q", i, gotLnspAxes[i], a)
		}
	}
}

func TestReadPlaneAndFillValue(t *testing.T) {
	f, err := Open(synthFixture(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	aermr, err := f.Var("aermr01")
	if err != nil {
		t.Fatalf("Var(aermr01): %v", err)
	}

	plane, err := aermr.ReadPlane(0, 1) // level=1
	if err != nil {
		t.Fatalf("ReadPlane(0,1): %v", err)
	}

	if len(plane) != 3*4 {
		t.Fatalf("ReadPlane(0,1) len = %d, want %d", len(plane), 3*4)
	}

	// level=1: values are 100 + lat*10 + lon, except the deliberately
	// fill-valued last cell (lat=2, lon=3), which must be NaN.
	for lat := range 3 {
		for lon := range 4 {
			got := plane[lat*4+lon]
			if lat == 2 && lon == 3 {
				if !math.IsNaN(got) {
					t.Errorf("plane[lat=2,lon=3] = %v, want NaN (fill value)", got)
				}

				continue
			}

			want := float64(100 + lat*10 + lon)
			if got != want {
				t.Errorf("plane[lat=%d,lon=%d] = %v, want %v", lat, lon, got, want)
			}
		}
	}

	// level=0 plane must be untouched by level=1's fill-value cell.
	plane0, err := aermr.ReadPlane(0, 0)
	if err != nil {
		t.Fatalf("ReadPlane(0,0): %v", err)
	}

	if math.IsNaN(plane0[len(plane0)-1]) {
		t.Error("ReadPlane(0,0)'s last cell is NaN, want a real value (fill value was only injected at level=1)")
	}
}

func TestReadPlaneRejectsLevelOnSurfaceVariable(t *testing.T) {
	f, err := Open(synthFixture(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	lnsp, err := f.Var("lnsp")
	if err != nil {
		t.Fatalf("Var(lnsp): %v", err)
	}

	if _, err := lnsp.ReadPlane(0, 1); !errors.Is(err, ErrNoLevelDimension) {
		t.Errorf("lnsp.ReadPlane(0,1) err = %v, want ErrNoLevelDimension", err)
	}

	plane, err := lnsp.ReadPlane(0, 0)
	if err != nil {
		t.Fatalf("lnsp.ReadPlane(0,0): %v", err)
	}

	if len(plane) != 3*4 {
		t.Fatalf("lnsp.ReadPlane(0,0) len = %d, want %d", len(plane), 3*4)
	}

	if plane[0] != 1000 {
		t.Errorf("lnsp.ReadPlane(0,0)[0] = %v, want 1000", plane[0])
	}
}

func TestAtAndPlaneCache(t *testing.T) {
	f, err := Open(synthFixture(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	aermr, err := f.Var("aermr01")
	if err != nil {
		t.Fatalf("Var(aermr01): %v", err)
	}

	for _, level := range []int{0, 1} {
		for lat := range 3 {
			for lon := range 4 {
				got, err := aermr.At(0, level, lat, lon)
				if err != nil {
					t.Fatalf("At(0,%d,%d,%d): %v", level, lat, lon, err)
				}

				if level == 1 && lat == 2 && lon == 3 {
					if !math.IsNaN(got) {
						t.Errorf("At(0,1,2,3) = %v, want NaN", got)
					}

					continue
				}

				want := float64(level*100 + lat*10 + lon)
				if got != want {
					t.Errorf("At(0,%d,%d,%d) = %v, want %v", level, lat, lon, got, want)
				}
			}
		}
	}
}

func TestAtIndexOutOfRange(t *testing.T) {
	f, err := Open(synthFixture(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = f.Close() })

	aermr, err := f.Var("aermr01")
	if err != nil {
		t.Fatalf("Var(aermr01): %v", err)
	}

	cases := []struct{ lat, lon int }{
		{-1, 0}, {0, -1}, {3, 0}, {0, 4},
	}
	for _, c := range cases {
		if _, err := aermr.At(0, 0, c.lat, c.lon); !errors.Is(err, ErrIndexOutOfRange) {
			t.Errorf("At(0,0,%d,%d) err = %v, want ErrIndexOutOfRange", c.lat, c.lon, err)
		}
	}
}

func TestOpenRejectsNonLocalFile(t *testing.T) {
	_, err := Open(gofs.File("s3://some-bucket/some/key.nc"))
	if !errors.Is(err, ErrNotLocal) {
		t.Errorf("Open(non-local) err = %v, want ErrNotLocal", err)
	}
}
