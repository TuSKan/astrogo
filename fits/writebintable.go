package fits

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// encodeBintable builds a binary table extension's header and payload from its
// Arrow batch.
//
// The batch is the source of truth for the shape — column count, row count,
// types and names — and the stored header supplies everything else. A caller
// who filtered rows out of the batch gets a file whose NAXIS2 matches what it
// contains rather than what it was read with.
func encodeBintable(h *BintableHDU) (*Header, []byte, error) {
	if h.Batch == nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrNotWritable, ErrUninitBatch)
	}

	schema := h.Batch.Schema()
	cols := schema.NumFields()
	rows := int(h.Batch.NumRows())

	forms := make([]tformField, cols)
	widths := make([]int, cols)
	rowSize := 0

	for i := range cols {
		field := schema.Field(i)

		form, err := tformFor(field, h.Batch.Column(i))
		if err != nil {
			return nil, nil, fmt.Errorf("column %d (%s): %w", i+1, field.Name, err)
		}

		forms[i] = form
		widths[i] = form.size()
		rowSize += widths[i]
	}

	header := orderedHeader(h.Header(), bintableKeywords(schema, forms, rows, rowSize))

	payload, err := bintablePayload(h.Batch, forms, widths, rowSize, rows)
	if err != nil {
		return nil, nil, err
	}

	return header, payload, nil
}

// bintableKeywords builds the structural cards for a binary table.
func bintableKeywords(schema *arrow.Schema, forms []tformField, rows, rowSize int) []Card {
	cards := make([]Card, 0, 8+2*len(forms))
	cards = append(cards,
		Card{Keyword: "XTENSION", Value: "'BINTABLE'", Comment: "binary table extension"},
		Card{Keyword: "BITPIX", Value: strconv.Itoa(BitpixUint8), Comment: "8-bit bytes"},
		Card{Keyword: "NAXIS", Value: "2", Comment: "2-dimensional binary table"},
		Card{Keyword: "NAXIS1", Value: strconv.Itoa(rowSize), Comment: "width of table in bytes"},
		Card{Keyword: "NAXIS2", Value: strconv.Itoa(rows), Comment: "number of rows in table"},
		Card{Keyword: "PCOUNT", Value: "0", Comment: "size of special data area"},
		Card{Keyword: "GCOUNT", Value: "1", Comment: "one data group"},
		Card{Keyword: "TFIELDS", Value: strconv.Itoa(len(forms)), Comment: "number of fields in each row"},
	)

	for i, form := range forms {
		n := strconv.Itoa(i + 1)

		cards = append(cards,
			Card{
				Keyword: "TTYPE" + n,
				Value:   quote(schema.Field(i).Name),
				Comment: "label for field " + n,
			},
			Card{
				Keyword: "TFORM" + n,
				Value:   quote(strconv.Itoa(form.repeat) + string(form.code)),
				Comment: "data format of field " + n,
			},
		)
	}

	return cards
}

// tformFor chooses the TFORM for one Arrow column.
//
// A string column's repeat count is its width in bytes, which has to be the
// longest value present: FITS character columns are fixed-width, so a column
// sized to anything less would silently truncate. Measuring it means walking
// the column once, which is cheap beside writing it.
//
// listing the forty it has no column for would say the same thing at length.
//
//nolint:exhaustive // the default rejects every Arrow type FITS cannot store;
func tformFor(field arrow.Field, col arrow.Array) (tformField, error) {
	switch field.Type.ID() {
	case arrow.BOOL:
		return tformField{repeat: 1, code: 'L'}, nil
	case arrow.UINT8:
		return tformField{repeat: 1, code: 'B'}, nil
	case arrow.INT16:
		return tformField{repeat: 1, code: 'I'}, nil
	case arrow.INT32:
		return tformField{repeat: 1, code: 'J'}, nil
	case arrow.INT64:
		return tformField{repeat: 1, code: 'K'}, nil
	case arrow.FLOAT32:
		return tformField{repeat: 1, code: 'E'}, nil
	case arrow.FLOAT64:
		return tformField{repeat: 1, code: 'D'}, nil
	case arrow.STRING:
		return tformField{repeat: max(widestString(col), 1), code: 'A'}, nil
	default:
		return tformField{}, fmt.Errorf("%w: no TFORM for Arrow type %s", ErrNotWritable, field.Type)
	}
}

