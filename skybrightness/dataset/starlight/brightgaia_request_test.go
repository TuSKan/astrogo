package starlight

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// fetchGaiaBright's request path, against a fake Gaia archive.
//
// The halving is tested on its own in brightcap_internal_test.go; what only a
// server can show is the request this package sends and how it reads the
// answer: the magnitude bounds in the ADQL, the TABLEDATA format the VOTable
// reader decodes, and the OVERFLOW status the archive writes after a capped
// table. The live test covers the same path but needs the network, so CI never
// ran it.

// gaiaBrightSource is one row the fake archive holds.
type gaiaBrightSource struct {
	ra, dec, g float64
}

// gaiaBoundsPattern pulls the interval out of the ADQL fetchGaiaBrightPage
// writes.
var gaiaBoundsPattern = regexp.MustCompile(`phot_g_mean_mag >= (\S+) AND phot_g_mean_mag < (\S+) AND`)

// serveGaia points the Gaia TAP endpoint at handler for the rest of the test.
func serveGaia(t *testing.T, handler http.HandlerFunc) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	scope := remote.Capture(remote.GaiaTAP)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.GaiaTAP, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}
}

// gaiaBrightArchive serves the sources with lo <= G < hi, at most limit of
// them (a negative limit is none), as the archive does: a TABLEDATA VOTable,
// with an OVERFLOW status after the table when the cap cut the answer. It
// records each request's form, and refuses one that does not ask for
// TABLEDATA, since a BINARY2 answer is one the reader cannot decode.
func gaiaBrightArchive(t *testing.T, sources []gaiaBrightSource, limit int, forms *[]map[string]string) {
	t.Helper()

	serveGaia(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		form := map[string]string{}
		for _, key := range []string{"REQUEST", "LANG", "FORMAT", "QUERY"} {
			form[key] = r.PostForm.Get(key)
		}

		*forms = append(*forms, form)

		if form["FORMAT"] != "votable_plain" {
			http.Error(w, "only votable_plain here", http.StatusBadRequest)

			return
		}

		bounds := gaiaBoundsPattern.FindStringSubmatch(form["QUERY"])
		if bounds == nil {
			http.Error(w, "no magnitude interval", http.StatusBadRequest)

			return
		}

		lo, errLo := strconv.ParseFloat(bounds[1], 64)
		hi, errHi := strconv.ParseFloat(bounds[2], 64)

		if errLo != nil || errHi != nil {
			http.Error(w, "bad magnitude interval", http.StatusBadRequest)

			return
		}

		var rows strings.Builder

		status, n := "OK", 0

		for _, s := range sources {
			if s.g < lo || s.g >= hi {
				continue
			}

			if n == limit {
				status = "OVERFLOW"

				break
			}

			fmt.Fprintf(&rows, "<TR><TD>%g</TD><TD>%g</TD></TR>\n", s.ra, s.dec)

			n++
		}

		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?>
<VOTABLE version="1.4"><RESOURCE type="results">
<INFO name="QUERY_STATUS" value="OK"/>
<TABLE><FIELD name="ra" datatype="double"/><FIELD name="dec" datatype="double"/>
<DATA><TABLEDATA>
%s</TABLEDATA></DATA></TABLE>
<INFO name="QUERY_STATUS" value="%s"/>
</RESOURCE></VOTABLE>`, rows.String(), status)
	})
}

func TestFetchGaiaBrightPageSaysWhenTheArchiveCapped(t *testing.T) {
	sources := []gaiaBrightSource{{10, 20, 1.5}, {11, 21, 2.5}, {12, 22, 3.5}, {13, 23, 9.5}}

	tests := []struct {
		name          string
		limit         int
		wantRA        []float64
		wantTruncated bool
	}{
		{name: "whole", limit: -1, wantRA: []float64{10, 11, 12}},
		{name: "capped", limit: 2, wantRA: []float64{10, 11}, wantTruncated: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var forms []map[string]string

			gaiaBrightArchive(t, sources, tt.limit, &forms)

			ra, dec, truncated, err := fetchGaiaBrightPage(t.Context(), remote.Default(), gaiaBrightestG, 9)
			if err != nil {
				t.Fatalf("fetchGaiaBrightPage: %v", err)
			}

			if truncated != tt.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tt.wantTruncated)
			}

			if len(ra) != len(tt.wantRA) || len(dec) != len(tt.wantRA) {
				t.Fatalf("got %d ra and %d dec, want %d of each", len(ra), len(dec), len(tt.wantRA))
			}

			for i, want := range tt.wantRA {
				if got := ra[i].Degrees(); math.Abs(got-want) > 1e-12 {
					t.Errorf("ra[%d] = %g, want %g", i, got, want)
				}

				if got := dec[i].Degrees(); math.Abs(got-(want+10)) > 1e-12 {
					t.Errorf("dec[%d] = %g, want %g", i, got, want+10)
				}
			}

			if len(forms) != 1 {
				t.Fatalf("the archive saw %d requests, want 1", len(forms))
			}

			form := forms[0]

			if form["REQUEST"] != "doQuery" || form["LANG"] != "ADQL" {
				t.Errorf("REQUEST = %q, LANG = %q; want a TAP doQuery in ADQL", form["REQUEST"], form["LANG"])
			}

			// The interval is half-open, and the bound is written as the
			// number it is: a rounded bound would let a source fall between
			// two intervals or into both.
			for _, want := range []string{
				"FROM gaiadr3.gaia_source",
				"phot_g_mean_mag >= -10 AND phot_g_mean_mag < 9 AND",
				"phot_g_mean_flux > 0",
			} {
				if !strings.Contains(form["QUERY"], want) {
					t.Errorf("query = %q, want it to contain %q", form["QUERY"], want)
				}
			}
		})
	}
}

// A row the archive leaves blank, or short, has no position to match a star
// against, and is skipped rather than read as the origin.
func TestFetchGaiaBrightPageSkipsRowsWithoutAPosition(t *testing.T) {
	serveGaia(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`<VOTABLE version="1.4"><RESOURCE type="results"><TABLE>
<FIELD name="ra" datatype="double"/><FIELD name="dec" datatype="double"/>
<DATA><TABLEDATA>
<TR><TD>83.45</TD><TD>-19.43</TD></TR>
<TR><TD></TD><TD>-18.68</TD></TR>
<TR><TD>NaN</TD><TD>-18.68</TD></TR>
<TR><TD>83.43</TD></TR>
</TABLEDATA></DATA></TABLE>
<INFO name="QUERY_STATUS" value="OK"/>
</RESOURCE></VOTABLE>`))
	})

	ra, dec, truncated, err := fetchGaiaBrightPage(t.Context(), remote.Default(), gaiaBrightestG, 9)
	if err != nil {
		t.Fatalf("fetchGaiaBrightPage: %v", err)
	}

	if truncated {
		t.Error("truncated = true for an answer the archive said was complete")
	}

	if len(ra) != 1 || len(dec) != 1 ||
		math.Abs(ra[0].Degrees()-83.45) > 1e-12 || math.Abs(dec[0].Degrees()+19.43) > 1e-12 {
		t.Errorf("got ra %v, dec %v; want the one row with a position", ra, dec)
	}
}

func TestFetchGaiaBrightPageRefusesWhatItCannotRead(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "no position columns",
			body: `<VOTABLE version="1.4"><RESOURCE type="results"><TABLE>
<FIELD name="source_id" datatype="long"/><DATA><TABLEDATA><TR><TD>1</TD></TR></TABLEDATA></DATA></TABLE>
</RESOURCE></VOTABLE>`,
		},
		{
			name: "not a VOTable",
			body: "ra,dec\n83.45,-19.43\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serveGaia(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			})

			_, _, _, err := fetchGaiaBrightPage(t.Context(), remote.Default(), gaiaBrightestG, 9)
			if !errors.Is(err, ErrGaiaResponse) {
				t.Fatalf("fetchGaiaBrightPage = %v, want ErrGaiaResponse", err)
			}
		})
	}
}

// A refused query is the archive's error, carried up with its status, rather
// than an empty answer read as no sources.
func TestFetchGaiaBrightPageCarriesTheArchivesRefusal(t *testing.T) {
	serveGaia(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad query", http.StatusBadRequest)
	})

	_, _, _, err := fetchGaiaBrightPage(t.Context(), remote.Default(), gaiaBrightestG, 9)

	var httpErr *remote.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("fetchGaiaBrightPage = %v, want the archive's 400", err)
	}
}

// fetchGaiaBright asks for the whole range in one query when the archive does
// not cap it. A test of the halving through the server would pay the
// package's two-second pacing per extra query; collectGaiaBright's own tests
// cover the halving without it.
func TestFetchGaiaBrightAsksForTheWholeRange(t *testing.T) {
	sources := []gaiaBrightSource{{10, 20, -1.4}, {11, 21, 5}, {12, 22, 8.99}, {13, 23, 9}}

	var forms []map[string]string

	gaiaBrightArchive(t, sources, -1, &forms)

	ra, dec, err := fetchGaiaBright(t.Context(), 9)
	if err != nil {
		t.Fatalf("fetchGaiaBright: %v", err)
	}

	if len(ra) != 3 || len(dec) != 3 {
		t.Fatalf("got %d sources, want the 3 brighter than G = 9", len(ra))
	}

	if len(forms) != 1 {
		t.Errorf("the archive saw %d requests for an uncapped answer, want 1", len(forms))
	}
}

func TestFetchGaiaBrightRefusesAnEmptyAnswer(t *testing.T) {
	var forms []map[string]string

	gaiaBrightArchive(t, nil, -1, &forms)

	_, _, err := fetchGaiaBright(t.Context(), 9)
	if !errors.Is(err, ErrBrightStar) {
		t.Fatalf("fetchGaiaBright = %v, want ErrBrightStar", err)
	}
}
