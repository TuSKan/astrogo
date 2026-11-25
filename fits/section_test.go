package fits_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/tensor"

	"github.com/TuSKan/astrogo/fits"
)

// header returns the cards as header blocks, END appended.
func header(cards ...string) []byte {
	var b bytes.Buffer

	for _, c := range append(cards, "END") {
		b.WriteString(c + strings.Repeat(" ", fits.CardSize-len(c)))
	}

	padBlock(&b)

	return b.Bytes()
}

// imageCards are the structural cards of an image of BITPIX bitpix and
// extents naxis, NAXIS1 first, after first, the SIMPLE or XTENSION card.
func imageCards(first string, bitpix int, naxis []int) []string {
	cards := make([]string, 0, 3+len(naxis))
	cards = append(cards, first, card("BITPIX", strconv.Itoa(bitpix)), card("NAXIS", strconv.Itoa(len(naxis))))

	for i, d := range naxis {
		cards = append(cards, card("NAXIS"+strconv.Itoa(i+1), strconv.Itoa(d)))
	}

	return cards
}

// primaryImageOf is the primary HDU of file, which must be an image.
func primaryImageOf(t *testing.T, file *fits.File) *fits.ImageHDU {
	t.Helper()

	img, ok := file.HDUs[0].(*fits.ImageHDU)
	if !ok {
		t.Fatalf("the primary HDU is %T, want an image", file.HDUs[0])
	}

	return img
}

// int16s is sec, which must hold int16 pixels.
func int16s(t *testing.T, sec tensor.Interface) *tensor.Int16 {
	t.Helper()

	values, ok := sec.(*tensor.Int16)
	if !ok {
		t.Fatalf("a section of %s, want int16", sec.DataType())
	}

	return values
}

// pixelData returns the data of an image of n pixels whose pixel i, in storage
// order, holds stored(i) as BITPIX bitpix stores it, padded to a block.
func pixelData(bitpix, n int, stored func(i int) float64) []byte {
	var b bytes.Buffer

	for i := range n {
		v := stored(i)

		switch bitpix {
		case fits.BitpixUint8:
			b.WriteByte(byte(int64(v)))
		case fits.BitpixInt16:
			_ = binary.Write(&b, binary.BigEndian, int16(v))
		case fits.BitpixInt32:
			_ = binary.Write(&b, binary.BigEndian, int32(v))
		case fits.BitpixInt64:
			_ = binary.Write(&b, binary.BigEndian, int64(v))
		case fits.BitpixFloat32:
			_ = binary.Write(&b, binary.BigEndian, float32(v))
		case fits.BitpixFloat64:
			_ = binary.Write(&b, binary.BigEndian, v)
		}
	}

	padBlock(&b)

	return b.Bytes()
}

// primaryImage returns a FITS file holding one primary image, as imageCards
// and pixelData describe it, with the extra cards.
func primaryImage(bitpix int, naxis []int, extra []string, stored func(i int) float64) []byte {
	n := 1
	for _, d := range naxis {
		n *= d
	}

	cards := append(imageCards(card("SIMPLE", "T"), bitpix, naxis), extra...)

	return append(header(cards...), pixelData(bitpix, n, stored)...)
}

// valueAt is the pixel of t at idx, as its own element type, so that two
// tensors compare exactly whatever that type is.
func valueAt(t *testing.T, tsr tensor.Interface, idx []int64) any {
	t.Helper()

	switch v := tsr.(type) {
	case *tensor.Uint8:
		return v.Value(idx)
	case *tensor.Int16:
		return v.Value(idx)
	case *tensor.Uint16:
		return v.Value(idx)
	case *tensor.Int32:
		return v.Value(idx)
	case *tensor.Uint32:
		return v.Value(idx)
	case *tensor.Int64:
		return v.Value(idx)
	case *tensor.Uint64:
		return v.Value(idx)
	case *tensor.Float32:
		return v.Value(idx)
	case *tensor.Float64:
		return v.Value(idx)
	}

	t.Fatalf("a tensor of %s", tsr.DataType())

	return nil
}

