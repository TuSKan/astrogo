package fits

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
)

// BintableHDU represents a FITS Binary Table extension.
type BintableHDU struct {
	basicHDU

	Batch   arrow.RecordBatch
	Rows    int
	Cols    int
	RowSize int
}

// tformField is one parsed TFORMn: a repeat count and a FITS data-type code.
type tformField struct {
	// null is the column's TNULLn: the stored value that means "undefined".
	// hasNull distinguishes a declared sentinel of zero from no declaration,
	// which are different things.
	null    int64
	hasNull bool

	repeat int
	code   byte
}

// parseTForm splits a TFORM value such as "1E", "20A" or "J" into its repeat
// count and type code. An absent count means one, per the FITS standard.
func parseTForm(tform string) (tformField, error) {
	t := strings.TrimSpace(tform)

	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}

	repeat := 1

	if i > 0 {
		n, err := strconv.Atoi(t[:i])
		if err != nil {
			return tformField{}, fmt.Errorf("%w: repeat count in %q", ErrBadTForm, tform)
		}

		repeat = n
	}

	if i >= len(t) {
		return tformField{}, fmt.Errorf("%w: no type code in %q", ErrBadTForm, tform)
	}

	return tformField{repeat: repeat, code: t[i]}, nil
}

// width returns the byte width of one element of this field's type.
func (f tformField) width() int {
	switch f.code {
	case 'L', 'B', 'A':
		return 1
	case 'I':
		return 2
	case 'J', 'E':
		return 4
	case 'K', 'D', 'C', 'P':
		return 8
	case 'M', 'Q':
		return 16
	default:
		return 0
	}
}

// size returns the field's total byte width within a row.
func (f tformField) size() int {
	if f.code == 'X' {
		// A bit array: the repeat count is a number of bits.
		return (f.repeat + 7) / 8
	}

	return f.repeat * f.width()
}

// arrowType maps the field to the Arrow type its values are decoded into.
//
// A repeat count above one is a vector per row — a spectrum, a covariance row,
// a per-filter magnitude set — and becomes a fixed-size list of the element
// type. Those columns used to decode as nulls, which discarded the data of
// every table that uses them, and BINTABLEs use them constantly.
//
// A character column is the exception: its repeat count is a width in bytes,
// not a count of values, so "20A" is one twenty-character string rather than
// twenty characters.
func (f tformField) arrowType() arrow.DataType {
	if f.code == 'A' {
		return arrow.BinaryTypes.String
	}

	if f.repeat != 1 {
		elem := f.elementType()

		// A TFORM repeat is bounded by the row width the header declares, so
		// it cannot exceed what an int32 holds; the check is here because that
		// bound lives in the file rather than in the type.
		if elem != arrow.Null && f.repeat > 0 && f.repeat <= math.MaxInt32 {
			n := int32(f.repeat)

			return arrow.FixedSizeListOf(n, elem)
		}

		return arrow.Null
	}

	return f.elementType()
}

// elementType is the Arrow type of one value of this field, ignoring the
// repeat count.
func (f tformField) elementType() arrow.DataType {
	switch f.code {
	case 'L':
		return arrow.FixedWidthTypes.Boolean
	case 'B':
		return arrow.PrimitiveTypes.Uint8
	case 'I':
		return arrow.PrimitiveTypes.Int16
	case 'J':
		return arrow.PrimitiveTypes.Int32
	case 'K':
		return arrow.PrimitiveTypes.Int64
	case 'E':
		return arrow.PrimitiveTypes.Float32
	case 'D':
		return arrow.PrimitiveTypes.Float64
	default:
		return arrow.Null
	}
}

// appendValue decodes one field from a row and appends it to its builder.
//
// FITS binary tables are big-endian regardless of the host, which is why the
// bytes are assembled explicitly rather than cast.
func (f tformField) appendValue(bldr array.Builder, row []byte) {
	// A cell holding the column's declared undefined value is a null, not the
	// number it happens to be stored as. Without this a TNULL of -32768 reads
	// back as the temperature -32768.
	if f.hasNull {
		if v, ok := f.storedInt(row); ok && v == f.null {
			bldr.AppendNull()

			return
		}
	}

	switch b := bldr.(type) {
	case *array.NullBuilder:
		b.AppendNull()
	case *array.FixedSizeListBuilder:
		// A vector cell is its elements laid end to end, each at the element
		// width, so the same decoder runs once per element into the list's
		// value builder.
		b.Append(true)

		width := f.width()
		elem := tformField{repeat: 1, code: f.code, null: f.null, hasNull: f.hasNull}

		for i := range f.repeat {
			elem.appendValue(b.ValueBuilder(), row[i*width:(i+1)*width])
		}
	case *array.StringBuilder:
		b.Append(strings.TrimRightFunc(string(row[:f.repeat]), func(r rune) bool {
			return r == ' ' || r == 0
		}))
	case *array.BooleanBuilder:
		b.Append(row[0] == 'T')
	case *array.Uint8Builder:
		b.Append(row[0])
	case *array.Int16Builder:
		// FITS stores signed integers in two's complement, so this is a
		// reinterpretation of the same bits rather than a range conversion.
		b.Append(int16(binary.BigEndian.Uint16(row))) //nolint:gosec // two's-complement reinterpretation
	case *array.Int32Builder:
		b.Append(int32(binary.BigEndian.Uint32(row))) //nolint:gosec // two's-complement reinterpretation
	case *array.Int64Builder:
		b.Append(int64(binary.BigEndian.Uint64(row))) //nolint:gosec // two's-complement reinterpretation
	case *array.Float32Builder:
		// NaN is how a FITS floating-point column says "no value" — it needs
		// no TNULLn because the representation already has room for it. A
		// reader that passed it through would hand a caller a NaN to notice
		// for themselves, in a column where every other absence is a null.
		if v := math.Float32frombits(binary.BigEndian.Uint32(row)); math.IsNaN(float64(v)) {
			b.AppendNull()
		} else {
			b.Append(v)
		}
	case *array.Float64Builder:
		if v := math.Float64frombits(binary.BigEndian.Uint64(row)); math.IsNaN(v) {
			b.AppendNull()
		} else {
			b.Append(v)
		}
	default:
		bldr.AppendNull()
	}
}

