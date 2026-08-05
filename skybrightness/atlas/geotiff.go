package atlas

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// ErrUnsupportedTIFF is returned for a TIFF/GeoTIFF feature this minimal reader
// does not handle (e.g. BigTIFF, a multi-band image, or a non-float sample
// format). The wrapped detail says which.
var ErrUnsupportedTIFF = errors.New("atlas: unsupported TIFF feature")

// ErrBadTIFF is returned when the bytes are not a well-formed classic TIFF.
var ErrBadTIFF = errors.New("atlas: malformed TIFF")

// TIFF Compression tag (259) values this reader handles. 32946 is the
// pre-standard deflate code some older writers still emit; it is byte-identical
// to 8.
const (
	compressionNone       = 1
	compressionLZW        = 5
	compressionDeflate    = 8
	compressionOldDeflate = 32946
)

// TIFF Predictor tag (317) values. Horizontal differencing (2) is defined only
// for integer samples, so it cannot appear on a file this float-only reader
// accepts at all; the floating-point predictor (3) is the one that can, and it
// shuffles each row into per-byte planes before differencing rather than
// differencing whole samples.
const (
	predictorNone  = 1
	predictorFloat = 3
)

// TIFF tag numbers used here.
const (
	tagImageWidth          = 256
	tagImageLength         = 257
	tagBitsPerSample       = 258
	tagCompression         = 259
	tagSamplesPerPixel     = 277
	tagRowsPerStrip        = 278
	tagStripOffsets        = 273
	tagStripByteCounts     = 279
	tagPredictor           = 317
	tagTileWidth           = 322
	tagTileLength          = 323
	tagTileOffsets         = 324
	tagTileByteCounts      = 325
	tagSampleFormat        = 339
	tagModelPixelScale     = 33550
	tagModelTiepoint       = 33922
	tagModelTransformation = 34264
	tagGDALNoData          = 42113
)

// TIFF field types.
const (
	typeByte     = 1
	typeASCII    = 2
	typeShort    = 3
	typeLong     = 4
	typeRational = 5
	typeDouble   = 12
)

// fieldTypeSize is the byte size of each TIFF field type used here.
var fieldTypeSize = map[uint16]int{
	typeByte: 1, typeASCII: 1, typeShort: 2, typeLong: 4, typeRational: 8, typeDouble: 8,
}

// field is one resolved IFD entry: its type, element count, and the raw bytes of
// the value array (fetched from the inline value field or from an offset).
type field struct {
	typ   uint16
	count uint32
	data  []byte
	bo    binary.ByteOrder
}

// ints decodes the field as unsigned integers (BYTE/SHORT/LONG).
func (f field) ints() []uint64 {
	out := make([]uint64, 0, f.count)

	size := fieldTypeSize[f.typ]
	for i := 0; i < int(f.count) && (i+1)*size <= len(f.data); i++ {
		b := f.data[i*size : (i+1)*size]

		switch f.typ {
		case typeByte:
			out = append(out, uint64(b[0]))
		case typeShort:
			out = append(out, uint64(f.bo.Uint16(b)))
		case typeLong:
			out = append(out, uint64(f.bo.Uint32(b)))
		}
	}

	return out
}

// doubles decodes the field as float64 values (DOUBLE).
func (f field) doubles() []float64 {
	out := make([]float64, 0, f.count)
	for i := 0; i < int(f.count) && (i+1)*8 <= len(f.data); i++ {
		out = append(out, math.Float64frombits(f.bo.Uint64(f.data[i*8:i*8+8])))
	}

	return out
}

// ascii decodes the field as a NUL-trimmed string.
func (f field) ascii() string {
	return strings.TrimRight(string(f.data), "\x00")
}

// clampPos converts a non-negative TIFF integer to int, clamping pathological
// values to MaxInt32 (which then fail the later length/extent checks). The
// explicit bound keeps the conversion provably in range on all platforms.
func clampPos(u uint64) int {
	if u > uint64(math.MaxInt32) {
		return math.MaxInt32
	}

	return int(u)
}

