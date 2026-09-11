package fits_test

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/arrow/tensor"

	"github.com/TuSKan/astrogo/fits"
)

// The writer's claim is that a file it produces is one this package — and any
// other FITS reader — can read back unchanged. Round-tripping is therefore the
// test that matters, and the byte-level tests below exist only for the things a
// round trip cannot see: this reader would happily read a file with the wrong
// padding or a non-standard card layout, and another reader would not.

// float32Image builds an image HDU over the given pixel values.
func float32Image(t *testing.T, cols, rows int, pixels []float32) *fits.ImageHDU {
	t.Helper()

	if len(pixels) != cols*rows {
		t.Fatalf("test setup: %d pixels for a %dx%d image", len(pixels), cols, rows)
	}

	bldr := array.NewFloat32Builder(memory.NewGoAllocator())
	defer bldr.Release()

	bldr.AppendValues(pixels, nil)

	arr := bldr.NewFloat32Array()
	defer arr.Release()

	// Axes are slowest-varying first, the reverse of NAXISn.
	axes := []int64{int64(rows), int64(cols)}

	return &fits.ImageHDU{
		Tensor: tensor.New(arr.Data(), axes, nil, nil),
		Axes:   axes,
		Bitpix: fits.BitpixFloat32,
	}
}

func TestWriteImageRoundTrips(t *testing.T) {
	t.Parallel()

	pixels := []float32{1, 2, 3, 4, 5, 6}

	src := float32Image(t, 3, 2, pixels)
	src.Header().Append(fits.Card{Keyword: "OBJECT", Value: "'M31'", Comment: "target"})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	if len(got.HDUs) != 1 {
		t.Fatalf("read %d HDUs, want 1", len(got.HDUs))
	}

	img, ok := got.HDUs[0].(*fits.ImageHDU)
	if !ok {
		t.Fatalf("HDU is %T, want *fits.ImageHDU", got.HDUs[0])
	}

	if img.Bitpix != fits.BitpixFloat32 {
		t.Errorf("BITPIX = %d, want %d", img.Bitpix, fits.BitpixFloat32)
	}

	// The axes have to survive the NAXISn flip in both directions; getting
	// this wrong transposes the image, which is not visible in the pixel
	// count and is very visible on the sky.
	if len(img.Axes) != 2 || img.Axes[0] != 2 || img.Axes[1] != 3 {
		t.Errorf("Axes = %v, want [2 3] (rows, cols)", img.Axes)
	}

	// A keyword the caller set must survive alongside the structural ones.
	if obj, err := img.Header().GetString("OBJECT"); err != nil || obj != "M31" {
		t.Errorf("OBJECT = %q (%v), want M31", obj, err)
	}

	assertFloat32Pixels(t, img, pixels)
}

// assertFloat32Pixels compares an image's pixels against the values written.
func assertFloat32Pixels(t *testing.T, img *fits.ImageHDU, want []float32) {
	t.Helper()

	if img.Tensor == nil {
		t.Fatal("image carries no tensor")
	}

	got, ok := img.Tensor.(*tensor.Float32)
	if !ok {
		t.Fatalf("tensor is %T, want *tensor.Float32", img.Tensor)
	}

	values := got.Float32Values()
	if len(values) != len(want) {
		t.Fatalf("read %d pixels, want %d", len(values), len(want))
	}

	for i, w := range want {
		if values[i] != w {
			t.Errorf("pixel %d = %v, want %v — byte order is the usual cause", i, values[i], w)
		}
	}
}

