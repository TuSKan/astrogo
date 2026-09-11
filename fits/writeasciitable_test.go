package fits_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/TuSKan/astrogo/fits"
)

// ASCII tables are the older extension and BINTABLE superseded them, but
// archives are full of them. Writing one is what makes a TABLE read from an
// archive something that can be written back out.

func TestASCIITableRoundTrips(t *testing.T) {
	t.Parallel()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "NAME", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "RA", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "HIP", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
	}, nil)

	rb := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer rb.Release()

	builderAs[*array.StringBuilder](t, rb, 0).AppendValues([]string{"Vega", "Betelgeuse"}, nil)
	builderAs[*array.Float64Builder](t, rb, 1).AppendValues([]float64{279.23473479, 0}, []bool{true, false})
	builderAs[*array.Int32Builder](t, rb, 2).AppendValues([]int32{91262, 27989}, nil)

	batch := rb.NewRecordBatch()
	defer batch.Release()

	var buf bytes.Buffer

	err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{
		&fits.ImageHDU{}, &fits.ASCIITableHDU{Batch: batch},
	}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	out, ok := got.HDUs[1].(*fits.ASCIITableHDU)
	if !ok {
		t.Fatalf("extension is %T, want *fits.ASCIITableHDU", got.HDUs[1])
	}

	if out.Batch == nil {
		t.Fatal("no batch decoded")
	}

	if out.Rows != 2 || out.Cols != 3 {
		t.Fatalf("read %d rows x %d cols, want 2 x 3", out.Rows, out.Cols)
	}

	names, ok := out.Batch.Column(0).(*array.String)
	if !ok {
		t.Fatalf("NAME is %T", out.Batch.Column(0))
	}

	for i, w := range []string{"Vega", "Betelgeuse"} {
		if got := names.Value(i); got != w {
			t.Errorf("NAME row %d = %q, want %q", i, got, w)
		}
	}

	ra, ok := out.Batch.Column(1).(*array.Float64)
	if !ok {
		t.Fatalf("RA is %T", out.Batch.Column(1))
	}

	// A text format has to hold enough digits to read the value back.
	if math.Abs(ra.Value(0)-279.23473479) > 1e-9 {
		t.Errorf("RA row 0 = %.10f, want 279.23473479 — the format lost precision", ra.Value(0))
	}

	// The missing value stays missing rather than becoming zero.
	if !ra.IsNull(1) {
		t.Errorf("RA row 1 = %v, want null — a blank field is undefined, not zero", ra.Value(1))
	}

	counts, ok := out.Batch.Column(2).(*array.Float64)
	if !ok {
		t.Fatalf("HIP is %T", out.Batch.Column(2))
	}

	for i, w := range []float64{91262, 27989} {
		if counts.Value(i) != w {
			t.Errorf("HIP row %d = %v, want %v", i, counts.Value(i), w)
		}
	}
}

// TestASCIITableDeclaresWhereItsColumnsStart pins TBCOLn, which is the only
// statement of a field's position — the columns are positional and a reader has
// nothing else to go on.
func TestASCIITableDeclaresWhereItsColumnsStart(t *testing.T) {
	t.Parallel()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "A", Type: arrow.BinaryTypes.String},
		{Name: "B", Type: arrow.BinaryTypes.String},
	}, nil)

	rb := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer rb.Release()

	builderAs[*array.StringBuilder](t, rb, 0).AppendValues([]string{"xxx"}, nil)
	builderAs[*array.StringBuilder](t, rb, 1).AppendValues([]string{"yy"}, nil)

	batch := rb.NewRecordBatch()
	defer batch.Release()

	var buf bytes.Buffer

	err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{
		&fits.ImageHDU{}, &fits.ASCIITableHDU{Batch: batch},
	}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := fits.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	h := got.HDUs[1].Header()

	// Three characters, a blank, then two: the second field starts at column 5.
	for _, tc := range []struct {
		keyword string
		want    int
	}{
		{keyword: "TBCOL1", want: 1},
		{keyword: "TBCOL2", want: 5},
		{keyword: "NAXIS1", want: 6},
	} {
		if v, err := h.GetInt(tc.keyword); err != nil || v != tc.want {
			t.Errorf("%s = %d (%v), want %d", tc.keyword, v, err, tc.want)
		}
	}
}

// TestASCIITablePadsWithSpaces covers the one place this extension's padding
// differs from every other: a zero byte in an ASCII table's last block is not
// blank text.
func TestASCIITablePadsWithSpaces(t *testing.T) {
	t.Parallel()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "A", Type: arrow.BinaryTypes.String},
	}, nil)

	rb := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer rb.Release()

	builderAs[*array.StringBuilder](t, rb, 0).AppendValues([]string{"x"}, nil)

	batch := rb.NewRecordBatch()
	defer batch.Release()

	var buf bytes.Buffer

	err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{
		&fits.ImageHDU{}, &fits.ASCIITableHDU{Batch: batch},
	}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	data := buf.Bytes()[len(buf.Bytes())-fits.BlockSize:]

	if i := bytes.IndexByte(data, 0); i >= 0 {
		t.Errorf("the data block holds a zero byte at %d; an ASCII table pads with spaces", i)
	}

	if trimmed := strings.TrimRight(string(data), " "); trimmed != "x" {
		t.Errorf("data block = %q, want the single value padded with blanks", trimmed)
	}
}
