package sbdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// SearchBright's perihelion bound (#779).
//
// It ranked candidates by absolute magnitude alone, and the bodies with the
// lowest H are distant ones: of the 50 asteroids SBDB listed first under H 6,
// 9 or 11, 47 were trans-Neptunian, while Juno, Hebe and Iris never made the
// list at any bound. The bodies below are real, with H, G, M1, K1 and q as
// SBDB published them on 2026-10-14.

func TestCouldReachBoundsByPerihelion(t *testing.T) {
	t.Parallel()

	asteroid := func(h, g float64) resolve.Target { return resolve.Target{H: h, G: g, HasH: true} }
	comet := func(m1, k1 float64) resolve.Target { return resolve.Target{M1: m1, K1: k1, HasM1: true} }

	tests := []struct {
		name    string
		body    resolve.Target
		q       float64
		maxVMag float64
		want    bool
	}{
		// 5.19 + 5·log10(1.988·0.9713) = 6.62 at best.
		{"3 Juno at 7", asteroid(5.19, 0.15), 1.988, 7, true},
		{"3 Juno at 6.5", asteroid(5.19, 0.15), 1.988, 6.5, false},
		// −1.26 + 5·log10(38.16·37.14) = 14.50 at best.
		{"136199 Eris at 14", asteroid(-1.26, 0.15), 38.16, 14, false},
		{"136199 Eris at 15", asteroid(-1.26, 0.15), 38.16, 15, true},
		// Inside the Earth's aphelion nothing bounds it.
		{"an Earth-crosser", asteroid(20, 0.15), 0.9, 3, true},
		// The H-G phase term can brighten a body when G > 1.
		{"G outside [0, 1]", asteroid(20, 1.5), 30, 3, true},
		// 4.2 + 5·log10(1.7338 − 1.0167) + 16.5·log10(1.7338) = 7.42 at best.
		{"C/1937 C1 at 7", comet(4.2, 16.5), 1.733768710859798, 7, false},
		{"C/1937 C1 at 8", comet(4.2, 16.5), 1.733768710859798, 8, true},
		// A negative K1 brightens a comet with distance, so no bound.
		{"K1 below zero", comet(10, -1), 30, 3, true},
		{"a comet inside the Earth's aphelion", comet(10, 10), 0.5, 3, true},
	}

	for _, tt := range tests {
		if got := couldReach(tt.body, tt.q, tt.maxVMag); got != tt.want {
			t.Errorf("%s: couldReach = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// A body whose best magnitude is the bound itself does not pass: the bound
// asks for bodies brighter than it, as Stage 2 does.
func TestCouldReachIsStrict(t *testing.T) {
	t.Parallel()

	const q, maxVMag = 2.0, 7.0

	h := maxVMag - 5*math.Log10(q*(q-earthAphelionAU))

	if couldReach(resolve.Target{H: h, HasH: true}, q, maxVMag) {
		t.Errorf("a body whose best magnitude is exactly %g passed", maxVMag)
	}

	if !couldReach(resolve.Target{H: h - 1e-9, HasH: true}, q, maxVMag) {
		t.Errorf("a body a hair brighter than %g at best did not pass", maxVMag)
	}
}

// cdataAdmits evaluates an sb-cdata constraint, of the subset of SBDB's
// syntax brightConstraint writes, against one body's fields. A field absent
// from fields is undefined.
func cdataAdmits(t *testing.T, node any, fields map[string]float64) bool {
	t.Helper()

	switch n := node.(type) {
	case string:
		parts := strings.Split(n, "|")

		v, defined := fields[parts[0]]
		if parts[1] == "ND" {
			return !defined
		}

		bound, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			t.Fatalf("constraint %q: %v", n, err)
		}

		switch parts[1] {
		case "LT":
			return defined && v < bound
		case "GE":
			return defined && v >= bound
		}

		t.Fatalf("constraint %q: operator not in brightConstraint's subset", n)
	case map[string]any:
		for op, list := range n {
			all := op == "AND"

			children, ok := list.([]any)
			if !ok {
				t.Fatalf("constraint %q holds %#v, not a list", op, list)
			}

			for _, child := range children {
				if cdataAdmits(t, child, fields) != all {
					return !all
				}
			}

			return all
		}
	}

	t.Fatalf("unexpected constraint node %#v", node)

	return false
}

// The constraint SBDB evaluates must admit every body the exact test would
// keep, or the exact test never sees it. Checked over a grid of absolute
// magnitudes and perihelia for both kinds, at several bounds.
func TestBrightConstraintAdmitsEveryBodyThatCouldReach(t *testing.T) {
	t.Parallel()

	for _, maxVMag := range []float64{2, 7, 10.5, 16} {
		for _, kind := range []string{"H", "M1"} {
			var root any
			if err := json.Unmarshal([]byte(brightConstraint(kind, maxVMag)), &root); err != nil {
				t.Fatalf("%s at %g: the constraint is not JSON: %v", kind, maxVMag, err)
			}

			checked := 0

			for mag := maxVMag - 25; mag < maxVMag+brightnessMargin; mag += 0.25 {
				for lq := -1.0; lq <= 2.0; lq += 0.05 {
					q := math.Pow(10, lq)

					for _, slope := range []float64{0, 0.15, 1, 5, 10, 20} {
						body := resolve.Target{H: mag, G: slope, HasH: true}
						fields := map[string]float64{"H": mag, "q": q}

						if kind == "M1" {
							body = resolve.Target{M1: mag, K1: slope, HasM1: true}
							fields = map[string]float64{"M1": mag, "K1": slope, "q": q}
						}

						if body.HasH && slope > 1 {
							continue
						}

						if couldReach(body, q, maxVMag) && !cdataAdmits(t, root, fields) {
							t.Fatalf("%s at %g: a body with %s %g, q %g, slope %g could reach the bound but SBDB would not send it",
								kind, maxVMag, kind, mag, q, slope)
						}

						checked++
					}
				}
			}

			if checked < 1000 {
				t.Fatalf("%s at %g: only %d bodies checked", kind, maxVMag, checked)
			}
		}
	}
}

// And it must exclude what the bug let in: Eris at a bound of 7, and a comet
// whose K1 is missing or negative must still be admitted.
func TestBrightConstraintExcludesTheDistantBodies(t *testing.T) {
	t.Parallel()

	parse := func(kind string, maxVMag float64) any {
		var root any
		if err := json.Unmarshal([]byte(brightConstraint(kind, maxVMag)), &root); err != nil {
			t.Fatalf("the constraint is not JSON: %v", err)
		}

		return root
	}

	asteroids := parse("H", 7)

	if cdataAdmits(t, asteroids, map[string]float64{"H": -1.26, "q": 38.16}) {
		t.Error("Eris is admitted at a bound of 7, where it is never brighter than 14.5")
	}

	if !cdataAdmits(t, asteroids, map[string]float64{"H": 5.19, "q": 1.988}) {
		t.Error("Juno is not admitted at a bound of 7, where it reaches 6.6")
	}

	comets := parse("M1", 7)

	if !cdataAdmits(t, comets, map[string]float64{"M1": 8, "q": 30}) {
		t.Error("a comet with no K1 is not admitted, though nothing bounds it")
	}

	if !cdataAdmits(t, comets, map[string]float64{"M1": 8, "K1": -2, "q": 30}) {
		t.Error("a comet with a negative K1 is not admitted, though nothing bounds it")
	}

	if cdataAdmits(t, comets, map[string]float64{"M1": 8, "K1": 10, "q": 30}) {
		t.Error("a comet with perihelion at 30 AU and M1 8 is admitted at a bound of 7")
	}
}

// perihelionArchive serves the SBDB Query API with the asteroid rows given,
// each a name, H and q, ignoring the constraint as a server that did not
// apply it would, and honoring the limit. It records the last sb-cdata.
func perihelionArchive(t *testing.T, rows [][3]string, cdata *string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		if query.Get("sb-kind") != "a" {
			_, _ = fmt.Fprint(w, `{"signature":{"version":"1.0"},"fields":["full_name","spkid","M1","K1","class","e","q"],"data":[],"count":0}`)

			return
		}

		*cdata = query.Get("sb-cdata")

		limit, err := strconv.Atoi(query.Get("limit"))
		if err != nil {
			http.Error(w, "no limit", http.StatusBadRequest)

			return
		}

		var data []string

		for i, row := range rows {
			if i == limit {
				break
			}

			data = append(data, fmt.Sprintf(`[%q,"%d",%q,"0.15","MBA","0.1",%q]`, row[0], 2000001+i, row[1], row[2]))
		}

		_, _ = fmt.Fprintf(w, `{"signature":{"version":"1.0"},"fields":["full_name","spkid","H","G","class","e","q"],"data":[%s],"count":%d}`,
			strings.Join(data, ","), len(rows))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDBQuery, srv.URL); err != nil {
		t.Fatal(err)
	}
}

func TestSearchBrightKeepsOnlyBodiesThatCouldReach(t *testing.T) {
	var cdata string

	perihelionArchive(t, [][3]string{
		{"136199 Eris (2003 UB313)", "-1.26", "38.16"},
		{"3 Juno (A804 RA)", "5.19", "1.988"},
		{"7 Iris (A847 PA)", "5.70", "1.836"},
	}, &cdata)

	var names []string

	for tgt, err := range New().SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 7}) {
		if err != nil {
			t.Fatalf("SearchBright: %v", err)
		}

		names = append(names, tgt.Name)
	}

	if strings.Join(names, "; ") != "3 Juno (A804 RA); 7 Iris (A847 PA)" {
		t.Errorf("candidates %q, want Juno and Iris and not Eris", names)
	}

	if !strings.Contains(cdata, `"q|LT|`) {
		t.Errorf("sb-cdata = %s, want a perihelion bound", cdata)
	}
}

// Whether the list was cut is decided by the rows SBDB sent. A full page of
// bodies that the exact test then drops still means SBDB had more.
func TestSearchBrightSaysItWasCutWhenTheExactTestDropsTheRows(t *testing.T) {
	var cdata string

	rows := make([][3]string, 60)
	for i := range rows {
		rows[i] = [3]string{fmt.Sprintf("TNO %d", i), "4", "40"}
	}

	perihelionArchive(t, rows, &cdata)

	got, cut := 0, false

	for _, err := range New().SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 7}) {
		switch {
		case errors.Is(err, resolve.ErrTruncated):
			cut = true
		case err != nil:
			t.Fatalf("SearchBright: %v", err)
		default:
			got++
		}
	}

	if got != 0 {
		t.Errorf("got %d candidates, want none: every row is a TNO that never reaches 7", got)
	}

	if !cut {
		t.Error("no ErrTruncated, though SBDB sent a full page")
	}
}
