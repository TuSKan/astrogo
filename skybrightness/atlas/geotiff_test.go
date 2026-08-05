package atlas

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"math"
	"sort"
	"strconv"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// synthTIFF describes a minimal classic little-endian Float32 GeoTIFF to build
// in-memory for the reader tests. The geotransform is north-up, with the
// top-left corner of pixel (0,0) at (originLon, originLat) and square pixels of
// pxSize degrees.
type synthTIFF struct {
	width, height int
	pixels        []float32 // row-major, length width*height (mcd/m²)
	rowsPerStrip  int       // 0 ⇒ single strip
	tiled         bool
	tileW, tileH  int
	deflate       bool
	lzw           bool // mutually exclusive with deflate
	// floatPredictor emits TIFF predictor 3, applying the same byte-plane
	// shuffle + differencing a real writer would before compressing.
	floatPredictor bool
	noData         *float64
	originLon      float64
	originLat      float64
	pxSize         float64
}

// packBlock applies the configured predictor and compression to one strip or
// tile, in the order a real TIFF writer does: predict, then compress.
// rowSamples is the block's row width in samples, which the float predictor
// needs since it shuffles byte planes per row.
func packBlock(t *testing.T, raw []byte, s synthTIFF, rowSamples int) []byte {
	t.Helper()

	if s.floatPredictor {
		raw = applyFloatPredictor(raw, rowSamples)
	}

	var buf bytes.Buffer

	switch {
	case s.deflate:
		zw := zlib.NewWriter(&buf)
		if _, err := zw.Write(raw); err != nil {
			t.Fatalf("zlib write: %v", err)
		}

		if err := zw.Close(); err != nil {
			t.Fatalf("zlib close: %v", err)
		}
	case s.lzw:
		return tiffLZWEncode(raw)
	default:
		return raw
	}

	return buf.Bytes()
}

// tiffLZWEncode is a straightforward TIFF 6.0 §13 LZW encoder, written from the
// spec's own algorithm rather than wrapping compress/lzw.
//
// That distinction is the whole point of the fixture. compress/lzw's MSB mode
// looks like TIFF's variant but does not implement the early code-width change,
// so a round trip through Go's writer AND Go's reader agrees with itself while
// disagreeing with every real TIFF file — which is exactly the false green this
// test existed to give before the real VIIRS composite disproved it. Encoding
// here from the spec means the round trip pins the decoder against the format,
// not against a sibling implementation.
func tiffLZWEncode(raw []byte) []byte {
	var (
		out    []byte
		bitBuf uint32
		bitCnt uint
	)

	width := uint(lzwMinWidth)

	put := func(code int32) {
		bitBuf = bitBuf<<width | uint32(code)
		bitCnt += width

		for bitCnt >= 8 {
			bitCnt -= 8
			out = append(out, byte(bitBuf>>bitCnt))
		}
	}

	table := map[string]int32{}
	next := int32(lzwFirstCode)

	reset := func() {
		clear(table)

		next, width = lzwFirstCode, lzwMinWidth
	}

	put(lzwClearCode)
	reset()

	var omega []byte

	for _, b := range raw {
		k := append(append([]byte(nil), omega...), b)

		// A single byte is always representable (it is its own literal
		// code), so only longer strings need a table lookup.
		if _, ok := table[string(k)]; ok || len(k) == 1 {
			omega = k

			continue
		}

		put(codeFor(table, omega))

		if next < lzwMaxCode {
			table[string(k)] = next
			next++
		} else {
			put(lzwClearCode)
			reset()
		}

		// Early change, mirroring the decoder: the encoder must widen one
		// code before the value would actually need the extra bit.
		if next >= 1<<width-1 && width < lzwMaxWidth {
			width++
		}

		omega = []byte{b}
	}

	if len(omega) > 0 {
		put(codeFor(table, omega))
	}

	put(lzwEOICode)

	if bitCnt > 0 { // flush the final partial byte
		out = append(out, byte(bitBuf<<(8-bitCnt)))
	}

	return out
}

