package starlight

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/internal/votable"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

// resultRows is one aggregation result, read a row at a time.
//
// The aggregation is the same table however it arrives, so the accumulation
// reads it through this rather than through a format. Two implementations back
// it: [csvRows], which is what a synchronous query returns, and
// [parquetRows], which is what the asynchronous endpoint returns and the
// better of the two — columnar, typed and compressed, so a whole-sky result is
// a quarter the bytes and no number passes through a decimal string on the way
// in.
//
// A column absent from the result is reported as absent rather than as zero.
// The difference matters for the colour-recovery columns, which older results
// do not carry: a missing column means "no correction available" and a zero
// would mean "nothing was dropped", and those call for opposite behaviour.
type resultRows interface {
	// Next advances to the next row, reporting whether there is one.
	Next() bool

	// Has reports whether the result carries a column at all.
	//
	// Distinct from Number returning false, and the distinction is the one
	// this interface exists to keep. A column missing from the result means
	// the query never asked for it, which is a malformed build and an error;
	// a null value in a column that exists means that pixel had nothing to
	// contribute, which is ordinary and is skipped. Reading them the same way
	// turns a band nobody selected into a band that is silently zero
	// everywhere.
	Has(column string) bool

	// Number returns a column's value for the current row.
	Number(column string) (float64, bool)

	// Err reports what stopped the iteration, if anything did.
	Err() error
}

// csvRows reads the text form.
type csvRows struct {
	reader *csv.Reader
	index  map[string]int
	row    []string
	err    error
}

// newCSVRows reads the header and prepares to stream the body.
func newCSVRows(r io.Reader) (*csvRows, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	index := make(map[string]int, len(header))
	for i, name := range header {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}

	return &csvRows{reader: reader, index: index}, nil
}

func (c *csvRows) Next() bool {
	row, err := c.reader.Read()
	if errors.Is(err, io.EOF) {
		return false
	}

	if err != nil {
		c.err = fmt.Errorf("%w: %w", ErrGaiaResponse, err)

		return false
	}

	c.row = row

	return true
}

func (c *csvRows) Has(column string) bool {
	_, ok := c.index[column]

	return ok
}

func (c *csvRows) Number(column string) (float64, bool) {
	i, ok := c.index[column]
	if !ok || i >= len(c.row) {
		return 0, false
	}

	v, err := strconv.ParseFloat(strings.TrimSpace(c.row[i]), 64)
	if err != nil {
		return 0, false
	}

	return v, true
}

func (c *csvRows) Err() error { return c.err }

// parquetRows reads the columnar form.
//
// The whole table is materialised, which is what the format requires: the
// footer holding the schema and the row-group offsets is at the end of the
// file, so it cannot be streamed from the front. A whole-sky four-band result
// is around sixty megabytes this way against two hundred and forty-five as
// CSV, so materialising it is cheaper than streaming the alternative.
type parquetRows struct {
	columns map[string]arrow.Array
	rows    int64
	at      int64
	release func()
	err     error
}