// clampPos64 is [clampPos] for byte offsets, which may exceed 2 GiB.
func clampPos64(u uint64) int64 {
	if u > uint64(math.MaxInt64) {
		return math.MaxInt64
	}

	return int64(u)
}

// geoTIFF is a windowed reader over a classic single-band float GeoTIFF. It
// reads only the strip/tile covering a requested pixel (with a one-entry cache),
// so a multi-gigabyte atlas is never loaded whole.
type geoTIFF struct {
	r  io.ReaderAt
	bo binary.ByteOrder

	width, height int
	bits          int
	sampleFormat  int
	compression   int
	predictor     int

	tiled                 bool
	rowsPerStrip          int
	tileWidth, tileHeight int
	blockOffsets          []int64
	blockByteCounts       []int64

	gt        GeoTransform
	noData    float64
	hasNoData bool

	cacheIdx int
	cache    []float64
}

// readAt reads exactly n bytes at offset off.
func readAt(r io.ReaderAt, off int64, n int) ([]byte, error) {
	buf := make([]byte, n)
	if _, err := r.ReadAt(buf, off); err != nil {
		return nil, fmt.Errorf("atlas: read %d bytes at %d: %w", n, off, err)
	}

	return buf, nil
}

// openGeoTIFF parses the TIFF header and first IFD. The optional override sets
// the geotransform when the file carries no model tags.
func openGeoTIFF(r io.ReaderAt, override *GeoTransform) (*geoTIFF, error) {
	head, err := readAt(r, 0, 8)
	if err != nil {
		return nil, err
	}

	t := &geoTIFF{r: r, cacheIdx: -1}

	switch {
	case head[0] == 'I' && head[1] == 'I':
		t.bo = binary.LittleEndian
	case head[0] == 'M' && head[1] == 'M':
		t.bo = binary.BigEndian
	default:
		return nil, fmt.Errorf("%w: bad byte-order mark", ErrBadTIFF)
	}

	if t.bo.Uint16(head[2:4]) != 42 {
		return nil, fmt.Errorf("%w: not classic TIFF (BigTIFF unsupported)", ErrBadTIFF)
	}

	fields, err := t.readIFD(int64(t.bo.Uint32(head[4:8])))
	if err != nil {
		return nil, err
	}

	if err := t.configure(fields, override); err != nil {
		return nil, err
	}

	return t, nil
}

// ReadGrid materializes the entire raster into an in-memory [Grid]. For very
// large atlases prefer the windowed [geoTIFF.sampleBilinear] path; this is
// intended for clipped/regional files and tests.
func (t *geoTIFF) ReadGrid() (*Grid, error) {
	data := make([]float64, t.width*t.height)

	for row := range t.height {
		for col := range t.width {
			v, err := t.pixel(col, row)
			if err != nil {
				return nil, err
			}

			data[row*t.width+col] = v
		}
	}

	return &Grid{
		Width:     t.width,
		Height:    t.height,
		Data:      data,
		NoData:    t.noData,
		HasNoData: t.hasNoData,
		GT:        t.gt,
	}, nil
}

// readIFD reads the IFD at off and resolves every entry's value bytes.
func (t *geoTIFF) readIFD(off int64) (map[uint16]field, error) {
	cntBuf, err := readAt(t.r, off, 2)
	if err != nil {
		return nil, err
	}

	count := int(t.bo.Uint16(cntBuf))

	entries, err := readAt(t.r, off+2, count*12)
	if err != nil {
		return nil, err
	}

	fields := make(map[uint16]field, count)

	for i := range count {
		e := entries[i*12 : i*12+12]
		tag := t.bo.Uint16(e[0:2])
		typ := t.bo.Uint16(e[2:4])
		cnt := t.bo.Uint32(e[4:8])

		size, ok := fieldTypeSize[typ]
		if !ok {
			continue // unknown type: skip
		}

		total := int(cnt) * size

		var data []byte
		if total <= 4 {
			data = e[8 : 8+total]
		} else {
			data, err = readAt(t.r, int64(t.bo.Uint32(e[8:12])), total)
			if err != nil {
				return nil, err
			}
		}

		fields[tag] = field{typ: typ, count: cnt, data: data, bo: t.bo}
	}

	return fields, nil
}

