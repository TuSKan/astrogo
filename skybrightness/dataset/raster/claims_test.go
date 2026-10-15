package raster

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
)

// A file's declarations are not its data. Each test here is a file that
// declares far more than it holds, built by hand since no writer would emit
// it, and each held the reader to the declaration until #799: memory was
// sized from a tag's count, a strip's byte count, a codec's output and the
// image's dimensions, before anything showed the bytes were there.
//
// None of these is parallel: the allocation they measure is the whole
// process's.

// ifdEntry is one 12-byte IFD entry whose value fits in its 4-byte field.
type ifdEntry struct {
	tag, typ uint16
	count    uint32
	value    uint32
}

// handTIFF is a little-endian classic TIFF: the header, one IFD of entries,
// then tail. It returns the file and the offset at which tail starts.
func handTIFF(entries []ifdEntry, tail []byte) ([]byte, uint32) {
	tailAt := 8 + 2 + 12*len(entries) + 4
	b := make([]byte, tailAt, tailAt+len(tail))

	copy(b, "II")
	binary.LittleEndian.PutUint16(b[2:], 42)
	binary.LittleEndian.PutUint32(b[4:], 8)
	binary.LittleEndian.PutUint16(b[8:], uint16(len(entries)))

	for i, e := range entries {
		o := 10 + 12*i
		binary.LittleEndian.PutUint16(b[o:], e.tag)
		binary.LittleEndian.PutUint16(b[o+2:], e.typ)
		binary.LittleEndian.PutUint32(b[o+4:], e.count)
		binary.LittleEndian.PutUint32(b[o+8:], e.value)
	}

	return append(b, tail...), uint32(tailAt)
}

// stripTIFF is a float64 image of width × height whose first strip holds data,
// right after the IFD, and declares byteCount bytes. extra entries follow the
// rest.
func stripTIFF(width, height, compression, byteCount uint32, data []byte, extra ...ifdEntry) []byte {
	entries := make([]ifdEntry, 0, 8+len(extra))
	entries = append(entries,
		ifdEntry{tagImageWidth, typeLong, 1, width},
		ifdEntry{tagImageLength, typeLong, 1, height},
		ifdEntry{tagBitsPerSample, typeShort, 1, 64},
		ifdEntry{tagCompression, typeShort, 1, compression},
		ifdEntry{tagSamplesPerPixel, typeShort, 1, 1},
		ifdEntry{tagSampleFormat, typeShort, 1, 3},
		ifdEntry{tagStripOffsets, typeLong, 1, 0},
		ifdEntry{tagStripByteCounts, typeLong, 1, byteCount},
	)
	entries = append(entries, extra...)

	_, at := handTIFF(entries, nil)
	entries[6].value = at

	b, _ := handTIFF(entries, data)

	return b
}

// anyGeo is a transform for files with no model tags.
var anyGeo = &GeoTransform{B: 1, F: -1}

// allocatedBy is how many bytes f allocated.
func allocatedBy(f func()) uint64 {
	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)

	return after.TotalAlloc - before.TotalAlloc
}

// claimBudget is what reading any file here may allocate: readAt's first
// 64 KiB chunk, the bytes these files actually hold, and change. Each claim
// below asked for at least 232 MB before #799.
const claimBudget = 1 << 20

// TestOpenReadsATagsValuesAsTheyArrive: a 26-byte file whose one IFD entry
// declares 2³²−1 doubles made Open allocate 34 GB before finding nothing there.
func TestOpenReadsATagsValuesAsTheyArrive(t *testing.T) {
	b, _ := handTIFF([]ifdEntry{{tagImageWidth, typeDouble, 0xFFFFFFFF, 0}}, nil)

	var err error

	if n := allocatedBy(func() { _, err = Open(bytes.NewReader(b), anyGeo) }); n > claimBudget {
		t.Errorf("Open of a %d-byte file allocated %d bytes, want at most %d", len(b), n, claimBudget)
	}

	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Open: err %v, want the read to end at the end of the file", err)
	}
}

