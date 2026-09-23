package sbdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// TestParseFloatReadsAnExponent pins the numeric prefix parseFloat keeps.
// SBDB writes small values in E-notation — C/1937 C1's mean anomaly is
// "-2.593805408851336E-5" — and a prefix that stopped at the E read that as
// −2.59°, off by five orders of magnitude for every near-perihelion element
// set.
func TestParseFloatReadsAnExponent(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"-2.593805408851336E-5", -2.593805408851336e-5},
		{"3.748266769806928E-6", 3.748266769806928e-6},
		{"-1.068e+04", -1.068e+04},
		{".3914300748355564", 0.3914300748355564},
		{"3.53 (assumed)", 3.53},
		{" 42 ", 42},
		{"1.2E", 1.2}, // an exponent marker with no digits is not part of the number
	}

	for _, tc := range cases {
		got, err := parseFloat(tc.in)
		if err != nil {
			t.Errorf("parseFloat(%q): %v", tc.in, err)

			continue
		}

		if got != tc.want {
			t.Errorf("parseFloat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}

	for _, in := range []string{"", "n/a", "E5"} {
		if _, err := parseFloat(in); err == nil {
			t.Errorf("parseFloat(%q) succeeded; there is no number in it", in)
		}
	}
}

// serveJSON points endpoint at a server answering every request with body,
// and returns the query strings it saw.
func serveJSON(t *testing.T, endpoint remote.EndpointID, body func(q map[string][]string) string) *[]map[string][]string {
	t.Helper()

	var seen []map[string][]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query())

		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, body(r.URL.Query())); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(remote.Reset)

	if err := remote.SetURL(endpoint, server.URL); err != nil {
		t.Fatal(err)
	}

	return &seen
}

// TestSBDBResolverDecodesAnOpenOrbit reads C/2023 A3 as SBDB's identify
// endpoint returned it on 2026-09-23 with full-prec=true, trimmed to the
// fields parsing needs. An open orbit carries both forms: the asteroid form
// with its negative semi-major axis, and the comet form, which is the one that
// can be propagated.
func TestSBDBResolverDecodesAnOpenOrbit(t *testing.T) {
	serveJSON(t, remote.JPLSBDB, func(map[string][]string) string {
		return `{
			"object": {"spkid": "1004083", "fullname": "C/2023 A3 (Tsuchinshan-ATLAS)", "des": "2023 A3",
				"kind": "cu", "orbit_class": {"code": "HYP"}},
			"orbit": {
				"epoch": "2460448.5",
				"elements": [
					{"name": "e", "value": "1.000095368540586"},
					{"name": "a", "value": "-4104.394095058612"},
					{"name": "q", "value": ".3914300748355564"},
					{"name": "i", "value": "139.112109080566"},
					{"name": "om", "value": "21.55947897244586"},
					{"name": "w", "value": "308.4917649633916"},
					{"name": "ma", "value": "-.0004975480989684438"},
					{"name": "tp", "value": "2460581.240845175775"},
					{"name": "per", "value": null},
					{"name": "n", "value": "3.748266769806928E-6"}
				]
			}
		}`
	})

	tar, err := New().Resolve(context.Background(), "C/2023 A3")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if !tar.HasElements {
		t.Fatal("HasElements = false for a complete element set")
	}

	testutil.AssertNear(t, "q", tar.PerihelionDistance.AU(), 0.3914300748355564, 1e-15)
	testutil.AssertNear(t, "tp JD", tar.PerihelionTime.JD(), 2460581.240845175775, 1e-8)
	testutil.AssertEqual(t, "tp scale", tar.PerihelionTime.Scale().String(), "TDB")
	testutil.AssertNear(t, "e", tar.Eccentricity, 1.000095368540586, 1e-15)
	testutil.AssertNear(t, "a", tar.SemiMajorAxis.AU(), -4104.394095058612, 1e-9)
	testutil.AssertNear(t, "ma", tar.MeanAnomaly.Degrees(), -0.0004975480989684438, 1e-18)
}

// TestSBDBResolverDecodesAParabola reads C/-146 P1 as the identify endpoint
// returned it on 2026-09-23: e = 1, and no semi-major axis or mean anomaly at
// all. Before the comet form was decoded, the all-or-nothing gate on a and ma
// left a parabola with no elements.
func TestSBDBResolverDecodesAParabola(t *testing.T) {
	serveJSON(t, remote.JPLSBDB, func(map[string][]string) string {
		return `{
			"object": {"spkid": "1000589", "fullname": "C/-146 P1", "des": "-146 P1",
				"kind": "cu", "orbit_class": {"code": "PAR"}},
			"orbit": {
				"epoch": "1667909.5",
				"elements": [
					{"name": "e", "value": "1.0"}, {"name": "a", "value": null},
					{"name": "q", "value": "0.43"}, {"name": "i", "value": "71"},
					{"name": "om", "value": "330"}, {"name": "w", "value": "261"},
					{"name": "ma", "value": null}, {"name": "tp", "value": "1667909.5"},
					{"name": "per", "value": null}, {"name": "n", "value": null}, {"name": "ad", "value": null}
				]
			}
		}`
	})

	tar, err := New().Resolve(context.Background(), "C/-146 P1")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if !tar.HasElements {
		t.Fatal("HasElements = false for a parabola with a complete comet form")
	}

	testutil.AssertNear(t, "q", tar.PerihelionDistance.AU(), 0.43, 1e-15)
	testutil.AssertNear(t, "tp JD", tar.PerihelionTime.JD(), 1667909.5, 1e-8)

	if tar.SemiMajorAxis != 0 || tar.MeanAnomaly.Radians() != 0 {
		t.Errorf("a parabola has no semi-major axis or mean anomaly; got %v, %v", tar.SemiMajorAxis, tar.MeanAnomaly)
	}
}

