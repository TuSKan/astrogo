package fits

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestReadImage(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "8"})
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "NAXIS1", Value: "2"})
	h.Append(Card{Keyword: "NAXIS2", Value: "2"})

	payload := []byte{1, 2, 3, 4}

	// Add padding
	padded := make([]byte, 2880)
	copy(padded, payload)

	r := bytes.NewReader(padded)
	img, err := ReadImage(h, r)
	testutil.AssertNoError(t, err)

	if img.Bitpix != 8 {
		t.Errorf("expected 8 bitpix, got %d", img.Bitpix)
	}
	if len(img.Axes) != 2 {
		t.Errorf("expected 2 axes, got %d", len(img.Axes))
	}
	if img.Tensor == nil {
		t.Errorf("expected Tensor, got nil")
	}

	// Default BSCALE/BZERO
	testutil.AssertEqual(t, "default BScale", img.BScale, 1.0)
	testutil.AssertEqual(t, "default BZero", img.BZero, 0.0)
	testutil.AssertEqual(t, "default HasBlank", img.HasBlank, false)

	// Test Invalid BITPIX
	h2 := NewHeader()
	// BITPIX parsing error
	_, err = ReadImage(h2, bytes.NewReader(nil))
	if err == nil {
		t.Errorf("expected error for missing BITPIX")
	}

	h2.Append(Card{Keyword: "BITPIX", Value: "99"})
	h2.Append(Card{Keyword: "NAXIS", Value: "1"})
	h2.Append(Card{Keyword: "NAXIS1", Value: "1"})
	_, err = ReadImage(h2, bytes.NewReader(nil))
	if err == nil {
		t.Errorf("expected error for invalid BITPIX value")
	}
}

func TestReadImage_Int16_Endian(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "16"})
	h.Append(Card{Keyword: "NAXIS", Value: "1"})
	h.Append(Card{Keyword: "NAXIS1", Value: "2"})

	// Write two Int16 values in big-endian: 256 (0x0100) and -1 (0xFFFF)
	payload := make([]byte, 2880)
	binary.BigEndian.PutUint16(payload[0:], 256)
	binary.BigEndian.PutUint16(payload[2:], uint16(0xFFFF)) // -1 as int16

	img, err := ReadImage(h, bytes.NewReader(payload))
	testutil.AssertNoError(t, err)

	// After endian swap, reading native int16 from the tensor buffer should give correct values.
	buf := img.Tensor.Data().Buffers()[1].Bytes()
	v0 := int16(binary.NativeEndian.Uint16(buf[0:]))
	v1 := int16(binary.NativeEndian.Uint16(buf[2:]))
	testutil.AssertEqual(t, "Int16 pixel 0", v0, int16(256))
	testutil.AssertEqual(t, "Int16 pixel 1", v1, int16(-1))
}

func TestReadImage_Float32_Endian(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "-32"})
	h.Append(Card{Keyword: "NAXIS", Value: "1"})
	h.Append(Card{Keyword: "NAXIS1", Value: "1"})

	// Write one Float32 value in big-endian: 3.14
	payload := make([]byte, 2880)
	bits := math.Float32bits(3.14)
	binary.BigEndian.PutUint32(payload[0:], bits)

	img, err := ReadImage(h, bytes.NewReader(payload))
	testutil.AssertNoError(t, err)

	buf := img.Tensor.Data().Buffers()[1].Bytes()
	v := math.Float32frombits(binary.NativeEndian.Uint32(buf[0:]))
	testutil.AssertNear(t, "Float32 pixel 0", float64(v), 3.14, 1e-5)
}

func TestReadImage_Float64_Endian(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "-64"})
	h.Append(Card{Keyword: "NAXIS", Value: "1"})
	h.Append(Card{Keyword: "NAXIS1", Value: "1"})

	// Write one Float64 value in big-endian: 2.718281828459045
	payload := make([]byte, 2880)
	bits := math.Float64bits(2.718281828459045)
	binary.BigEndian.PutUint64(payload[0:], bits)

	img, err := ReadImage(h, bytes.NewReader(payload))
	testutil.AssertNoError(t, err)

	buf := img.Tensor.Data().Buffers()[1].Bytes()
	v := math.Float64frombits(binary.NativeEndian.Uint64(buf[0:]))
	testutil.AssertNear(t, "Float64 pixel 0", v, 2.718281828459045, 1e-15)
}

func TestReadImage_BScaleBZero(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "8"})
	h.Append(Card{Keyword: "NAXIS", Value: "1"})
	h.Append(Card{Keyword: "NAXIS1", Value: "1"})
	h.Append(Card{Keyword: "BSCALE", Value: "2.5"})
	h.Append(Card{Keyword: "BZERO", Value: "100.0"})

	payload := make([]byte, 2880)
	payload[0] = 10

	img, err := ReadImage(h, bytes.NewReader(payload))
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "BScale", img.BScale, 2.5)
	testutil.AssertEqual(t, "BZero", img.BZero, 100.0)

	// physical = 100.0 + 2.5 * 10 = 125.0
	physical := img.PhysicalValue(10.0)
	testutil.AssertNear(t, "PhysicalValue", physical, 125.0, 1e-10)
}

func TestReadImage_Blank(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "BITPIX", Value: "16"})
	h.Append(Card{Keyword: "NAXIS", Value: "1"})
	h.Append(Card{Keyword: "NAXIS1", Value: "1"})
	h.Append(Card{Keyword: "BLANK", Value: "-32768"})
	h.Append(Card{Keyword: "BSCALE", Value: "1.0"})
	h.Append(Card{Keyword: "BZERO", Value: "32768.0"})

	payload := make([]byte, 2880)
	// Store BLANK sentinel in big-endian
	binary.BigEndian.PutUint16(payload[0:], uint16(0x8000)) // -32768 as int16

	img, err := ReadImage(h, bytes.NewReader(payload))
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "HasBlank", img.HasBlank, true)
	testutil.AssertEqual(t, "Blank", img.Blank, int64(-32768))

	// BLANK value should return NaN
	physical := img.PhysicalValue(-32768.0)
	if !math.IsNaN(physical) {
		t.Errorf("expected NaN for BLANK pixel, got %f", physical)
	}
}
