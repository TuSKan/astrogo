package fits

import (
	"fmt"
	"io"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// BintableHDU represents a FITS Binary Table extension.
type BintableHDU struct {
	basicHDU
	Rows    int // NAXIS2
	Cols    int // TFIELDS
	RowSize int // NAXIS1

	// Batch represents the fully converted columnar Dataframe natively inside Apache Arrow.
	Batch arrow.RecordBatch
}

// ReadBintable scaffolds the ingestion of BINTABLE payloads structurally translating them into Arrow schemas.
func ReadBintable(h *Header, r io.Reader) (*BintableHDU, error) {
	tfields, err := h.GetInt("TFIELDS")
	if err != nil {
		return nil, fmt.Errorf("missing TFIELDS: %w", err)
	}

	rows, _ := h.GetInt("NAXIS2")
	rowSize, _ := h.GetInt("NAXIS1")

	hdu := &BintableHDU{
		basicHDU: basicHDU{header: h, hType: HDUTypeBinary},
		Rows:     rows,
		Cols:     tfields,
		RowSize:  rowSize,
	}

	if rows == 0 || tfields == 0 {
		return hdu, nil
	}

	fields := make([]arrow.Field, tfields)
	for i := 1; i <= tfields; i++ {
		ttype, _ := h.GetString(fmt.Sprintf("TTYPE%d", i))
		tform, _ := h.GetString(fmt.Sprintf("TFORM%d", i))

		// Basic mapper matching FITS Data types to Arrow standard datatypes
		tform = strings.TrimSpace(tform)
		var dt arrow.DataType
		if strings.HasSuffix(tform, "J") { // 32-bit int
			dt = arrow.PrimitiveTypes.Int32
		} else if strings.HasSuffix(tform, "K") { // 64-bit int
			dt = arrow.PrimitiveTypes.Int64
		} else if strings.HasSuffix(tform, "E") { // float32
			dt = arrow.PrimitiveTypes.Float32
		} else if strings.HasSuffix(tform, "D") { // float64
			dt = arrow.PrimitiveTypes.Float64
		} else if strings.Contains(tform, "A") { // Characters
			dt = arrow.BinaryTypes.String
		} else {
			dt = arrow.PrimitiveTypes.Float64 // Fallback
		}

		fields[i-1] = arrow.Field{Name: ttype, Type: dt}
	}

	schema := arrow.NewSchema(fields, nil)
	mem := memory.NewGoAllocator()
	bldr := array.NewRecordBuilder(mem, schema)
	defer bldr.Release()

	// Constructing columns from row-based binary buffers requires traversing the chunk stream.
	// For P1 prototype scaffolding we build out empty RecordBatches.
	bldr.Reserve(rows)
	hdu.Batch = bldr.NewRecordBatch()

	// Consume entire payload exactly to advance stream
	totalPayloadBytes := int64(rowSize) * int64(rows)
	pcount, err := h.GetInt("PCOUNT")
	if err == nil {
		totalPayloadBytes += int64(pcount)
	}

	// Just read out the bytes and toss them to skip
	skipBuf := make([]byte, totalPayloadBytes)
	if _, err := io.ReadFull(r, skipBuf); err != nil {
		return nil, fmt.Errorf("failed allocating/reading bintable payload: %w", err)
	}

	paddingBytes := int(totalPayloadBytes) % BlockSize
	if paddingBytes != 0 {
		padLen := BlockSize - paddingBytes
		padBuf := make([]byte, padLen)
		if _, err := io.ReadFull(r, padBuf); err != nil {
			return nil, fmt.Errorf("failed reading bintable padding: %w", err)
		}
	}

	return hdu, nil
}