// configure validates the supported feature set and populates the reader.
func (t *geoTIFF) configure(f map[uint16]field, override *GeoTransform) error {
	first := func(tag uint16, def uint64) uint64 {
		if fl, ok := f[tag]; ok {
			if v := fl.ints(); len(v) > 0 {
				return v[0]
			}
		}

		return def
	}

	t.width = clampPos(first(tagImageWidth, 0))
	t.height = clampPos(first(tagImageLength, 0))

	if t.width <= 0 || t.height <= 0 {
		return fmt.Errorf("%w: missing image dimensions", ErrBadTIFF)
	}

	if spp := first(tagSamplesPerPixel, 1); spp != 1 {
		return fmt.Errorf("%w: %d samples/pixel (only single-band supported)", ErrUnsupportedTIFF, spp)
	}

	t.bits = clampPos(first(tagBitsPerSample, 0))
	t.sampleFormat = clampPos(first(tagSampleFormat, 1))

	if t.sampleFormat != 3 || (t.bits != 32 && t.bits != 64) {
		return fmt.Errorf("%w: sample format %d / %d bits (only 32/64-bit float)", ErrUnsupportedTIFF, t.sampleFormat, t.bits)
	}

	t.compression = clampPos(first(tagCompression, 1))
	if t.compression != compressionNone && t.compression != compressionLZW &&
		t.compression != compressionDeflate && t.compression != compressionOldDeflate {
		return fmt.Errorf("%w: compression %d (only none/LZW/deflate)", ErrUnsupportedTIFF, t.compression)
	}

	t.predictor = clampPos(first(tagPredictor, predictorNone))
	if t.predictor != predictorNone && t.predictor != predictorFloat {
		return fmt.Errorf("%w: predictor %d (only none/floating-point)", ErrUnsupportedTIFF, t.predictor)
	}

	if err := t.configureLayout(f); err != nil {
		return err
	}

	if err := t.configureGeo(f, override); err != nil {
		return err
	}

	if nd, ok := f[tagGDALNoData]; ok {
		if v, err := strconv.ParseFloat(strings.TrimSpace(nd.ascii()), 64); err == nil {
			t.noData, t.hasNoData = v, true
		}
	}

	return nil
}

// configureLayout resolves the strip or tile block layout.
func (t *geoTIFF) configureLayout(f map[uint16]field) error {
	toInt64 := func(u []uint64) []int64 {
		out := make([]int64, len(u))
		for i, v := range u {
			out[i] = clampPos64(v)
		}

		return out
	}

	if tw, ok := f[tagTileWidth]; ok {
		t.tiled = true
		t.tileWidth = clampPos(tw.ints()[0])
		t.tileHeight = clampPos(f[tagTileLength].ints()[0])
		t.blockOffsets = toInt64(f[tagTileOffsets].ints())
		t.blockByteCounts = toInt64(f[tagTileByteCounts].ints())

		if t.tileWidth <= 0 || t.tileHeight <= 0 || len(t.blockOffsets) == 0 {
			return fmt.Errorf("%w: bad tile layout", ErrBadTIFF)
		}

		return nil
	}

	so, ok := f[tagStripOffsets]
	if !ok {
		return fmt.Errorf("%w: neither strip nor tile offsets present", ErrBadTIFF)
	}

	t.rowsPerStrip = t.height

	if fl, ok := f[tagRowsPerStrip]; ok {
		if v := fl.ints(); len(v) > 0 && v[0] > 0 {
			t.rowsPerStrip = clampPos(v[0])
		}
	}

	t.blockOffsets = toInt64(so.ints())
	t.blockByteCounts = toInt64(f[tagStripByteCounts].ints())

	if len(t.blockOffsets) == 0 || len(t.blockOffsets) != len(t.blockByteCounts) {
		return fmt.Errorf("%w: bad strip layout", ErrBadTIFF)
	}

	return nil
}

