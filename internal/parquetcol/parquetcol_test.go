package parquetcol_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/metadata"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/TuSKan/astrogo/internal/parquetcol"
)

// fixture writes a table of n rows holding one column of each kind
// parquetcol reads, and one it does not, in row groups of rowGroup rows. Every
// third row of the nullable columns is null.
func fixture(tb testing.TB, n, rowGroup int) []byte {
	tb.Helper()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "dbl", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "flt", Type: arrow.PrimitiveTypes.Float32},
		{Name: "i32", Type: arrow.PrimitiveTypes.Int32},
		{Name: "i64", Type: arrow.PrimitiveTypes.Int64, Nullable: true},
		{Name: "u64", Type: arrow.PrimitiveTypes.Uint64},
		{Name: "Name", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "flag", Type: arrow.FixedWidthTypes.Boolean},
	}, nil)

	b := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer b.Release()

	dbl := builder[*array.Float64Builder](tb, b, 0)
	flt := builder[*array.Float32Builder](tb, b, 1)
	i32 := builder[*array.Int32Builder](tb, b, 2)
	i64 := builder[*array.Int64Builder](tb, b, 3)
	u64 := builder[*array.Uint64Builder](tb, b, 4)
	str := builder[*array.StringBuilder](tb, b, 5)
	flag := builder[*array.BooleanBuilder](tb, b, 6)

	for i := range n {
		if i%3 == 2 {
			dbl.AppendNull()
			i64.AppendNull()
			str.AppendNull()
		} else {
			dbl.Append(float64(i) + 0.5)
			i64.Append(-int64(i))
			str.Append("row" + string(rune('a'+i%26)))
		}

		flt.Append(float32(i) / 4)
		i32.Append(int32(i))
		u64.Append(math.MaxUint64 - uint64(i))
		flag.Append(i%2 == 0)
	}

	rec := b.NewRecordBatch()
	defer rec.Release()

	tbl := array.NewTableFromRecords(schema, []arrow.RecordBatch{rec})
	defer tbl.Release()

	var buf bytes.Buffer

	props := parquet.NewWriterProperties(parquet.WithMaxRowGroupLength(int64(rowGroup)))
	if err := pqarrow.WriteTable(tbl, &buf, int64(rowGroup), props, pqarrow.DefaultWriterProps()); err != nil {
		tb.Fatalf("writing the fixture: %v", err)
	}

	return buf.Bytes()
}

// builder is the record builder's field i as the builder type T.
func builder[T any](tb testing.TB, b *array.RecordBuilder, i int) T {
	tb.Helper()

	fb, ok := b.Field(i).(T)
	if !ok {
		tb.Fatalf("field %d is a %T", i, b.Field(i))
	}

	return fb
}

func open(tb testing.TB, raw []byte) *file.Reader {
	tb.Helper()

	rdr, err := file.NewParquetReader(bytes.NewReader(raw))
	if err != nil {
		tb.Fatalf("opening the fixture: %v", err)
	}

	tb.Cleanup(func() { _ = rdr.Close() })

	return rdr
}