// TestWriteImageCarriesEveryBitpix walks the six pixel types, because each has
// its own width and its own byte-swap path, and a mistake in one is invisible
// from the others.
func TestWriteImageCarriesEveryBitpix(t *testing.T) {
	t.Parallel()

	mem := memory.NewGoAllocator()

	for _, tc := range []struct {
		build  func() arrow.Array
		name   string
		bitpix int
	}{
		{name: "uint8", bitpix: fits.BitpixUint8, build: func() arrow.Array {
			b := array.NewUint8Builder(mem)
			defer b.Release()

			b.AppendValues([]uint8{0, 1, 254, 255}, nil)

			return b.NewUint8Array()
		}},
		{name: "int16", bitpix: fits.BitpixInt16, build: func() arrow.Array {
			b := array.NewInt16Builder(mem)
			defer b.Release()

			b.AppendValues([]int16{math.MinInt16, -1, 0, math.MaxInt16}, nil)

			return b.NewInt16Array()
		}},
		{name: "int32", bitpix: fits.BitpixInt32, build: func() arrow.Array {
			b := array.NewInt32Builder(mem)
			defer b.Release()

			b.AppendValues([]int32{math.MinInt32, -1, 0, math.MaxInt32}, nil)

			return b.NewInt32Array()
		}},
		{name: "int64", bitpix: fits.BitpixInt64, build: func() arrow.Array {
			b := array.NewInt64Builder(mem)
			defer b.Release()

			b.AppendValues([]int64{math.MinInt64, -1, 0, math.MaxInt64}, nil)

			return b.NewInt64Array()
		}},
		{name: "float32", bitpix: fits.BitpixFloat32, build: func() arrow.Array {
			b := array.NewFloat32Builder(mem)
			defer b.Release()

			b.AppendValues([]float32{-1.5, 0, 0.1, 3.4e38}, nil)

			return b.NewFloat32Array()
		}},
		{name: "float64", bitpix: fits.BitpixFloat64, build: func() arrow.Array {
			b := array.NewFloat64Builder(mem)
			defer b.Release()

			b.AppendValues([]float64{-1.5, 0, 0.1, 1.7e308}, nil)

			return b.NewFloat64Array()
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			arr := tc.build()
			defer arr.Release()

			axes := []int64{4}

			src := &fits.ImageHDU{
				Tensor: tensor.New(arr.Data(), axes, nil, nil),
				Axes:   axes,
				Bitpix: tc.bitpix,
			}

			var buf bytes.Buffer
			if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
				t.Fatalf("Write: %v", err)
			}

			got, err := fits.Read(bytes.NewReader(buf.Bytes()))
			if err != nil {
				t.Fatalf("Read back: %v", err)
			}

			img, ok := got.HDUs[0].(*fits.ImageHDU)
			if !ok {
				t.Fatalf("HDU is %T", got.HDUs[0])
			}

			if img.Bitpix != tc.bitpix {
				t.Fatalf("BITPIX = %d, want %d", img.Bitpix, tc.bitpix)
			}

			// Compare the raw stored bytes: the values above include each
			// type's extremes, where a wrong width or a missed swap shows up
			// as a specific wrong number rather than as noise.
			wantBytes := arr.Data().Buffers()[1].Bytes()[:arr.Len()*byteWidth(tc.bitpix)]

			gotBytes := img.Tensor.Data().Buffers()[1].Bytes()[:arr.Len()*byteWidth(tc.bitpix)]
			if !bytes.Equal(gotBytes, wantBytes) {
				t.Errorf("pixel bytes differ after a round trip\n got %x\nwant %x", gotBytes, wantBytes)
			}
		})
	}
}

// byteWidth is the stored width of one pixel of the given BITPIX.
func byteWidth(bitpix int) int {
	if bitpix < 0 {
		bitpix = -bitpix
	}

	return bitpix / 8
}