// configureGeo resolves the affine geotransform from the GeoTIFF model tags or
// the caller-supplied override.
func (t *geoTIFF) configureGeo(f map[uint16]field, override *GeoTransform) error {
	if mt, ok := f[tagModelTransformation]; ok {
		m := mt.doubles()
		if len(m) >= 16 {
			// 4×4 row-major: x = m0·col + m1·row + m3; y = m4·col + m5·row + m7.
			t.gt = GeoTransform{A: m[3], B: m[0], C: m[1], D: m[7], E: m[4], F: m[5]}

			return nil
		}
	}

	scale, sOK := f[tagModelPixelScale]
	tie, tOK := f[tagModelTiepoint]

	if sOK && tOK {
		s := scale.doubles()
		p := tie.doubles()

		if len(s) >= 2 && len(p) >= 6 {
			// Tiepoint maps raster (i,j) → (x,y); pixel scale gives (sx,sy).
			// lon = x − i·sx + col·sx ; lat = y + j·sy − row·sy  (sy applied negative).
			t.gt = GeoTransform{
				A: p[3] - p[0]*s[0], B: s[0], C: 0,
				D: p[4] + p[1]*s[1], E: 0, F: -s[1],
			}

			return nil
		}
	}

	if override != nil {
		t.gt = *override

		return nil
	}

	return fmt.Errorf("%w: no geotransform tags and no override supplied", ErrBadTIFF)
}

// decodeBlock reads and decodes block i into a flat []float64 of its samples,
// caching the most recent block.
func (t *geoTIFF) decodeBlock(i int) ([]float64, error) {
	if i == t.cacheIdx {
		return t.cache, nil
	}

	if i < 0 || i >= len(t.blockOffsets) {
		return nil, fmt.Errorf("%w: block %d out of range", ErrBadTIFF, i)
	}

	raw, err := readAt(t.r, t.blockOffsets[i], int(t.blockByteCounts[i]))
	if err != nil {
		return nil, err
	}

	switch t.compression {
	case compressionDeflate, compressionOldDeflate:
		raw, err = inflate(raw)
	case compressionLZW:
		raw, err = unlzw(raw)
	}

	if err != nil {
		return nil, err
	}

	if t.predictor != predictorNone {
		t.undoPredictor(raw, t.blockRowSamples())
	}

	samples := t.decodeSamples(raw)

	t.cacheIdx, t.cache = i, samples

	return samples, nil
}

// decodeSamples interprets raw bytes as the block's float samples.
func (t *geoTIFF) decodeSamples(raw []byte) []float64 {
	step := t.bits / 8
	n := len(raw) / step
	out := make([]float64, n)

	for i := range n {
		b := raw[i*step : i*step+step]
		if t.bits == 32 {
			out[i] = float64(math.Float32frombits(t.bo.Uint32(b)))
		} else {
			out[i] = math.Float64frombits(t.bo.Uint64(b))
		}
	}

	return out
}

// blockRowSamples is the number of samples in one row of a strip or tile.
// Constant across blocks: a tile is padded out to its full declared width, and
// a short final strip is short in ROWS, never in columns.
func (t *geoTIFF) blockRowSamples() int {
	if t.tiled {
		return t.tileWidth
	}

	return t.width
}

// LZW code-space constants from TIFF 6.0 §13. Codes 0-255 are literals, so the
// dictionary proper starts at 258.
const (
	lzwClearCode = 256
	lzwEOICode   = 257
	lzwFirstCode = 258
	lzwMaxCode   = 4096
	lzwMinWidth  = 9
	lzwMaxWidth  = 12
)

