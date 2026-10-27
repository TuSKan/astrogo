package cams

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/scigolib/hdf5"

	"github.com/TuSKan/astrogo/remote"
)

// File is an open CAMS NetCDF-4/HDF5 file. See the package doc comment
// for how shape and axis order are discovered.
type File struct {
	mu  sync.Mutex // guards every call into hf; see the package doc comment's concurrency note
	hf  *hdf5.File
	key string // the fsys key Open was given, kept only for error messages

	// scratchPath is the temp file Open staged bucket/key's content into
	// (see Open's doc comment for why); removed by Close.
	scratchPath string

	// size is the file's length in bytes, which bounds how many values any
	// of its datasets can hold; see checkExtent.
	size uint64

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

// Open opens bucket/key — e.g. the result of remote.GetFile against
// remote.CopernicusEODATA — as a CAMS NetCDF-4/HDF5 file, reading its
// content through remote.Open rather than assuming it's backed by
// the local filesystem (fsys may be local, S3, or any other
// remote/file backend). Every dimension-scale dataset (longitude,
// latitude, level if present, time) is read eagerly; data variables are
// indexed by name but not read until Var.ReadPlane/Var.At is called.
func Open(ctx context.Context, fsys remote.FS, key string) (*File, error) {
	r, err := remote.Open(ctx, fsys, key)
	if err != nil {
		return nil, fmt.Errorf("cams: open %s: %w", key, err)
	}
	defer func() { _ = r.Close() }()

	// scigolib/hdf5's only public entry point is Open(filename string) --
	// no io.Reader/io.ReaderAt constructor exists in this dependency, and
	// HDF5's own on-disk format needs true random access (B-tree chunk
	// index traversal) a plain io.Reader cannot support anyway. Staging
	// through a scratch temp file, read via remote.Open rather
	// than assuming the fsys is already local, is this package's
	// equivalent of catalog/fink's os.CreateTemp exception (CLAUDE.md) --
	// a narrow, documented workaround for a third-party library that
	// needs a real OS path, not a way to special-case a local fsys.
	tmp, err := os.CreateTemp("", "cams-*.nc")
	if err != nil {
		return nil, fmt.Errorf("cams: %s: create scratch file: %w", key, err)
	}

	tmpPath := tmp.Name()

	size, err := io.Copy(tmp, r)
	if err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)

		return nil, fmt.Errorf("cams: %s: stage to scratch file: %w", key, err)
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)

		return nil, fmt.Errorf("cams: %s: close scratch file: %w", key, err)
	}

	var hf *hdf5.File

	if err := decode(key, "open", func() (err error) {
		hf, err = hdf5.Open(tmpPath)

		return err //nolint:wrapcheck // decode wraps it with the key and the operation
	}); err != nil {
		_ = os.Remove(tmpPath)

		return nil, err
	}

	cf := &File{
		hf:          hf,
		key:         key,
		scratchPath: tmpPath,
		size:        nonNegUint64(int(size)),
		dims:        make(map[string]int),
		dimNameByID: make(map[int32]string),
		vars:        make(map[string]*hdf5.Dataset),
	}

	if err := decode(key, "index", cf.index); err != nil {
		_ = release(key, hf.Close)
		_ = os.Remove(tmpPath)

		return nil, err
	}

	return cf, nil
}

// decode runs one call into the HDF5 decoder that reads the file's content,
// and makes whatever goes wrong inside it an error wrapping [ErrMalformed]
// (#794): a panic, and an error the decoder returns.
//
// A panic is the index or nil dereference a malformed structure leads a
// decoder into. A returned error is the decoder recognizing the structure as
// malformed itself: from scigolib/hdf5 v0.14.2 a local heap claiming 720 PB is
// refused as "exceeds maximum" where it used to panic, and the file is just as
// corrupt either way. The file is a scratch copy this package wrote a moment
// before, so the decoder's error is about its content, not the disk.
//
// It cannot catch everything. A failed allocation is a fatal error in Go, not
// a panic, and no deferred recover sees it; that is what checkExtent is for,
// before any read that sizes a buffer from the file.
func decode(key, op string, fn func() error) error {
	return guarded(key, op, fn, true)
}

// release closes the decoder, turning a panic inside it into an error wrapping
// [ErrMalformed] as decode does. An error Close returns is the operating
// system's, closing the scratch file, and is returned as such.
func release(key string, fn func() error) error {
	return guarded(key, "close", fn, false)
}

// guarded is decode and release: the panic made an error, and a returned
// error marked malformed when it comes from reading the file.
func guarded(key, op string, fn func() error, content bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %s: %s: the decoder failed: %v", ErrMalformed, key, op, r)
		}
	}()

	if err := fn(); err != nil {
		if content && !errors.Is(err, ErrMalformed) {
			return fmt.Errorf("%w: %s: %s: %w", ErrMalformed, key, op, err)
		}

		return fmt.Errorf("cams: %s: %s: %w", key, op, err)
	}

	return nil
}