// TestBlockReadsWhatItsLayoutHolds: a strip declaring 2³²−1 bytes made a
// one-pixel ReadGrid allocate 4.29 GB. Uncompressed, a strip's bytes are its
// samples, so only the eight the pixel needs are read and the pixel is there;
// compressed, the strip is read as its bytes arrive and fails where they end.
func TestBlockReadsWhatItsLayoutHolds(t *testing.T) {
	pixel := make([]byte, 8)
	binary.LittleEndian.PutUint64(pixel, 0x4045000000000000) // 42.0

	r, err := Open(bytes.NewReader(stripTIFF(1, 1, compressionNone, 0xFFFFFFFF, pixel)), anyGeo)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	var g *Grid

	if n := allocatedBy(func() { g, err = r.ReadGrid() }); n > claimBudget {
		t.Errorf("uncompressed: ReadGrid allocated %d bytes, want at most %d", n, claimBudget)
	}

	if err != nil || len(g.Data) != 1 || g.Data[0] != 42 {
		t.Errorf("uncompressed: ReadGrid = %v, %v; want the one pixel, 42", g, err)
	}

	r, err = Open(bytes.NewReader(stripTIFF(1, 1, compressionDeflate, 0xFFFFFFFF, pixel)), anyGeo)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if n := allocatedBy(func() { _, err = r.ReadGrid() }); n > claimBudget {
		t.Errorf("deflate: ReadGrid allocated %d bytes, want at most %d", n, claimBudget)
	}

	if err == nil {
		t.Error("deflate: ReadGrid succeeded on a strip ending 4 GB short")
	}
}

