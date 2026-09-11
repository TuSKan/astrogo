package fits

import (
	"fmt"
	"strconv"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
)

// encodeASCIITable builds an ASCII TABLE extension's header and payload.
//
// Written for completeness rather than because anything should prefer it:
// BINTABLE stores the same data in a fraction of the space and without the
// rounding a text format imposes. It exists so a TABLE read from an archive can
// be written back out, which was not previously possible in either direction.
func encodeASCIITable(h *ASCIITableHDU) (*Header, []byte, error) {
	if h.Batch == nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrNotWritable, ErrUninitBatch)
	}

	schema := h.Batch.Schema()
	cols := schema.NumFields()
	rows := int(h.Batch.NumRows())

	forms := make([]asciiForm, cols)
	starts := make([]int, cols)

	// One blank column between fields, which is what makes a written table
	// readable as text and is what every writer does.
	offset := 0

	for i := range cols {
		form, err := asciiFormFor(schema.Field(i), h.Batch.Column(i))
		if err != nil {
			return nil, nil, fmt.Errorf("column %d (%s): %w", i+1, schema.Field(i).Name, err)
		}

		forms[i] = form
		starts[i] = offset
		offset += form.width + 1
	}

	rowSize := max(offset-1, 0)

	header := orderedHeader(h.Header(), asciiTableKeywords(schema, forms, starts, rows, rowSize))

	payload, err := asciiPayload(h.Batch, forms, starts, rowSize, rows)
	if err != nil {
		return nil, nil, err
	}

	return header, payload, nil
}

// asciiTableKeywords builds the structural cards, including the TBCOLn that
// say where each field begins.
func asciiTableKeywords(schema *arrow.Schema, forms []asciiForm, starts []int, rows, rowSize int) []Card {
	cards := make([]Card, 0, 8+3*len(forms))
	cards = append(cards,
		Card{Keyword: "XTENSION", Value: "'TABLE   '", Comment: "ASCII table extension"},
		Card{Keyword: "BITPIX", Value: strconv.Itoa(BitpixUint8), Comment: "8-bit bytes"},
		Card{Keyword: "NAXIS", Value: "2", Comment: "2-dimensional ASCII table"},
		Card{Keyword: "NAXIS1", Value: strconv.Itoa(rowSize), Comment: "width of table in characters"},
		Card{Keyword: "NAXIS2", Value: strconv.Itoa(rows), Comment: "number of rows in table"},
		Card{Keyword: "PCOUNT", Value: "0", Comment: "no special data area"},
		Card{Keyword: "GCOUNT", Value: "1", Comment: "one data group"},
		Card{Keyword: "TFIELDS", Value: strconv.Itoa(len(forms)), Comment: "number of fields in each row"},
	)

	for i, form := range forms {
		n := strconv.Itoa(i + 1)

		cards = append(cards,
			Card{Keyword: "TTYPE" + n, Value: quote(schema.Field(i).Name), Comment: "label for field " + n},
			// TBCOLn is one-based, and is the only statement of where a field
			// sits: the columns may have gaps, and a reader has nothing else
			// to go on.
			Card{Keyword: "TBCOL" + n, Value: strconv.Itoa(starts[i] + 1), Comment: "beginning column of field " + n},
			Card{Keyword: "TFORM" + n, Value: quote(form.String()), Comment: "format of field " + n},
		)
	}

	return cards
}

// asciiFormFor chooses a column's text format and width.
//
// The width has to hold every value the column contains, since the fields are
// positional: one value too wide would run into its neighbour and shift every
// column after it. So the column is measured rather than guessed at.
func asciiFormFor(field arrow.Field, col arrow.Array) (asciiForm, error) {
	switch field.Type.ID() { //nolint:exhaustive // the default rejects the rest
	case arrow.STRING:
		return asciiForm{code: 'A', width: max(widestString(col), 1)}, nil
	case arrow.INT16, arrow.INT32, arrow.INT64, arrow.UINT8:
		return asciiForm{code: 'I', width: widestInteger(col)}, nil
	case arrow.FLOAT32, arrow.FLOAT64:
		// Sixteen significant digits in exponential form holds a float64
		// exactly enough to read back as the same value, which a fixed-point
		// format cannot promise across a column's whole range.
		return asciiForm{code: 'E', width: 24, precision: 16}, nil
	default:
		return asciiForm{}, fmt.Errorf("%w: no ASCII table format for %s", ErrNotWritable, field.Type)
	}
}

// widestInteger is the number of characters the widest value in an integer
// column needs, including a sign.
func widestInteger(col arrow.Array) int {
	widest := 1

	for row := range col.Len() {
		if col.IsNull(row) {
			continue
		}

		v, ok := integerAt(col, row)
		if !ok {
			continue
		}

		if n := len(strconv.FormatInt(v, 10)); n > widest {
			widest = n
		}
	}

	return widest
}

// asciiPayload renders the rows as text.
func asciiPayload(batch arrow.RecordBatch, forms []asciiForm, starts []int, rowSize, rows int) ([]byte, error) {
	if rows == 0 || len(forms) == 0 {
		return nil, nil
	}

	payload := make([]byte, rowSize*rows)
	for i := range payload {
		payload[i] = ' '
	}

	for i, form := range forms {
		col := batch.Column(i)

		for row := range rows {
			text, err := asciiCellText(col, form, row)
			if err != nil {
				return nil, fmt.Errorf("column %d row %d: %w", i+1, row+1, err)
			}

			copy(payload[row*rowSize+starts[i]:], text)
		}
	}

	return payload, nil
}

// asciiCellText renders one value into its field.
func asciiCellText(col arrow.Array, form asciiForm, row int) (string, error) {
	if row >= col.Len() || col.IsNull(row) {
		// A blank field is how this format says the value is undefined; there
		// is no TNULL here.
		return form.formatCell(0, "", true)
	}

	switch v := col.(type) {
	case *array.String:
		return form.formatCell(0, v.Value(row), false)
	case *array.Uint8:
		return form.formatCell(float64(v.Value(row)), "", false)
	case *array.Int16:
		return form.formatCell(float64(v.Value(row)), "", false)
	case *array.Int32:
		return form.formatCell(float64(v.Value(row)), "", false)
	case *array.Int64:
		return form.formatCell(float64(v.Value(row)), "", false)
	case *array.Float32:
		return form.formatCell(float64(v.Value(row)), "", false)
	case *array.Float64:
		return form.formatCell(v.Value(row), "", false)
	default:
		return "", fmt.Errorf("%w: no ASCII encoder for %T", ErrNotWritable, col)
	}
}