// eachIndex calls f with every index of a box of the given shape, the last
// axis fastest.
func eachIndex(shape []int64, f func(idx []int64)) {
	idx := make([]int64, len(shape))

	for {
		f(idx)

		i := len(shape) - 1
		for ; i >= 0; i-- {
			idx[i]++
			if idx[i] < shape[i] {
				break
			}

			idx[i] = 0
		}

		if i < 0 {
			return
		}
	}
}

// openBoth reads data with Read and with ReadAt, and returns the image HDU i
// of each.
func openBoth(t *testing.T, data []byte, i int) (eager, lazy *fits.ImageHDU) {
	t.Helper()

	whole, err := fits.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	sectioned, err := fits.ReadAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}

	eager, ok := whole.HDUs[i].(*fits.ImageHDU)
	if !ok {
		t.Fatalf("Read: HDU %d is %T", i, whole.HDUs[i])
	}

	lazy, ok = sectioned.HDUs[i].(*fits.ImageHDU)
	if !ok {
		t.Fatalf("ReadAt: HDU %d is %T", i, sectioned.HDUs[i])
	}

	return eager, lazy
}

// checkSection requires got, the section [lower, upper) of an image, to hold
// what want, the whole image's tensor, holds over that box.
func checkSection(t *testing.T, label string, got, want tensor.Interface, lower, upper []int64) {
	t.Helper()

	if !arrowTypesEqual(got, want) {
		t.Fatalf("%s: a section of %s from an image of %s", label, got.DataType(), want.DataType())
	}

	shape := make([]int64, len(lower))
	for i := range lower {
		shape[i] = upper[i] - lower[i]
	}

	if !reflect.DeepEqual(got.Shape(), shape) {
		t.Fatalf("%s: shape %v, want %v", label, got.Shape(), shape)
	}

	at := make([]int64, len(lower))

	eachIndex(shape, func(idx []int64) {
		for i := range idx {
			at[i] = lower[i] + idx[i]
		}

		if g, w := valueAt(t, got, idx), valueAt(t, want, at); g != w {
			t.Errorf("%s: pixel %v of the section is %v, want the image's pixel %v, %v", label, idx, g, at, w)
		}
	})
}

func arrowTypesEqual(a, b tensor.Interface) bool { return a.DataType().ID() == b.DataType().ID() }

// sectionBoxes are boxes of an image of C-order extents [3, 5, 7]: the whole,
// one pixel, a corner, whole rows, the interior and the last plane.
var sectionBoxes = []struct {
	name         string
	lower, upper []int64
}{
	{"the whole image", []int64{0, 0, 0}, []int64{3, 5, 7}},
	{"one pixel", []int64{1, 2, 3}, []int64{2, 3, 4}},
	{"a corner", []int64{0, 0, 0}, []int64{2, 3, 4}},
	{"whole rows", []int64{0, 1, 0}, []int64{3, 4, 7}},
	{"the interior", []int64{1, 1, 2}, []int64{3, 4, 6}},
	{"the last plane", []int64{2, 0, 0}, []int64{3, 5, 7}},
}

