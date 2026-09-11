package fits_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"

	"github.com/TuSKan/astrogo/fits"
)

// A FITS binary table has no null bitmap. An integer column says "no value" by
// declaring TNULLn and storing that value; a float column says it with NaN
// (FITS 4.0 §7.3.2).
//
// Before this, every null was written as zero — so a missing magnitude became
// magnitude 0, which is a very bright star rather than an absent measurement.

// nullableCatalog builds a batch with a null in each nullable kind.
func nullableCatalog(t *testing.T) arrow.RecordBatch {
	t.Helper()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "HIP", Type: arrow.PrimitiveTypes.Int32, Nullable: true},
		{Name: "VMAG", Type: arrow.PrimitiveTypes.Float32, Nullable: true},
		{Name: "PARALLAX", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "BAND", Type: arrow.PrimitiveTypes.Int16, Nullable: true},
	}, nil)

	rb := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer rb.Release()

	// Row 1 is complete; row 2 is missing everything.
	builderAs[*array.Int32Builder](t, rb, 0).AppendValues([]int32{91262, 0}, []bool{true, false})
	builderAs[*array.Float32Builder](t, rb, 1).AppendValues([]float32{0.03, 0}, []bool{true, false})
	builderAs[*array.Float64Builder](t, rb, 2).AppendValues([]float64{130.23, 0}, []bool{true, false})
	builderAs[*array.Int16Builder](t, rb, 3).AppendValues([]int16{5, 0}, []bool{true, false})

	return rb.NewRecordBatch()
}

func TestNullsSurviveAsNullsNotZeros(t *testing.T) {
	t.Parallel()

	batch := nullableCatalog(t)
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

	for i := range out.Batch.Schema().NumFields() {
		name := out.Batch.Schema().Field(i).Name
		col := out.Batch.Column(i)

		if col.IsNull(0) {
			t.Errorf("column %q row 0 came back null, but it had a value", name)
		}

		if !col.IsNull(1) {
			t.Errorf("column %q row 1 = %s, want null.\n"+
				"  A missing value written as zero is a measurement rather than "+
				"an absence, which is what TNULLn and NaN exist to prevent.",
				name, col.ValueStr(1))
		}
	}

	// The integer columns must declare their sentinel, or a reader has no way
	// to know which stored value means undefined.
	for _, kw := range []string{"TNULL1", "TNULL4"} {
		if _, err := out.Header().GetInt(kw); err != nil {
			t.Errorf("%s is missing: %v", kw, err)
		}
	}
}

// TestAColumnHoldingItsOwnSentinelKeepsItsValues is the safety check.
//
// The sentinel is a stored value like any other, so reserving one that the
// column genuinely contains would turn those measurements into absences — the
// convention's failure mode inverted, and silent. Such a column declares no
// TNULL and keeps every value it has.
func TestAColumnHoldingItsOwnSentinelKeepsItsValues(t *testing.T) {
	t.Parallel()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "OFFSET", Type: arrow.PrimitiveTypes.Int16, Nullable: true},
	}, nil)

	rb := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer rb.Release()

	// math.MinInt16 is the conventional sentinel, and here it is real data.
	builderAs[*array.Int16Builder](t, rb, 0).AppendValues(
		[]int16{math.MinInt16, 0, 7}, []bool{true, false, true})

	batch := rb.NewRecordBatch()
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

	if _, err := out.Header().GetInt("TNULL1"); err == nil {
		t.Error("a TNULL was declared for a column that contains that value as data")
	}

	col, ok := out.Batch.Column(0).(*array.Int16)
	if !ok {
		t.Fatalf("column is %T", out.Batch.Column(0))
	}

	if col.IsNull(0) || col.Value(0) != math.MinInt16 {
		t.Errorf("row 0 = %s, want %d — a real measurement was read as absent",
			col.ValueStr(0), math.MinInt16)
	}

	if col.Value(2) != 7 {
		t.Errorf("row 2 = %d, want 7", col.Value(2))
	}
}

// TestFloatNullsAreNaN pins the other half of the convention: a float column
// expresses absence in the value itself, so it needs no declaration.
func TestFloatNullsAreNaN(t *testing.T) {
	t.Parallel()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "FLUX", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
	}, nil)

	rb := array.NewRecordBuilder(memory.NewGoAllocator(), schema)
	defer rb.Release()

	builderAs[*array.Float64Builder](t, rb, 0).AppendValues([]float64{1.5, 0}, []bool{true, false})

	batch := rb.NewRecordBatch()
	defer batch.Release()

	var buf bytes.Buffer

	err := fits.Write(&buf, &fits.File{HDUs: []fits.HDU{
		&fits.ImageHDU{}, &fits.BintableHDU{Batch: batch},
	}})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	// The stored bytes are what another reader sees, and NaN is what the
	// standard says absence looks like there. Checked as a NaN rather than as
	// exact bits: every quiet NaN means undefined, and Go's math.NaN carries a
	// payload of its own.
	raw := buf.Bytes()
	stored := raw[len(raw)-fits.BlockSize:] // the table's data block

	second := math.Float64frombits(binary.BigEndian.Uint64(stored[8:16]))
	if !math.IsNaN(second) {
		t.Errorf("the missing value was stored as %v, want a NaN", second)
	}

	first := math.Float64frombits(binary.BigEndian.Uint64(stored[0:8]))
	if first != 1.5 {
		t.Errorf("the present value was stored as %v, want 1.5", first)
	}

	// And it comes back as a null rather than as a NaN the caller has to
	// notice.
	got, err := fits.Read(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("Read back: %v", err)
	}

	out, ok := got.HDUs[1].(*fits.BintableHDU)
	if !ok {
		t.Fatalf("extension is %T", got.HDUs[1])
	}

	if !out.Batch.Column(0).IsNull(1) {
		t.Error("a stored NaN did not read back as a null")
	}
}
