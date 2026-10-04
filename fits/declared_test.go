package fits_test

import (
	"bytes"
	"errors"
	"io"
	"math"
	"runtime"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/tensor"

	"github.com/TuSKan/astrogo/fits"
)

// onlyReader hides everything but Read, so the source cannot say how much it
// holds: a gzip stream, as fits.Open hands Read for a .fits.gz, or a pipe.
type onlyReader struct{ io.Reader }

// allocatedBy is the number of bytes allocated while f runs. TotalAlloc counts
// every goroutine, so a test using it must not run in parallel.
func allocatedBy(f func()) uint64 {
	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)

	return after.TotalAlloc - before.TotalAlloc
}

// TestReadAllocatesWhatArrivesNotWhatIsClaimed: a file that is nothing but a
// header claiming a large data area must cost what it holds. Each header used
// to be allocated in full before a byte of data was read: the 2880 byte image
// below took 400 MB, then failed at EOF (#461).
//
// Not parallel: allocatedBy counts the whole process.
func TestReadAllocatesWhatArrivesNotWhatIsClaimed(t *testing.T) {
	// One growChunk (1 MiB) for a source that cannot say what it holds, plus
	// parsing, with room to spare; the claims are 400 MB and 200 MB.
	const bound = 4 << 20

	image := hduBytes(nil, card("SIMPLE", "T"), card("BITPIX", "-32"), card("NAXIS", "2"),
		card("NAXIS1", "9999"), card("NAXIS2", "9999"))

	table := append(emptyPrimary(), hduBytes(nil,
		card("XTENSION", "'BINTABLE'"), card("BITPIX", "8"), card("NAXIS", "2"),
		card("NAXIS1", "1000"), card("NAXIS2", "200000"), card("PCOUNT", "0"), card("GCOUNT", "1"),
		card("TFIELDS", "1"), card("TFORM1", "'1000A   '"))...)

	for _, file := range []struct {
		name string
		raw  []byte
	}{
		{"400 MB image", image},
		{"200 MB BINTABLE", table},
	} {
		for _, source := range []struct {
			name string
			open func([]byte) io.Reader
		}{
			{"seekable", func(b []byte) io.Reader { return bytes.NewReader(b) }},
			{"stream", func(b []byte) io.Reader { return onlyReader{bytes.NewReader(b)} }},
		} {
			var err error

			allocated := allocatedBy(func() {
				_, err = fits.Read(source.open(file.raw))
			})

			if !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Errorf("%s, %s: err %v, want io.ErrUnexpectedEOF", file.name, source.name, err)
			}

			if allocated > bound {
				t.Errorf("%s, %s: a %d byte file allocated %.1f MB, want at most %.1f",
					file.name, source.name, len(file.raw), float64(allocated)/1e6, float64(bound)/1e6)
			}
		}
	}
}

// TestReadDecodesDataLargerThanOneChunk: from a source that cannot say what it
// holds, the data is read in a buffer that doubles from 1 MiB. An image of
// 2.4 MB crosses two of those doublings, and must come back pixel for pixel
// with the stream positioned for the HDU after it.
func TestReadDecodesDataLargerThanOneChunk(t *testing.T) {
	t.Parallel()

	const width, height = 1024, 600 // 2,457,600 bytes of float32

	value := func(x, y int) float32 { return float32(x) + 1e4*float32(y) }

	raw := append(imageFITS(width, height, value), hduBytes(nil, card("XTENSION", "'IMAGE   '"),
		card("BITPIX", "8"), card("NAXIS", "0"), card("PCOUNT", "0"), card("GCOUNT", "1"),
		card("EXTNAME", "'AFTER   '"))...)

	for _, source := range []struct {
		name string
		r    io.Reader
	}{
		{"seekable", bytes.NewReader(raw)},
		{"stream", onlyReader{bytes.NewReader(raw)}},
	} {
		t.Run(source.name, func(t *testing.T) {
			t.Parallel()

			f, err := fits.Read(source.r)
			if err != nil {
				t.Fatalf("Read: %v", err)
			}

			if len(f.HDUs) != 2 {
				t.Fatalf("Read: %d HDUs, want 2", len(f.HDUs))
			}

			img, ok := f.HDUs[0].(*fits.ImageHDU)
			if !ok {
				t.Fatalf("the primary HDU is %T, want *fits.ImageHDU", f.HDUs[0])
			}

			values, ok := img.Tensor.(*tensor.Float32)
			if !ok {
				t.Fatalf("the payload decoded as %T, want *tensor.Float32", img.Tensor)
			}

			pixels := values.Float32Values()
			if len(pixels) != width*height {
				t.Fatalf("got %d pixels, want %d", len(pixels), width*height)
			}

			for y := range height {
				for x := range width {
					if got, want := pixels[y*width+x], value(x, y); math.Abs(float64(got-want)) > 1e-3 {
						t.Fatalf("pixel (%d, %d) is %g, want %g", x, y, got, want)
					}
				}
			}

			if name, err := f.HDUs[1].Header().GetString("EXTNAME"); err != nil || name != "AFTER" {
				t.Errorf("second HDU EXTNAME %q, err %v; want AFTER", name, err)
			}
		})
	}
}
