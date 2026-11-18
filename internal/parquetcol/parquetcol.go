// Package parquetcol reads the columns of a Parquet file into memory with
// Parquet's own column readers.
//
// It exists so that reading a Parquet table links no more than the Parquet
// format: arrow-go's pqarrow, which builds Arrow tables instead, imports
// arrow/flight and with it 64 gRPC and 34 protobuf packages.
//
// A column is stored typed, one slice of its physical type, so a column of a
// million doubles costs eight megabytes plus a byte per row when it has nulls.
// What is decoded at a time is bounded by a fixed batch, not by the row counts
// a file declares.
//
// Every column read holds the file's rows, one value or null per row: a row
// group whose column holds a different number of rows than the row group
// declares is ErrMalformed, and so is a file whose row groups do not add up to
// the rows it declares. A repeated column, which holds a list per row, is
// ErrUnsupportedType.
package parquetcol

import (
	"errors"
	"fmt"
	"strings"

	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/schema"
)

// Kind is what a value is: a number of one of three kinds, a string, or null.
type Kind uint8

const (
	// Null is a row with no value.
	Null Kind = iota
	// Float is a DOUBLE or FLOAT.
	Float
	// Int is a signed INT32 or INT64.
	Int
	// Uint is an INT32 or INT64 whose logical type is unsigned.
	Uint
	// String is a BYTE_ARRAY.
	String
)

// Value is one row of a column. Only the field its Kind names is set.
type Value struct {
	S    string
	F    float64
	I    int64
	U    uint64
	Kind Kind
}

// Column is one column of a file, every row group's rows in order.
type Column struct {
	f     []float64
	i     []int64
	u     []uint64
	s     []string
	valid []bool // nil while every row has a value
	n     int
	kind  Kind
}

// Len is the column's number of rows.
func (c *Column) Len() int { return c.n }

// At is the value at row, which is Null past the end.
func (c *Column) At(row int) Value {
	if row < 0 || row >= c.n || (c.valid != nil && !c.valid[row]) {
		return Value{}
	}

	switch c.kind {
	case Float:
		return Value{Kind: Float, F: c.f[row]}
	case Int:
		return Value{Kind: Int, I: c.i[row]}
	case Uint:
		return Value{Kind: Uint, U: c.u[row]}
	case String:
		return Value{Kind: String, S: c.s[row]}
	case Null:
		return Value{}
	}

	return Value{}
}

// ErrUnsupportedType marks a column of a physical type this package does not
// read: BOOLEAN, INT96 or FIXED_LEN_BYTE_ARRAY.
var ErrUnsupportedType = errors.New("parquetcol: unsupported column type")

// ErrNoColumn marks a column the file does not have.
var ErrNoColumn = errors.New("parquetcol: no such column")

// ErrMalformed marks a file the Parquet decoder could not read without
// panicking: it sizes what it builds from what the file declares, and a
// malformed file can lead it to slice or allocate out of range. A failed
// allocation is fatal in Go rather than a panic, and is not caught.
var ErrMalformed = errors.New("parquetcol: malformed parquet file")

// Open reads a Parquet file's footer from r. A footer the decoder panics on,
// or whose row groups do not add up to the rows it declares, is ErrMalformed.
func Open(r parquet.ReaderAtSeeker) (rdr *file.Reader, err error) {
	defer recoverMalformed(&err)

	rdr, err = file.NewParquetReader(r)
	if err != nil {
		return nil, fmt.Errorf("parquetcol: %w", err)
	}

	var rows int64
	for g := range rdr.NumRowGroups() {
		rows += rdr.MetaData().RowGroup(g).NumRows()
	}

	if rows != rdr.NumRows() {
		_ = rdr.Close()

		return nil, fmt.Errorf("%w: its row groups hold %d rows, it declares %d", ErrMalformed, rows, rdr.NumRows())
	}

	return rdr, nil
}

// recoverMalformed turns a panic in the decoder into ErrMalformed.
func recoverMalformed(err *error) {
	if r := recover(); r != nil {
		*err = fmt.Errorf("%w: %v", ErrMalformed, r)
	}
}

// batchRows is how many rows of a column are decoded at once.
const batchRows = 4096

// Read reads the column named name from every row group. A file without the
// column is ErrNoColumn.
func Read(rdr *file.Reader, name string) (*Column, error) {
	idx := rdr.MetaData().Schema.ColumnIndexByName(name)
	if idx < 0 {
		return nil, fmt.Errorf("%w: %q", ErrNoColumn, name)
	}

	return readIndex(rdr, idx)
}

// ReadAll reads every leaf column of the file, keyed by its name lowered. A
// column of an unsupported type is left out.
func ReadAll(rdr *file.Reader) (map[string]*Column, error) {
	sch := rdr.MetaData().Schema
	out := make(map[string]*Column, sch.NumColumns())

	for idx := range sch.NumColumns() {
		c, err := readIndex(rdr, idx)
		if errors.Is(err, ErrUnsupportedType) {
			continue
		}

		if err != nil {
			return nil, err
		}

		out[strings.ToLower(sch.Column(idx).Name())] = c
	}

	return out, nil
}

