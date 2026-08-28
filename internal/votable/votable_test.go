package votable_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/votable"
)

// aipResult is a real Gaia@AIP synchronous response, trimmed to two rows.
//
// Kept verbatim rather than tidied, because the details are the test: the
// FIELDs carry DESCRIPTION children, the whitespace between elements is the
// service's own, and a second RESOURCE follows the results describing a
// DataLink service. A hand-written fixture would omit whichever of those the
// author did not think of, which is how a parser comes to pass its tests and
// fail on the wire.
const aipResult = `<?xml version="1.0"?>
<VOTABLE version="1.3"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    xmlns="http://www.ivoa.net/xml/VOTable/v1.3"
    xmlns:stc="http://www.ivoa.net/xml/STC/v1.30">
    <RESOURCE type="results">
        <LINK title="gaiadr3.gaia_source" content-role="doc" href="https://doi.org/10.17876/gaia/dr.3/1"/>
        <INFO name="QUERY_STATUS" value="OK" />
        <INFO name="QUERY" value="SELECT TOP 2 source_id, ra, dec FROM gaiadr3.gaia_source" />
        <TABLE name="gaia_user_anonymous.2026-08-26">
                <FIELD name="source_id" ucd="meta.id;meta.main" ID="datalinkID" datatype="long"> <DESCRIPTION>Unique source identifier</DESCRIPTION> </FIELD>

                <FIELD name="ra" unit="deg" ucd="pos.eq.ra;meta.main" datatype="double"> <DESCRIPTION>Right ascension</DESCRIPTION> </FIELD>

                <FIELD name="dec" unit="deg" ucd="pos.eq.dec;meta.main" datatype="double"> <DESCRIPTION>Declination</DESCRIPTION> </FIELD>

                <FIELD name="bp_rp" unit="mag" ucd="phot.color" datatype="float"> <DESCRIPTION>BP - RP colour</DESCRIPTION> </FIELD>
            <DATA>
                <TABLEDATA>
                    <TR>
                        <TD>3961707932962051968</TD>
                        <TD>192.8385144891487</TD>
                        <TD>27.166349566644193</TD>
                        <TD>2.408863</TD>
                    </TR>
                    <TR>
                        <TD>3961707692443880960</TD>
                        <TD>192.8657303004276</TD>
                        <TD>27.149103819739953</TD>
                        <TD/>
                    </TR>
                </TABLEDATA>
            </DATA>
        </TABLE>
    </RESOURCE>
    <RESOURCE type="meta" utype="adhoc:service">
        <PARAM name="standardID" datatype="char" arraysize="*" value="ivo://ivoa.net/std/DataLink#links-1.0" />
        <PARAM name="accessURL" datatype="char" arraysize="*" value="https://gaia.aip.de/datalink/links" />
        <GROUP name="inputParams">
            <PARAM name="ID" datatype="char" arraysize="*" value="" ref="datalinkID"/>
        </GROUP>
    </RESOURCE>
</VOTABLE>`

// A real TAP response parses to its fields and rows.
func TestReadParsesAServiceResponse(t *testing.T) {
	t.Parallel()

	table, err := votable.Read(strings.NewReader(aipResult))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if table.Truncated {
		t.Error("a QUERY_STATUS of OK reported as truncated")
	}

	want := []string{"source_id", "ra", "dec", "bp_rp"}
	if len(table.Fields) != len(want) {
		t.Fatalf("fields %v, want %v — the PARAMs in the trailing meta RESOURCE "+
			"must not be read as columns", table.Fields, want)
	}

	for i, f := range want {
		if table.Fields[i] != f {
			t.Errorf("field %d is %q, want %q — order is what aligns a row to its header",
				i, table.Fields[i], f)
		}
	}

	if len(table.Rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(table.Rows))
	}

	if got := table.Value(table.Rows[0], "source_id"); got != "3961707932962051968" {
		t.Errorf("source_id is %q — a 19-digit identifier must survive as text, since "+
			"it does not fit a float and is never arithmetic", got)
	}

	if got := table.Value(table.Rows[0], "ra"); got != "192.8385144891487" {
		t.Errorf("ra is %q, want the full stored precision", got)
	}

	// An empty element is a null cell, not a missing column: the row still has
	// to line up with every later field.
	if got := table.Value(table.Rows[1], "bp_rp"); got != "" {
		t.Errorf("an empty TD read as %q, want empty", got)
	}

	if got := table.Value(table.Rows[1], "dec"); got != "27.149103819739953" {
		t.Errorf("dec of the second row is %q; a null in a later column must not "+
			"shift the ones before it", got)
	}
}

