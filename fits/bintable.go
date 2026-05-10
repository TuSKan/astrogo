package fits

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// BintableHDU represents a FITS Binary Table extension.
type BintableHDU struct {
	Batch arrow.RecordBatch
	basicHDU
	Rows    int
	Cols    int
	RowSize int
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

		switch {
		case strings.HasSuffix(tform, "J"): // 32-bit int
			dt = arrow.PrimitiveTypes.Int32
		case strings.HasSuffix(tform, "K"): // 64-bit int
			dt = arrow.PrimitiveTypes.Int64
		case strings.HasSuffix(tform, "E"): // float32
			dt = arrow.PrimitiveTypes.Float32
		case strings.HasSuffix(tform, "D"): // float64
			dt = arrow.PrimitiveTypes.Float64
		case strings.Contains(tform, "A"): // Characters
			dt = arrow.BinaryTypes.String
		default:
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

// GetStringColumn extracts a FITS binary table column and safely converts it to a standard Go string slice.
func (hdu *BintableHDU) GetStringColumn(colName string) ([]string, error) {
	if hdu.Batch == nil {
		return nil, ErrUninitBatch
	}

	schema := hdu.Batch.Schema()
	idx := -1

	for i, f := range schema.Fields() {
		if f.Name == colName {
			idx = i
			break
		}
	}

	if idx < 0 {
		return nil, fmt.Errorf("%w: %s", ErrColumnNotFound, colName)
	}

	arr := hdu.Batch.Column(idx)
	rows := int(hdu.Batch.NumRows())
	res := make([]string, rows)

	for i := range rows {
		if arr.IsNull(i) {
			continue
		}

		switch a := arr.(type) {
		case *array.String:
			res[i] = a.Value(i)
		case *array.Int32:
			res[i] = strconv.FormatInt(int64(a.Value(i)), 10)
		case *array.Int64:
			res[i] = strconv.FormatInt(a.Value(i), 10)
		case *array.Float32:
			res[i] = strconv.FormatFloat(float64(a.Value(i)), 'f', -1, 32)
		case *array.Float64:
			res[i] = strconv.FormatFloat(a.Value(i), 'f', -1, 64)
		}
	}

	return res, nil
}

// GetFloatColumn extracts a FITS binary table column safely into a standard Go float64 slice.
func (hdu *BintableHDU) GetFloatColumn(colName string) ([]float64, error) {
	if hdu.Batch == nil {
		return nil, ErrUninitBatch
	}

	schema := hdu.Batch.Schema()
	idx := -1

	for i, f := range schema.Fields() {
		if f.Name == colName {
			idx = i
			break
		}
	}

	if idx < 0 {
		return nil, fmt.Errorf("%w: %s", ErrColumnNotFound, colName)
	}

	arr := hdu.Batch.Column(idx)
	rows := int(hdu.Batch.NumRows())
	res := make([]float64, rows)

	for i := range rows {
		if arr.IsNull(i) {
			continue
		}

		switch a := arr.(type) {
		case *array.Float64:
			res[i] = a.Value(i)
		case *array.Float32:
			res[i] = float64(a.Value(i))
		case *array.Int32:
			res[i] = float64(a.Value(i))
		case *array.Int64:
			res[i] = float64(a.Value(i))
		case *array.String:
			if val, err := strconv.ParseFloat(a.Value(i), 64); err == nil {
				res[i] = val
			}
		}
	}

	return res, nil
}
