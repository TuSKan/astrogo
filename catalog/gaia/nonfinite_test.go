package gaia

import (
	"strings"
	"testing"
)

// A non-finite number in a Gaia answer is read as an empty cell is (#1080).
//
// A row with no usable position was already dropped; a NaN one was not,
// since strconv.ParseFloat accepts it. And a NaN proper motion, which VOTable
// allows for a null float, was kept on a star at Gaia's J2016 epoch, where it
// made the star's position NaN on every other date.
func TestParseResultReadsNonFiniteNumbersAsAbsent(t *testing.T) {
	t.Parallel()

	targets, _, err := parseResult(strings.NewReader(
		"source_id,ra,dec,pmra,pmdec,parallax,phot_g_mean_mag,bp_rp\n" +
			"1,NaN,4.73942,1,2,3,10,0.5\n" +
			"2,269.4485,Infinity,1,2,3,10,0.5\n" +
			"3,269.4485,91,1,2,3,10,0.5\n" +
			"4,269.4485,4.73942,NaN,-Inf,NaN,10,0.5\n" +
			"5,269.4485,4.73942,-801.6,10362.4,546.98,9.5,2.8\n"))
	if err != nil {
		t.Fatalf("parseResult: %v", err)
	}

	byID := map[string]int{}
	for i, tgt := range targets {
		byID[tgt.ID] = i
	}

	for _, id := range []string{"1", "2", "3"} {
		if _, ok := byID[id]; ok {
			t.Errorf("row %s has a NaN, infinite or impossible position and was kept", id)
		}
	}

	i, ok := byID["4"]
	if !ok {
		t.Fatal("row 4 has a real position and was dropped")
	}

	if p := targets[i]; p.PmRA.Radians() != 0 || p.PmDec.Radians() != 0 || p.Parallax.Radians() != 0 {
		t.Errorf("non-finite kinematics were kept: pmRA %v, pmDec %v, parallax %v", p.PmRA, p.PmDec, p.Parallax)
	}

	j, ok := byID["5"]
	if !ok || targets[j].PmRA.Radians() == 0 || targets[j].Parallax.Radians() == 0 {
		t.Errorf("the real row was not read in full")
	}
}

// The same holds for the VOTable form, where NaN is how a null float is
// written.
func TestParseResultReadsVOTableNaNAsAbsent(t *testing.T) {
	t.Parallel()

	const votable = `<?xml version="1.0"?><VOTABLE><RESOURCE><TABLE>` +
		`<FIELD name="source_id" datatype="long"/><FIELD name="ra" datatype="double"/>` +
		`<FIELD name="dec" datatype="double"/><FIELD name="pmra" datatype="double"/>` +
		`<DATA><TABLEDATA>` +
		`<TR><TD>1</TD><TD>NaN</TD><TD>4.7</TD><TD>1</TD></TR>` +
		`<TR><TD>2</TD><TD>269.4</TD><TD>4.7</TD><TD>NaN</TD></TR>` +
		`</TABLEDATA></DATA></TABLE></RESOURCE></VOTABLE>`

	targets, _, err := parseResult(strings.NewReader(votable))
	if err != nil {
		t.Fatalf("parseResult: %v", err)
	}

	if len(targets) != 1 || targets[0].ID != "2" {
		t.Fatalf("got %d targets (%v); want only row 2, row 1's position being NaN", len(targets), targets)
	}

	if targets[0].PmRA.Radians() != 0 {
		t.Errorf("row 2's NaN proper motion was kept as %v", targets[0].PmRA)
	}
}