// codeFor resolves a string to its code: single bytes are their own literal
// code, everything else must already be in the table.
func codeFor(table map[string]int32, s []byte) int32 {
	if len(s) == 1 {
		return int32(s[0])
	}

	return table[string(s)]
}

// applyFloatPredictor is the forward TIFF predictor-3 transform — the exact
// inverse of geoTIFF.undoPredictor, so a round trip through both is what
// actually pins the byte-plane order and differencing stride.
func applyFloatPredictor(raw []byte, rowSamples int) []byte {
	const step = 4 // this fixture is always 32-bit float

	rowBytes := rowSamples * step
	out := make([]byte, len(raw))

	for off := 0; off+rowBytes <= len(raw); off += rowBytes {
		row := raw[off : off+rowBytes]
		shuffled := make([]byte, rowBytes)

		for i := range rowSamples {
			for b := range step {
				shuffled[b*rowSamples+i] = row[i*step+b]
			}
		}

		for i := len(shuffled) - 1; i > 0; i-- {
			shuffled[i] -= shuffled[i-1]
		}

		copy(out[off:off+rowBytes], shuffled)
	}

	return out
}

// blocks encodes the pixel data into strip or tile blocks (little-endian
// Float32), tiles zero-padded at the image edge.
func (s synthTIFF) blocks(t *testing.T) [][]byte {
	t.Helper()

	le := binary.LittleEndian

	put := func(buf []byte, idx int, v float32) {
		le.PutUint32(buf[idx*4:], math.Float32bits(v))
	}

	var out [][]byte

	if s.tiled {
		across := (s.width + s.tileW - 1) / s.tileW
		down := (s.height + s.tileH - 1) / s.tileH

		for tr := range down {
			for tc := range across {
				buf := make([]byte, s.tileW*s.tileH*4)

				for j := range s.tileH {
					for i := range s.tileW {
						col, row := tc*s.tileW+i, tr*s.tileH+j

						var v float32
						if col < s.width && row < s.height {
							v = s.pixels[row*s.width+col]
						}

						put(buf, j*s.tileW+i, v)
					}
				}

				out = append(out, packBlock(t, buf, s, s.tileW))
			}
		}

		return out
	}

	rps := s.rowsPerStrip
	if rps == 0 {
		rps = s.height
	}

	for r0 := 0; r0 < s.height; r0 += rps {
		rEnd := min(r0+rps, s.height)
		buf := make([]byte, (rEnd-r0)*s.width*4)
		idx := 0

		for row := r0; row < rEnd; row++ {
			for col := range s.width {
				put(buf, idx, s.pixels[row*s.width+col])
				idx++
			}
		}

		out = append(out, packBlock(t, buf, s, s.width))
	}

	return out
}

type tentry struct {
	tag, typ uint16
	count    uint32
	data     []byte // full value-array bytes
	extOff   int    // assigned external offset (0 ⇒ inline)
}

