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

	// A column with nulls needs somewhere to put them, and the sentinel has to
	// be chosen before the header is built, since it is declared there.
	nulls := nullSentinels(h.Batch, forms)

	cards := bintableKeywords(schema, forms, rows, rowSize)

	for i, sentinel := range nulls {
		if sentinel.used {
			cards = append(cards, tnullCard(i, sentinel.value))
		}
	}

	header := orderedHeader(h.Header(), cards)

	payload, err := bintablePayload(h.Batch, forms, widths, rowSize, rows, nulls)
	if err != nil {
		return nil, nil, err
	}

	return header, payload, nil
}

// columnNull is the undefined value one column uses, if it has one.
type columnNull struct {
	value int64
	used  bool
}

// nullSentinels decides, per column, whether a TNULLn is needed and safe.
//
// Needed when the column actually contains a null. Safe when the sentinel does
// not also occur as real data: reserving a value the column genuinely holds
// would turn those measurements into absences on the next read, which is the
// convention's own failure mode inverted. A column that collides keeps its
// values and its nulls become zeros, which is what this did everywhere before.
func nullSentinels(batch arrow.RecordBatch, forms []tformField) []columnNull {
	out := make([]columnNull, len(forms))

	for i, form := range forms {
		col := batch.Column(i)

		if col.NullN() == 0 {
			continue
		}

		sentinel, ok := nullSentinel(form.code)
		if !ok {
			continue
		}

		if columnContains(col, sentinel) {
			continue
		}

		out[i] = columnNull{value: sentinel, used: true}
	}

	return out
}

// columnContains reports whether an integer column holds v as real data.
func columnContains(col arrow.Array, v int64) bool {
	for row := range col.Len() {
		if col.IsNull(row) {
			continue
		}

		got, ok := integerAt(col, row)
		if ok && got == v {
			return true
		}
	}

	return false
}

// integerAt reads one integer cell, widened. ok is false for a column that is
// not an integer one.
func integerAt(col arrow.Array, row int) (int64, bool) {
	switch v := col.(type) {
	case *array.Uint8:
		return int64(v.Value(row)), true
	case *array.Int16:
		return int64(v.Value(row)), true
	case *array.Int32:
		return int64(v.Value(row)), true
	case *array.Int64:
		return v.Value(row), true
	default:
		return 0, false
	}
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
	case arrow.FIXED_SIZE_LIST:
		return vectorTForm(field)
	default:
		return tformField{}, fmt.Errorf("%w: no TFORM for Arrow type %s", ErrNotWritable, field.Type)
	}
}

// vectorTForm builds the TFORM for a fixed-size list column: the element's
// code with the list's length as its repeat count.
func vectorTForm(field arrow.Field) (tformField, error) {
	list, ok := field.Type.(*arrow.FixedSizeListType)
	if !ok {
		return tformField{}, fmt.Errorf("%w: %s is not a fixed-size list", ErrNotWritable, field.Type)
	}

	elem, err := tformFor(arrow.Field{Name: field.Name, Type: list.Elem()}, nil)
	if err != nil {
		return tformField{}, err
	}

	if elem.code == 'A' {
		return tformField{}, fmt.Errorf("%w: a list of strings has no FITS column type", ErrNotWritable)
	}

	return tformField{repeat: int(list.Len()), code: elem.code}, nil
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
func bintablePayload(batch arrow.RecordBatch, forms []tformField, widths []int, rowSize, rows int, nulls []columnNull) ([]byte, error) {
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

		if err := writeColumn(payload, col, form, offset, rowSize, rows, nulls[i]); err != nil {
			return nil, fmt.Errorf("column %d: %w", i+1, err)
		}
	}

	return payload, nil
}

// writeColumn places one column's values into their slot in every row.
//
// A null goes in as the column's declared TNULLn where it has one, as NaN for a
// float column, and as the type's zero otherwise — a logical or character
// column, or an integer column whose sentinel would have collided with real
// data. Which of those applies was decided in nullSentinels, before the header
// was written, since a TNULLn has to be declared to mean anything.
func writeColumn(payload []byte, col arrow.Array, form tformField, offset, rowSize, rows int, null columnNull) error {
	for row := range rows {
		at := row*rowSize + offset
		cell := payload[at : at+form.size()]

		if row >= col.Len() || col.IsNull(row) {
			writeNull(cell, form, null)

			continue
		}

		if list, ok := col.(*array.FixedSizeList); ok {
			if err := writeVector(cell, list, form, row); err != nil {
				return err
			}

			continue
		}

		if err := writeCell(cell, col, row); err != nil {
			return err
		}
	}

	return nil
}

// writeNull fills one cell with whatever this column means by "undefined".
func writeNull(cell []byte, form tformField, null columnNull) {
	switch {
	case null.used:
		putInt(cell, null.value)
	case form.code == 'E':
		binary.BigEndian.PutUint32(cell, math.Float32bits(float32(math.NaN())))
	case form.code == 'D':
		binary.BigEndian.PutUint64(cell, math.Float64bits(math.NaN()))
	case form.code == 'A':
		for i := range cell {
			cell[i] = ' '
		}
	default:
		// Already zero, which is what a logical column's undefined value is.
	}
}

// putInt writes v big-endian at the cell's own width.
func putInt(cell []byte, v int64) {
	switch len(cell) {
	case 1:
		// A B column is one unsigned byte, and its sentinel is 255; the
		// value reaching here came from nullSentinel and is in range.
		cell[0] = byte(v & 0xFF)
	case 2:
		binary.BigEndian.PutUint16(cell, uint16(v)) //nolint:gosec // two's-complement bit pattern, the FITS I encoding
	case 4:
		binary.BigEndian.PutUint32(cell, uint32(v)) //nolint:gosec // two's-complement bit pattern, the FITS J encoding
	case 8:
		binary.BigEndian.PutUint64(cell, uint64(v)) //nolint:gosec // two's-complement bit pattern, the FITS K encoding
	}
}

// writeVector encodes a fixed-size list cell: its elements end to end, each at
// the element width.
func writeVector(cell []byte, list *array.FixedSizeList, form tformField, row int) error {
	values := list.ListValues()
	width := form.width()
	start := row * form.repeat

	for i := range form.repeat {
		at := start + i
		if at >= values.Len() {
			break
		}

		elem := cell[i*width : (i+1)*width]

		if values.IsNull(at) {
			writeNull(elem, tformField{repeat: 1, code: form.code}, columnNull{})

			continue
		}

		if err := writeCell(elem, values, at); err != nil {
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
