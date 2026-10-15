package sbdb

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// A dated SearchBright: comets bounded by where their orbits have them on
// the date (#779).
//
// Without a date a comet is a candidate if it could be bright at perihelion,
// however long ago that was, and at magLimit 5 there were more than 50 of
// them. With one, a comet decades from perihelion is far out and drops out.

// barkerDistance is a parabola's distance from the Sun dt days from
// perihelion, by Barker's equation: with W = 3·√(μ/2q³)·|dt|,
// tan(ν/2) = y − 1/y where y = ∛(W/2 + √(W²/4 + 1)), and r = q(1 + tan²(ν/2)).
// It is the closed form, independent of the propagator under test.
func barkerDistance(q, dtDays float64) float64 {
	au := unit.AU(1).Meters()
	day := constants.Derived.JulianDaySeconds.Value
	mu := constants.IAU.SunGravitationalParameter.Value * day * day / (au * au * au)

	w := 3 * math.Sqrt(mu/(2*q*q*q)) * math.Abs(dtDays)
	y := math.Cbrt(w/2 + math.Sqrt(w*w/4+1))
	s := y - 1/y

	return q * (1 + s*s)
}

func TestSunDistanceAroundMatchesBarkersEquation(t *testing.T) {
	t.Parallel()

	const q = 1.2

	tp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	window := datedWindow.Days()

	for _, tt := range []struct {
		name   string
		offset float64 // days from perihelion
		want   float64
	}{
		// Receding: nearest at the window's start.
		{"400 days after perihelion", 400, barkerDistance(q, 400-window)},
		// Approaching: nearest at the window's end.
		{"400 days before perihelion", -400, barkerDistance(q, -400+window)},
		// Perihelion inside the window.
		{"10 days after perihelion", 10, q},
		{"at perihelion", 0, q},
	} {
		got := sunDistanceAround(q, 1, tp, tp.Add(unit.Days(tt.offset)))

		if math.Abs(got-tt.want) > 1e-9*tt.want {
			t.Errorf("%s: %.12f AU, Barker gives %.12f", tt.name, got, tt.want)
		}
	}
}

// An orbit that cannot be propagated keeps the comet: its perihelion
// distance comes back, the undated bound.
func TestSunDistanceAroundKeepsWhatItCannotPropagate(t *testing.T) {
	t.Parallel()

	tp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	if got := sunDistanceAround(1.2, -0.5, tp, tp.Add(unit.Days(4000))); got != 1.2 {
		t.Errorf("a negative eccentricity gave %g AU, want the perihelion distance back", got)
	}
}

// cometArchive serves the SBDB Query API with no asteroids and the comets
// given, each a name, perihelion distance and perihelion time (TDB JD), all
// with M1 6, K1 10 and e 0.999. It ignores the constraint, honors the limit,
// and records the limit each kind was asked for.
func cometArchive(t *testing.T, comets [][3]string) map[string]string {
	t.Helper()

	var mu sync.Mutex

	limits := map[string]string{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		mu.Lock()
		limits[query.Get("sb-kind")] = query.Get("limit")
		mu.Unlock()

		if query.Get("sb-kind") != "c" {
			_, _ = fmt.Fprint(w, `{"signature":{"version":"1.0"},"fields":["full_name","spkid","H","G","class","e","q"],"data":[],"count":0}`)

			return
		}

		limit, err := strconv.Atoi(query.Get("limit"))
		if err != nil {
			http.Error(w, "no limit", http.StatusBadRequest)

			return
		}

		var data []string

		for i, c := range comets {
			if i == limit {
				break
			}

			data = append(data, fmt.Sprintf(`[%q,"%d","6","10","COM","0.999",%q,%q]`, c[0], 1000001+i, c[1], c[2]))
		}

		_, _ = fmt.Fprintf(w, `{"signature":{"version":"1.0"},"fields":["full_name","spkid","M1","K1","class","e","q","tp"],"data":[%s],"count":%d}`,
			strings.Join(data, ","), len(comets))
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDBQuery, srv.URL); err != nil {
		t.Fatal(err)
	}

	return limits
}

// searchNames runs one search and returns the names, whether it was cut, and
// any other error.
func searchNames(req resolve.BrightRequest) (names []string, cut bool, err error) {
	for tgt, e := range New().SearchBright(context.Background(), req) {
		switch {
		case errors.Is(e, resolve.ErrTruncated):
			cut = true
		case e != nil:
			return names, cut, e
		default:
			names = append(names, tgt.Name)
		}
	}

	return names, cut, nil
}

func TestSearchBrightDatedKeepsTheCometsNearPerihelion(t *testing.T) {
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.LocationUTC)
	jd := at.TT().JD()

	// Both reach perihelion at 1.2 AU, where M1 6 and K1 10 make them about
	// 3.1 at best, so neither is ruled out without a date.
	limits := cometArchive(t, [][3]string{
		{"C/2026 A1 (Now)", "1.2", strconv.FormatFloat(jd, 'f', 3, 64)},
		{"C/2006 B1 (Twenty years ago)", "1.2", strconv.FormatFloat(jd-7305, 'f', 3, 64)},
	})

	names, cut, err := searchNames(resolve.BrightRequest{MaxVMag: 9})
	if err != nil || cut || strings.Join(names, "; ") != "C/2026 A1 (Now); C/2006 B1 (Twenty years ago)" {
		t.Errorf("undated: %q, cut %v, error %v; want both comets", names, cut, err)
	}

	if limits["c"] != "51" {
		t.Errorf("undated: comets asked for limit %s, want 51", limits["c"])
	}

	names, cut, err = searchNames(resolve.BrightRequest{MaxVMag: 9, At: at})
	if err != nil || cut || strings.Join(names, "; ") != "C/2026 A1 (Now)" {
		t.Errorf("dated: %q, cut %v, error %v; want only the comet at perihelion", names, cut, err)
	}

	if limits["c"] != strconv.Itoa(datedCometRows) || limits["a"] != "51" {
		t.Errorf("dated: comets asked for limit %s and asteroids %s, want %d and 51", limits["c"], limits["a"], datedCometRows)
	}
}

// A dated search has every comet SBDB holds, so whether it was cut is
// whether more than the limit passed the date, and a caller's own limit is
// not a cut.
func TestSearchBrightDatedCutsAtTheLimit(t *testing.T) {
	at := time.Date(2026, 10, 15, 0, 0, 0, 0, time.LocationUTC)
	tp := strconv.FormatFloat(at.TT().JD(), 'f', 3, 64)

	comets := make([][3]string, 60)
	for i := range comets {
		comets[i] = [3]string{fmt.Sprintf("C/2026 A%d", i+1), "1.2", tp}
	}

	cometArchive(t, comets)

	names, cut, err := searchNames(resolve.BrightRequest{MaxVMag: 9, At: at})
	if err != nil || !cut || len(names) != 50 {
		t.Errorf("%d comets, cut %v, error %v; want 50 and the cut", len(names), cut, err)
	}

	names, cut, err = searchNames(resolve.BrightRequest{MaxVMag: 9, At: at, Limit: 5})
	if err != nil || cut || len(names) != 5 {
		t.Errorf("caller's limit: %d comets, cut %v, error %v; want 5 and no cut", len(names), cut, err)
	}
}