// widestString returns the longest value in a string column, in bytes.
func widestString(col arrow.Array) int {
	sa, ok := col.(*array.String)
	if !ok {
		return 0
	}

	widest := 0

	for i := range sa.Len() {
		if sa.IsNull(i) {
			continue
		}

		if n := len(sa.Value(i)); n > widest {
			widest = n
		}
	}

	return widest
}

// bintablePayload writes the batch row by row.
//
// FITS binary tables are row-major and Arrow is columnar, so this transposes —
// the inverse of what ReadBintable does on the way in. It is written as a
// whole-payload buffer rather than streamed because the row width is already
// known and a table that does not fit in memory would not have fit in the
// Arrow batch it came from.
func bintablePayload(batch arrow.RecordBatch, forms []tformField, widths []int, rowSize, rows int) ([]byte, error) {
	if rows == 0 || len(forms) == 0 {
		return nil, nil
	}

	payload := make([]byte, rowSize*rows)

	for i, form := range forms {
		col := batch.Column(i)

		offset := 0
		for j := range i {
			offset += widths[j]
		}

		if err := writeColumn(payload, col, form, offset, rowSize, rows); err != nil {
			return nil, fmt.Errorf("column %d: %w", i+1, err)
		}
	}

	return payload, nil
}

// writeColumn places one column's values into their slot in every row.
//
// A null is written as the type's zero rather than skipped. FITS binary tables
// have no null bitmap: absence is expressed by TNULLn for integers and by NaN
// for floats, and inventing a TNULLn here would reserve a value that might be
// real data. Zero is the honest choice for a format with nowhere to say
// "missing", and it is what the reader will hand back.
func writeColumn(payload []byte, col arrow.Array, form tformField, offset, rowSize, rows int) error {
	for row := range rows {
		at := row*rowSize + offset
		cell := payload[at : at+form.size()]

		if row >= col.Len() || col.IsNull(row) {
			continue // already zero
		}

		if err := writeCell(cell, col, row); err != nil {
			return err
		}
	}

	return nil
}

// writeCell encodes one value in FITS byte order.
//
// The integer cases reinterpret a signed value as its two's-complement bit
// pattern, which is exactly what FITS stores — a J column holds a signed 32-bit
// integer big-endian, and PutUint32 is how Go writes those four bytes. Nothing
// is truncated: the widths match on both sides.
func writeCell(cell []byte, col arrow.Array, row int) error {
	switch v := col.(type) {
	case *array.Boolean:
		// FITS writes a logical as the ASCII letter, not as 0/1.
		cell[0] = 'F'
		if v.Value(row) {
			cell[0] = 'T'
		}
	case *array.Uint8:
		cell[0] = v.Value(row)
	case *array.Int16:
		binary.BigEndian.PutUint16(cell, uint16(v.Value(row))) //nolint:gosec // two's-complement bit pattern, the FITS I encoding
	case *array.Int32:
		binary.BigEndian.PutUint32(cell, uint32(v.Value(row))) //nolint:gosec // two's-complement bit pattern, the FITS J encoding
	case *array.Int64:
		binary.BigEndian.PutUint64(cell, uint64(v.Value(row))) //nolint:gosec // two's-complement bit pattern, the FITS K encoding
	case *array.Float32:
		binary.BigEndian.PutUint32(cell, math.Float32bits(v.Value(row)))
	case *array.Float64:
		binary.BigEndian.PutUint64(cell, math.Float64bits(v.Value(row)))
	case *array.String:
		// Blank-padded, not NUL-padded, and truncated never: the column was
		// sized to the longest value in tformFor.
		s := v.Value(row)
		copy(cell, s)

		for i := len(s); i < len(cell); i++ {
			cell[i] = ' '
		}
	default:
		return fmt.Errorf("%w: no encoder for Arrow array %T", ErrNotWritable, col)
	}

	return nil
}

// quote wraps a value in the single quotes a FITS string card carries,
// doubling any quote inside it as the standard requires.
func quote(s string) string {
	out := make([]byte, 0, len(s)+2)
	out = append(out, '\'')

	for i := range len(s) {
		if s[i] == '\'' {
			out = append(out, '\'')
		}

		out = append(out, s[i])
	}

	return string(append(out, '\''))
}