// build assembles the synthetic GeoTIFF byte stream.
func (s synthTIFF) build(t *testing.T) []byte {
	t.Helper()

	le := binary.LittleEndian

	u16 := func(v uint16) []byte { b := make([]byte, 2); le.PutUint16(b, v); return b }
	u32 := func(v uint32) []byte { b := make([]byte, 4); le.PutUint32(b, v); return b }
	u32s := func(vs []uint32) []byte {
		b := make([]byte, 4*len(vs))
		for i, v := range vs {
			le.PutUint32(b[i*4:], v)
		}

		return b
	}
	f64s := func(vs []float64) []byte {
		b := make([]byte, 8*len(vs))
		for i, v := range vs {
			le.PutUint64(b[i*8:], math.Float64bits(v))
		}

		return b
	}

	blocks := s.blocks(t)
	n := len(blocks)

	compression := uint16(1)

	switch {
	case s.deflate:
		compression = 8
	case s.lzw:
		compression = 5
	}

	predictor := uint16(1)
	if s.floatPredictor {
		predictor = 3
	}

	entries := []tentry{
		{tag: tagImageWidth, typ: typeLong, count: 1, data: u32(uint32(s.width))},
		{tag: tagImageLength, typ: typeLong, count: 1, data: u32(uint32(s.height))},
		{tag: tagBitsPerSample, typ: typeShort, count: 1, data: u16(32)},
		{tag: tagCompression, typ: typeShort, count: 1, data: u16(compression)},
		{tag: tagPredictor, typ: typeShort, count: 1, data: u16(predictor)},
		{tag: tagSamplesPerPixel, typ: typeShort, count: 1, data: u16(1)},
		{tag: tagSampleFormat, typ: typeShort, count: 1, data: u16(3)},
		{tag: tagModelPixelScale, typ: typeDouble, count: 3, data: f64s([]float64{s.pxSize, s.pxSize, 0})},
		{tag: tagModelTiepoint, typ: typeDouble, count: 6, data: f64s([]float64{0, 0, 0, s.originLon, s.originLat, 0})},
	}

	byteCounts := make([]uint32, n)
	for i, b := range blocks {
		byteCounts[i] = uint32(len(b))
	}

	var offsetsEntry *tentry

	if s.tiled {
		entries = append(entries,
			tentry{tag: tagTileWidth, typ: typeLong, count: 1, data: u32(uint32(s.tileW))},
			tentry{tag: tagTileLength, typ: typeLong, count: 1, data: u32(uint32(s.tileH))},
			tentry{tag: tagTileOffsets, typ: typeLong, count: uint32(n), data: make([]byte, 4*n)},
			tentry{tag: tagTileByteCounts, typ: typeLong, count: uint32(n), data: u32s(byteCounts)},
		)
	} else {
		rps := s.rowsPerStrip
		if rps == 0 {
			rps = s.height
		}

		entries = append(entries,
			tentry{tag: tagRowsPerStrip, typ: typeLong, count: 1, data: u32(uint32(rps))},
			tentry{tag: tagStripOffsets, typ: typeLong, count: uint32(n), data: make([]byte, 4*n)},
			tentry{tag: tagStripByteCounts, typ: typeLong, count: uint32(n), data: u32s(byteCounts)},
		)
	}

	if s.noData != nil {
		asc := append([]byte(formatFloat(*s.noData)), 0)
		entries = append(entries, tentry{tag: tagGDALNoData, typ: typeASCII, count: uint32(len(asc)), data: asc})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].tag < entries[j].tag })

	for i := range entries {
		if entries[i].tag == tagStripOffsets || entries[i].tag == tagTileOffsets {
			offsetsEntry = &entries[i]
		}
	}

	// Layout: header(8) | IFD | external entry data | pixel blocks.
	ifdSize := 2 + 12*len(entries) + 4
	cursor := 8 + ifdSize

	for i := range entries {
		if len(entries[i].data) > 4 {
			entries[i].extOff = cursor
			cursor += len(entries[i].data)

			if cursor%2 == 1 {
				cursor++
			}
		}
	}

	blockOffsets := make([]uint32, n)

	for i := range blocks {
		if cursor%2 == 1 {
			cursor++
		}

		blockOffsets[i] = uint32(cursor)
		cursor += len(blocks[i])
	}

	copy(offsetsEntry.data, u32s(blockOffsets)) // patch now that block offsets are known

	buf := make([]byte, cursor)
	copy(buf[0:2], "II")
	copy(buf[2:4], u16(42))
	copy(buf[4:8], u32(8))

	copy(buf[8:10], u16(uint16(len(entries))))

	for i, e := range entries {
		p := 10 + i*12
		copy(buf[p:p+2], u16(e.tag))
		copy(buf[p+2:p+4], u16(e.typ))
		copy(buf[p+4:p+8], u32(e.count))

		if len(e.data) > 4 {
			copy(buf[p+8:p+12], u32(uint32(e.extOff)))
			copy(buf[e.extOff:e.extOff+len(e.data)], e.data)
		} else {
			copy(buf[p+8:p+8+len(e.data)], e.data)
		}
	}

	for i, b := range blocks {
		copy(buf[blockOffsets[i]:int(blockOffsets[i])+len(b)], b)
	}

	return buf
}

