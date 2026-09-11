package fits

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/arrow/tensor"
)

// BITPIX constants
const (
	BitpixUint8   = 8
	BitpixInt16   = 16
	BitpixInt32   = 32
	BitpixInt64   = 64
	BitpixFloat32 = -32
	BitpixFloat64 = -64
)

// ImageHDU represents a FITS image matrix (N-dimensional).
type ImageHDU struct {
	basicHDU

	Tensor   tensor.Interface
	Axes     []int64
	Bitpix   int
	BScale   float64
	BZero    float64
	Blank    int64
	HasBlank bool
}

// ReadImage reads an N-dimensional FITS image payload mapped directly into an Arrow Tensor representation.
func ReadImage(h *Header, r io.Reader) (*ImageHDU, error) {
	bitpix, err := h.GetInt("BITPIX")
	if err != nil {
		return nil, fmt.Errorf("missing or invalid BITPIX: %w", err)
	}

	naxis, err := h.GetInt("NAXIS")
	if err != nil {
		return nil, fmt.Errorf("missing or invalid NAXIS: %w", err)
	}

	axes := make([]int64, naxis)

	var totalPixels int64 = 1

	for i := 1; i <= naxis; i++ {
		dim, err := h.GetInt(fmt.Sprintf("NAXIS%d", i))
		if err != nil {
			return nil, fmt.Errorf("missing NAXIS%d: %w", i, err)
		}
		// FITS axis order is Fortran-contiguous (fastest varying index first).
		// We'll retain the extents for Tensor metadata.
		axes[naxis-i] = int64(dim) // C-contiguous flip for Arrow standard
		totalPixels *= int64(dim)
	}

	if naxis == 0 {
		totalPixels = 0
	}

	// Parse BSCALE / BZERO / BLANK from header.
	bscale := 1.0
	if v, err := h.GetFloat("BSCALE"); err == nil {
		bscale = v
	}

	bzero := 0.0
	if v, err := h.GetFloat("BZERO"); err == nil {
		bzero = v
	}

	var (
		blank    int64
		hasBlank bool
	)

	if v, err := h.GetInt("BLANK"); err == nil {
		blank = int64(v)
		hasBlank = true
	}

	hdu := &ImageHDU{
		header: h, hType: HDUTypeImage,
		Bitpix:   bitpix,
		Axes:     axes,
		BScale:   bscale,
		BZero:    bzero,
		Blank:    blank,
		HasBlank: hasBlank,
	}

	if totalPixels == 0 {
		return hdu, nil
	}

	// Calculate total bytes
	var (
		pixelBytes int64
		dt         arrow.DataType
	)

	// FITS stores unsigned integers wider than a byte as the signed type of
	// the same width, offset by BZERO = 2^(n-1) (FITS 4.0 section 5.2.5). A
	// reader that ignores that hands back negative pixels for the upper half
	// of an unsigned image — 40000 read as -25536 — which is a plausible
	// number in the wrong place rather than an error anyone notices.
	unsigned := isUnsignedOffset(bitpix, bzero)

	switch {
	case bitpix == BitpixUint8:
		pixelBytes = 1
		dt = arrow.PrimitiveTypes.Uint8
	case bitpix == BitpixInt16 && unsigned:
		pixelBytes = 2
		dt = arrow.PrimitiveTypes.Uint16
	case bitpix == BitpixInt16:
		pixelBytes = 2
		dt = arrow.PrimitiveTypes.Int16
	case bitpix == BitpixInt32 && unsigned:
		pixelBytes = 4
		dt = arrow.PrimitiveTypes.Uint32
	case bitpix == BitpixInt32:
		pixelBytes = 4
		dt = arrow.PrimitiveTypes.Int32
	case bitpix == BitpixInt64 && unsigned:
		pixelBytes = 8
		dt = arrow.PrimitiveTypes.Uint64
	case bitpix == BitpixInt64:
		pixelBytes = 8
		dt = arrow.PrimitiveTypes.Int64
	case bitpix == BitpixFloat32:
		pixelBytes = 4
		dt = arrow.PrimitiveTypes.Float32
	case bitpix == BitpixFloat64:
		pixelBytes = 8
		dt = arrow.PrimitiveTypes.Float64
	default:
		return nil, fmt.Errorf("%w: %d", ErrInvalidBitpix, bitpix)
	}

	totalPayloadBytes := totalPixels * pixelBytes

	// Allocate Arrow Buffer
	mem := memory.NewGoAllocator()
	buf := memory.NewResizableBuffer(mem)
	buf.Resize(int(totalPayloadBytes))

	// FITS mandates big-endian byte order for all data.
	// Arrow tensors expect native byte order. Read the raw stream, then
	// convert big-endian → native for multi-byte pixel types.
	rawBytes := buf.Bytes()

	_, err = io.ReadFull(r, rawBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read image payload: %w", err)
	}

	swapBigEndianToNative(rawBytes, int(pixelBytes))

	// The stored values are signed; adding BZERO back is what makes them the
	// unsigned values they were written from. Done here, on the raw buffer,
	// rather than left to PhysicalValue, so the tensor a caller receives has
	// the type and the values the file describes.
	if unsigned {
		offsetToUnsigned(rawBytes, int(pixelBytes))
	}

	// After reading the payload, discard the padding to reach 2880 byte bounds
	paddingBytes := int(totalPayloadBytes) % BlockSize
	if paddingBytes != 0 {
		padLen := BlockSize - paddingBytes

		padBuf := make([]byte, padLen)
		if _, err := io.ReadFull(r, padBuf); err != nil {
			return nil, fmt.Errorf("failed to read padding block: %w", err)
		}
	}

	// Tensor instantiation
	// A new buffer needs to be mapped to the native tensor wrapper.
	data := array.NewData(dt, int(totalPixels), []*memory.Buffer{nil, buf}, nil, 0, 0)
	defer data.Release()

	// Assign raw tensor instance mapping the memory view
	hdu.Tensor = tensor.New(data, axes, nil, nil)

	return hdu, nil
}