// deflateMaxRatio is the most a DEFLATE stream can expand, zlib's documented
// limit: RFC 1951's longest match, 258 bytes, can be coded in as few as two
// bits, so a byte of compressed data inflates to at most 1032. A chunked
// dataset compressed that way decompresses to no more than this many times
// the bytes of the file holding it.
const deflateMaxRatio = 1032

// datasetShape matches the dataspace, the chunk shape and the element size in
// the description the decoder gives of a dataset, Dataset.Info: "1D array [n]",
// "2D array [n x m]", "3D array [a b c]", "chunked (chunks=[a b c])", and the
// datatype's "float (size=8 bytes)".
var (
	datasetShape = regexp.MustCompile(`\d+D array \[([0-9 x]*)\]`)
	chunkShape   = regexp.MustCompile(`chunked \(chunks=\[([0-9 ]*)\]\)`)
	valueSize    = regexp.MustCompile(`\(size=([0-9]+) bytes\)`)
)

// whole is checkExtent's n for a read of the entire dataset. No read asks for
// this many values, so it cannot be mistaken for a real count.
const whole = math.MaxUint64

// declaredValues is the number of values the decoder's description of a
// dataset declares, and whether the description could be read at all.
func declaredValues(info string) (uint64, bool) {
	switch m := datasetShape.FindStringSubmatch(info); {
	case m != nil:
		return shapeProduct(m[1]), true
	case strings.Contains(info, ", scalar, "):
		return 1, true
	case strings.Contains(info, ", null, "):
		return 0, true
	default:
		return 0, false
	}
}

// shapeProduct is the number of values in a shape the decoder printed, its
// dimensions separated by spaces or " x ", saturating rather than wrapping.
func shapeProduct(dims string) uint64 {
	n := uint64(1)

	for f := range strings.FieldsSeq(strings.ReplaceAll(dims, "x", " ")) {
		d, err := strconv.ParseUint(f, 10, 64)
		if err != nil {
			return math.MaxUint64
		}

		n = saturatingMul(n, d)
	}

	return n
}

// elementSize is the bytes per value the decoder's description of a dataset
// declares, and whether the description could be read at all.
func elementSize(info string) (uint64, bool) {
	m := valueSize.FindStringSubmatch(info)
	if m == nil {
		return 0, false
	}

	size, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil {
		return 0, false
	}

	return size, true
}

// saturatingMul is a·b, or the largest uint64 where that overflows.
func saturatingMul(a, b uint64) uint64 {
	if a != 0 && b > math.MaxUint64/a {
		return math.MaxUint64
	}

	return a * b
}