// formatFloat renders a no-data value the way GDAL writes the GDAL_NODATA tag.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// rampPixels builds a width×height ramp where pixel (col,row) = base + col + row*10.
func rampPixels(width, height int, base float32) []float32 {
	px := make([]float32, width*height)
	for r := range height {
		for c := range width {
			px[r*width+c] = base + float32(c) + float32(r)*10
		}
	}

	return px
}

// centerLonLat returns the lon/lat of the centre of pixel (col,row) for a synth.
func (s synthTIFF) centerLonLat(col, row int) (lon, lat float64) {
	return s.originLon + (float64(col)+0.5)*s.pxSize, s.originLat - (float64(row)+0.5)*s.pxSize
}

// readPixelCenters asserts every pixel reads back at its centre for the given
// synth configuration, exercising the strip/tile indexing and decoding.
func readPixelCenters(t *testing.T, s synthTIFF) {
	t.Helper()

	gt, err := openGeoTIFF(bytes.NewReader(s.build(t)), nil)
	if err != nil {
		t.Fatalf("openGeoTIFF: %v", err)
	}

	for row := range s.height {
		for col := range s.width {
			lon, lat := s.centerLonLat(col, row)

			got, err := gt.sampleBilinear(lon, lat)
			if err != nil {
				t.Fatalf("sampleBilinear(%d,%d): %v", col, row, err)
			}

			want := float64(s.pixels[row*s.width+col])
			testutil.AssertNear(t, "pixel", got, want, 1e-4)
		}
	}
}

func TestGeoTIFFStripUncompressed(t *testing.T) {
	t.Parallel()
	readPixelCenters(t, synthTIFF{
		width: 4, height: 3, pixels: rampPixels(4, 3, 5),
		originLon: -10, originLat: 40, pxSize: 0.5,
	})
}

func TestGeoTIFFMultiStrip(t *testing.T) {
	t.Parallel()
	readPixelCenters(t, synthTIFF{
		width: 4, height: 4, pixels: rampPixels(4, 4, 5), rowsPerStrip: 2,
		originLon: 100, originLat: -20, pxSize: 1.0,
	})
}

func TestGeoTIFFTiled(t *testing.T) {
	t.Parallel()
	readPixelCenters(t, synthTIFF{
		width: 4, height: 4, pixels: rampPixels(4, 4, 5), tiled: true, tileW: 2, tileH: 2,
		originLon: 0, originLat: 0, pxSize: 2.0,
	})
}

func TestGeoTIFFDeflate(t *testing.T) {
	t.Parallel()
	readPixelCenters(t, synthTIFF{
		width: 4, height: 3, pixels: rampPixels(4, 3, 5), deflate: true,
		originLon: -46, originLat: -23, pxSize: 0.25,
	})
}

// TestGeoTIFFLZW covers the real VIIRS annual composites, which
// lightpollutionmap.info publishes as LZW-compressed one-row strips
// (compression 5, predictor 1) — the layout that made every VIIRS layer fail
// validation after a ~1 GB download.
//
// The round trip is the point: compress/lzw's MSB order has to agree with
// TIFF's LZW variant on when the code width increases. If it didn't, the code
// table would desync partway through a strip and the pixels would come back
// wrong rather than erroring, so a decode-only test against hand-written bytes
// would not catch it.
func TestGeoTIFFLZW(t *testing.T) {
	t.Parallel()
	readPixelCenters(t, synthTIFF{
		width: 4, height: 3, pixels: rampPixels(4, 3, 5), lzw: true,
		originLon: -46, originLat: -23, pxSize: 0.25,
	})
}

