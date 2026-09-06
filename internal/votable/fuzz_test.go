package votable_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/votable"
)

// This file fuzzes the VOTable reader. Every seed is a string literal — no
// checked-in fixture — so the seed corpus runs under `go test ./...` and is in
// every CI run for free. Extended fuzzing is manual and periodic; see CLAUDE.md,
// including the -fuzzminimizetime flag (#140).
//
// # What is actually under test
//
// encoding/xml does the tokenising and is not what needs fuzzing. What does is
// what this package builds on top: the walk that decides which RESOURCE holds
// the results, the FIELD/TD alignment, and the type conversion driven by each
// field's declared datatype. A row with more cells than the table has columns,
// a datatype that does not match its values, a RESOURCE nested inside itself —
// none of those are XML errors, so the decoder hands them straight through.
//
// The bytes come from SIMBAD, VizieR, Gaia and MAST TAP endpoints, so they are
// as attacker-influenceable as anything else astrogo reads over the network.
//
// Note the reader sets dec.Strict = false, which makes malformed markup that
// encoding/xml would reject reach this code instead. That widens the input
// class this target explores rather than narrowing it.

const validVOTable = `<?xml version="1.0"?>
<VOTABLE version="1.3">
 <RESOURCE type="results">
  <TABLE>
   <FIELD name="ra" datatype="double" unit="deg"/>
   <FIELD name="dec" datatype="double" unit="deg"/>
   <FIELD name="name" datatype="char" arraysize="*"/>
   <DATA><TABLEDATA>
    <TR><TD>101.287</TD><TD>-16.716</TD><TD>Sirius</TD></TR>
    <TR><TD>88.793</TD><TD>7.407</TD><TD>Betelgeuse</TD></TR>
   </TABLEDATA></DATA>
  </TABLE>
 </RESOURCE>
</VOTABLE>`

func FuzzRead(f *testing.F) {
	f.Add(validVOTable)

	// A meta RESOURCE before the results one: the case the reader's doc comment
	// says it exists to get right.
	f.Add(strings.Replace(validVOTable,
		`<RESOURCE type="results">`,
		`<RESOURCE type="meta"><PARAM name="x" value="1"/></RESOURCE><RESOURCE type="results">`, 1))

	// More cells than columns, and fewer: the two ends of row/field alignment.
	f.Add(strings.Replace(validVOTable,
		`<TD>Sirius</TD></TR>`, `<TD>Sirius</TD><TD>extra</TD><TD>more</TD></TR>`, 1))
	f.Add(strings.Replace(validVOTable, `<TD>-16.716</TD><TD>Sirius</TD>`, ``, 1))

	// Values that do not match their declared datatype, which is a conversion
	// problem rather than a parse one.
	f.Add(strings.Replace(validVOTable, `<TD>101.287</TD>`, `<TD>not a number</TD>`, 1))
	f.Add(strings.Replace(validVOTable, `datatype="double"`, `datatype="banana"`, 1))
	f.Add(strings.Replace(validVOTable, `<TD>101.287</TD>`, `<TD></TD>`, 1))

	// Structure that is well-formed XML and not a VOTable at all.
	f.Add(`<VOTABLE><RESOURCE/></VOTABLE>`)
	f.Add(`<VOTABLE><TABLE><DATA><TABLEDATA/></DATA></TABLE></VOTABLE>`)
	f.Add(`<html><body>503 Service Unavailable</body></html>`)

	// Truncated mid-document, which is what a dropped connection looks like.
	f.Add(validVOTable[:len(validVOTable)/2])

	// Malformed markup, which Strict = false lets through to this code.
	f.Add(`<VOTABLE><TABLE><FIELD name="a"><DATA><TABLEDATA><TR><TD>1</TABLEDATA></VOTABLE>`)

	f.Add("")

	f.Fuzz(func(t *testing.T, doc string) {
		table, err := votable.Read(strings.NewReader(doc))
		if err != nil {
			return
		}

		if table == nil {
			t.Fatal("Read returned a nil table and a nil error")
		}

		// A table that parsed must be addressable: every declared field, on
		// every row, through the accessor callers actually use. Value is where
		// a field index meets a row of a different length, and it is the only
		// place this package promises to cope with that.
		for _, row := range table.Rows {
			for _, field := range table.Fields {
				_ = table.Value(row, field)
			}

			// And a field that is not declared at all, which is what a caller
			// asking for a column the service omitted looks like.
			_ = table.Value(row, "no-such-field")
		}

		// Rows longer than Fields are reachable -- a seed with extra <TD>
		// cells produces one -- and are deliberately not an error here. Value
		// indexes by field position and bounds-checks, so the surplus cells
		// are unaddressable rather than dangerous. Asserting len(row) <=
		// len(Fields) would be inventing an invariant the package never
		// claimed; what it claims is that Value copes, which is what the loop
		// above checks.
		if len(table.Fields) == 0 && len(table.Rows) > 0 {
			t.Fatal("rows were parsed for a table with no declared fields, so nothing in them " +
				"can ever be addressed by name")
		}
	})
}
