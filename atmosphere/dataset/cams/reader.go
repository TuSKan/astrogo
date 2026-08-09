package cams

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"sync"

	"github.com/scigolib/hdf5"
	gofs "github.com/ungerik/go-fs"
)

// File is an open CAMS NetCDF-4/HDF5 file. See the package doc comment
// for how shape and axis order are discovered.
type File struct {
	mu   sync.Mutex // guards every call into hf; see the package doc comment's concurrency note
	hf   *hdf5.File
	path gofs.File

	// dims maps a dimension name (longitude/latitude/level/time) to its
	// length, read once at Open time from the file's dimension-scale
	// datasets.
	dims map[string]int

	// dimNameByID maps a NetCDF-4 dimension ID (from a dimension-scale
	// dataset's own _Netcdf4Dimid attribute) to that dimension's name —
	// used to translate a data variable's _Netcdf4Coordinates into real
	// axis names.
	dimNameByID map[int32]string

	// vars maps a data-variable name to its still-unopened dataset
	// handle; only its attributes have been read at Open time, never its
	// bulk data.
	vars map[string]*hdf5.Dataset
}

// Open opens f — which must already be on the local filesystem, e.g. the
// result of remote.GetFile against remote.CopernicusEODATA — as a CAMS
// NetCDF-4/HDF5 file. Every dimension-scale dataset (longitude, latitude,
// level if present, time) is read eagerly; data variables are indexed by
// name but not read until Var.ReadPlane/Var.At is called.
func Open(f gofs.File) (*File, error) {
	path := f.LocalPath()
	if path == "" {
		return nil, fmt.Errorf("%w: %s", ErrNotLocal, f)
	}

	hf, err := hdf5.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cams: open %s: %w", f, err)
	}

	cf := &File{
		hf:          hf,
		path:        f,
		dims:        make(map[string]int),
		dimNameByID: make(map[int32]string),
		vars:        make(map[string]*hdf5.Dataset),
	}

	if err := cf.index(); err != nil {
		_ = hf.Close()
		return nil, err
	}

	return cf, nil
}

// Close closes the underlying file. Safe to call once; a File must not
// be used afterward.
func (cf *File) Close() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	err := cf.hf.Close()
	if err != nil {
		return fmt.Errorf("cams: close %s: %w", cf.path, err)
	}

	return nil
}

// Dims returns every dimension this file declares (longitude, latitude,
// time, and level if present) mapped to its length. The returned map is
// a copy; mutating it does not affect the File.
func (cf *File) Dims() map[string]int {
	out := make(map[string]int, len(cf.dims))
	maps.Copy(out, cf.dims)

	return out
}

// Var opens the named data variable (e.g. "lnsp", "den", "aermr01") in
// this file. Returns ErrVariableNotFound (check with errors.Is) if the
// file has no variable by that name — real and expected, since tracer
// availability is dataset/version-specific (docs/skybrightness.md §8) —
// distinct from any other error, which means the variable exists but its
// metadata could not be decoded. This never fabricates a Var for a name
// the file doesn't have, and never silently downgrades a genuine decode
// failure into "not found".
func (cf *File) Var(name string) (*Var, error) {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	ds, ok := cf.vars[name]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrVariableNotFound, name)
	}

	v, err := cf.newVar(name, ds)
	if err != nil {
		return nil, err
	}

	return v, nil
}

// index walks the file once, classifying every dataset as either a
// dimension-scale (read fully, its length and ID recorded) or a data
// variable (attributes read, bulk data left untouched).
func (cf *File) index() error {
	type dimEntry struct {
		name   string
		length int
		id     int32
		hasID  bool
	}

	var (
		dimEntries []dimEntry
		walkErr    error
	)

	cf.hf.Walk(func(_ string, obj hdf5.Object) {
		if walkErr != nil {
			return
		}

		ds, ok := obj.(*hdf5.Dataset)
		if !ok {
			return
		}

		if !isDimensionScale(ds) {
			cf.vars[ds.Name()] = ds
			return
		}

		vals, err := ds.Read()
		if err != nil {
			walkErr = fmt.Errorf("cams: %s: read dimension-scale %s: %w", cf.path, ds.Name(), err)
			return
		}

		id, hasID, err := readInt32Attribute(ds, "_Netcdf4Dimid")
		if err != nil {
			walkErr = fmt.Errorf("cams: %s: %s: %w", cf.path, ds.Name(), err)
			return
		}

		dimEntries = append(dimEntries, dimEntry{name: ds.Name(), length: len(vals), id: id, hasID: hasID})
	})

	if walkErr != nil {
		return walkErr
	}

	for _, e := range dimEntries {
		cf.dims[e.name] = e.length
		if e.hasID {
			cf.dimNameByID[e.id] = e.name
		}
	}

	return nil
}

