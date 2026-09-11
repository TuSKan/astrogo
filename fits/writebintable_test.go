package fits_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/TuSKan/astrogo/fits"
)

// A binary table is the half of #127 that catalogue pipelines need: a table
// astrogo produced has to be readable by the tools the rest of the field uses,
// which means the row-major transpose, the big-endian cells and the fixed-width
// string columns all have to be right at once.

// builderAs returns one of a record builder's field builders, typed.
func builderAs[T array.Builder](t *testing.T, rb *array.RecordBuilder, i int) T {
	t.Helper()

	b, ok := rb.Field(i).(T)
	if !ok {
		t.Fatalf("field %d is %T, not the expected builder type", i, rb.Field(i))
	}

	return b
}

// catalogBatch builds a small catalogue-shaped batch: a name, a position, a
// magnitude and a flag, which between them cover a string column, both float
// widths, an integer and a logical.
func catalogBatch(t *testing.T) arrow.RecordBatch {
	t.Helper()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "NAME", Type: arrow.BinaryTypes.String},
		{Name: "RA", Type: arrow.PrimitiveTypes.Float64},
		{Name: "DEC", Type: arrow.PrimitiveTypes.Float64},
		{Name: "VMAG", Type: arrow.PrimitiveTypes.Float32},
		{Name: "HIP", Type: arrow.PrimitiveTypes.Int32},
		{Name: "VARIABLE", Type: arrow.FixedWidthTypes.Boolean},
	}, nil)

	bldr := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer bldr.Release()

	builderAs[*array.StringBuilder](t, bldr, 0).AppendValues([]string{"Vega", "Betelgeuse", "Sun"}, nil)
	builderAs[*array.Float64Builder](t, bldr, 1).AppendValues([]float64{279.23473479, 88.79293899, 0}, nil)
	builderAs[*array.Float64Builder](t, bldr, 2).AppendValues([]float64{38.78368896, 7.40706400, 0}, nil)
	builderAs[*array.Float32Builder](t, bldr, 3).AppendValues([]float32{0.03, 0.42, -26.74}, nil)
	builderAs[*array.Int32Builder](t, bldr, 4).AppendValues([]int32{91262, 27989, 0}, nil)
	builderAs[*array.BooleanBuilder](t, bldr, 5).AppendValues([]bool{false, true, false}, nil)

	return bldr.NewRecordBatch()
}

func TestWriteBintableRoundTrips(t *testing.T) {
	t.Parallel()

	batch := catalogBatch(t)
	defer batch.Release()

	table := &fits.BintableHDU{Batch: batch}
	table.Header().Append(fits.Card{Keyword: "EXTNAME", Value: "'CATALOG '", Comment: "extension name"})

	file := &fits.File{HDUs: []fits.HDU{&fits.ImageHDU{}, table}}

	var buf bytes.Buffer
	if err := fits.Write(&buf, file); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	if len(got.HDUs) != 2 {
		t.Fatalf("read %d HDUs, want 2 (primary + table)", len(got.HDUs))
	}

	out, ok := got.HDUs[1].(*fits.BintableHDU)
	if !ok {
		t.Fatalf("extension is %T, want *fits.BintableHDU", got.HDUs[1])
	}

	if out.Rows != 3 || out.Cols != 6 {
		t.Fatalf("read %d rows x %d cols, want 3 x 6", out.Rows, out.Cols)
	}

	if name, err := out.Header().GetString("EXTNAME"); err != nil || name != "CATALOG" {
		t.Errorf("EXTNAME = %q (%v), want CATALOG", name, err)
	}

	assertColumnsMatch(t, batch, out.Batch)
}