// TestBlockDecodesNoFurtherThanItsLayout: a one-pixel image's strip holds
// eight bytes, and a strip that decodes past them is not this image. A 65 KB
// deflate strip of 64 MiB of zeros made ReadGrid allocate 232 MB and return a
// valid grid; the LZW decoder had the same unbounded output.
func TestBlockDecodesNoFurtherThanItsLayout(t *testing.T) {
	zeros := make([]byte, 64<<20)

	var z bytes.Buffer

	zw, err := zlib.NewWriterLevel(&z, zlib.BestCompression)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := zw.Write(zeros); err != nil {
		t.Fatal(err)
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	lzw := tiffLZWEncode(zeros[:1<<20])

	for _, c := range []struct {
		name        string
		compression uint32
		strip       []byte
	}{
		{"deflate", compressionDeflate, z.Bytes()},
		{"LZW", compressionLZW, lzw},
	} {
		r, err := Open(bytes.NewReader(stripTIFF(1, 1, c.compression, uint32(len(c.strip)), c.strip)), anyGeo)
		if err != nil {
			t.Fatalf("%s: Open: %v", c.name, err)
		}

		var g *Grid

		n := allocatedBy(func() { g, err = r.ReadGrid() })

		if !errors.Is(err, ErrBadTIFF) || !strings.Contains(err.Error(), "past the 8 bytes") {
			t.Errorf("%s: ReadGrid = %v, %v; want ErrBadTIFF for decoding past the strip's 8 bytes", c.name, g, err)
		}

		// The compressed strip is read whole, so the budget is what it holds.
		if budget := uint64(claimBudget + 2*len(c.strip)); n > budget {
			t.Errorf("%s: ReadGrid allocated %d bytes from a %d-byte strip, want at most %d", c.name, n, len(c.strip), budget)
		}
	}
}

// TestReadGridGrowsAsPixelsDecode: an image declaring 2³¹−1 pixels a side made
// ReadGrid panic out of a 118-byte file, since the grid was allocated at the
// declared size. That file is now refused by Open, its one strip being too
// large to exist; with one row a strip, it opens, and ReadGrid fails at the
// first pixel the file lacks, having allocated only what it read.
func TestReadGridGrowsAsPixelsDecode(t *testing.T) {
	const side = 0x7FFFFFFF

	if _, err := Open(bytes.NewReader(stripTIFF(side, side, compressionNone, 8, make([]byte, 8))), anyGeo); !errors.Is(err, ErrBadTIFF) {
		t.Errorf("Open with one strip of %d rows: err %v, want ErrBadTIFF", side, err)
	}

	r, err := Open(bytes.NewReader(stripTIFF(side, side, compressionNone, 8, make([]byte, 8),
		ifdEntry{tagRowsPerStrip, typeLong, 1, 1})), anyGeo)
	if err != nil {
		t.Fatalf("Open with a row a strip: %v", err)
	}

	defer func() {
		if p := recover(); p != nil {
			t.Errorf("ReadGrid panicked: %v", p)
		}
	}()

	if n := allocatedBy(func() { _, err = r.ReadGrid() }); n > claimBudget {
		t.Errorf("ReadGrid allocated %d bytes, want at most %d", n, claimBudget)
	}

	if !errors.Is(err, ErrBadTIFF) {
		t.Errorf("ReadGrid: err %v, want ErrBadTIFF for the pixels the file lacks", err)
	}
}

// TestBlockSizeMustFit: a block's size is computed once, from the layout, and a
// tile of 2³¹−1 × 2³¹−1 float64 samples does not fit an int.
func TestBlockSizeMustFit(t *testing.T) {
	b, _ := handTIFF([]ifdEntry{
		{tagImageWidth, typeLong, 1, 1},
		{tagImageLength, typeLong, 1, 1},
		{tagBitsPerSample, typeShort, 1, 64},
		{tagCompression, typeShort, 1, compressionNone},
		{tagSamplesPerPixel, typeShort, 1, 1},
		{tagSampleFormat, typeShort, 1, 3},
		{tagTileWidth, typeLong, 1, 0x7FFFFFFF},
		{tagTileLength, typeLong, 1, 0x7FFFFFFF},
		{tagTileOffsets, typeLong, 1, 0},
		{tagTileByteCounts, typeLong, 1, 8},
	}, nil)

	if _, err := Open(bytes.NewReader(b), anyGeo); !errors.Is(err, ErrBadTIFF) || !strings.Contains(err.Error(), "overflows") {
		t.Errorf("Open: err %v, want ErrBadTIFF for a block size that overflows", err)
	}
}

// silentReader is a ReaderAt that delivers nothing and says nothing, which the
// io.ReaderAt contract forbids but a reader must survive.
type silentReader struct{}

func (silentReader) ReadAt([]byte, int64) (int, error) { return 0, nil }

// eofWithLastByte delivers everything asked for and io.EOF with it, as the
// io.ReaderAt contract allows at the end of the data.
type eofWithLastByte struct{ data []byte }

func (e eofWithLastByte) ReadAt(p []byte, off int64) (int, error) {
	n := copy(p, e.data[off:])
	if int(off)+n == len(e.data) {
		return n, io.EOF
	}

	return n, nil
}

// TestReadAt: what readAt reads, and when it stops.
func TestReadAt(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789"), 20000) // 200,000 bytes: several chunks

	got, err := readAt(bytes.NewReader(data), 3, len(data)-3)
	if err != nil || !bytes.Equal(got, data[3:]) {
		t.Errorf("readAt of the rest: %d bytes, err %v; want %d bytes equal to the source", len(got), err, len(data)-3)
	}

	if got, err := readAt(bytes.NewReader(data), 0, 0); err != nil || len(got) != 0 {
		t.Errorf("readAt of nothing: %v, %v", got, err)
	}

	if got, err := readAt(eofWithLastByte{data[:10]}, 0, 10); err != nil || !bytes.Equal(got, data[:10]) {
		t.Errorf("readAt with io.EOF on the last byte: %q, %v; want all ten bytes", got, err)
	}

	if _, err := readAt(bytes.NewReader(data), 10, len(data)); !errors.Is(err, io.EOF) {
		t.Errorf("readAt past the end: err %v, want io.EOF", err)
	}

	if _, err := readAt(silentReader{}, 0, 8); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("readAt from a reader that delivers nothing: err %v, want io.ErrUnexpectedEOF", err)
	}

	if _, err := readAt(bytes.NewReader(data), 0, -1); !errors.Is(err, ErrBadTIFF) {
		t.Errorf("readAt of -1 bytes: err %v, want ErrBadTIFF", err)
	}
}