// Close closes the underlying file and removes Open's scratch temp
// file. Safe to call once; a File must not be used afterward.
func (cf *File) Close() error {
	cf.mu.Lock()
	defer cf.mu.Unlock()

	err := release(cf.key, cf.hf.Close)

	if cf.scratchPath != "" {
		if rmErr := os.Remove(cf.scratchPath); rmErr != nil && err == nil {
			err = rmErr
		}

		cf.scratchPath = ""
	}

	if err != nil {
		return fmt.Errorf("cams: close %s: %w", cf.key, err)
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

	var v *Var

	if err := decode(cf.key, "variable "+name, func() (err error) {
		v, err = cf.newVar(name, ds)

		return err
	}); err != nil {
		return nil, err
	}

	return v, nil
}

// checkExtent refuses a read of n values from ds that the file could not
// hold, before the decoder allocates a buffer for it.
//
// The decoder sizes its buffer from what the file declares: a dataset's
// dataspace for Read, the chunk shape for a chunked one, and in every read the
// datatype's size per value. It does not check all of that against the file:
// one corrupted byte made it ask for 8.8 TB, which ended the process with a
// fatal error no recover could catch (#794), and v0.14.2's own bounds leave out
// the contiguous slice ReadPlane reads, where a corrupted element size asked
// for 40.9 GB to hold 12 values. Contiguous or compact data is stored in the
// file, so its values cannot take more bytes than the file has, and chunked
// data, which is compressed, cannot take more than deflateMaxRatio times that.
// A real CAMS file is far inside both: a 0.4° global plane is 406,000 values,
// 3.2 MB even at 8 bytes each, in a file of tens of megabytes.
//
// whole, passed as n, asks about the entire dataset, as Read reads it, whose
// size is taken from its declared dataspace.
//
// # Why this reads Dataset.Info's string
//
// This package does not take shape from that string: it is the decoder's own
// format, not a convention, and shape comes from NetCDF-4 metadata (see the
// package doc). Here it is the only place the declared dataspace, chunk shape
// and element size can be seen before a read sizes a buffer from them, so it
// is read as a bound, and the bound fails closed: a description it cannot read
// refuses the read rather than skip the check, and
// TestCheckExtentReadsTheDecodersDescription pins the format against the
// version in go.mod.
func (cf *File) checkExtent(ds *hdf5.Dataset, n uint64) error {
	info, err := ds.Info()
	if err != nil {
		return fmt.Errorf("%w: %s: %s: %w", ErrMalformed, cf.key, ds.Name(), err)
	}

	limit := cf.size

	if chunked := chunkShape.FindStringSubmatch(info); chunked != nil {
		limit = saturatingMul(limit, deflateMaxRatio)

		if chunk := shapeProduct(chunked[1]); chunk > limit {
			return fmt.Errorf("%w: %s: %s declares chunks of %d values, more than a %d-byte file could hold",
				ErrMalformed, cf.key, ds.Name(), chunk, cf.size)
		}
	}

	if n == whole {
		declared, ok := declaredValues(info)
		if !ok {
			return fmt.Errorf("%w: %s: %s: cannot read its declared shape from %q", ErrMalformed, cf.key, ds.Name(), info)
		}

		n = declared
	}

	size, ok := elementSize(info)
	if !ok {
		return fmt.Errorf("%w: %s: %s: cannot read its element size from %q", ErrMalformed, cf.key, ds.Name(), info)
	}

	if saturatingMul(n, size) > limit {
		return fmt.Errorf("%w: %s: %s declares %d values of %d bytes, more than a %d-byte file could hold",
			ErrMalformed, cf.key, ds.Name(), n, size, cf.size)
	}

	return nil
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

		isDim, err := isDimensionScale(ds)
		if err != nil {
			walkErr = fmt.Errorf("cams: %s: %s: %w", cf.key, ds.Name(), err)
			return
		}

		if !isDim {
			cf.vars[ds.Name()] = ds
			return
		}

		if err := cf.checkExtent(ds, whole); err != nil {
			walkErr = err
			return
		}

		vals, err := ds.Read()
		if err != nil {
			walkErr = fmt.Errorf("cams: %s: read dimension-scale %s: %w", cf.key, ds.Name(), err)
			return
		}

		id, hasID, err := readInt32Attribute(ds, "_Netcdf4Dimid")
		if err != nil {
			walkErr = fmt.Errorf("cams: %s: %s: %w", cf.key, ds.Name(), err)
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
		return nil, fmt.Errorf("cams: %s: %s: %w", cf.key, name, err)
	}

	axisNames := make([]string, len(coordIDs))

	for i, id := range coordIDs {
		axisName, ok := cf.dimNameByID[id]
		if !ok {
			return nil, fmt.Errorf("cams: %s: %s: %w: dimension id %d has no matching dimension-scale dataset",
				cf.key, name, ErrUnsupportedAxis, id)
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
// absent CLASS attribute is the common case for a data variable and means
// false, with a nil error.
//
// A header that cannot be read is an error, not a false. Answering false
// there files the dataset as a data variable, so an axis silently vanishes
// from cf.dims and the file indexes with a shape it does not have — the
// failure then surfaces much later as a confusing "dimension not found" on a
// variable whose dimensions are in fact present. See attributePresent for why
// the underlying library's error cannot be classified after the fact.
func isDimensionScale(ds *hdf5.Dataset) (bool, error) {
	present, err := attributePresent(ds, "CLASS")
	if err != nil {
		return false, err
	}

	if !present {
		return false, nil
	}

	v, err := readAttribute(ds, "CLASS")
	if err != nil {
		return false, fmt.Errorf("cams: read attribute %q: %w", "CLASS", err)
	}

	s, ok := v.(string)

	return ok && s == "DIMENSION_SCALE", nil
}

// attributePresent reports whether ds carries an attribute called name.
//
// # Why this exists rather than reading the error from ReadAttribute
//
// ReadAttribute returns an error from three different places, and only one of
// them means the attribute is absent:
//
//	attrs, err := d.Attributes()          // 1. a real failure: corrupt header, I/O
//	if err != nil { return nil, err }
//	for _, attr := range attrs {
//	    if attr.Name == name {
//	        return attr.ReadValue()       // 2. a real failure: present, undecodable
//	    }
//	}
//	return nil, fmt.Errorf("attribute %q not found", name)   // 3. genuine absence
//
// All three arrive as a dynamic fmt.Errorf with no sentinel, so errors.Is
// cannot separate them and matching the message would tie astrogo to another
// library's wording. Treating every one as absence — which the readers below
// used to do — means a corrupt file reports every attribute as missing and the
// reader carries on with defaults, looking exactly like a file that legitimately
// omits optional ones.
//
// Asking which attributes exist first costs a second header read on a
// per-dataset metadata path, and separates the cases exactly.
func attributePresent(ds *hdf5.Dataset, name string) (bool, error) {
	attrs, err := ds.Attributes()
	if err != nil {
		return false, fmt.Errorf("cams: read attributes of %q: %w", name, err)
	}

	for _, a := range attrs {
		if a.Name == name {
			return true, nil
		}
	}

	return false, nil
}

// readInt32Attribute reads name as an int32, tolerating the small set of
// numeric Go types ReadAttribute may return it as. ok is false (with a
// nil error) when the attribute is simply absent; a non-nil error means
// the attribute exists but decoded to something this reader cannot
// interpret as an integer.
func readInt32Attribute(ds *hdf5.Dataset, name string) (value int32, ok bool, err error) {
	present, err := attributePresent(ds, name)
	if err != nil {
		return 0, false, err
	}

	if !present {
		return 0, false, nil
	}

	v, err := readAttribute(ds, name)
	if err != nil {
		return 0, false, fmt.Errorf("cams: read attribute %q: %w", name, err)
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
	present, err := attributePresent(ds, name)
	if err != nil {
		return 0, false, err
	}

	if !present {
		return 0, false, nil
	}

	v, err := readAttribute(ds, name)
	if err != nil {
		return 0, false, fmt.Errorf("cams: read attribute %q: %w", name, err)
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
	present, err := attributePresent(ds, name)
	if err != nil {
		return "", err
	}

	if !present {
		return "", nil
	}

	v, err := readAttribute(ds, name)
	if err != nil {
		return "", fmt.Errorf("cams: read attribute %q: %w", name, err)
	}

	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: attribute %q is %T, want a string", ErrUnexpectedAttributeType, name, v)
	}

	return s, nil
}

// readAttribute reads ds's attribute name, after checking that the values it
// declares are in the bytes it carries (#794).
//
// An attribute's value is stored inline, in its own header message, so its
// declared count times its element size can be no more than the bytes that
// message holds. The decoder checks that for numbers (CVE-2025-6269) but not
// for strings, whose slice it allocates from the declared count before
// looking: one corrupted byte made it ask for 16.8 TB, a fatal error no
// recover can catch. The count is a saturating product of the dimensions
// rather than the decoder's own, which wraps.
func readAttribute(ds *hdf5.Dataset, name string) (any, error) {
	attrs, err := ds.Attributes()
	if err != nil {
		return nil, fmt.Errorf("read attributes: %w", err)
	}

	for _, a := range attrs {
		if a.Name != name {
			continue
		}

		if a.Dataspace == nil || a.Datatype == nil {
			return nil, fmt.Errorf("%w: attribute %q has no dataspace or datatype", ErrMalformed, name)
		}

		n := a.Dataspace.TotalElements() // scalar 1, null 0
		if len(a.Dataspace.Dimensions) > 0 {
			n = 1
			for _, d := range a.Dataspace.Dimensions {
				n = saturatingMul(n, d)
			}
		}

		if need := saturatingMul(n, uint64(max(a.Datatype.Size, 1))); need > uint64(len(a.Data)) {
			return nil, fmt.Errorf("%w: attribute %q declares %d values, %d bytes, in a %d-byte message",
				ErrMalformed, name, n, need, len(a.Data))
		}

		v, err := a.ReadValue()
		if err != nil {
			return nil, fmt.Errorf("decode attribute %q: %w", name, err)
		}

		return v, nil
	}

	return nil, fmt.Errorf("%w: attribute %q", errAttributeAbsent, name)
}

// errAttributeAbsent is readAttribute's error for an attribute ds does not
// carry. The callers that allow absence ask attributePresent first.
var errAttributeAbsent = errors.New("no such attribute")

// readInt32SliceAttribute reads name as a []int32.
func readInt32SliceAttribute(ds *hdf5.Dataset, name string) ([]int32, error) {
	v, err := readAttribute(ds, name)
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

	values := uint64(1)
	for _, c := range count {
		values = saturatingMul(values, c)
	}

	var raw any

	v.file.mu.Lock()

	err := v.file.checkExtent(v.ds, values)
	if err == nil {
		err = decode(v.file.key, fmt.Sprintf("%s.ReadPlane(time=%d,level=%d)", v.name, timeIdx, level), func() (err error) {
			raw, err = v.ds.ReadSlice(start, count)

			return err //nolint:wrapcheck // decode wraps it with the key and the operation
		})
	}
	v.file.mu.Unlock()

	if err != nil {
		return nil, err
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