// newVar builds a Var from ds, discovering its real axis order from its
// own _Netcdf4Coordinates attribute (see the package doc comment) and
// reading its units/long_name/_FillValue/missing_value attributes.
func (cf *File) newVar(name string, ds *hdf5.Dataset) (*Var, error) {
	coordIDs, err := readInt32SliceAttribute(ds, "_Netcdf4Coordinates")
	if err != nil {
		return nil, fmt.Errorf("cams: %s: %s: %w", cf.path, name, err)
	}

	axisNames := make([]string, len(coordIDs))

	for i, id := range coordIDs {
		axisName, ok := cf.dimNameByID[id]
		if !ok {
			return nil, fmt.Errorf("cams: %s: %s: %w: dimension id %d has no matching dimension-scale dataset",
				cf.path, name, ErrUnsupportedAxis, id)
		}

		axisNames[i] = axisName
	}

	units, err := readStringAttribute(ds, "units")
	if err != nil {
		return nil, err
	}

	longName, err := readStringAttribute(ds, "long_name")
	if err != nil {
		return nil, err
	}

	fillValue, hasFill, err := readFloat64Attribute(ds, "_FillValue")
	if err != nil {
		return nil, err
	}

	missingVal, hasMissing, err := readFloat64Attribute(ds, "missing_value")
	if err != nil {
		return nil, err
	}

	return &Var{
		file:         cf,
		ds:           ds,
		name:         name,
		axisNames:    axisNames,
		units:        units,
		longName:     longName,
		fillValue:    fillValue,
		hasFillValue: hasFill,
		missingVal:   missingVal,
		hasMissing:   hasMissing,
	}, nil
}

// isDimensionScale reports whether ds is a NetCDF-4 dimension-scale
// dataset (CLASS == "DIMENSION_SCALE") rather than a data variable. An
// absent CLASS attribute (the common case for a data variable) simply
// means false — never treated as a decode failure.
func isDimensionScale(ds *hdf5.Dataset) bool {
	v, err := ds.ReadAttribute("CLASS")
	if err != nil {
		return false
	}

	s, ok := v.(string)

	return ok && s == "DIMENSION_SCALE"
}

// readInt32Attribute reads name as an int32, tolerating the small set of
// numeric Go types ReadAttribute may return it as. ok is false (with a
// nil error) when the attribute is simply absent; a non-nil error means
// the attribute exists but decoded to something this reader cannot
// interpret as an integer.
func readInt32Attribute(ds *hdf5.Dataset, name string) (value int32, ok bool, err error) {
	v, err := ds.ReadAttribute(name)
	if err != nil {
		return 0, false, nil //nolint:nilerr // attribute absence is expected, not an error
	}

	switch n := v.(type) {
	case int32:
		return n, true, nil
	case int64:
		//nolint:gosec // G115: NetCDF-4 dimension IDs are always small (0-3 for CAMS's 4 axes)
		return int32(n), true, nil
	case int:
		//nolint:gosec // G115: NetCDF-4 dimension IDs are always small (0-3 for CAMS's 4 axes)
		return int32(n), true, nil
	default:
		return 0, false, fmt.Errorf("%w: attribute %q is %T, want an integer", ErrUnexpectedAttributeType, name, v)
	}
}

// readFloat64Attribute reads name as a float64, tolerating the numeric
// Go types ReadAttribute may return it as. ok is false (nil error) when
// the attribute is absent.
func readFloat64Attribute(ds *hdf5.Dataset, name string) (value float64, ok bool, err error) {
	v, err := ds.ReadAttribute(name)
	if err != nil {
		return 0, false, nil //nolint:nilerr // attribute absence is expected, not an error
	}

	switch n := v.(type) {
	case float64:
		return n, true, nil
	case float32:
		return float64(n), true, nil
	case int32:
		return float64(n), true, nil
	case int64:
		return float64(n), true, nil
	default:
		return 0, false, fmt.Errorf("%w: attribute %q is %T, want a float", ErrUnexpectedAttributeType, name, v)
	}
}

// readStringAttribute reads name as a string, returning "" (nil error)
// when the attribute is absent.
func readStringAttribute(ds *hdf5.Dataset, name string) (string, error) {
	v, err := ds.ReadAttribute(name)
	if err != nil {
		return "", nil //nolint:nilerr // attribute absence is expected, not an error
	}

	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: attribute %q is %T, want a string", ErrUnexpectedAttributeType, name, v)
	}

	return s, nil
}

// readInt32SliceAttribute reads name as a []int32.
func readInt32SliceAttribute(ds *hdf5.Dataset, name string) ([]int32, error) {
	v, err := ds.ReadAttribute(name)
	if err != nil {
		return nil, fmt.Errorf("attribute %q: %w", name, err)
	}

	s, ok := v.([]int32)
	if !ok {
		return nil, fmt.Errorf("%w: attribute %q is %T, want []int32", ErrUnexpectedAttributeType, name, v)
	}

	return s, nil
}

// nonNegUint64 converts n to uint64 for use as a hyperslab start/count
// value. n is always a grid index or dimension length here — CAMS's own
// grid is tiny (longitude/latitude in the hundreds, level at most 137) —
// so this never truncates in practice; a negative n (a caller error,
// e.g. a negative timeIdx/level) clamps to 0 rather than wrapping around
// to a huge uint64, so it fails ReadSlice's own bounds check with a
// clear "out of bounds" error instead of silently addressing an
// unrelated, huge offset.
func nonNegUint64(n int) uint64 {
	if n < 0 {
		return 0
	}

	return uint64(n)
}