// TestEveryKindReadsBackAsWritten: each column reads back row for row,
// across row groups and across the reader's batches, nulls where they were
// written and values in their kind. 10,000 rows in groups of 3,000 cross both
// boundaries, a row group's and a batch's.
func TestEveryKindReadsBackAsWritten(t *testing.T) {
	t.Parallel()

	const rows = 10000

	rdr := open(t, fixture(t, rows, 3000))

	cols, err := parquetcol.ReadAll(rdr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if _, ok := cols["flag"]; ok {
		t.Error("the BOOLEAN column was read; ReadAll leaves out what it does not read")
	}

	for _, name := range []string{"dbl", "flt", "i32", "i64", "u64", "name"} {
		if c, ok := cols[name]; !ok || c.Len() != rows {
			t.Fatalf("column %q: present %v, want %d rows", name, ok, rows)
		}
	}

	for i := range rows {
		null := i%3 == 2

		want := map[string]parquetcol.Value{
			"flt": {Kind: parquetcol.Float, F: float64(float32(i) / 4)},
			"i32": {Kind: parquetcol.Int, I: int64(i)},
			"u64": {Kind: parquetcol.Uint, U: math.MaxUint64 - uint64(i)},
		}

		if null {
			want["dbl"], want["i64"], want["name"] = parquetcol.Value{}, parquetcol.Value{}, parquetcol.Value{}
		} else {
			want["dbl"] = parquetcol.Value{Kind: parquetcol.Float, F: float64(i) + 0.5}
			want["i64"] = parquetcol.Value{Kind: parquetcol.Int, I: -int64(i)}
			want["name"] = parquetcol.Value{Kind: parquetcol.String, S: "row" + string(rune('a'+i%26))}
		}

		for name, w := range want {
			if got := cols[name].At(i); got != w {
				t.Fatalf("row %d of %q = %+v, want %+v", i, name, got, w)
			}
		}
	}
}

// TestReadOneColumn: Read finds a column by its name as the file writes it,
// refuses one the file lacks and one of a type it does not read, and reads
// past the end as null.
func TestReadOneColumn(t *testing.T) {
	t.Parallel()

	rdr := open(t, fixture(t, 5, 5))

	c, err := parquetcol.Read(rdr, "Name")
	if err != nil || c == nil || c.Len() != 5 {
		t.Fatalf("Read(Name) = %v, %v; want its 5 rows", c, err)
	}

	if v := c.At(5); v.Kind != parquetcol.Null {
		t.Errorf("past the end = %+v, want null", v)
	}

	if _, err := parquetcol.Read(rdr, "absent"); !errors.Is(err, parquetcol.ErrNoColumn) {
		t.Errorf("Read(absent) = %v, want ErrNoColumn", err)
	}

	if _, err := parquetcol.Read(rdr, "flag"); !errors.Is(err, parquetcol.ErrUnsupportedType) {
		t.Errorf("Read(flag) = %v, want ErrUnsupportedType", err)
	}
}

// TestEveryColumnHoldsTheFilesRows: a row group whose column holds a
// different number of rows than the row group declares is ErrMalformed when
// the column is read, even with the row groups still adding up to the file's
// rows, and a footer whose row groups do not add up to the rows it declares is
// ErrMalformed at Open.
func TestEveryColumnHoldsTheFilesRows(t *testing.T) {
	t.Parallel()

	raw := fixture(t, 10, 4) // row groups of 4, 4 and 2 rows

	shifted := rewriteFooter(t, raw, func(md *metadata.FileMetaData) {
		md.RowGroups[0].NumRows++
		md.RowGroups[1].NumRows--
	})

	rdr, err := parquetcol.Open(bytes.NewReader(shifted))
	if err != nil {
		t.Fatalf("Open: %v; the row groups still add up to the file's rows", err)
	}

	t.Cleanup(func() { _ = rdr.Close() })

	if _, err := parquetcol.Read(rdr, "dbl"); !errors.Is(err, parquetcol.ErrMalformed) {
		t.Errorf("Read(dbl) of a row group declaring 5 rows and holding 4 = %v, want ErrMalformed", err)
	}

	if _, err := parquetcol.ReadAll(rdr); !errors.Is(err, parquetcol.ErrMalformed) {
		t.Errorf("ReadAll of a row group declaring 5 rows and holding 4 = %v, want ErrMalformed", err)
	}

	overstated := rewriteFooter(t, raw, func(md *metadata.FileMetaData) { md.NumRows = 1 << 40 })
	if _, err := parquetcol.Open(bytes.NewReader(overstated)); !errors.Is(err, parquetcol.ErrMalformed) {
		t.Errorf("Open of a file declaring 2^40 rows in row groups holding 10 = %v, want ErrMalformed", err)
	}
}

// TestARepeatedColumnIsUnsupported: a column holding a list per row is
// refused by Read and left out by ReadAll, whose other columns still read.
func TestARepeatedColumnIsUnsupported(t *testing.T) {
	t.Parallel()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "id", Type: arrow.PrimitiveTypes.Int64},
		{Name: "list", Type: arrow.ListOf(arrow.PrimitiveTypes.Int64)},
	}, nil)

	b := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer b.Release()

	ids := builder[*array.Int64Builder](t, b, 0)
	lists := builder[*array.ListBuilder](t, b, 1)
	values := lists.ValueBuilder().(*array.Int64Builder) //nolint:forcetypeassert // a list of Int64 builds Int64 values

	for i := range 3 {
		ids.Append(int64(i))
		lists.Append(true)

		for k := range i + 2 {
			values.Append(int64(k))
		}
	}

	rec := b.NewRecordBatch()
	defer rec.Release()

	tbl := array.NewTableFromRecords(schema, []arrow.RecordBatch{rec})
	defer tbl.Release()

	var buf bytes.Buffer
	if err := pqarrow.WriteTable(tbl, &buf, 3, nil, pqarrow.DefaultWriterProps()); err != nil {
		t.Fatalf("writing the table: %v", err)
	}

	rdr := open(t, buf.Bytes())

	if _, err := parquetcol.Read(rdr, "list.list.element"); !errors.Is(err, parquetcol.ErrUnsupportedType) {
		t.Errorf("Read(list) = %v, want ErrUnsupportedType", err)
	}

	cols, err := parquetcol.ReadAll(rdr)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if len(cols) != 1 || cols["id"] == nil || cols["id"].Len() != 3 {
		t.Errorf("ReadAll = %v, want only id, of 3 rows", cols)
	}
}