// A primary HDU with no pixels is how a file whose data lives entirely in
// extensions begins, and it is the shape most multi-extension files have.
func TestWriteHeaderOnlyPrimaryHDU(t *testing.T) {
	t.Parallel()

	src := &fits.ImageHDU{}
	src.Header().Append(fits.Card{Keyword: "ORIGIN", Value: "'astrogo'", Comment: "written by"})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if buf.Len() != fits.BlockSize {
		t.Errorf("a header-only HDU wrote %d bytes, want one %d-byte block", buf.Len(), fits.BlockSize)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	// A header-only HDU comes back through the HDU interface rather than as
	// an *ImageHDU: there are no pixels to decode into one, which is the
	// reader's existing behaviour and is what NAXIS 0 means.
	hdu := got.HDUs[0]

	if hdu.Type() != fits.HDUTypeImage {
		t.Errorf("Type = %v, want HDUTypeImage", hdu.Type())
	}

	if n, err := hdu.Header().GetInt("NAXIS"); err != nil || n != 0 {
		t.Errorf("NAXIS = %d (%v), want 0", n, err)
	}

	if origin, err := hdu.Header().GetString("ORIGIN"); err != nil || origin != "astrogo" {
		t.Errorf("ORIGIN = %q (%v), want astrogo", origin, err)
	}
}

// TestWriteRefusesAnEmptyFile keeps a zero-HDU file from being written as zero
// bytes, which is not a FITS file and which no reader reports usefully.
func TestWriteRefusesAnEmptyFile(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	for _, f := range []*fits.File{nil, {}} {
		if err := fits.Write(&buf, f); !errors.Is(err, fits.ErrNoPrimaryHDU) {
			t.Errorf("Write(%v) = %v, want ErrNoPrimaryHDU", f, err)
		}
	}

	if buf.Len() != 0 {
		t.Errorf("a refused write emitted %d bytes", buf.Len())
	}
}

// Every FITS structure is a whole number of 2880-byte blocks. A reader that
// seeks by block — which is most of them, and which this package's own
// BlockReader does — lands in the middle of the next header otherwise.
func TestEverythingWrittenIsBlockAligned(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		file *fits.File
		name string
	}{
		{name: "header only", file: &fits.File{HDUs: []fits.HDU{&fits.ImageHDU{}}}},
		{name: "small image", file: &fits.File{HDUs: []fits.HDU{float32Image(t, 3, 2, []float32{1, 2, 3, 4, 5, 6})}}},
		{name: "image larger than a block", file: &fits.File{HDUs: []fits.HDU{
			float32Image(t, 40, 40, make([]float32, 1600)), // 6400 bytes: 3 blocks
		}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := fits.Write(&buf, tc.file); err != nil {
				t.Fatalf("Write: %v", err)
			}

			if rem := buf.Len() % fits.BlockSize; rem != 0 {
				t.Errorf("wrote %d bytes, %d past a %d-byte block boundary",
					buf.Len(), rem, fits.BlockSize)
			}
		})
	}
}

// The header's first three keywords are fixed in order by the standard, and
// this package's own VerifyPrimaryHeader rejects a file that gets it wrong — so
// a header written here and refused by the reader beside it would be a plain
// bug.
func TestWrittenPrimaryHeaderVerifies(t *testing.T) {
	t.Parallel()

	src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})

	// A caller's own SIMPLE, with the wrong value, must not survive into the
	// file: the structural keywords come from the data, not from whatever the
	// header happened to carry.
	src.Header().Append(fits.Card{Keyword: "SIMPLE", Value: "F"})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	if err := fits.VerifyPrimaryHeader(got.HDUs[0].Header()); err != nil {
		t.Errorf("VerifyPrimaryHeader on our own output: %v", err)
	}

	cards := got.HDUs[0].Header().Cards
	for i, want := range []string{"SIMPLE", "BITPIX", "NAXIS"} {
		if cards[i].Keyword != want {
			t.Errorf("card %d is %q, want %q", i, cards[i].Keyword, want)
		}
	}

	if cards[0].Value != "T" {
		t.Errorf("SIMPLE = %q, want T — a caller's own value overrode the structural one", cards[0].Value)
	}
}

// Header records are a fixed 80-byte grid with no separators, so a card of any
// other length shifts every card after it. Nothing in a round trip through this
// package would notice.
func TestEveryCardIsExactlyEightyBytes(t *testing.T) {
	t.Parallel()

	src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
	src.Header().Append(fits.Card{Keyword: "OBJECT", Value: "'NGC 7000'", Comment: "North America Nebula"})
	src.Header().Append(fits.Card{Keyword: "COMMENT", Comment: "commentary carries no value indicator"})
	src.Header().Append(fits.Card{Keyword: "EXPTIME", Value: "120.5", Comment: "seconds"})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	header := buf.Bytes()[:fits.BlockSize]

	var sawEnd bool

	for off := 0; off+fits.CardSize <= len(header); off += fits.CardSize {
		card := string(header[off : off+fits.CardSize])

		if strings.HasPrefix(card, "END     ") {
			sawEnd = true

			if strings.TrimSpace(card) != "END" {
				t.Errorf("END card carries trailing content: %q", card)
			}

			continue
		}

		if sawEnd {
			if strings.TrimSpace(card) != "" {
				t.Errorf("card after END is not blank: %q", card)
			}

			continue
		}

		// Columns 1-8 are the keyword, and a value card's indicator is
		// exactly columns 9-10.
		if kw := card[:8]; kw != strings.ToUpper(kw) {
			t.Errorf("keyword %q is not upper case", kw)
		}

		if strings.HasPrefix(card, "COMMENT ") && card[8:10] == "= " {
			t.Errorf("commentary card carries a value indicator: %q", card)
		}
	}

	if !sawEnd {
		t.Error("header has no END card")
	}
}