// TestSectionHoldsWhatTheWholeImageHolds: a section of an image equals the same
// box of the image read whole, for every BITPIX, for unsigned data and with
// scaling and BLANK, whether the image was read by ReadAt, whose section
// reads its bytes, or by Read, whose section is cut from its tensor.
func TestSectionHoldsWhatTheWholeImageHolds(t *testing.T) {
	t.Parallel()

	naxis := []int{7, 5, 3}

	for _, tc := range []struct {
		name   string
		bitpix int
		extra  []string
		stored func(i int) float64
	}{
		{"BITPIX 8", fits.BitpixUint8, nil, func(i int) float64 { return float64(i % 251) }},
		{"BITPIX 16", fits.BitpixInt16, nil, func(i int) float64 { return float64(i*997%60000 - 30000) }},
		{"BITPIX 16 unsigned", fits.BitpixInt16, []string{card("BZERO", "32768")},
			func(i int) float64 { return float64(i*997%65536 - 32768) }},
		{"BITPIX 16 scaled with BLANK", fits.BitpixInt16,
			[]string{card("BSCALE", "0.5"), card("BZERO", "10"), card("BLANK", "-1")},
			func(i int) float64 { return float64(i%9 - 1) }},
		{"BITPIX 32", fits.BitpixInt32, nil, func(i int) float64 { return float64(i*104729 - 1_000_000) }},
		{"BITPIX 32 unsigned", fits.BitpixInt32, []string{card("BZERO", "2147483648")},
			func(i int) float64 { return float64(int32(uint32(i) * 2654435761)) }},
		{"BITPIX 64", fits.BitpixInt64, nil, func(i int) float64 { return float64(i) * 1e12 }},
		{"BITPIX 64 unsigned", fits.BitpixInt64, []string{card("BZERO", "9223372036854775808")},
			func(i int) float64 { return float64(i)*1e15 - 4e16 }},
		{"BITPIX -32", fits.BitpixFloat32, nil, func(i int) float64 { return float64(i)*0.25 - 3 }},
		{"BITPIX -64", fits.BitpixFloat64, nil, func(i int) float64 { return float64(i)*1e-3 - 0.05 }},
	} {
		data := primaryImage(tc.bitpix, naxis, tc.extra, tc.stored)
		eager, lazy := openBoth(t, data, 0)

		if lazy.Tensor != nil {
			t.Fatalf("%s: ReadAt read the image's pixels", tc.name)
		}

		if lazy.BScale != eager.BScale || lazy.BZero != eager.BZero || lazy.HasBlank != eager.HasBlank || lazy.Blank != eager.Blank {
			t.Errorf("%s: ReadAt's calibration %v %v %v %v, Read's %v %v %v %v", tc.name,
				lazy.BScale, lazy.BZero, lazy.HasBlank, lazy.Blank, eager.BScale, eager.BZero, eager.HasBlank, eager.Blank)
		}

		for _, box := range sectionBoxes {
			for _, img := range []struct {
				how string
				hdu *fits.ImageHDU
			}{{"ReadAt", lazy}, {"Read", eager}} {
				sec, err := img.hdu.Section(box.lower, box.upper)
				if err != nil {
					t.Fatalf("%s, %s, %s: %v", tc.name, img.how, box.name, err)
				}

				checkSection(t, tc.name+", "+img.how+", "+box.name, sec, eager.Tensor, box.lower, box.upper)
				sec.Release()
			}
		}
	}
}

// TestSectionOfABlankPixelIsUndefined: BLANK applies to a section's pixels as
// it does to the image's, through PhysicalValue.
func TestSectionOfABlankPixelIsUndefined(t *testing.T) {
	t.Parallel()

	data := primaryImage(fits.BitpixInt16, []int{3, 2}, []string{card("BLANK", "-7"), card("BSCALE", "2")},
		func(i int) float64 { return []float64{1, -7, 3, 4, 5, 6}[i] })

	_, lazy := openBoth(t, data, 0)

	sec, err := lazy.Section([]int64{0, 0}, []int64{1, 3})
	if err != nil {
		t.Fatal(err)
	}
	defer sec.Release()

	values := int16s(t, sec).Int16Values()

	if got := lazy.PhysicalValue(float64(values[1])); !math.IsNaN(got) {
		t.Errorf("the BLANK pixel's physical value is %v, want NaN", got)
	}

	if got := lazy.PhysicalValue(float64(values[2])); got != 6 {
		t.Errorf("pixel 2's physical value is %v, want 2 × 3", got)
	}
}

// countingReaderAt counts the reads of r at or past from: the image data of a
// file whose header ends there.
type countingReaderAt struct {
	r     io.ReaderAt
	from  int64
	reads atomic.Int32
}

func (c *countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= c.from {
		c.reads.Add(1)
	}

	return c.r.ReadAt(p, off) //nolint:wrapcheck // a test double passing reads through
}