// assertColumnsMatch compares two batches value by value, by column name.
//
// By name rather than by index, because a column order that silently rotated
// would otherwise pass while putting every star's RA under its neighbour's
// name — which is the failure mode a catalogue writer must not have.
func assertColumnsMatch(t *testing.T, want, got arrow.RecordBatch) {
	t.Helper()

	if got == nil {
		t.Fatal("read batch is nil")
	}

	for i := range want.Schema().NumFields() {
		name := want.Schema().Field(i).Name

		indices := got.Schema().FieldIndices(name)
		if len(indices) == 0 {
			t.Errorf("column %q is missing after the round trip", name)

			continue
		}

		w := want.Column(i)
		g := got.Column(indices[0])

		if w.Len() != g.Len() {
			t.Errorf("column %q has %d rows, want %d", name, g.Len(), w.Len())

			continue
		}

		for row := range w.Len() {
			if ws, gs := w.ValueStr(row), g.ValueStr(row); ws != gs {
				t.Errorf("column %q row %d = %s, want %s", name, row, gs, ws)
			}
		}
	}
}

// TestWriteBintableSizesStringColumnsToTheLongestValue is the fixed-width trap.
//
// FITS character columns have one width for the whole column, so a column sized
// to anything less than its longest value truncates — silently, and only for
// the rows that happen to be long.
func TestWriteBintableSizesStringColumnsToTheLongestValue(t *testing.T) {
	t.Parallel()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "NAME", Type: arrow.BinaryTypes.String},
	}, nil)

	bldr := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer bldr.Release()

	// The long one is last, so a writer sizing from the first row truncates.
	names := []string{"a", "bb", "a rather long designation"}
	builderAs[*array.StringBuilder](t, bldr, 0).AppendValues(names, nil)

	batch := bldr.NewRecordBatch()
	defer batch.Release()

	var buf bytes.Buffer

	err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{
		&fits.ImageHDU{}, &fits.BintableHDU{Batch: batch},
	}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	out, ok := got.HDUs[1].(*fits.BintableHDU)
	if !ok {
		t.Fatalf("extension is %T", got.HDUs[1])
	}

	col, ok := out.Batch.Column(0).(*array.String)
	if !ok {
		t.Fatalf("column is %T, want *array.String", out.Batch.Column(0))
	}

	for i, want := range names {
		if got := col.Value(i); got != want {
			t.Errorf("row %d = %q, want %q — the column was sized to a shorter value", i, got, want)
		}
	}
}

// TestWriteBintableRefusesAnEmptyBatch keeps an uninitialised table from being
// written as a well-formed extension describing no columns.
func TestWriteBintableRefusesAnEmptyBatch(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{
		&fits.ImageHDU{}, &fits.BintableHDU{},
	}})
	if !errors.Is(err, fits.ErrUninitBatch) {
		t.Errorf("Write with no batch = %v, want ErrUninitBatch", err)
	}
}

// TestWriteBintableDerivesRowCountFromTheBatch is the reason the header is
// rebuilt rather than carried over.
//
// A table read from one file and filtered before writing has fewer rows than
// its header says. Trusting the stored NAXIS2 writes a file claiming rows that
// are not there, and a reader believes the header.
func TestWriteBintableDerivesRowCountFromTheBatch(t *testing.T) {
	t.Parallel()

	batch := catalogBatch(t)
	defer batch.Release()

	table := &fits.BintableHDU{Batch: batch}

	// A stale header, as a read-then-filter would leave behind.
	table.Header().Append(fits.Card{Keyword: "NAXIS2", Value: "9999"})
	table.Header().Append(fits.Card{Keyword: "TFIELDS", Value: "99"})

	var buf bytes.Buffer
	if err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{&fits.ImageHDU{}, table}}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	out, ok := got.HDUs[1].(*fits.BintableHDU)
	if !ok {
		t.Fatalf("extension is %T", got.HDUs[1])
	}

	if out.Rows != 3 {
		t.Errorf("NAXIS2 = %d, want 3 — the stale header was written instead of the batch's shape", out.Rows)
	}

	if out.Cols != 6 {
		t.Errorf("TFIELDS = %d, want 6", out.Cols)
	}
}