// TestWriteRefusesACardThatWillNotFit is the failure that would otherwise
// corrupt a file silently: an over-long card shifts the grid, so every card
// after it is misread.
func TestWriteRefusesACardThatWillNotFit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		card fits.Card
		name string
		want error
	}{
		{
			name: "keyword over eight characters",
			card: fits.Card{Keyword: "TOOLONGKEYWORD", Value: "1"},
			want: fits.ErrCardTooLong,
		},
		{
			name: "comment overflows the record",
			card: fits.Card{Keyword: "OBJECT", Value: "'M31'", Comment: strings.Repeat("x", 80)},
			want: fits.ErrCardTooLong,
		},
		{
			name: "newline in a comment",
			card: fits.Card{Keyword: "OBJECT", Value: "'M31'", Comment: "two\nlines"},
			want: fits.ErrCardNotPrintable,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
			src.Header().Append(tc.card)

			var buf bytes.Buffer
			if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); !errors.Is(err, tc.want) {
				t.Errorf("Write = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestWriteRefusesAnHDUItCannotEncode covers the two shapes that would
// otherwise produce a file describing something it does not contain.
func TestWriteRefusesAnHDUItCannotEncode(t *testing.T) {
	t.Parallel()

	t.Run("bintable as the primary HDU", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{&fits.BintableHDU{}}})
		if !errors.Is(err, fits.ErrNotWritable) {
			t.Errorf("Write = %v, want ErrNotWritable — FITS has no primary table", err)
		}
	})

	t.Run("axes disagree with the pixels", func(t *testing.T) {
		t.Parallel()

		src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
		src.Axes = []int64{100, 100} // 10000 pixels, 4 present

		var buf bytes.Buffer

		err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}})
		if !errors.Is(err, fits.ErrNotWritable) {
			t.Errorf("Write = %v, want ErrNotWritable.\n"+
				"  The header is the only statement of an image's shape, so one that "+
				"overstates it produces a file no reader can detect as wrong.", err)
		}
	})
}

// TestWrittenHeaderMatchesTheStandardLayoutByte pins the card layout against
// the standard rather than against this package's own reader.
//
// Every other test here round-trips, which cannot catch a mistake the reader
// and the writer share — a value in the wrong column reads back perfectly from
// a writer that put it there. These are the exact bytes FITS 4.0 §4.1.2
// prescribes and the exact bytes astropy and cfitsio emit: keyword in columns
// 1-8, "= " in 9-10, a logical or numeric value right-justified to column 30,
// then " / " and the comment.
func TestWrittenHeaderMatchesTheStandardLayoutByte(t *testing.T) {
	t.Parallel()

	src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := []string{
		"SIMPLE  =                    T / conforms to FITS standard",
		"BITPIX  =                  -32 / bits per data pixel",
		"NAXIS   =                    2 / number of data axes",
		"NAXIS1  =                    2 / length of data axis 1",
		"NAXIS2  =                    2 / length of data axis 2",
		"END",
	}

	header := buf.Bytes()

	for i, w := range want {
		off := i * fits.CardSize

		got := string(header[off : off+fits.CardSize])
		if got != pad80(w) {
			t.Errorf("card %d\n got %q\nwant %q", i, got, pad80(w))
		}
	}
}

// pad80 blank-fills a card to its record width.
func pad80(s string) string {
	return s + strings.Repeat(" ", fits.CardSize-len(s))
}

// TestWrittenStringValueMatchesTheStandardLayout covers the other card shape,
// which is laid out differently on purpose: a string starts at column 11,
// left-justified inside its quotes, and is blank-padded to at least eight
// characters.
func TestWrittenStringValueMatchesTheStandardLayout(t *testing.T) {
	t.Parallel()

	src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
	src.Header().Append(fits.Card{Keyword: "OBJECT", Value: "'M31'", Comment: "target"})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := pad80("OBJECT  = 'M31     ' / target")

	if got := findCard(t, buf.Bytes(), "OBJECT"); got != want {
		t.Errorf("OBJECT card\n got %q\nwant %q", got, want)
	}
}

