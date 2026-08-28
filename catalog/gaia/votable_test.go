package gaia

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// aipVOTable is what Gaia@AIP's synchronous endpoint answers, verbatim apart
// from being trimmed to one row.
//
// The service ignores FORMAT and RESPONSEFORMAT and sends this whatever the
// request asked for, so this is not a fallback shape — it is what the default
// archive returns for every query.
const aipVOTable = `<?xml version="1.0"?>
<VOTABLE version="1.3" xmlns="http://www.ivoa.net/xml/VOTable/v1.3">
    <RESOURCE type="results">
        <INFO name="QUERY_STATUS" value="OK" />
        <TABLE name="gaia_user_anonymous.2026-08-26">
                <FIELD name="source_id" ucd="meta.id;meta.main" datatype="long"> <DESCRIPTION>Unique source identifier</DESCRIPTION> </FIELD>
                <FIELD name="ra" unit="deg" datatype="double"> <DESCRIPTION>Right ascension</DESCRIPTION> </FIELD>
                <FIELD name="dec" unit="deg" datatype="double"> <DESCRIPTION>Declination</DESCRIPTION> </FIELD>
                <FIELD name="pmra" unit="mas.yr**-1" datatype="double"> <DESCRIPTION>Proper motion</DESCRIPTION> </FIELD>
                <FIELD name="pmdec" unit="mas.yr**-1" datatype="double"> <DESCRIPTION>Proper motion</DESCRIPTION> </FIELD>
                <FIELD name="parallax" unit="mas" datatype="double"> <DESCRIPTION>Parallax</DESCRIPTION> </FIELD>
                <FIELD name="phot_g_mean_mag" unit="mag" datatype="float"> <DESCRIPTION>G magnitude</DESCRIPTION> </FIELD>
                <FIELD name="bp_rp" unit="mag" datatype="float"> <DESCRIPTION>BP - RP</DESCRIPTION> </FIELD>
            <DATA>
                <TABLEDATA>
                    <TR>
                        <TD>123456789</TD>
                        <TD>10.684</TD>
                        <TD>41.269</TD>
                        <TD>1.1</TD>
                        <TD>-2.2</TD>
                        <TD>5.5</TD>
                        <TD>12.5</TD>
                        <TD>0.8</TD>
                    </TR>
                </TABLEDATA>
            </DATA>
        </TABLE>
    </RESOURCE>
</VOTABLE>`

// The provider reads a VOTable answer as readily as a CSV one.
//
// The format is sniffed from the payload because the request cannot determine
// it: the default archive ignores what was asked for. Without this the
// provider parses AIP's every reply as CSV and fails on the first line, which
// is exactly what it did.
func TestConeSearchReadsVOTable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// The content type says XML while the query asked for csv, which is
		// the mismatch the real service produces.
		w.Header().Set("Content-Type", "application/x-votable+xml")

		if _, err := fmt.Fprint(w, aipVOTable); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	iter := prov.ConeSearch(context.Background(), resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(41)),
		Radius: angle.Deg(1),
	})

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("ConeSearch: %v", err)
		}

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("got %d targets, want 1", len(targets))
	}

	got := targets[0]

	if got.ID != "123456789" {
		t.Errorf("ID is %q, want 123456789", got.ID)
	}

	if d := math.Abs(got.Coord.RA().Degrees() - 10.684); d > 1e-9 {
		t.Errorf("RA is %g, want 10.684", got.Coord.RA().Degrees())
	}

	if d := math.Abs(got.Coord.Dec().Degrees() - 41.269); d > 1e-9 {
		t.Errorf("Dec is %g, want 41.269", got.Coord.Dec().Degrees())
	}

	// Milliarcseconds on the wire, arcseconds in the target.
	if d := math.Abs(got.PmRA.Arcsec() - 0.0011); d > 1e-12 {
		t.Errorf("pmRA is %g arcsec, want 0.0011", got.PmRA.Arcsec())
	}

	if d := math.Abs(got.Parallax.Arcsec() - 0.0055); d > 1e-12 {
		t.Errorf("parallax is %g arcsec, want 0.0055", got.Parallax.Arcsec())
	}

	// G = 12.5 at BP−RP = 0.8 through the Table 5.9 polynomial.
	if !got.HasVMag {
		t.Fatal("no V magnitude derived from G and the colour")
	}

	gMinusV := -0.02704 + 0.01424*0.8 - 0.2156*0.8*0.8 + 0.01426*0.8*0.8*0.8
	if d := math.Abs(got.VMag - (12.5 - gMinusV)); d > 1e-12 {
		t.Errorf("V is %g, want %g", got.VMag, 12.5-gMinusV)
	}
}

// A service error arrives as a well-formed document with a 200, and must not
// read as an empty sky.
func TestConeSearchSurfacesAServiceError(t *testing.T) {
	const doc = `<VOTABLE version="1.3"><RESOURCE type="results">
		<INFO name="QUERY_STATUS" value="ERROR">Unknown column 'phot_x_mean_mag'</INFO>
	</RESOURCE></VOTABLE>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, doc); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	var (
		got  error
		rows int
	)

	prov.ConeSearch(context.Background(), resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(41)),
		Radius: angle.Deg(1),
	})(func(_ resolve.Target, err error) bool {
		if err != nil {
			got = err

			return false
		}

		rows++

		return true
	})

	if rows != 0 {
		t.Errorf("a failed query yielded %d targets", rows)
	}

	if got == nil {
		t.Fatal("a rejected query reported no error, so the caller sees an empty field " +
			"where the service actually refused the request")
	}
}
