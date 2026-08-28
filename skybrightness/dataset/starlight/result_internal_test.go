package starlight

import (
	"errors"
	"strings"
	"testing"
)

// A VOTable carrying the columns the aggregation selects, in the shape
// Gaia@AIP actually returns: an XML declaration, namespaced root, FIELD names
// in the case the query gave them, and an empty cell for a null.
const voTableResult = `<?xml version="1.0"?>
<VOTABLE version="1.3" xmlns="http://www.ivoa.net/xml/VOTable/v1.3">
  <RESOURCE type="results">
    <INFO name="QUERY_STATUS" value="OK"/>
    <TABLE>
      <FIELD name="hpx" datatype="long"/>
      <FIELD name="n" datatype="long"/>
      <FIELD name="b_V" datatype="double"/>
      <DATA>
        <TABLEDATA>
          <TR><TD>100000</TD><TD>567</TD><TD>6121937.5</TD></TR>
          <TR><TD>100001</TD><TD>511</TD><TD></TD></TR>
        </TABLEDATA>
      </DATA>
    </TABLE>
  </RESOURCE>
</VOTABLE>`

const csvResult = "hpx,n,b_v\n100000,567,6121937.5\n100001,511,\n"

// The bug this file exists for.
//
// The synchronous path asked for FORMAT=csv and handed whatever came back to a
// CSV reader. This package's own default endpoint — Gaia@AIP, set by
// withDefaults — answers VOTable regardless, so every synchronous aggregation
// against the default failed with a message about byte 15 of an XML
// declaration that named neither the format nor the service.
func TestSyncRowsReadsWhicheverFormatArrives(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		body string
	}{
		{"votable", voTableResult},
		{"csv", csvResult},
		// Leading whitespace belongs to neither format and must not decide
		// the answer.
		{"votable after blank lines", "\n\n  " + voTableResult},
		{"csv after blank lines", "\n  " + csvResult},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rows, err := newSyncRows(strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("newSyncRows: %v", err)
			}

			if !rows.Next() {
				t.Fatalf("no first row (err %v)", rows.Err())
			}

			// Column names are lowercased on the way in, so a FIELD named
			// b_V and a CSV header named b_v are the same column. The
			// archives disagree about case and the lookups are lowercased.
			if got, ok := rows.Number("b_v"); !ok || got != 6121937.5 {
				t.Errorf("b_v = %v (ok %v), want 6121937.5", got, ok)
			}

			if got, ok := rows.Number("hpx"); !ok || got != 100000 {
				t.Errorf("hpx = %v (ok %v), want 100000", got, ok)
			}

			// Present-but-null and absent are different answers: a null
			// pixel contributed nothing, while a missing column means the
			// query never selected it, and the accumulation treats them
			// oppositely.
			if !rows.Has("b_v") {
				t.Error("Has reports the selected column missing")
			}

			if rows.Has("b_i") {
				t.Error("Has reports a column the query never selected")
			}

			if !rows.Next() {
				t.Fatalf("no second row (err %v)", rows.Err())
			}

			if _, ok := rows.Number("b_v"); ok {
				t.Error("an empty cell read as a number rather than as a null")
			}

			if rows.Next() {
				t.Error("a third row appeared in a two-row result")
			}

			if err := rows.Err(); err != nil {
				t.Errorf("Err: %v", err)
			}
		})
	}
}

// A truncated aggregation is refused rather than accumulated.
//
// The query is a GROUP BY over a source_id range, so a server-side row limit
// does not return fewer pixels of the same map — it returns pixels whose sums
// are missing however many sources fell past the cut, with nothing in the
// numbers to say which. A map quietly too faint across an unknown part of the
// sky is worse than no map.
func TestSyncRowsRefusesATruncatedResult(t *testing.T) {
	t.Parallel()

	overflow := strings.Replace(voTableResult,
		`<INFO name="QUERY_STATUS" value="OK"/>`,
		`<INFO name="QUERY_STATUS" value="OVERFLOW"/>`, 1)

	_, err := newSyncRows(strings.NewReader(overflow))
	if !errors.Is(err, ErrGaiaResponse) {
		t.Fatalf("error = %v, want %v", err, ErrGaiaResponse)
	}

	if !strings.Contains(err.Error(), "truncated") {
		t.Errorf("the failure does not say the result was truncated: %v", err)
	}
}

// A rejected query still returns HTTP 200 with the reason inside the
// document, so the status code cannot catch it. Surfacing it matters more
// than it looks: silently reading zero rows out of an error document reports
// an empty patch of sky where the service actually refused the request.
func TestSyncRowsSurfacesAnArchiveError(t *testing.T) {
	t.Parallel()

	body := `<?xml version="1.0"?>
<VOTABLE version="1.3" xmlns="http://www.ivoa.net/xml/VOTable/v1.3">
  <RESOURCE type="results">
    <INFO name="QUERY_STATUS" value="ERROR">Unknown column 'phot_x_mean_flux'</INFO>
  </RESOURCE>
</VOTABLE>`

	_, err := newSyncRows(strings.NewReader(body))
	if !errors.Is(err, ErrGaiaResponse) {
		t.Fatalf("error = %v, want %v", err, ErrGaiaResponse)
	}

	if !strings.Contains(err.Error(), "phot_x_mean_flux") {
		t.Errorf("the archive's own reason was dropped: %v", err)
	}
}

// An empty body is not a format to guess at.
//
// Handed to the CSV reader it produces a bare "EOF", which reads as a parse
// failure rather than as a service that answered with nothing.
func TestSyncRowsRejectsAnEmptyBody(t *testing.T) {
	t.Parallel()

	for _, body := range []string{"", "   \n\t "} {
		_, err := newSyncRows(strings.NewReader(body))
		if !errors.Is(err, ErrGaiaResponse) {
			t.Errorf("body %q: error = %v, want %v", body, err, ErrGaiaResponse)
		}

		if err != nil && !strings.Contains(err.Error(), "empty body") {
			t.Errorf("body %q: failure does not name the cause: %v", body, err)
		}
	}
}