// Var represents one CAMS data variable, opened against its parent File.
type Var struct {
	file      *File
	ds        *hdf5.Dataset
	name      string
	axisNames []string // on-disk axis order, e.g. ["time","level","latitude","longitude"]

	units, longName          string
	fillValue, missingVal    float64
	hasFillValue, hasMissing bool

	planeMu     sync.Mutex
	planeTime   int
	planeLevel  int
	planeCached bool
	plane       []float64
}

// Name returns the variable's name (e.g. "aermr01").
func (v *Var) Name() string { return v.name }

// Units returns the variable's "units" attribute, or "" if absent.
func (v *Var) Units() string { return v.units }

// LongName returns the variable's "long_name" attribute, or "" if absent.
func (v *Var) LongName() string { return v.longName }

// AxisNames returns the variable's on-disk axis order, e.g.
// ["time","level","latitude","longitude"] or ["time","latitude","longitude"].
func (v *Var) AxisNames() []string {
	out := make([]string, len(v.axisNames))
	copy(out, v.axisNames)

	return out
}

// HasLevel reports whether this variable has a level axis. lnsp does
// not; aermrNN/den do.
func (v *Var) HasLevel() bool {
	return slices.Contains(v.axisNames, "level")
}

// ReadPlane reads exactly one (time, level) horizontal plane —
// latitude x longitude values — applying fill-value substitution (NaN)
// at the read boundary. For a variable with no level axis (HasLevel()
// false), level must be 0. This is the cheap, chunk-aligned access
// pattern (docs/skybrightness.md §8): a real ~86x faster than a full
// decode in the live spike this reader's design was validated against,
// since CAMS's own chunking is one plane per (time, level) pair.
func (v *Var) ReadPlane(timeIdx, level int) ([]float64, error) {
	if !v.HasLevel() && level != 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoLevelDimension, v.name)
	}

	start := make([]uint64, len(v.axisNames))
	count := make([]uint64, len(v.axisNames))

	for i, axis := range v.axisNames {
		switch axis {
		case "time":
			start[i], count[i] = nonNegUint64(timeIdx), 1
		case "level":
			start[i], count[i] = nonNegUint64(level), 1
		case "latitude":
			start[i], count[i] = 0, nonNegUint64(v.file.dims["latitude"])
		case "longitude":
			start[i], count[i] = 0, nonNegUint64(v.file.dims["longitude"])
		default:
			return nil, fmt.Errorf("%w: %q on variable %q", ErrUnsupportedAxis, axis, v.name)
		}
	}

	v.file.mu.Lock()
	raw, err := v.ds.ReadSlice(start, count)
	v.file.mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("cams: %s.ReadPlane(time=%d,level=%d): %w", v.name, timeIdx, level, err)
	}

	vals, ok := raw.([]float64)
	if !ok {
		return nil, fmt.Errorf("cams: %s.ReadPlane: %w: got %T, want []float64", v.name, ErrUnexpectedAttributeType, raw)
	}

	v.applyFillValues(vals)

	return vals, nil
}

// At returns the value at exactly one (time, level, lat, lon) grid
// index. level is ignored (must be 0) for a variable with no level axis.
// Repeated calls at the same (time, level) reuse the last ReadPlane
// result via a small single-plane cache rather than re-reading (and, for
// a chunked variable, re-decompressing) the chunk every time; calls
// across different (time, level) pairs each cost one ReadPlane.
func (v *Var) At(timeIdx, level, lat, lon int) (float64, error) {
	latN := v.file.dims["latitude"]
	lonN := v.file.dims["longitude"]

	if lat < 0 || lat >= latN || lon < 0 || lon >= lonN {
		return math.NaN(), fmt.Errorf("%w: lat=%d (0..%d) lon=%d (0..%d)", ErrIndexOutOfRange, lat, latN-1, lon, lonN-1)
	}

	v.planeMu.Lock()
	defer v.planeMu.Unlock()

	if !v.planeCached || v.planeTime != timeIdx || v.planeLevel != level {
		plane, err := v.ReadPlane(timeIdx, level)
		if err != nil {
			return math.NaN(), err
		}

		v.plane = plane
		v.planeTime = timeIdx
		v.planeLevel = level
		v.planeCached = true
	}

	return v.plane[lat*lonN+lon], nil
}

// applyFillValues replaces any element matching the variable's
// _FillValue or missing_value with NaN, in place, so a caller never sees
// the raw sentinel as if it were a real measurement.
func (v *Var) applyFillValues(vals []float64) {
	if !v.hasFillValue && !v.hasMissing {
		return
	}

	for i, x := range vals {
		if (v.hasFillValue && x == v.fillValue) || (v.hasMissing && x == v.missingVal) {
			vals[i] = math.NaN()
		}
	}
}