// TestSectionReadsOneRunPerContiguousStretch: opening reads no pixel, and a
// section reads each contiguous stretch of its box once. A box spanning whole
// rows of the fastest axis merges them, and the whole image is one read.
func TestSectionReadsOneRunPerContiguousStretch(t *testing.T) {
	t.Parallel()

	data := primaryImage(fits.BitpixInt16, []int{7, 5, 3}, nil, func(i int) float64 { return float64(i) })
	src := &countingReaderAt{r: bytes.NewReader(data), from: fits.BlockSize}

	file, err := fits.ReadAt(src, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	if n := src.reads.Load(); n != 0 {
		t.Fatalf("opening read the image data %d times, want none", n)
	}

	img := primaryImageOf(t, file)

	for _, tc := range []struct {
		name         string
		lower, upper []int64
		reads        int32
	}{
		{"part of each row", []int64{0, 0, 2}, []int64{3, 5, 5}, 15},
		{"whole rows", []int64{0, 1, 0}, []int64{3, 4, 7}, 3},
		{"whole planes", []int64{1, 0, 0}, []int64{3, 5, 7}, 1},
		{"the whole image", []int64{0, 0, 0}, []int64{3, 5, 7}, 1},
		{"one pixel", []int64{2, 4, 6}, []int64{3, 5, 7}, 1},
	} {
		before := src.reads.Load()

		sec, err := img.Section(tc.lower, tc.upper)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		sec.Release()

		if n := src.reads.Load() - before; n != tc.reads {
			t.Errorf("%s: %d reads, want %d", tc.name, n, tc.reads)
		}
	}
}

// fabricatedImage is a FITS file holding one BITPIX 16 image of 400000 ×
// 400000 pixels, 320 GB, its bytes made up as they are read: pixel i holds
// i mod 30011.
type fabricatedImage struct {
	header      []byte
	size        int64
	pixelReads  atomic.Int32
	pixelsWidth int64
}

func newFabricatedImage() *fabricatedImage {
	const side = 400_000

	head := header(imageCards(card("SIMPLE", "T"), fits.BitpixInt16, []int{side, side})...)

	data := int64(side) * side * 2
	if rem := data % fits.BlockSize; rem != 0 {
		data += fits.BlockSize - rem
	}

	return &fabricatedImage{header: head, size: int64(len(head)) + data, pixelsWidth: side}
}

func (f *fabricatedImage) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(f.header)) {
		f.pixelReads.Add(1)
	}

	n := 0

	for ; n < len(p) && off+int64(n) < f.size; n++ {
		pos := off + int64(n)
		if pos < int64(len(f.header)) {
			p[n] = f.header[pos]

			continue
		}

		d := pos - int64(len(f.header))
		v := uint16(int16(d / 2 % 30011))

		p[n] = byte(v >> 8)
		if d%2 == 1 {
			p[n] = byte(v)
		}
	}

	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// pixel is the value fabricatedImage holds at row and col.
func (f *fabricatedImage) pixel(row, col int64) int16 {
	return int16((row*f.pixelsWidth + col) % 30011)
}

// TestSectionOfAnImageLargerThanMemoryAllocatesTheBox: a 100 × 100 box of a
// 320 GB image is read with one read per row, and opening the file and taking
// the box allocate in proportion to the box, not the image. The bound is on
// bytes allocated, not only on the result.
func TestSectionOfAnImageLargerThanMemoryAllocatesTheBox(t *testing.T) {
	src := newFabricatedImage()

	lower, upper := []int64{1000, 2000}, []int64{1100, 2100}

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	file, err := fits.ReadAt(src, src.size)
	if err != nil {
		t.Fatal(err)
	}

	img := primaryImageOf(t, file)

	sec, err := img.Section(lower, upper)
	if err != nil {
		t.Fatal(err)
	}
	defer sec.Release()

	runtime.ReadMemStats(&after)

	// The box is 20 kB; the header and the parsing around it a few more.
	const bound = 1 << 20

	alloc := after.TotalAlloc - before.TotalAlloc
	if alloc > bound {
		t.Errorf("opening and taking a 20 kB box allocated %d bytes, want at most %d", alloc, bound)
	}

	t.Logf("opening a 320 GB image and taking a 20 kB box allocated %d bytes", alloc)

	if n := src.pixelReads.Load(); n != 100 {
		t.Errorf("%d reads of the pixels, want one per row of the box, 100", n)
	}

	values := int16s(t, sec)

	eachIndex(sec.Shape(), func(idx []int64) {
		if got, want := values.Value(idx), src.pixel(lower[0]+idx[0], lower[1]+idx[1]); got != want {
			t.Fatalf("pixel %v of the box is %d, want %d", idx, got, want)
		}
	})
}