// unlzw decompresses LZW-compressed strip or tile bytes.
//
// This is TIFF's own LZW variant, not the one compress/lzw implements. Both are
// MSB-first over 8-bit literals, but TIFF (like PDF, unlike GIF) uses the
// "early change" convention: the code width grows one code SOONER than the
// textbook algorithm — at 511, 1023, 2047 rather than 512, 1024, 2048. Feeding
// a real TIFF strip to compress/lzw desyncs the code table within the first few
// hundred codes; against the real VIIRS 2025 composite it fails outright with
// "lzw: invalid code", which is how this was caught.
func unlzw(raw []byte) ([]byte, error) {
	var (
		prefix [lzwMaxCode]int32 // -1 for a single-byte root
		suffix [lzwMaxCode]byte  // last byte of the entry
		first  [lzwMaxCode]byte  // first byte, cached so KwKwK is O(1)
		length [lzwMaxCode]int32
	)

	reset := func() {
		for i := range int32(256) {
			prefix[i], suffix[i], first[i], length[i] = -1, byte(i), byte(i), 1
		}
	}

	reset()

	var (
		out    = make([]byte, 0, len(raw)*3)
		next   = int32(lzwFirstCode)
		width  = uint(lzwMinWidth)
		prev   = int32(-1)
		bitBuf uint32
		bitCnt uint
		pos    int
	)

	// readCode pulls the next width-bit big-endian code, reporting false at
	// end of input. A strip whose data ends without an explicit EOI is
	// tolerated — some writers omit it — so this is not itself an error.
	readCode := func() (uint32, bool) {
		for bitCnt < width {
			if pos >= len(raw) {
				return 0, false
			}

			bitBuf = bitBuf<<8 | uint32(raw[pos])
			pos++
			bitCnt += 8
		}

		bitCnt -= width

		return (bitBuf >> bitCnt) & (1<<width - 1), true
	}

	// firstByteOf is first[] behind a bounds check. Every caller passes a
	// code already validated against next, so the guard is unreachable in
	// practice; it is here so the indexing is provably in range rather than
	// in-range-by-argument.
	firstByteOf := func(code int32) byte {
		if code < 0 || code >= lzwMaxCode {
			return 0
		}

		return first[code]
	}

	// emit appends code's full expansion, walking the prefix chain backwards
	// into the space just reserved for it.
	emit := func(code int32) {
		n := int(length[code])
		start := len(out)
		out = append(out, make([]byte, n)...)

		for i := n - 1; i >= 0; i-- {
			out[start+i] = suffix[code]
			code = prefix[code]
		}
	}

	add := func(prevCode int32, firstByte byte) {
		if next >= lzwMaxCode || prevCode < 0 || prevCode >= lzwMaxCode {
			return
		}

		prefix[next], suffix[next], first[next] = prevCode, firstByte, firstByteOf(prevCode)
		length[next] = length[prevCode] + 1
		next++
	}

	for {
		raw, ok := readCode()
		if !ok {
			return out, nil
		}

		// Unreachable while width <= lzwMaxWidth (12 bits caps a code at
		// 4095), but it is what makes the code provably a valid index into
		// the dictionary arrays below rather than one by argument.
		if raw >= lzwMaxCode {
			return nil, fmt.Errorf("%w: lzw code %d outside the 12-bit code space", ErrBadTIFF, raw)
		}

		code := int32(raw)

		switch {
		case code == lzwEOICode:
			return out, nil

		case code == lzwClearCode:
			reset()

			next, width, prev = lzwFirstCode, lzwMinWidth, -1

			continue

		case prev < 0: // first code after a clear is always a literal
			if code >= next {
				return nil, fmt.Errorf("%w: lzw code %d before any dictionary entry", ErrBadTIFF, code)
			}

			emit(code)

		case code < next: // in the dictionary
			emit(code)
			add(prev, firstByteOf(code))

		case code == next: // the KwKwK case: entry defined by this very code
			add(prev, firstByteOf(prev))
			emit(code)

		default:
			return nil, fmt.Errorf("%w: lzw code %d past dictionary end %d", ErrBadTIFF, code, next)
		}

		prev = code

		// Early change: grow one code before the width would actually
		// overflow. This single "-1" is the whole difference from
		// compress/lzw.
		if next >= 1<<width-1 && width < lzwMaxWidth {
			width++
		}
	}
}