// swapBigEndianToNative converts a byte buffer from big-endian to native byte order in-place.
// bytesPerPixel must be 1, 2, 4, or 8. For bytesPerPixel==1, this is a no-op.
func swapBigEndianToNative(buf []byte, bytesPerPixel int) {
	switch bytesPerPixel {
	case 1:
		// No swap needed for single-byte data.
	case 2:
		for i := 0; i < len(buf); i += 2 {
			v := binary.BigEndian.Uint16(buf[i:])
			binary.NativeEndian.PutUint16(buf[i:], v)
		}
	case 4:
		for i := 0; i < len(buf); i += 4 {
			v := binary.BigEndian.Uint32(buf[i:])
			binary.NativeEndian.PutUint32(buf[i:], v)
		}
	case 8:
		for i := 0; i < len(buf); i += 8 {
			v := binary.BigEndian.Uint64(buf[i:])
			binary.NativeEndian.PutUint64(buf[i:], v)
		}
	}
}

// PhysicalValue applies the BSCALE/BZERO linear calibration transform to a stored pixel value:
//
//	physical = BZero + BScale * stored
//
// If the stored value matches the BLANK sentinel (integer BITPIX only, when HasBlank is true),
// NaN is returned to indicate an undefined pixel.
func (img *ImageHDU) PhysicalValue(stored float64) float64 {
	if img.HasBlank && int64(stored) == img.Blank {
		return math.NaN()
	}

	return img.BZero + img.BScale*stored
}

// offsetToUnsigned adds the unsigned BZERO back in place, in native byte order.
//
// The inverse of the writer's offsetToSigned, and the same XOR of the sign bit:
// for an offset of 2^(n-1) the two directions are one operation.
func offsetToUnsigned(buf []byte, pixelBytes int) {
	switch pixelBytes {
	case 2:
		for i := 0; i+2 <= len(buf); i += 2 {
			binary.NativeEndian.PutUint16(buf[i:], binary.NativeEndian.Uint16(buf[i:])^0x8000)
		}
	case 4:
		for i := 0; i+4 <= len(buf); i += 4 {
			binary.NativeEndian.PutUint32(buf[i:], binary.NativeEndian.Uint32(buf[i:])^0x80000000)
		}
	case 8:
		for i := 0; i+8 <= len(buf); i += 8 {
			binary.NativeEndian.PutUint64(buf[i:], binary.NativeEndian.Uint64(buf[i:])^0x8000000000000000)
		}
	}
}
