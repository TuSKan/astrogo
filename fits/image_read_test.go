package fits_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow/tensor"

	"github.com/TuSKan/astrogo/fits"
)

// imageFITS builds a minimal single-image file: a primary header with the
// given pixels as BITPIX -32, big-endian and block-padded, which is what a
// FITS image is.
func imageFITS(width, height int, pixel func(x, y int) float32) []byte {
	cards := []string{
		"SIMPLE  =                    T",
		"BITPIX  =                  -32",
		"NAXIS   =                    2",
		fmt.Sprintf("NAXIS1  = %20d", width),
		fmt.Sprintf("NAXIS2  = %20d", height),
		"BUNIT   = 'MJy/sr  '",
		"END",
	}

	var head bytes.Buffer

	for _, c := range cards {
		head.WriteString(c + strings.Repeat(" ", fits.CardSize-len(c)))
	}

	padBlock(&head)

	var body bytes.Buffer

	for y := range height {
		for x := range width {
			_ = binary.Write(&body, binary.BigEndian, pixel(x, y))
		}
	}

	padBlock(&body)

	return append(head.Bytes(), body.Bytes()...)
}

// padBlock rounds a buffer up to a whole FITS block.
func padBlock(b *bytes.Buffer) {
	if rem := b.Len() % fits.BlockSize; rem != 0 {
		b.Write(bytes.Repeat([]byte{' '}, fits.BlockSize-rem))
	}
}

// Read decodes an image rather than skipping past it.
//
// It used to append a header-only HDU and seek over the pixels, so a caller
// type-asserting to *ImageHDU got neither an image nor an error — the same
// defect ReadBintable had, where a table consumer silently got no rows. Tables
// were fixed and images were missed, which the SFD dust map found by asking
// for the one thing Read would not give it.
func TestReadDecodesAnImage(t *testing.T) {
	t.Parallel()

	const width, height = 7, 5

	value := func(x, y int) float32 { return float32(x) + 100*float32(y) }

	f, err := fits.Read(bytes.NewReader(imageFITS(width, height, value)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(f.HDUs) != 1 {
		t.Fatalf("got %d HDUs, want 1", len(f.HDUs))
	}

	img, ok := f.HDUs[0].(*fits.ImageHDU)
	if !ok {
		t.Fatalf("the primary HDU is %T, want *fits.ImageHDU — Read is skipping the "+
			"payload again", f.HDUs[0])
	}

	// Axes is C-contiguous, the reverse of the FITS NAXISn order: the slowest
	// varying axis first. So a NAXIS1 by NAXIS2 image reports [NAXIS2, NAXIS1]
	// — rows then columns. Worth pinning, because for a square image the two
	// orders are indistinguishable and a consumer can read it the wrong way
	// round for as long as its images stay square.
	if len(img.Axes) != 2 || img.Axes[0] != height || img.Axes[1] != width {
		t.Fatalf("axes %v, want [%d %d] — rows then columns", img.Axes, height, width)
	}

	values, ok := img.Tensor.(*tensor.Float32)
	if !ok {
		t.Fatalf("the payload decoded as %T, want *tensor.Float32", img.Tensor)
	}

	raw := values.Float32Values()
	if len(raw) != width*height {
		t.Fatalf("got %d pixels, want %d", len(raw), width*height)
	}

	// Row-major, and big-endian on the wire: a byte-order slip leaves the
	// count right and every value astronomically wrong.
	for y := range height {
		for x := range width {
			want := value(x, y)
			if got := raw[y*width+x]; math.Abs(float64(got-want)) > 1e-6 {
				t.Fatalf("pixel (%d, %d) is %g, want %g", x, y, got, want)
			}
		}
	}

	if unit, err := img.Header().GetString("BUNIT"); err != nil ||
		strings.TrimSpace(unit) != "MJy/sr" {
		t.Errorf("BUNIT is %q (%v), want MJy/sr", unit, err)
	}
}

// A header-only HDU has no payload to decode and must not be treated as though
// it had one.
//
// This is the ordinary shape of a primary header in a file whose data lives in
// extensions, so reading it as an image would break every table file in the
// module.
func TestReadHandlesAHeaderOnlyHDU(t *testing.T) {
	t.Parallel()

	cards := []string{
		"SIMPLE  =                    T",
		"BITPIX  =                    8",
		"NAXIS   =                    0",
		"END",
	}

	var head bytes.Buffer

	for _, c := range cards {
		head.WriteString(c + strings.Repeat(" ", fits.CardSize-len(c)))
	}

	padBlock(&head)

	f, err := fits.Read(bytes.NewReader(head.Bytes()))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(f.HDUs) != 1 {
		t.Fatalf("got %d HDUs, want 1", len(f.HDUs))
	}

	if naxis, err := f.HDUs[0].Header().GetInt("NAXIS"); err != nil || naxis != 0 {
		t.Errorf("NAXIS is %d (%v), want 0", naxis, err)
	}
}
