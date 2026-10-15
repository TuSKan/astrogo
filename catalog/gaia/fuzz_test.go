package gaia

import (
	"strings"
	"testing"
)

// FuzzParseResult holds the Gaia result reader to never panicking or hanging
// on whatever the archive answers (#746). It sniffs CSV from VOTable by the
// first non-blank byte, then reads rows by column name either way — and a
// VOTable row may be shorter than its fields.
func FuzzParseResult(f *testing.F) {
	f.Add("source_id,ra,dec\n4472832130942575872,269.4485025254,4.7394200511\n")
	f.Add("  \nsource_id,ra,dec\n1,,\n")
	f.Add(`<?xml version="1.0"?><VOTABLE><RESOURCE><TABLE>` +
		`<FIELD name="source_id"/><FIELD name="ra"/><FIELD name="dec"/>` +
		`<DATA><TABLEDATA><TR><TD>1</TD><TD>10.5</TD></TR></TABLEDATA></DATA>` +
		`</TABLE></RESOURCE></VOTABLE>`)
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _, _ = parseResult(strings.NewReader(s))
	})
}