// A service error inside a well-formed document is an error, not an empty
// result.
//
// This is the failure a status code cannot catch: TAP answers 200 with a
// VOTable whose QUERY_STATUS is ERROR. A reader that ignored it would report
// "no sources here" for a query that never ran, which is a wrong answer rather
// than a missing one.
func TestReadReportsAQueryError(t *testing.T) {
	t.Parallel()

	const doc = `<VOTABLE version="1.3"><RESOURCE type="results">
		<INFO name="QUERY_STATUS" value="ERROR">Unknown table 'gaiadr9.gaia_source'</INFO>
	</RESOURCE></VOTABLE>`

	_, err := votable.Read(strings.NewReader(doc))
	if !errors.Is(err, votable.ErrQueryFailed) {
		t.Fatalf("got %v, want ErrQueryFailed", err)
	}

	if !strings.Contains(err.Error(), "gaiadr9") {
		t.Errorf("the error is %q and does not carry the service's own message; "+
			"the message is the only thing that says what was wrong", err)
	}
}

// A truncated result says so.
//
// Two archives compared on a truncated result are compared on two arbitrary
// subsets, so a caller doing that has to be able to tell.
func TestReadReportsOverflow(t *testing.T) {
	t.Parallel()

	const doc = `<VOTABLE version="1.3"><RESOURCE type="results">
		<INFO name="QUERY_STATUS" value="OVERFLOW"/>
		<TABLE><FIELD name="a" datatype="int"/><DATA><TABLEDATA>
			<TR><TD>1</TD></TR>
		</TABLEDATA></DATA></TABLE>
	</RESOURCE></VOTABLE>`

	table, err := votable.Read(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if !table.Truncated {
		t.Error("OVERFLOW did not set Truncated")
	}

	if len(table.Rows) != 1 {
		t.Errorf("got %d rows, want 1 — an overflow still returns what it has", len(table.Rows))
	}
}

// A document with no table is empty rather than an error.
//
// An ADQL query that matches nothing is a legitimate answer, and a service may
// send the header with no TABLEDATA at all.
func TestReadHandlesAnEmptyResult(t *testing.T) {
	t.Parallel()

	const doc = `<VOTABLE version="1.3"><RESOURCE type="results">
		<INFO name="QUERY_STATUS" value="OK"/>
		<TABLE><FIELD name="a" datatype="int"/><DATA><TABLEDATA></TABLEDATA></DATA></TABLE>
	</RESOURCE></VOTABLE>`

	table, err := votable.Read(strings.NewReader(doc))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if len(table.Rows) != 0 {
		t.Errorf("got %d rows, want none", len(table.Rows))
	}

	if _, ok := table.Column("a"); !ok {
		t.Error("the field list is lost when there are no rows")
	}
}

// Column reports absence rather than index zero.
//
// A lookup that returned zero for a missing name would read the first column
// as though it were the one asked for, which is the shape of bug that returns
// plausible values from the wrong place.
func TestColumnReportsAbsence(t *testing.T) {
	t.Parallel()

	table, err := votable.Read(strings.NewReader(aipResult))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	if _, ok := table.Column("phot_g_mean_mag"); ok {
		t.Error("a column that is not in the result reported present")
	}

	if got := table.Value(table.Rows[0], "phot_g_mean_mag"); got != "" {
		t.Errorf("a missing column yielded %q, want empty", got)
	}
}