// TestSectionRefusesABoxThatIsNotInsideTheImage: a box of the wrong rank,
// reaching outside the image, reversed or empty is ErrInvalidSection, never a
// short or empty tensor, and so is a section of an image holding no pixels.
func TestSectionRefusesABoxThatIsNotInsideTheImage(t *testing.T) {
	t.Parallel()

	data := primaryImage(fits.BitpixInt16, []int{7, 5, 3}, nil, func(i int) float64 { return float64(i) })
	eager, lazy := openBoth(t, data, 0)

	for _, tc := range []struct {
		name         string
		lower, upper []int64
	}{
		{"too few axes", []int64{0, 0}, []int64{3, 5}},
		{"too many axes", []int64{0, 0, 0, 0}, []int64{3, 5, 7, 1}},
		{"axes that disagree", []int64{0, 0, 0}, []int64{3, 5}},
		{"below zero", []int64{0, -1, 0}, []int64{3, 5, 7}},
		{"past NAXIS1", []int64{0, 0, 0}, []int64{3, 5, 8}},
		{"past NAXIS3", []int64{0, 0, 0}, []int64{4, 5, 7}},
		{"reversed", []int64{0, 4, 0}, []int64{3, 2, 7}},
		{"empty", []int64{0, 2, 0}, []int64{3, 2, 7}},
		{"wholly outside", []int64{5, 0, 0}, []int64{6, 5, 7}},
	} {
		for _, img := range []*fits.ImageHDU{lazy, eager} {
			sec, err := img.Section(tc.lower, tc.upper)
			if !errors.Is(err, fits.ErrInvalidSection) || sec != nil {
				t.Errorf("%s: tensor %v, err %v, want ErrInvalidSection", tc.name, sec, err)
			}
		}
	}

	noPixels := &fits.ImageHDU{Axes: []int64{2, 2}, Bitpix: fits.BitpixInt16}
	if _, err := noPixels.Section([]int64{0, 0}, []int64{1, 1}); !errors.Is(err, fits.ErrInvalidSection) {
		t.Errorf("a section of an image holding no pixels: err %v, want ErrInvalidSection", err)
	}
}

// multiHDUFile is a primary image, a binary table and an image extension.
func multiHDUFile() []byte {
	primary := primaryImage(fits.BitpixInt16, []int{4, 3}, nil, func(i int) float64 { return float64(i - 5) })

	table := append(header(
		card("XTENSION", "'BINTABLE'"), card("BITPIX", "8"), card("NAXIS", "2"),
		card("NAXIS1", "4"), card("NAXIS2", "2"), card("PCOUNT", "0"), card("GCOUNT", "1"),
		card("TFIELDS", "1"), card("TFORM1", "'J       '"), card("TTYPE1", "'N       '"),
	), pixelData(fits.BitpixInt32, 2, func(i int) float64 { return float64(100 + i) })...)

	extension := append(header(append(imageCards(card("XTENSION", "'IMAGE   '"), fits.BitpixFloat32, []int{5, 2}),
		card("PCOUNT", "0"), card("GCOUNT", "1"))...),
		pixelData(fits.BitpixFloat32, 10, func(i int) float64 { return float64(i) / 4 })...)

	return append(append(primary, table...), extension...)
}

// TestReadAtReadsWhatReadReads: ReadAt finds the HDUs Read finds, with the same
// headers, decodes tables as Read does, and leaves each image's pixels for a
// section, which holds what Read's tensor does.
func TestReadAtReadsWhatReadReads(t *testing.T) {
	t.Parallel()

	data := multiHDUFile()

	whole, err := fits.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	sectioned, err := fits.ReadAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	if len(sectioned.HDUs) != len(whole.HDUs) || len(whole.HDUs) != 3 {
		t.Fatalf("ReadAt found %d HDUs, Read %d, want 3", len(sectioned.HDUs), len(whole.HDUs))
	}

	for i, w := range whole.HDUs {
		g := sectioned.HDUs[i]

		if g.Type() != w.Type() || !reflect.DeepEqual(g.Header().Cards, w.Header().Cards) {
			t.Errorf("HDU %d: ReadAt's %v header %v, Read's %v %v", i, g.Type(), g.Header().Cards, w.Type(), w.Header().Cards)
		}

		switch w := w.(type) {
		case *fits.BintableHDU:
			g, ok := g.(*fits.BintableHDU)
			if !ok || g.Rows != w.Rows || !array.Equal(g.Batch.Column(0), w.Batch.Column(0)) {
				t.Errorf("HDU %d: ReadAt's table %v differs from Read's %v", i, g, w)
			}
		case *fits.ImageHDU:
			g, ok := g.(*fits.ImageHDU)
			if !ok || g.Tensor != nil {
				t.Fatalf("HDU %d: ReadAt gave %v, want an image with its pixels left in the file", i, g)
			}

			lower, upper := make([]int64, len(w.Axes)), w.Axes

			sec, err := g.Section(lower, upper)
			if err != nil {
				t.Fatalf("HDU %d: %v", i, err)
			}

			checkSection(t, fmt.Sprintf("HDU %d", i), sec, w.Tensor, lower, upper)
			sec.Release()
		default:
			t.Errorf("HDU %d: %T", i, w)
		}
	}
}