// newParquetRows reads a Parquet result into columns addressable by name.
func newParquetRows(ctx context.Context, r io.Reader) (*parquetRows, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	// The magic is worth checking by hand. A service that refuses a query
	// answers with an XML document under the same content type, and the
	// reader's own error for that is about a bad footer rather than about the
	// refusal the body actually describes.
	if len(raw) < 4 || string(raw[:4]) != "PAR1" {
		summary := strings.Join(strings.Fields(string(raw)), " ")
		if len(summary) > 300 {
			summary = summary[:300] + "..."
		}

		return nil, fmt.Errorf("%w: the response is not Parquet: %s", ErrGaiaResponse, summary)
	}

	pf, err := file.NewParquetReader(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, memory.DefaultAllocator)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	table, err := reader.ReadTable(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	out := &parquetRows{
		columns: make(map[string]arrow.Array, int(table.NumCols())),
		rows:    table.NumRows(),
		at:      -1,
		release: table.Release,
	}

	for i := range int(table.NumCols()) {
		col := table.Column(i)

		chunks := col.Data().Chunks()
		if len(chunks) != 1 {
			// Concatenating would work and would double the memory for a
			// result this size. The reader returns one chunk per column for
			// what this package asks for, so a second chunk means the shape
			// changed and the caller should know rather than pay for it
			// silently.
			table.Release()

			return nil, fmt.Errorf("%w: column %q arrived in %d chunks, want one",
				ErrGaiaResponse, col.Name(), len(chunks))
		}

		out.columns[strings.ToLower(col.Name())] = chunks[0]
	}

	return out, nil
}

func (p *parquetRows) Next() bool {
	p.at++

	return p.at < p.rows
}

// Number reads a column as a float, whatever width the service chose for it.
//
// The archive types these columns as it sees fit — a count as INT64, a summed
// flux as DOUBLE, a mean colour as FLOAT — and every one of them is a number
// this package divides or multiplies. Converting at the edge keeps that choice
// out of the accumulation, where it would be a per-column special case.
func (p *parquetRows) Has(column string) bool {
	_, ok := p.columns[column]

	return ok
}

func (p *parquetRows) Number(column string) (float64, bool) {
	col, ok := p.columns[column]
	if !ok || p.at < 0 || p.at >= p.rows || col.IsNull(int(p.at)) {
		return 0, false
	}

	switch c := col.(type) {
	case *array.Float64:
		return c.Value(int(p.at)), true
	case *array.Float32:
		return float64(c.Value(int(p.at))), true
	case *array.Int64:
		return float64(c.Value(int(p.at))), true
	case *array.Int32:
		return float64(c.Value(int(p.at))), true
	case *array.Uint64:
		return float64(c.Value(int(p.at))), true
	case *array.String:
		v, err := strconv.ParseFloat(strings.TrimSpace(c.Value(int(p.at))), 64)

		return v, err == nil
	default:
		return 0, false
	}
}

func (p *parquetRows) Err() error { return p.err }

// Close releases the table's buffers.
func (p *parquetRows) Close() {
	if p.release != nil {
		p.release()
		p.release = nil
	}
}

// integer reads a column that has to be a whole number.
func integer(rows resultRows, column string) (int64, bool) {
	v, ok := rows.Number(column)
	if !ok || math.IsNaN(v) || v < math.MinInt64 || v > math.MaxInt64 {
		return 0, false
	}

	return int64(v), true
}

// openResult picks a reader by what the bytes actually are.
//
// Parquet announces itself with "PAR1" in its first four bytes, and an
// aggregation result is one or the other, so sniffing is exact rather than a
// guess. It saves a caller from tracking which format a job was submitted
// with — which is a thing worth not tracking, because a result outlives the
// program that asked for it.
func openResult(ctx context.Context, r io.Reader) (resultRows, error) {
	head := make([]byte, 4)

	read, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	joined := io.MultiReader(bytes.NewReader(head[:read]), r)

	if read == 4 && string(head) == "PAR1" {
		return newParquetRows(ctx, joined)
	}

	return newCSVRows(joined)
}

// voTableRows reads the IVOA VOTable serialisation.
//
// It exists because asking for CSV does not get CSV. Gaia@AIP's synchronous
// endpoint — this package's own default, set by [GaiaBuild.withDefaults] —
// answers VOTable for FORMAT=csv, RESPONSEFORMAT=csv and text/csv alike,
// checked against the live service. TAP services must serve VOTable and are
// only encouraged to serve anything else, so this is conformant behaviour
// rather than a fault, and a client that assumes its requested format is the
// format it receives is the thing at fault.
//
// Before this existed, every synchronous aggregation against the default
// endpoint handed an XML document to a CSV reader and failed with
// "bare \" in non-quoted-field" — a message about byte 15 of an XML
// declaration, naming neither the format nor the service, for a build that had
// asked for nothing unusual.
type voTableRows struct {
	table *votable.Table
	index map[string]int
	at    int
	row   []string
}

// newVOTableRows parses a VOTable result into rows addressable by column name.
//
// Column names are lowercased on the way in, matching [newCSVRows] and the
// lowercased lookups in the accumulation: the archives disagree about case —
// ESA lowercases its CSV headers and VOTable FIELD names keep whatever case
// the query gave them — and a lookup that depended on that would work against
// one service and silently find no columns against the other.
func newVOTableRows(r io.Reader) (*voTableRows, error) {
	table, err := votable.Read(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
	}

	// A truncated result is refused rather than accumulated. The query is a
	// GROUP BY over a source_id range, so a server-side row limit does not
	// return fewer pixels of the same map — it returns pixels whose sums are
	// missing however many sources fell past the cut, and nothing in the
	// numbers says which. Accepting it would write a map that is quietly too
	// faint in an unknown subset of the sky, which is worse than no map.
	if table.Truncated {
		return nil, fmt.Errorf("%w: the archive truncated the result (QUERY_STATUS OVERFLOW); "+
			"the aggregation is incomplete and its sums cannot be trusted", ErrGaiaResponse)
	}

	index := make(map[string]int, len(table.Fields))
	for i, name := range table.Fields {
		index[strings.ToLower(strings.TrimSpace(name))] = i
	}

	return &voTableRows{table: table, index: index}, nil
}

func (v *voTableRows) Next() bool {
	if v.at >= len(v.table.Rows) {
		return false
	}

	v.row = v.table.Rows[v.at]
	v.at++

	return true
}

func (v *voTableRows) Has(column string) bool {
	_, ok := v.index[column]

	return ok
}

func (v *voTableRows) Number(column string) (float64, bool) {
	i, ok := v.index[column]
	if !ok || i >= len(v.row) {
		return 0, false
	}

	// A VOTable renders a null as an empty cell, which is the ordinary way a
	// pixel says it had nothing to contribute — not a parse failure.
	f, err := strconv.ParseFloat(strings.TrimSpace(v.row[i]), 64)
	if err != nil {
		return 0, false
	}

	return f, true
}

// Err always returns nil: votable.Read consumes the whole document before
// returning, so anything that could fail has already failed by construction.
func (v *voTableRows) Err() error { return nil }

// newSyncRows reads whichever serialisation the archive actually sent.
//
// The format is decided by the payload rather than by the request, the same
// way catalog/gaia decides it and the same way the star-map loader tells
// Parquet from CSV by the file's own magic. It has to be: ESA honours
// FORMAT=csv and Gaia@AIP does not, so what was asked for does not determine
// what arrives, and a service that changes its mind is then handled rather
// than discovered.
//
// A VOTable begins with an XML declaration or its root element and a CSV with
// a column name, so the first non-space byte separates them without ambiguity.
func newSyncRows(r io.Reader) (resultRows, error) {
	buf := bufio.NewReader(r)

	for {
		b, err := buf.Peek(1)
		if err != nil {
			// An empty body is not a format to guess at. Handing it to the
			// CSV reader produces "EOF", which reads as a parse failure
			// rather than as a service that answered with nothing.
			if errors.Is(err, io.EOF) {
				return nil, fmt.Errorf("%w: the archive returned an empty body", ErrGaiaResponse)
			}

			return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
		}

		// Leading whitespace belongs to neither format, so it is skipped
		// rather than allowed to decide the answer.
		if b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t' {
			if _, err := buf.Discard(1); err != nil {
				return nil, fmt.Errorf("%w: %w", ErrGaiaResponse, err)
			}

			continue
		}

		if b[0] == '<' {
			return newVOTableRows(buf)
		}

		return newCSVRows(buf)
	}
}