// undoPredictor reverses the floating-point predictor (TIFF tag 317 = 3) in
// place, one row at a time.
//
// The encoder splits each row's samples into per-byte planes (all the
// high-order bytes, then all the second bytes, ...) and horizontally
// differences the result, which makes the near-constant exponent bytes of a
// smooth float raster compress far better. Undoing it is therefore two passes:
// accumulate the differences across the whole row, then de-interleave the byte
// planes back into consecutive samples.
//
// The accumulation stride is one BYTE, not one sample — the differencing runs
// over the shuffled byte planes, where adjacent bytes are the same byte
// position of neighbouring samples. (libtiff's fpAcc uses the sample-count
// stride, which is 1 here because this reader only accepts single-band files;
// see the SamplesPerPixel check in configure.)
func (t *geoTIFF) undoPredictor(raw []byte, rowSamples int) {
	step := t.bits / 8
	rowBytes := rowSamples * step

	for off := 0; off+rowBytes <= len(raw); off += rowBytes {
		row := raw[off : off+rowBytes]

		for i := 1; i < len(row); i++ {
			row[i] += row[i-1]
		}

		shuffled := make([]byte, rowBytes)
		copy(shuffled, row)

		for i := range rowSamples {
			for b := range step {
				row[i*step+b] = shuffled[b*rowSamples+i]
			}
		}
	}
}

// inflate decompresses zlib/deflate-compressed strip or tile bytes.
func inflate(raw []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("atlas: zlib: %w", err)
	}
	defer func() { _ = zr.Close() }()

	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("atlas: inflate: %w", err)
	}

	return out, nil
}

// pixel returns the sample value at integer (col,row).
func (t *geoTIFF) pixel(col, row int) (float64, error) {
	var blockIdx, within int

	if t.tiled {
		tilesAcross := (t.width + t.tileWidth - 1) / t.tileWidth
		tc, tr := col/t.tileWidth, row/t.tileHeight
		blockIdx = tr*tilesAcross + tc
		within = (row%t.tileHeight)*t.tileWidth + (col % t.tileWidth)
	} else {
		blockIdx = row / t.rowsPerStrip
		within = (row%t.rowsPerStrip)*t.width + col
	}

	samples, err := t.decodeBlock(blockIdx)
	if err != nil {
		return 0, err
	}

	if within < 0 || within >= len(samples) {
		return 0, fmt.Errorf("%w: sample %d outside block %d (len %d)", ErrBadTIFF, within, blockIdx, len(samples))
	}

	return samples[within], nil
}

// isNoData reports whether v is a no-data sample.
func (t *geoTIFF) isNoData(v float64) bool {
	if math.IsNaN(v) {
		return true
	}

	return t.hasNoData && v == t.noData
}

// sampleBilinear interpolates the artificial brightness (mcd/m²) at a
// longitude/latitude by reading only the strips/tiles covering the four
// neighbouring pixels — the windowed path that never loads the whole atlas.
func (t *geoTIFF) sampleBilinear(lonDeg, latDeg float64) (float64, error) {
	var ioErr error

	at := func(col, row int) (float64, bool) {
		if col < 0 || row < 0 || col >= t.width || row >= t.height {
			return 0, false
		}

		v, err := t.pixel(col, row)
		if err != nil {
			ioErr = err

			return 0, false
		}

		if t.isNoData(v) {
			return 0, false
		}

		return v, true
	}

	v, err := bilinear(t.gt, t.width, t.height, lonDeg, latDeg, at)

	if ioErr != nil {
		return 0, ioErr
	}

	return v, err
}
