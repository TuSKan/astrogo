package fits_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/arrow/tensor"

	"github.com/TuSKan/astrogo/fits"
)

// FITS has no unsigned integer type above 8 bits. The standard's answer — FITS
// 4.0 §5.2.5, and what every other library does — is to store the signed type
// of the same width offset by BZERO = 2^(n-1).
//
// The failure this prevents is quiet: a detector frame of uint16 counts read
// without the convention hands back 40000 as -25536, which is a plausible
// number in the wrong place rather than anything a caller notices.

// TestUnsignedImagesRoundTripThroughBZero walks all three widths, at the values
// where the convention is visible: zero, the midpoint, and the maximum. The
// upper half is what reads back negative when the offset is ignored.
func TestUnsignedImagesRoundTripThroughBZero(t *testing.T) {
	t.Parallel()

	t.Run("uint16", func(t *testing.T) {
		t.Parallel()

		want := []uint16{0, 1, 32767, 32768, 40000, math.MaxUint16}

		img := roundTripUnsigned(t, want, 32768, arrow.UINT16)

		tn, ok := img.Tensor.(*tensor.Uint16)
		if !ok {
			t.Fatalf("tensor is %T, want *tensor.Uint16", img.Tensor)
		}

		assertValues(t, tn.Uint16Values(), want)
	})

	t.Run("uint32", func(t *testing.T) {
		t.Parallel()

		want := []uint32{0, 1, math.MaxInt32, math.MaxInt32 + 1, math.MaxUint32}

		img := roundTripUnsigned(t, want, 2147483648, arrow.UINT32)

		tn, ok := img.Tensor.(*tensor.Uint32)
		if !ok {
			t.Fatalf("tensor is %T, want *tensor.Uint32", img.Tensor)
		}

		assertValues(t, tn.Uint32Values(), want)
	})

	t.Run("uint64", func(t *testing.T) {
		t.Parallel()

		want := []uint64{0, 1, math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64}

		img := roundTripUnsigned(t, want, 9223372036854775808, arrow.UINT64)

		tn, ok := img.Tensor.(*tensor.Uint64)
		if !ok {
			t.Fatalf("tensor is %T, want *tensor.Uint64", img.Tensor)
		}

		assertValues(t, tn.Uint64Values(), want)
	})
}

// unsignedValue is the set of unsigned widths FITS stores through BZERO.
type unsignedValue interface {
	~uint16 | ~uint32 | ~uint64
}

// roundTripUnsigned writes values as an image, reads it back, and checks the
// keywords the convention is carried by.
func roundTripUnsigned[T unsignedValue](t *testing.T, values []T, wantBZero float64, wantType arrow.Type) *fits.ImageHDU {
	t.Helper()

	arr := buildUnsigned(t, values)
	defer arr.Release()

	axes := []int64{int64(arr.Len())}

	src := &fits.ImageHDU{Tensor: tensor.New(arr.Data(), axes, nil, nil), Axes: axes}

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

	// BZERO has to be the exact value the convention names, since that is what
	// every other reader matches against.
	if img.BZero != wantBZero {
		t.Errorf("BZERO = %v, want %v — other readers match this exactly", img.BZero, wantBZero)
	}

	if img.BScale != 1 {
		t.Errorf("BSCALE = %v, want 1", img.BScale)
	}

	if id := img.Tensor.DataType().ID(); id != wantType {
		t.Fatalf("tensor type = %v, want %v — the offset was not applied on read", id, wantType)
	}

	return img
}

// buildUnsigned puts values into the Arrow array of their own width.
func buildUnsigned[T unsignedValue](t *testing.T, values []T) arrow.Array {
	t.Helper()

	mem := memory.NewGoAllocator()

	switch v := any(values).(type) {
	case []uint16:
		b := array.NewUint16Builder(mem)
		defer b.Release()

		b.AppendValues(v, nil)

		return b.NewUint16Array()
	case []uint32:
		b := array.NewUint32Builder(mem)
		defer b.Release()

		b.AppendValues(v, nil)

		return b.NewUint32Array()
	case []uint64:
		b := array.NewUint64Builder(mem)
		defer b.Release()

		b.AppendValues(v, nil)

		return b.NewUint64Array()
	default:
		t.Fatalf("no builder for %T", values)

		return nil
	}
}

// assertValues compares what came back against what went in.
func assertValues[T unsignedValue](t *testing.T, got, want []T) {
	t.Helper()

	if len(got) < len(want) {
		t.Fatalf("read %d pixels, want %d", len(got), len(want))
	}

	for i, w := range want {
		if got[i] != w {
			t.Errorf("pixel %d = %d, want %d", i, got[i], w)
		}
	}
}

// TestASignedImageIsNotReadAsUnsigned is the other half, and the one that
// stops the convention from swallowing real calibration data.
//
// A BZERO near the magic value but not equal to it is an ordinary scaling
// offset, and reading it as an unsigned marker would reinterpret every pixel.
func TestASignedImageIsNotReadAsUnsigned(t *testing.T) {
	t.Parallel()

	b := array.NewInt16Builder(memory.NewGoAllocator())
	defer b.Release()

	b.AppendValues([]int16{-100, 0, 100}, nil)

	arr := b.NewInt16Array()
	defer arr.Release()

	axes := []int64{3}

	src := &fits.ImageHDU{
		Tensor: tensor.New(arr.Data(), axes, nil, nil),
		Axes:   axes,
		Bitpix: fits.BitpixInt16,
		BZero:  32768.5, // a real offset, not the unsigned marker
		BScale: 1,
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

	if id := img.Tensor.DataType().ID(); id != arrow.INT16 {
		t.Fatalf("tensor type = %v, want INT16 — a calibration offset was read "+
			"as the unsigned marker", id)
	}

	tn, ok := img.Tensor.(*tensor.Int16)
	if !ok {
		t.Fatalf("tensor is %T", img.Tensor)
	}

	for i, w := range []int16{-100, 0, 100} {
		if got := tn.Int16Values()[i]; got != w {
			t.Errorf("pixel %d = %d, want %d", i, got, w)
		}
	}
}

// TestUnsignedStoredBytesAreTheSignedOnes pins the encoding itself against the
// standard, not against this package's reader.
//
// A uint16 of 0 is stored as int16 -32768, and 65535 as +32767. Any other
// reader will apply BZERO to exactly these bytes, so they are the contract.
func TestUnsignedStoredBytesAreTheSignedOnes(t *testing.T) {
	t.Parallel()

	b := array.NewUint16Builder(memory.NewGoAllocator())
	defer b.Release()

	b.AppendValues([]uint16{0, 32768, math.MaxUint16}, nil)

	arr := b.NewUint16Array()
	defer arr.Release()

	axes := []int64{3}

	src := &fits.ImageHDU{Tensor: tensor.New(arr.Data(), axes, nil, nil), Axes: axes}

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{src}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The payload starts at the second block; three int16 big-endian values.
	payload := buf.Bytes()[fits.BlockSize : fits.BlockSize+6]

	want := []byte{
		0x80, 0x00, // -32768, which is uint16 0 minus 32768
		0x00, 0x00, // 0,      which is uint16 32768 minus 32768
		0x7F, 0xFF, // +32767, which is uint16 65535 minus 32768
	}

	if !bytes.Equal(payload, want) {
		t.Errorf("stored bytes = % x, want % x.\n"+
			"  These are what another reader adds BZERO to, so they are the "+
			"interoperable contract rather than an internal detail.", payload, want)
	}
}
