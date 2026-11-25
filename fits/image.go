package fits

import (
	"encoding/binary"
	"errors"
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
//
// Axes holds the extents slowest-varying first, the reverse of NAXISn, which
// is the order Tensor and [ImageHDU.Section] index in.
type ImageHDU struct {
	basicHDU

	// Tensor holds the pixels of an image read by [Read], [Open] or
	// [ReadImage], or built by a caller to write. It is nil for an image read
	// by [ReadAt] or [OpenMmap], whose pixels stay in the source until
	// [ImageHDU.Section] reads them.
	Tensor   tensor.Interface
	Axes     []int64
	Bitpix   int
	BScale   float64
	BZero    float64
	Blank    int64
	HasBlank bool

	// src holds a lazily read image's pixels from srcOffset on, stored as
	// format says. It is nil for an image whose pixels are in Tensor.
	src       io.ReaderAt
	srcOffset int64
	format    pixelFormat
}

// ReadImage reads an N-dimensional FITS image payload mapped directly into an Arrow Tensor representation.
func ReadImage(h *Header, r io.Reader) (*ImageHDU, error) {
	hdu, totalPixels, err := newImageHDU(h)
	if err != nil {
		return nil, err
	}

	if totalPixels == 0 {
		return hdu, nil
	}

	format, err := imageFormat(hdu.Bitpix, hdu.BZero)
	if err != nil {
		return nil, err
	}

	totalPayloadBytes, err := mulSize(totalPixels, format.bytes)
	if err != nil {
		return nil, err
	}

	// FITS mandates big-endian byte order for all data.
	// Arrow tensors expect native byte order. Read the raw stream, then
	// convert big-endian → native for multi-byte pixel types.
	rawBytes, err := readDeclared(r, totalPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to read image payload: %w", err)
	}

	buf := memory.NewBufferBytes(rawBytes)

	format.decode(rawBytes)

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
	data := array.NewData(format.dt, int(totalPixels), []*memory.Buffer{nil, buf}, nil, 0, 0)
	defer data.Release()

	// Assign raw tensor instance mapping the memory view
	hdu.Tensor = tensor.New(data, hdu.Axes, nil, nil)

	return hdu, nil
}

// newImageHDU reads what an image HDU's header says about its pixels: the
// axes, BITPIX and the calibration keywords. It returns the HDU without
// pixels, and how many pixels the header declares.
func newImageHDU(h *Header) (*ImageHDU, int64, error) {
	bitpix, err := requiredInt(h, "BITPIX")
	if err != nil {
		return nil, 0, err
	}

	fitsAxes, totalPixels, err := dataAxes(h)
	if err != nil {
		return nil, 0, err
	}

	// FITS axis order is Fortran-contiguous (fastest varying index first).
	// We'll retain the extents for Tensor metadata.
	axes := make([]int64, len(fitsAxes))
	for i, dim := range fitsAxes {
		axes[len(fitsAxes)-1-i] = int64(dim) // C-contiguous flip for Arrow standard
	}

	// Parse BSCALE / BZERO / BLANK from header. Absent, each takes its
	// default; present and unreadable, it is an error rather than the default
	// (#409). The difference is not academic: 16-bit unsigned data carries
	// BZERO = 32768, which Fortran-derived writers put as 3.2768D+04, and
	// until GetFloat read the D exponent that value was taken for absent and
	// every pixel came back 32768 low.
	bscale := 1.0
	if v, err := h.GetFloat("BSCALE"); err == nil {
		bscale = v
	} else if !errors.Is(err, ErrKeyNotFound) {
		return nil, 0, fmt.Errorf("invalid BSCALE: %w", err)
	}

	bzero := 0.0
	if v, err := h.GetFloat("BZERO"); err == nil {
		bzero = v
	} else if !errors.Is(err, ErrKeyNotFound) {
		return nil, 0, fmt.Errorf("invalid BZERO: %w", err)
	}

	var (
		blank    int64
		hasBlank bool
	)

	if v, err := h.GetInt("BLANK"); err == nil {
		blank = int64(v)
		hasBlank = true
	} else if !errors.Is(err, ErrKeyNotFound) {
		return nil, 0, fmt.Errorf("invalid BLANK: %w", err)
	}

	return &ImageHDU{
		header: h, hType: HDUTypeImage,
		Bitpix:   bitpix,
		Axes:     axes,
		BScale:   bscale,
		BZero:    bzero,
		Blank:    blank,
		HasBlank: hasBlank,
	}, totalPixels, nil
}

// pixelFormat is how an image's pixels are stored and the Arrow type that
// holds them once read.
type pixelFormat struct {
	dt       arrow.DataType
	bytes    int64
	unsigned bool
}

// imageFormat returns the pixel format of BITPIX bitpix with offset bzero.
//
// FITS stores unsigned integers wider than a byte as the signed type of the
// same width, offset by BZERO = 2^(n-1) (FITS 4.0 section 5.2.5). A reader that
// ignores that hands back negative pixels for the upper half of an unsigned
// image — 40000 read as -25536 — which is a plausible number in the wrong
// place rather than an error anyone notices.
func imageFormat(bitpix int, bzero float64) (pixelFormat, error) {
	unsigned := isUnsignedOffset(bitpix, bzero)

	switch {
	case bitpix == BitpixUint8:
		return pixelFormat{arrow.PrimitiveTypes.Uint8, 1, false}, nil
	case bitpix == BitpixInt16 && unsigned:
		return pixelFormat{arrow.PrimitiveTypes.Uint16, 2, true}, nil
	case bitpix == BitpixInt16:
		return pixelFormat{arrow.PrimitiveTypes.Int16, 2, false}, nil
	case bitpix == BitpixInt32 && unsigned:
		return pixelFormat{arrow.PrimitiveTypes.Uint32, 4, true}, nil
	case bitpix == BitpixInt32:
		return pixelFormat{arrow.PrimitiveTypes.Int32, 4, false}, nil
	case bitpix == BitpixInt64 && unsigned:
		return pixelFormat{arrow.PrimitiveTypes.Uint64, 8, true}, nil
	case bitpix == BitpixInt64:
		return pixelFormat{arrow.PrimitiveTypes.Int64, 8, false}, nil
	case bitpix == BitpixFloat32:
		return pixelFormat{arrow.PrimitiveTypes.Float32, 4, false}, nil
	case bitpix == BitpixFloat64:
		return pixelFormat{arrow.PrimitiveTypes.Float64, 8, false}, nil
	default:
		return pixelFormat{}, fmt.Errorf("%w: %d", ErrInvalidBitpix, bitpix)
	}
}

// decode turns stored pixels into the values an Arrow tensor of f.dt holds,
// in place: big-endian to native order, and for unsigned data the BZERO added
// back. The stored values are signed; adding BZERO back is what makes them the
// unsigned values they were written from. Done here, on the raw buffer,
// rather than left to PhysicalValue, so the tensor a caller receives has the
// type and the values the file describes.
func (f pixelFormat) decode(raw []byte) {
	swapBigEndianToNative(raw, int(f.bytes))

	if f.unsigned {
		offsetToUnsigned(raw, int(f.bytes))
	}
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

// Type reports that this is an image HDU.
//
// Declared on the concrete type rather than left to the embedded basicHDU,
// whose field is only filled in by the reader. An HDU a caller constructed —
// which is the whole point of having a writer — would otherwise report the
// zero value, and the zero value happens to be HDUTypeImage, so the mistake is
// invisible until the one kind it is wrong for.
func (*ImageHDU) Type() HDUType { return HDUTypeImage }
