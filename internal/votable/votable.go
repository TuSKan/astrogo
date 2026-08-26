// Package votable reads the tabular payload of an IVOA VOTable.
//
// # Why this exists
//
// TAP services are required to serve VOTable and are only encouraged to serve
// anything else, so a client that can read one format can talk to all of them
// and a client that reads CSV can talk to the subset that offers it. Gaia@AIP
// is outside that subset on its synchronous endpoint: it answers VOTable
// whatever FORMAT or RESPONSEFORMAT asks for, verified against the live
// service for csv and text/csv alike. Reading VOTable is what lets a provider
// treat one archive as a drop-in for another.
//
// # What it deliberately does not do
//
// This reads TABLEDATA — values as text, in document order — and nothing else.
// VOTable also permits BINARY, BINARY2 and FITS serialisations, arrays, and
// null-value conventions per field; none of that is handled, and a payload
// carrying it produces a table with no rows rather than a wrong one. TAP
// services return TABLEDATA for synchronous queries, and a format that needs
// more belongs in a reader written against a real example of it rather than
// against a guess.
//
// Values are returned as strings for the same reason: the caller knows which
// column is a magnitude and which is an identifier, and a reader that guessed
// would have to invent a policy for the ones it got wrong.
package votable

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrQueryFailed reports that the service returned an error inside a
// well-formed document.
//
// This is the failure mode a status code cannot catch. A TAP service that
// rejects a query still answers 200 with a VOTable whose QUERY_STATUS is
// ERROR and whose text is the message, so a reader that ignored it would
// report an empty result for a query that was never run — the difference
// between "this field holds no sources" and "your ADQL does not parse".
var ErrQueryFailed = errors.New("votable: the service reported a query error")

// Table is one result table: the field names in order, and the rows beneath
// them.
type Table struct {
	// Fields are the column names, in the order the values appear.
	Fields []string

	// Rows holds one slice of values per row, aligned to Fields. A row
	// shorter than Fields is possible when the service omits trailing empty
	// cells, so index through Column rather than assuming a length.
	Rows [][]string

	// Truncated records that the service reported QUERY_STATUS OVERFLOW: the
	// result hit a server-side limit and is a partial answer. Worth having
	// because a truncated result and a complete one are indistinguishable by
	// inspection, and a caller comparing two services would otherwise compare
	// two arbitrary subsets.
	Truncated bool
}

// Column returns the index of a named field, and whether it is present.
func (t *Table) Column(name string) (int, bool) {
	for i, f := range t.Fields {
		if f == name {
			return i, true
		}
	}

	return 0, false
}

// Value returns one cell by field name, empty when the row is short or the
// field is absent.
func (t *Table) Value(row []string, name string) string {
	i, ok := t.Column(name)
	if !ok || i >= len(row) {
		return ""
	}

	return row[i]
}

// Read parses the first table in a VOTable document.
//
// The first, not every one: a TAP response carries the result in a RESOURCE of
// type "results" and may carry further RESOURCEs of type "meta" describing
// DataLink services. Those hold PARAMs and GROUPs rather than a TABLEDATA, so
// stopping at the first table that has rows takes the result and ignores the
// service description. Reading to the end and keeping the last table would
// pick up whichever came last, which is a different thing on a different day.
func Read(r io.Reader) (*Table, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false

	table := &Table{}

	var (
		inData   bool
		inTD     bool
		gotRows  bool
		row      []string
		cell     strings.Builder
		status   string
		statusMu strings.Builder
	)

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, fmt.Errorf("votable: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch local(t.Name.Local) {
			case "info":
				// QUERY_STATUS carries the outcome in an attribute and the
				// detail, when there is one, in the element's own text.
				if attr(t, "name") == "QUERY_STATUS" {
					status = attr(t, "value")

					statusMu.Reset()
				}

			case "field":
				// Only before any rows: a second table's fields must not be
				// appended to the first table's header.
				if !gotRows {
					name := attr(t, "name")
					if name == "" {
						name = attr(t, "ID")
					}

					table.Fields = append(table.Fields, name)
				}

			case "tabledata":
				inData = true

			case "tr":
				if inData {
					row = make([]string, 0, len(table.Fields))
				}

			case "td":
				if inData {
					inTD = true

					cell.Reset()
				}
			}

		case xml.CharData:
			switch {
			case inTD:
				cell.Write(t)
			case status != "" && status != "OK":
				statusMu.Write(t)
			}

		case xml.EndElement:
			switch local(t.Name.Local) {
			case "td":
				if inTD {
					row = append(row, strings.TrimSpace(cell.String()))
					inTD = false
				}

			case "tr":
				if inData {
					table.Rows = append(table.Rows, row)
					gotRows = true
					row = nil
				}

			case "tabledata":
				if inData {
					// The first table with rows is the result; anything after
					// it describes a service rather than answering a query.
					return finish(table, status, statusMu.String())
				}
			}
		}
	}

	return finish(table, status, statusMu.String())
}

// finish applies the reported query status to an otherwise complete table.
func finish(t *Table, status, detail string) (*Table, error) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ERROR":
		msg := strings.TrimSpace(detail)
		if msg == "" {
			msg = "no detail given"
		}

		return nil, fmt.Errorf("%w: %s", ErrQueryFailed, msg)

	case "OVERFLOW":
		t.Truncated = true
	}

	return t, nil
}

// local lowercases an element name so the reader does not depend on the case
// a service happens to use. VOTable names them in upper case; nothing
// guarantees every service does.
func local(name string) string { return strings.ToLower(name) }

// attr returns a named attribute, matching on the local name so a namespace
// prefix does not hide it.
func attr(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}

	return ""
}
