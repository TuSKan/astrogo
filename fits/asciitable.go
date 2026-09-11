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

// ASCIITableHDU represents a FITS ASCII Table extension (TABLE).
//
// The older of the two table extensions: values are stored as text in
// fixed-width columns, positioned by TBCOLn and formatted by TFORMn. BINTABLE
// superseded it and is what modern pipelines write, but archives are full of
// these and a reader that skips them cannot read those archives.
type ASCIITableHDU struct {
	basicHDU

	// Batch holds the decoded columns, in the same shape a BintableHDU uses,
	// so a caller reading one kind of table reads the other the same way.
	Batch arrow.RecordBatch

	Rows    int // NAXIS2
	Cols    int // TFIELDS
	RowSize int // NAXIS1
}

// ReadASCIITable decodes an ASCII TABLE extension into an Arrow record batch.
//
// Numeric columns become float64 and character columns become strings. A field
// of all blanks is undefined — the format's own way of saying "no value", with
// no TNULL involved — and becomes a null rather than a zero.
func ReadASCIITable(h *Header, r io.Reader) (*ASCIITableHDU, error) {
	tfields, err := h.GetInt("TFIELDS")
	if err != nil {
		return nil, fmt.Errorf("missing TFIELDS: %w", err)
	}

	rows, _ := h.GetInt("NAXIS2")
	rowSize, _ := h.GetInt("NAXIS1")

	hdu := &ASCIITableHDU{
		header: h, hType: HDUTypeASCII,
		Rows:    rows,
		Cols:    tfields,
		RowSize: rowSize,
	}

	if rows == 0 || tfields == 0 {
		return hdu, discardPadding(r, int64(rowSize)*int64(rows))
	}

	forms, starts, fields, err := asciiColumns(h, tfields, rowSize)
	if err != nil {
		return nil, err
	}

	payload := make([]byte, int64(rowSize)*int64(rows))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("fits: failed reading asciitable payload: %w", err)
	}

	bldr := array.NewRecordBuilder(memory.NewGoAllocator(), arrow.NewSchema(fields, nil))
	defer bldr.Release()

	for row := range rows {
		line := payload[row*rowSize : (row+1)*rowSize]

		for i, form := range forms {
			end := min(starts[i]+form.width, len(line))
			if starts[i] >= len(line) {
				bldr.Field(i).AppendNull()

				continue
			}

			if err := appendASCIICell(bldr.Field(i), form, string(line[starts[i]:end])); err != nil {
				return nil, fmt.Errorf("fits: row %d column %d: %w", row+1, i+1, err)
			}
		}
	}

	hdu.Batch = bldr.NewRecordBatch()

	return hdu, discardPadding(r, int64(rowSize)*int64(rows))
}

// asciiColumns reads the per-column keywords: where each field starts, how it
// is formatted, and what to call it.
func asciiColumns(h *Header, tfields, rowSize int) (forms []asciiForm, starts []int, fields []arrow.Field, err error) {
	forms = make([]asciiForm, tfields)
	starts = make([]int, tfields)
	fields = make([]arrow.Field, tfields)

	for i := range tfields {
		n := strconv.Itoa(i + 1)

		tform, err := h.GetString("TFORM" + n)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fits: missing TFORM%s: %w", n, err)
		}

		form, err := parseASCIIForm(tform)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fits: column %s: %w", n, err)
		}

		// TBCOLn is 1-based, and is the only statement of where a field sits:
		// ASCII table columns may have gaps between them.
		tbcol, err := h.GetInt("TBCOL" + n)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("fits: missing TBCOL%s: %w", n, err)
		}

		if tbcol < 1 || tbcol+form.width-1 > rowSize {
			return nil, nil, nil, fmt.Errorf("%w: column %s starts at %d and is %d wide, "+
				"which does not fit a row of %d", ErrBadTForm, n, tbcol, form.width, rowSize)
		}

		name, _ := h.GetString("TTYPE" + n)
		if strings.TrimSpace(name) == "" {
			name = "COL" + n
		}

		forms[i] = form
		starts[i] = tbcol - 1
		fields[i] = arrow.Field{Name: strings.TrimSpace(name), Type: form.arrowType(), Nullable: true}
	}

	return forms, starts, fields, nil
}

// arrowType is the Arrow type an ASCII column decodes into.
func (f asciiForm) arrowType() arrow.DataType {
	if f.code == 'A' {
		return arrow.BinaryTypes.String
	}

	// Every numeric format becomes float64. The format writes text, so an I
	// column's values are exact integers either way, and one type keeps the
	// caller from having to branch on TFORM to read a number.
	return arrow.PrimitiveTypes.Float64
}

// appendASCIICell decodes one field and appends it.
func appendASCIICell(bldr array.Builder, form asciiForm, text string) error {
	value, str, blank, err := form.parseCell(text)
	if err != nil {
		return err
	}

	if blank {
		bldr.AppendNull()

		return nil
	}

	switch b := bldr.(type) {
	case *array.StringBuilder:
		b.Append(str)
	case *array.Float64Builder:
		b.Append(value)
	default:
		bldr.AppendNull()
	}

	return nil
}

// discardPadding consumes an extension's trailing block padding.
func discardPadding(r io.Reader, consumed int64) error {
	if pad := consumed % int64(BlockSize); pad != 0 {
		if _, err := io.CopyN(io.Discard, r, int64(BlockSize)-pad); err != nil {
			return fmt.Errorf("fits: failed reading asciitable padding: %w", err)
		}
	}

	return nil
}

// Type reports that this is an ASCII table HDU. See [ImageHDU.Type] for why it
// is declared here rather than left to the embedded basicHDU.
func (*ASCIITableHDU) Type() HDUType { return HDUTypeASCII }
