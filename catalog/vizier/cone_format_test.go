package vizier

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// coneServer answers every query with an empty CSV, recording the last ADQL
// it was sent and counting requests.
func coneServer(t *testing.T) (adql *string, requests *atomic.Int32) {
	t.Helper()

	var (
		last  string
		count atomic.Int32
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		count.Add(1)

		last = r.PostFormValue("QUERY")

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, "designation,ra,dec\n")
	}))
	t.Cleanup(server.Close)

	redirect(t, server.URL)

	return &last, &count
}

// TestVizierConeSendsItsRadiusExactly holds the radius in the query to the
// float64 the caller asked for (#762). It went out as %f, six decimals of a
// degree: 1 mas and 1.7 mas were sent as 0.000000, an empty cone, and 10 mas
// as 0.000003, 8% wide.
func TestVizierConeSendsItsRadiusExactly(t *testing.T) {
	adql, _ := coneServer(t)
	circle := regexp.MustCompile(`CIRCLE\('ICRS', [-0-9.]+, [-0-9.]+, ([0-9.]+)\)`)

	for _, mas := range []float64{1, 1.7, 10, 50, 1000} {
		radius := angle.Arcsec(mas / 1000)

		req := resolve.ConeRequest{Table: "I/239/hip_main", Center: coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), Radius: radius, Limit: 5}
		New().ConeSearch(context.Background(), req)(func(resolve.Target, error) bool { return true })

		m := circle.FindStringSubmatch(*adql)
		if m == nil {
			t.Fatalf("%g mas: no CIRCLE in the cone query:\n%s", mas, *adql)
		}

		got, err := strconv.ParseFloat(m[1], 64)
		if err != nil || got != radius.Degrees() {
			t.Errorf("%g mas: CIRCLE radius %q reads back as %g deg (%v), want exactly %g deg",
				mas, m[1], got, err, radius.Degrees())
		}
	}
}

// TestVizierConeKeepsNearbyConesApart: two cones whose centers differ by a
// milliarcsecond are two queries. The cache key printed them with %f, so any
// two within 3.6 mas shared an entry and the second was served the first's
// answer.
func TestVizierConeKeepsNearbyConesApart(t *testing.T) {
	_, requests := coneServer(t)
	prov := New()

	for _, ra := range []float64{83.8221, 83.8221 + 1.0/3.6e6} {
		req := resolve.ConeRequest{Table: "I/239/hip_main", Center: coord.NewICRS(angle.Deg(ra), angle.Deg(-5.3911)), Radius: angle.Arcsec(0.002), Limit: 5}
		prov.ConeSearch(context.Background(), req)(func(resolve.Target, error) bool { return true })
	}

	if n := requests.Load(); n != 2 {
		t.Errorf("two cones 1 mas apart sent %d queries, want 2", n)
	}
}

// TestVizierConeRefusesAnInvalidRadius: a NaN, infinite or negative radius is
// the caller's mistake, reported as one before anything is sent.
func TestVizierConeRefusesAnInvalidRadius(t *testing.T) {
	_, requests := coneServer(t)
	prov := New()

	for _, r := range []angle.Angle{angle.Rad(math.NaN()), angle.Rad(math.Inf(1)), angle.Arcsec(-1)} {
		req := resolve.ConeRequest{Table: "I/239/hip_main", Center: coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), Radius: r, Limit: 5}

		var got error

		prov.ConeSearch(context.Background(), req)(func(_ resolve.Target, err error) bool {
			got = err
			return false
		})

		if !errors.Is(got, resolve.ErrInvalidRadius) {
			t.Errorf("radius %g deg: %v, want ErrInvalidRadius", r.Degrees(), got)
		}
	}

	if n := requests.Load(); n != 0 {
		t.Errorf("%d queries sent for invalid radii, want none", n)
	}
}