// TestReadAtRefusesDataPastTheEnd: an HDU declaring more data than the file
// holds is an error wrapping io.ErrUnexpectedEOF, at open, rather than an image
// whose sections fail later.
func TestReadAtRefusesDataPastTheEnd(t *testing.T) {
	t.Parallel()

	data := multiHDUFile()
	cut := data[:len(data)-fits.BlockSize]

	if _, err := fits.ReadAt(bytes.NewReader(cut), int64(len(cut))); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("err %v, want io.ErrUnexpectedEOF", err)
	}

	if _, err := fits.ReadAt(bytes.NewReader(nil), 0); !errors.Is(err, fits.ErrNoPrimaryHDU) {
		t.Errorf("an empty file: err %v, want ErrNoPrimaryHDU", err)
	}
}

// TestOpenMmapReadsImagesLazily: a mapped file's images are read as ReadAt reads
// them, and a section can be taken until Close and not after.
func TestOpenMmapReadsImagesLazily(t *testing.T) {
	t.Parallel()

	data := multiHDUFile()

	path := filepath.Join(t.TempDir(), "image.fits")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	mapped, err := fits.OpenMmap(path)
	if err != nil {
		t.Fatal(err)
	}

	eager, _ := openBoth(t, data, 2)

	img, ok := mapped.HDUs[2].(*fits.ImageHDU)
	if !ok || img.Tensor != nil {
		t.Fatalf("OpenMmap gave %v, want an image with its pixels left in the mapping", mapped.HDUs[2])
	}

	lower, upper := []int64{0, 1}, []int64{2, 4}

	sec, err := img.Section(lower, upper)
	if err != nil {
		t.Fatal(err)
	}

	checkSection(t, "mapped", sec, eager.Tensor, lower, upper)
	sec.Release()

	if err := mapped.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := img.Section(lower, upper); err == nil {
		t.Error("a section of an unmapped file was taken")
	}
}

// TestWriteRefusesAnImageWithoutItsPixels: an image read by ReadAt holds no
// tensor, and writing it is ErrNotWritable rather than a header describing
// pixels the file does not carry. A section of it writes and reads back.
func TestWriteRefusesAnImageWithoutItsPixels(t *testing.T) {
	t.Parallel()

	data := primaryImage(fits.BitpixInt16, []int{7, 5, 3}, nil, func(i int) float64 { return float64(i - 50) })

	file, err := fits.ReadAt(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := fits.Write(&out, file); !errors.Is(err, fits.ErrNotWritable) {
		t.Fatalf("writing an image read by ReadAt: err %v, want ErrNotWritable", err)
	}

	lower, upper := []int64{1, 1, 1}, []int64{3, 4, 6}

	sec, err := primaryImageOf(t, file).Section(lower, upper)
	if err != nil {
		t.Fatal(err)
	}
	defer sec.Release()

	out.Reset()

	if err := fits.Write(&out, &fits.File{HDUs: []fits.HDU{&fits.ImageHDU{Tensor: sec, Axes: sec.Shape()}}}); err != nil {
		t.Fatalf("writing a section: %v", err)
	}

	back, err := fits.Read(&out)
	if err != nil {
		t.Fatal(err)
	}

	checkSection(t, "written section", primaryImageOf(t, back).Tensor, sec, make([]int64, 3), sec.Shape())
}
