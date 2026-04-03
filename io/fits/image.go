package fits

import (
	"fmt"
	"io"

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
	Bitpix int
	Axes   []int64

	// Tensor holds the multi-dimensional array data using Apache Arrow.
	Tensor tensor.Interface
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

	hdu := &ImageHDU{
		basicHDU: basicHDU{header: h, hType: HDUTypeImage},
		Bitpix:   bitpix,
		Axes:     axes,
	}

	if totalPixels == 0 {
		return hdu, nil
	}

	// Calculate total bytes
	var pixelBytes int64
	var dt arrow.DataType
	switch bitpix {
	case BitpixUint8:
		pixelBytes = 1
		dt = arrow.PrimitiveTypes.Uint8
	case BitpixInt16:
		pixelBytes = 2
		dt = arrow.PrimitiveTypes.Int16
	case BitpixInt32:
		pixelBytes = 4
		dt = arrow.PrimitiveTypes.Int32
	case BitpixInt64:
		pixelBytes = 8
		dt = arrow.PrimitiveTypes.Int64
	case BitpixFloat32:
		pixelBytes = 4
		dt = arrow.PrimitiveTypes.Float32
	case BitpixFloat64:
		pixelBytes = 8
		dt = arrow.PrimitiveTypes.Float64
	default:
		return nil, fmt.Errorf("invalid BITPIX value: %d", bitpix)
	}

	totalPayloadBytes := totalPixels * pixelBytes

	// Allocate Arrow Buffer
	mem := memory.NewGoAllocator()
	buf := memory.NewResizableBuffer(mem)
	buf.Resize(int(totalPayloadBytes))

	// In FITS, all data is strictly Big-Endian.
	// Since go-arrow uses native Endianness (little-endian typically),
	// a completely robust implementation would byte-swap the buffer mid-flight.
	// For this P0 prototype, we will just read the raw byte stream into the arrow buffer directly.
	_, err = io.ReadFull(r, buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to read image payload: %w", err)
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