// TestSearchBrightDecodesBothFormsAtFullPrecision reads the bulk query's
// answer for two real comets on 2026-09-23 with full-prec=true: C/1937 C1,
// hyperbolic, whose mean anomaly SBDB writes in E-notation, and C/1879 M1,
// a parabola, for which SBDB publishes no semi-major axis and no mean
// anomaly at all. Both have elements; before the comet form was decoded the
// parabola had none.
//
// It also pins the request: without full-prec the bulk query rounds to four
// significant figures, which turns C/1937 C1's e = 1.000162271 into 1.0002.
func TestSearchBrightDecodesBothFormsAtFullPrecision(t *testing.T) {
	const fields = `["full_name","spkid","M1","K1","class","e","a","i","om","w","ma","q","tp","epoch"]`

	seen := serveJSON(t, remote.JPLSBDBQuery, func(q map[string][]string) string {
		if q["sb-kind"][0] != "c" {
			return `{"signature":{"version":"1.0"},"fields":["full_name","spkid","H","G","class"],"data":[],"count":0}`
		}

		return `{"signature":{"version":"1.0"},"fields":` + fields + `,"data":[
			["     C/1937 C1 (Whipple)", 1001030, "4.2", "16.5", "HYP", "1.000162271189241", "-10684.39024184528",
				"41.5517215216521", "128.608063203224", "107.7360323865068", "-2.593805408851336E-5",
				"1.733768710859798", "2428704.564170373471", "2428675.5"],
			["     C/1879 M1 (Swift)", 1000869, "6.1", "17.75", "PAR", "1.0", null,
				"107.0446361700477", "47.45264108264573", "3.742746339649675", null,
				".8963644122870428", "2407467.425177621481", "2407545.5"]
		],"count":2}`
	})

	var comets []resolve.Target

	for tgt, err := range New().SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 6}) {
		if err != nil {
			t.Fatalf("SearchBright: %v", err)
		}

		if tgt.Kind == resolve.KindComet {
			comets = append(comets, tgt)
		}
	}

	if len(comets) != 2 {
		t.Fatalf("got %d comets, want 2", len(comets))
	}

	for _, q := range *seen {
		if q["full-prec"] == nil || q["full-prec"][0] != "true" {
			t.Errorf("a bulk query went out without full-prec=true: %v", q)
		}

		// The answer above carries q and tp whatever was asked; SBDB's does
		// not.
		fields := "," + strings.Join(q["fields"], ",") + ","
		if !strings.Contains(fields, ",q,") || !strings.Contains(fields, ",tp,") {
			t.Errorf("a bulk query did not ask for q and tp: fields=%v", q["fields"])
		}
	}

	whipple, parabola := comets[0], comets[1]

	if !whipple.HasElements || !parabola.HasElements {
		t.Fatalf("HasElements = %v, %v; want both, each has a complete comet form",
			whipple.HasElements, parabola.HasElements)
	}

	testutil.AssertNear(t, "C/1937 C1 e", whipple.Eccentricity, 1.000162271189241, 1e-15)
	testutil.AssertNear(t, "C/1937 C1 ma", whipple.MeanAnomaly.Degrees(), -2.593805408851336e-5, 1e-20)
	testutil.AssertNear(t, "C/1937 C1 q", whipple.PerihelionDistance.AU(), 1.733768710859798, 1e-15)
	testutil.AssertNear(t, "C/1937 C1 tp JD", whipple.PerihelionTime.JD(), 2428704.564170373471, 1e-8)

	testutil.AssertNear(t, "C/1879 M1 q", parabola.PerihelionDistance.AU(), 0.8963644122870428, 1e-15)
	testutil.AssertNear(t, "C/1879 M1 tp JD", parabola.PerihelionTime.JD(), 2407467.425177621481, 1e-8)

	if parabola.SemiMajorAxis != 0 || parabola.MeanAnomaly.Radians() != 0 {
		t.Errorf("a parabola has no semi-major axis or mean anomaly; got %v, %v",
			parabola.SemiMajorAxis, parabola.MeanAnomaly)
	}
}