// TestGeoTIFFLZWMultiStrip exercises LZW across strip boundaries with one row
// per strip — exactly how the real VIIRS files are written (33 600 strips of
// 86 400 samples each), where a per-strip decoder-state leak would show up.
func TestGeoTIFFLZWMultiStrip(t *testing.T) {
	t.Parallel()
	readPixelCenters(t, synthTIFF{
		width: 5, height: 4, pixels: rampPixels(5, 4, 3), lzw: true, rowsPerStrip: 1,
		originLon: 10, originLat: 50, pxSize: 0.1,
	})
}

// TestGeoTIFFFloatPredictor pins the predictor-3 undo pass against the forward
// transform a real writer applies. No VIIRS year uses it today (they all ship
// predictor 1), but it is the predictor float rasters actually get written
// with, so decoding it wrong later would be the same silent-wrong-value trap.
func TestGeoTIFFFloatPredictor(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		fix  synthTIFF
	}{
		{"lzw", synthTIFF{
			width: 6, height: 4, pixels: rampPixels(6, 4, 7), lzw: true, floatPredictor: true,
			rowsPerStrip: 2, originLon: -3, originLat: 40, pxSize: 0.05,
		}},
		{"deflate", synthTIFF{
			width: 6, height: 4, pixels: rampPixels(6, 4, 7), deflate: true, floatPredictor: true,
			originLon: -3, originLat: 40, pxSize: 0.05,
		}},
		{"tiled", synthTIFF{
			width: 6, height: 4, pixels: rampPixels(6, 4, 7), lzw: true, floatPredictor: true,
			tiled: true, tileW: 4, tileH: 2, originLon: -3, originLat: 40, pxSize: 0.05,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			readPixelCenters(t, tc.fix)
		})
	}
}

// TestGeoTIFFBilinearMidpoint checks interpolation halfway between two pixels.
func TestGeoTIFFBilinearMidpoint(t *testing.T) {
	t.Parallel()

	s := synthTIFF{
		width: 4, height: 3, pixels: rampPixels(4, 3, 5),
		originLon: -10, originLat: 40, pxSize: 0.5,
	}

	gt, err := openGeoTIFF(bytes.NewReader(s.build(t)), nil)
	if err != nil {
		t.Fatalf("openGeoTIFF: %v", err)
	}

	// Midpoint between pixel (0,0)=5 and (1,0)=6 ⇒ 5.5.
	lon0, lat0 := s.centerLonLat(0, 0)
	lon1, _ := s.centerLonLat(1, 0)

	got, err := gt.sampleBilinear((lon0+lon1)/2, lat0)
	if err != nil {
		t.Fatalf("sampleBilinear: %v", err)
	}

	testutil.AssertNear(t, "midpoint", got, 5.5, 1e-4)
}

// TestGeoTIFFUnsupported verifies a clear error for an unsupported sample format.
func TestGeoTIFFUnsupported(t *testing.T) {
	t.Parallel()

	raw := synthTIFF{
		width: 2, height: 2, pixels: rampPixels(2, 2, 1),
		originLon: 0, originLat: 0, pxSize: 1,
	}.build(t)

	// Corrupt the SampleFormat value (tag 339) from 3 (float) to 1 (uint).
	le := binary.LittleEndian
	count := int(le.Uint16(raw[8:10]))

	for i := range count {
		p := 10 + i*12
		if le.Uint16(raw[p:p+2]) == tagSampleFormat {
			le.PutUint16(raw[p+8:p+10], 1)
		}
	}

	if _, err := openGeoTIFF(bytes.NewReader(raw), nil); err == nil {
		t.Fatal("expected ErrUnsupportedTIFF for integer sample format, got nil")
	}
}