// storedInt reads an integer cell at this field's width, for comparison
// against TNULLn. ok is false for a field that is not an integer one.
func (f tformField) storedInt(row []byte) (int64, bool) {
	switch f.code {
	case 'B':
		return int64(row[0]), true
	case 'I':
		return int64(int16(binary.BigEndian.Uint16(row))), true //nolint:gosec // two's-complement reinterpretation
	case 'J':
		return int64(int32(binary.BigEndian.Uint32(row))), true //nolint:gosec // two's-complement reinterpretation
	case 'K':
		return int64(binary.BigEndian.Uint64(row)), true //nolint:gosec // two's-complement reinterpretation
	default:
		return 0, false
	}
}

// ReadBintable reads a FITS BINTABLE extension into an Arrow record batch.
//
// The payload is row-major and big-endian: each row is NAXIS1 bytes holding
// TFIELDS fields laid out consecutively, each sized by its TFORM. Decoding it
// means walking the rows and transposing into per-column builders.
//
// Scaling keywords (TSCALn, TZEROn) and variable-length arrays (the P and Q
// descriptors) are not applied; a column declaring them decodes to its raw
// stored values. Columns whose TFORM has a repeat count above one are vectors
// per row and are kept as null columns rather than being flattened, so a
// caller never receives one column's numbers under another column's name.
func ReadBintable(h *Header, r io.Reader) (*BintableHDU, error) {
	tfields, err := h.GetInt("TFIELDS")
	if err != nil {
		return nil, fmt.Errorf("missing TFIELDS: %w", err)
	}

	rows, _ := h.GetInt("NAXIS2")
	rowSize, _ := h.GetInt("NAXIS1")

	hdu := &BintableHDU{
		header: h, hType: HDUTypeBinary,
		Rows:    rows,
		Cols:    tfields,
		RowSize: rowSize,
	}

	if rows == 0 || tfields == 0 {
		return hdu, nil
	}

	fields := make([]arrow.Field, tfields)
	parsed := make([]tformField, tfields)
	offsets := make([]int, tfields)

	offset := 0

	for i := range tfields {
		ttype, _ := h.GetString(fmt.Sprintf("TTYPE%d", i+1))
		tform, _ := h.GetString(fmt.Sprintf("TFORM%d", i+1))

		field, err := parseTForm(tform)
		if err != nil {
			return nil, fmt.Errorf("column %d: %w", i+1, err)
		}

		// FITS pads string values out to the width of their card, so a
		// column arrives as "WAVELENGTH " rather than "WAVELENGTH".
		name := strings.TrimSpace(ttype)
		if name == "" {
			name = fmt.Sprintf("COL%d", i+1)
		}

		parsed[i] = field
		offsets[i] = offset
		fields[i] = arrow.Field{Name: name, Type: field.arrowType(), Nullable: true}

		if v, ok := tnullFor(h, i); ok {
			parsed[i].null, parsed[i].hasNull = v, true
		}

		offset += field.size()
	}

	if offset != rowSize {
		return nil, fmt.Errorf("%w: TFORM widths sum to %d bytes, NAXIS1 declares %d",
			ErrBadTForm, offset, rowSize)
	}

	payload := make([]byte, int64(rowSize)*int64(rows))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("failed reading bintable payload: %w", err)
	}

	bldr := array.NewRecordBuilder(memory.NewGoAllocator(), arrow.NewSchema(fields, nil))
	defer bldr.Release()

	for row := range rows {
		base := row * rowSize
		for i := range tfields {
			at := base + offsets[i]
			parsed[i].appendValue(bldr.Field(i), payload[at:at+parsed[i].size()])
		}
	}

	hdu.Batch = bldr.NewRecordBatch()

	// A heap-allocated PCOUNT area follows the table proper, and the whole
	// extension is padded out to a block boundary.
	pcount, err := h.GetInt("PCOUNT")
	if err != nil {
		pcount = 0
	}

	consumed := int64(rowSize)*int64(rows) + int64(pcount)

	if _, err := io.CopyN(io.Discard, r, int64(pcount)); err != nil {
		return nil, fmt.Errorf("failed reading bintable heap: %w", err)
	}

	if pad := consumed % int64(BlockSize); pad != 0 {
		if _, err := io.CopyN(io.Discard, r, int64(BlockSize)-pad); err != nil {
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
		// A null is NaN, not zero. Zero is a measurement — a flux of nothing,
		// a magnitude of nothing — and returning it for an absent value is how
		// a missing sample becomes a real one that nothing downstream can
		// identify. NaN is what the float world means by "no value", it is
		// what FITS stores for an undefined float, and every consumer of this
		// already screens for it.
		if arr.IsNull(i) {
			res[i] = math.NaN()

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
			} else {
				res[i] = math.NaN()
			}
		default:
			res[i] = math.NaN()
		}
	}

	return res, nil
}

// Type reports that this is a binary table HDU. See [ImageHDU.Type] for why it
// is declared here rather than left to the embedded basicHDU.
func (*BintableHDU) Type() HDUType { return HDUTypeBinary }