// findCard returns the 80-byte record carrying keyword.
func findCard(t *testing.T, header []byte, keyword string) string {
	t.Helper()

	prefix := keyword + strings.Repeat(" ", 8-len(keyword))

	for off := 0; off+fits.CardSize <= len(header); off += fits.CardSize {
		card := string(header[off : off+fits.CardSize])
		if strings.HasPrefix(card, prefix) {
			return card
		}
	}

	t.Fatalf("no %s card in the header", keyword)

	return ""
}

// TestExtendIsWrittenOnlyWhenExtensionsFollow covers the keyword that tells a
// reader to look past the primary HDU.
//
// It is not decoration: without it a reader is entitled to stop at the primary
// HDU, so the failure mode is a file carrying a table nobody finds. astropy and
// cfitsio both write it, and the standard puts it immediately after the last
// NAXISn.
//
// The absent case matters too — a single-HDU file that advertises extensions it
// does not have is equally wrong.
func TestExtendIsWrittenOnlyWhenExtensionsFollow(t *testing.T) {
	t.Parallel()

	batch := catalogBatch(t)

	// Cleanup rather than defer: the parent returns before its parallel
	// subtests run, so a deferred Release frees the batch out from under them.
	t.Cleanup(batch.Release)

	for _, tc := range []struct {
		name string
		hdus []fits.HDU
		want bool
	}{
		{
			name: "primary alone",
			hdus: []fits.HDU{float32Image(t, 2, 2, []float32{1, 2, 3, 4})},
			want: false,
		},
		{
			name: "primary with a table extension",
			hdus: []fits.HDU{float32Image(t, 2, 2, []float32{1, 2, 3, 4}), &fits.BintableHDU{Batch: batch}},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			if err := fits.Write(&buf, &fits.File{HDUs: tc.hdus}); err != nil {
				t.Fatalf("Write: %v", err)
			}

			header := buf.Bytes()[:fits.BlockSize]
			got := bytes.Contains(header, []byte("EXTEND  ="))

			if got != tc.want {
				t.Errorf("EXTEND present = %v, want %v", got, tc.want)
			}

			if !tc.want {
				return
			}

			// The standard fixes its position: immediately after the last
			// NAXISn, before anything else.
			if want := pad80("EXTEND  =                    T / file contains extensions"); findCard(t, header, "EXTEND") != want {
				t.Errorf("EXTEND card\n got %q\nwant %q", findCard(t, header, "EXTEND"), want)
			}

			naxis2 := bytes.Index(header, []byte("NAXIS2  ="))
			extend := bytes.Index(header, []byte("EXTEND  ="))

			if extend != naxis2+fits.CardSize {
				t.Errorf("EXTEND is %d bytes after NAXIS2, want %d — the standard "+
					"puts it immediately after the last NAXISn", extend-naxis2, fits.CardSize)
			}
		})
	}
}

// TestWriteRefusesANonFiniteHeaderValue covers the values FITS cannot express.
//
// Go renders these as "NaN" and "+Inf", which in a card claiming to hold a
// number is text no reader can parse. A BSCALE that arrived as NaN is a
// caller's bug worth reporting rather than one to encode into a file that then
// fails somewhere else, for someone else.
func TestWriteRefusesANonFiniteHeaderValue(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		apply func(*fits.ImageHDU)
		name  string
	}{
		{name: "BSCALE NaN", apply: func(h *fits.ImageHDU) { h.BScale = math.NaN() }},
		{name: "BSCALE +Inf", apply: func(h *fits.ImageHDU) { h.BScale = math.Inf(1) }},
		{name: "BZERO -Inf", apply: func(h *fits.ImageHDU) { h.BZero = math.Inf(-1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			src := float32Image(t, 2, 2, []float32{1, 2, 3, 4})
			tc.apply(src)

			var buf bytes.Buffer
			if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); !errors.Is(err, fits.ErrCardNotFinite) {
				t.Errorf("Write = %v, want ErrCardNotFinite", err)
			}
		})
	}
}