// rewriteFooter is raw with its footer's metadata changed by change and
// written again, the column chunks left where they are.
func rewriteFooter(tb testing.TB, raw []byte, change func(*metadata.FileMetaData)) []byte {
	tb.Helper()

	md := open(tb, raw).MetaData()
	body := raw[:len(raw)-8-md.Size()]

	change(md)

	var buf bytes.Buffer

	buf.Write(body)

	n, err := md.WriteTo(&buf, nil)
	if err != nil {
		tb.Fatalf("writing the footer: %v", err)
	}

	buf.Write(binary.LittleEndian.AppendUint32(nil, uint32(n)))
	buf.WriteString("PAR1")

	return buf.Bytes()
}

// TestADecoderPanicIsErrMalformed: the SSOFT table that panicked the decoder
// in #1062 is ErrMalformed, from Open or from the column reads. It is checked
// in as a FuzzReadAll crasher, which is where it is read from.
func TestADecoderPanicIsErrMalformed(t *testing.T) {
	t.Parallel()

	raw := crasher(t, "testdata/fuzz/FuzzReadAll/fink-1062")

	rdr, err := parquetcol.Open(bytes.NewReader(raw))
	if err == nil {
		defer func() { _ = rdr.Close() }()

		_, err = parquetcol.ReadAll(rdr)
	}

	if !errors.Is(err, parquetcol.ErrMalformed) {
		t.Fatalf("reading the #1062 table: %v, want ErrMalformed", err)
	}
}

// crasher is the one []byte value of a checked-in fuzz corpus file.
func crasher(tb testing.TB, name string) []byte {
	tb.Helper()

	data, err := os.ReadFile(name)
	if err != nil {
		tb.Fatal(err)
	}

	header, value, ok := strings.Cut(strings.TrimSpace(string(data)), "\n")
	if !ok || header != "go test fuzz v1" {
		tb.Fatalf("%s is not a fuzz corpus file", name)
	}

	quoted, ok := strings.CutPrefix(value, "[]byte(")
	if !ok {
		tb.Fatalf("%s does not hold a []byte", name)
	}

	s, err := strconv.Unquote(strings.TrimSuffix(quoted, ")"))
	if err != nil {
		tb.Fatalf("%s: %v", name, err)
	}

	return []byte(s)
}

// FuzzReadAll: any bytes, opened as a Parquet file and every column decoded,
// end in a table or an error, never a panic or a hang. A panic in the
// decoder is this package's to turn into ErrMalformed.
func FuzzReadAll(f *testing.F) {
	valid := fixture(f, 40, 16)
	f.Add(valid)
	f.Add(valid[:len(valid)/2])
	f.Add([]byte("PAR1"))
	f.Add([]byte("PAR1\x00\x00\x00\x00PAR1"))

	f.Fuzz(func(_ *testing.T, raw []byte) {
		rdr, err := parquetcol.Open(bytes.NewReader(raw))
		if err != nil {
			return
		}

		defer func() { _ = rdr.Close() }()

		_, _ = parquetcol.ReadAll(rdr)
	})
}