func readIndex(rdr *file.Reader, idx int) (_ *Column, err error) {
	defer recoverMalformed(&err)

	descr := rdr.MetaData().Schema.Column(idx)
	if descr.MaxRepetitionLevel() > 0 {
		return nil, fmt.Errorf("%w: %q is a repeated column", ErrUnsupportedType, descr.Name())
	}

	c := &Column{}

	for g := range rdr.NumRowGroups() {
		rg := rdr.RowGroup(g)

		cr, err := rg.Column(idx)
		if err != nil {
			return nil, fmt.Errorf("parquetcol: column %q, row group %d: %w", descr.Name(), g, err)
		}

		before := c.n
		if err := c.appendChunk(cr, descr); err != nil {
			return nil, fmt.Errorf("parquetcol: column %q, row group %d: %w", descr.Name(), g, err)
		}

		if got := int64(c.n - before); got != rg.NumRows() {
			return nil, fmt.Errorf("%w: column %q, row group %d holds %d rows, it declares %d",
				ErrMalformed, descr.Name(), g, got, rg.NumRows())
		}
	}

	return c, nil
}

// appendChunk appends one row group's rows of cr.
func (c *Column) appendChunk(cr file.ColumnChunkReader, descr *schema.Column) error {
	defs := make([]int16, batchRows)
	maxDef := descr.MaxDefinitionLevel()

	var (
		read func() (int64, error)
		put  func(k int)
	)

	switch r := cr.(type) {
	case *file.Float64ColumnChunkReader:
		vals := make([]float64, batchRows)
		c.kind = Float
		read = batches(r, vals, defs)
		put = func(k int) { c.f = append(c.f, vals[k]) }
	case *file.Float32ColumnChunkReader:
		vals := make([]float32, batchRows)
		c.kind = Float
		read = batches(r, vals, defs)
		put = func(k int) { c.f = append(c.f, float64(vals[k])) }
	case *file.Int32ColumnChunkReader:
		vals := make([]int32, batchRows)
		c.kind = intKind(descr)
		read = batches(r, vals, defs)
		put = func(k int) {
			if c.kind == Uint {
				c.u = append(c.u, uint64(uint32(vals[k]))) //nolint:gosec // an unsigned column's bits, read as stored
			} else {
				c.i = append(c.i, int64(vals[k]))
			}
		}
	case *file.Int64ColumnChunkReader:
		vals := make([]int64, batchRows)
		c.kind = intKind(descr)
		read = batches(r, vals, defs)
		put = func(k int) {
			if c.kind == Uint {
				c.u = append(c.u, uint64(vals[k])) //nolint:gosec // an unsigned column's bits, read as stored
			} else {
				c.i = append(c.i, vals[k])
			}
		}
	case *file.ByteArrayColumnChunkReader:
		vals := make([]parquet.ByteArray, batchRows)
		c.kind = String
		read = batches(r, vals, defs)
		put = func(k int) { c.s = append(c.s, string(vals[k])) }
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedType, descr.PhysicalType())
	}

	for cr.HasNext() {
		levels, err := read()
		if err != nil {
			return fmt.Errorf("reading a batch: %w", err)
		}

		if levels == 0 {
			break
		}

		// Values come packed: one per row whose definition level is the
		// column's maximum, none for a null.
		k := 0

		for r := range int(levels) {
			if maxDef > 0 && defs[r] < maxDef {
				c.null()

				continue
			}

			put(k)
			c.present()

			k++
		}
	}

	return nil
}

// batchReader is a typed column chunk reader of values of type T.
type batchReader[T any] interface {
	ReadBatch(batchSize int64, values []T, defLvls, repLvls []int16) (total int64, valuesRead int, err error)
}

// batches is a reader of r's next batch into vals and defs, returning how
// many rows it read.
func batches[T any](r batchReader[T], vals []T, defs []int16) func() (int64, error) {
	return func() (int64, error) {
		n, _, err := r.ReadBatch(batchRows, vals, defs, nil)
		if err != nil {
			return 0, fmt.Errorf("parquetcol: %w", err)
		}

		return n, nil
	}
}

// present records a row with a value.
func (c *Column) present() {
	if c.valid != nil {
		c.valid = append(c.valid, true)
	}

	c.n++
}

// null records a row without one. Its typed slot holds a zero, so every
// slice stays one entry per row.
func (c *Column) null() {
	if c.valid == nil {
		c.valid = make([]bool, c.n, c.n+batchRows)
		for i := range c.valid {
			c.valid[i] = true
		}
	}

	c.valid = append(c.valid, false)

	switch c.kind {
	case Float:
		c.f = append(c.f, 0)
	case Int:
		c.i = append(c.i, 0)
	case Uint:
		c.u = append(c.u, 0)
	case String:
		c.s = append(c.s, "")
	case Null:
	}

	c.n++
}

// intKind is Uint for an integer column whose logical type is unsigned, and
// Int otherwise.
func intKind(descr *schema.Column) Kind {
	if lt, ok := descr.LogicalType().(schema.IntLogicalType); ok && !lt.IsSigned() {
		return Uint
	}

	return Int
}
