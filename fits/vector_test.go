package fits_test

import (
	"bytes"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/TuSKan/astrogo/fits"
)

// A TFORM repeat count above one is a vector per row — a spectrum, a
// covariance row, a magnitude per filter — and BINTABLEs use them constantly.
//
// These columns used to decode as Arrow nulls, so every value in them was
// discarded on read: a table of 2048-channel spectra came back as a column of
// nothing, with no error.

// spectrumBatch builds a table with a vector column beside a scalar one.
func spectrumBatch(t *testing.T) arrow.RecordBatch {
	t.Helper()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "OBJECT", Type: arrow.BinaryTypes.String},
		{Name: "FLUX", Type: arrow.FixedSizeListOf(4, arrow.PrimitiveTypes.Float32)},
		{Name: "COUNTS", Type: arrow.FixedSizeListOf(3, arrow.PrimitiveTypes.Int32)},
	}, nil)

	rb := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer rb.Release()

	builderAs[*array.StringBuilder](t, rb, 0).AppendValues([]string{"M31", "M42"}, nil)

	flux, ok := rb.Field(1).(*array.FixedSizeListBuilder)
	if !ok {
		t.Fatalf("FLUX builder is %T", rb.Field(1))
	}

	fluxValues, ok := flux.ValueBuilder().(*array.Float32Builder)
	if !ok {
		t.Fatalf("FLUX value builder is %T", flux.ValueBuilder())
	}

	for _, row := range [][]float32{{1.5, 2.5, 3.5, 4.5}, {-1, 0, 1, 2}} {
		flux.Append(true)
		fluxValues.AppendValues(row, nil)
	}

	counts, ok := rb.Field(2).(*array.FixedSizeListBuilder)
	if !ok {
		t.Fatalf("COUNTS builder is %T", rb.Field(2))
	}

	countValues, ok := counts.ValueBuilder().(*array.Int32Builder)
	if !ok {
		t.Fatalf("COUNTS value builder is %T", counts.ValueBuilder())
	}

	for _, row := range [][]int32{{10, 20, 30}, {-5, 0, 5}} {
		counts.Append(true)
		countValues.AppendValues(row, nil)
	}

	return rb.NewRecordBatch()
}

func TestVectorColumnsRoundTrip(t *testing.T) {
	t.Parallel()

	batch := spectrumBatch(t)
	t.Cleanup(batch.Release)

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

	// The repeat count is the vector length, and it is what another reader
	// uses to lay the cell out.
	for _, tc := range []struct {
		keyword string
		want    string
	}{
		{keyword: "TFORM2", want: "4E"},
		{keyword: "TFORM3", want: "3J"},
	} {
		if v, err := out.Header().GetString(tc.keyword); err != nil || v != tc.want {
			t.Errorf("%s = %q (%v), want %q", tc.keyword, v, err, tc.want)
		}
	}

	assertFloatVector(t, out.Batch, "FLUX", [][]float32{{1.5, 2.5, 3.5, 4.5}, {-1, 0, 1, 2}})
	assertIntVector(t, out.Batch, "COUNTS", [][]int32{{10, 20, 30}, {-5, 0, 5}})

	// The scalar column beside them must be unaffected: a vector column that
	// laid its cell out wrongly would shift every column after it.
	names, ok := out.Batch.Column(0).(*array.String)
	if !ok {
		t.Fatalf("OBJECT is %T", out.Batch.Column(0))
	}

	for i, w := range []string{"M31", "M42"} {
		if got := names.Value(i); got != w {
			t.Errorf("OBJECT row %d = %q, want %q — the row layout is wrong", i, got, w)
		}
	}
}

// assertFloatVector checks a float vector column value by value.
func assertFloatVector(t *testing.T, batch arrow.RecordBatch, name string, want [][]float32) {
	t.Helper()

	list := vectorColumn(t, batch, name)

	values, ok := list.ListValues().(*array.Float32)
	if !ok {
		t.Fatalf("%s values are %T, want *array.Float32", name, list.ListValues())
	}

	n := int(list.DataType().(*arrow.FixedSizeListType).Len()) //nolint:forcetypeassert // checked by vectorColumn

	for row, wantRow := range want {
		for i, w := range wantRow {
			if got := values.Value(row*n + i); got != w {
				t.Errorf("%s row %d element %d = %v, want %v", name, row, i, got, w)
			}
		}
	}
}

// assertIntVector checks an integer vector column value by value.
func assertIntVector(t *testing.T, batch arrow.RecordBatch, name string, want [][]int32) {
	t.Helper()

	list := vectorColumn(t, batch, name)

	values, ok := list.ListValues().(*array.Int32)
	if !ok {
		t.Fatalf("%s values are %T, want *array.Int32", name, list.ListValues())
	}

	n := int(list.DataType().(*arrow.FixedSizeListType).Len()) //nolint:forcetypeassert // checked by vectorColumn

	for row, wantRow := range want {
		for i, w := range wantRow {
			if got := values.Value(row*n + i); got != w {
				t.Errorf("%s row %d element %d = %v, want %v", name, row, i, got, w)
			}
		}
	}
}

// vectorColumn returns a named column as a fixed-size list.
func vectorColumn(t *testing.T, batch arrow.RecordBatch, name string) *array.FixedSizeList {
	t.Helper()

	idx := batch.Schema().FieldIndices(name)
	if len(idx) == 0 {
		t.Fatalf("no column named %q", name)
	}

	col := batch.Column(idx[0])

	list, ok := col.(*array.FixedSizeList)
	if !ok {
		t.Fatalf("column %q is %T, want *array.FixedSizeList — a vector column "+
			"decoded as something else discards its values", name, col)
	}

	if _, ok := list.DataType().(*arrow.FixedSizeListType); !ok {
		t.Fatalf("column %q has type %T", name, list.DataType())
	}

	return list
}
