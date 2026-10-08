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
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"

	"github.com/TuSKan/astrogo/remote"
)

func TestVizierOfflineConeSearch(t *testing.T) {
	csvData := "designation,ra,dec,epoch_jd\n" +
		`"18375080-4835411 ",279.461678,-48.594772,2451462.5171` + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")

		if _, err := fmt.Fprint(w, csvData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(5),
	}

	iter := prov.ConeSearch(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("Unexpected err: %v", err)
		}

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("expected 1 parsed target, got %d", len(targets))
	}

	got := targets[0]
	if got.Designation != "18375080-4835411" {
		t.Errorf("Designation = %q, want %q", got.Designation, "18375080-4835411")
	}

	if got.Catalog != "vizier" {
		t.Errorf("Catalog = %q, want vizier", got.Catalog)
	}

	if math.Abs(got.Coord.RA().Degrees()-279.461678) > 1e-6 {
		t.Errorf("RA = %v, want 279.461678", got.Coord.RA().Degrees())
	}

	if math.Abs(got.Coord.Dec().Degrees()-(-48.594772)) > 1e-6 {
		t.Errorf("Dec = %v, want -48.594772", got.Coord.Dec().Degrees())
	}
}

func TestParseCSVMissingColumn(t *testing.T) {
	if _, err := parseCSV(strings.NewReader("ra,dec\n1,2\n"), tableSchemas[defaultTable]); !errors.Is(err, ErrUnexpectedSchema) {
		t.Errorf("expected ErrUnexpectedSchema, got %v", err)
	}
}

// TestParseCSV_PopulatesIDAndEpoch is a regression test: ID used to be left
// empty (only Name/Designation were set from the same designation value),
// and Epoch used to never be set despite each table having a genuinely
// different native reference epoch.
func TestParseCSV_PopulatesIDAndEpoch(t *testing.T) {
	schema := tableSchemas["I/239/hip_main"]

	targets, err := parseCSV(strings.NewReader("designation,ra,dec\n32349,101.28,-16.71\n"), schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	got := targets[0]

	if got.ID != "32349" {
		t.Errorf("ID = %q, want %q", got.ID, "32349")
	}

	if got.ID != got.Designation {
		t.Errorf("expected ID to match Designation (%q), got %q", got.Designation, got.ID)
	}

	if !got.Epoch.Equal(epochHipparcos) {
		t.Errorf("Epoch = %v, want the table's own Hipparcos epoch %v", got.Epoch, epochHipparcos)
	}
}

// TestVizierConeSearch_RegisteredTable confirms ConeSearch works against a
// second registered table (Hipparcos), not just the default 2MASS one —
// proving the table-parameterization mechanism generalizes, per the schema
// registry in tables.go.
func TestVizierConeSearch_RegisteredTable(t *testing.T) {
	csvData := "designation,ra,dec\n" + "1,10.68470,41.26875\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		adql := r.PostFormValue("QUERY")
		if !strings.Contains(adql, `FROM "I/239/hip_main"`) {
			t.Errorf("expected query against I/239/hip_main, got: %s", adql)
		}

		if !strings.Contains(adql, "RAICRS") || !strings.Contains(adql, "DEICRS") {
			t.Errorf("expected Hipparcos RA/Dec columns in query, got: %s", adql)
		}

		w.Header().Set("Content-Type", "text/csv")

		if _, err := fmt.Fprint(w, csvData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	req := resolve.ConeRequest{
		Table:  "I/239/hip_main",
		Center: coord.NewICRS(angle.Deg(10.684), angle.Deg(41.269)),
		Radius: angle.Deg(0.01),
	}

	var targets []resolve.Target

	prov.ConeSearch(context.Background(), req)(func(tar resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "Kind", targets[0].Kind, resolve.KindStar)
	testutil.AssertEqual(t, "HasCoord", targets[0].HasCoord, true)
}

// TestVizierConeSearch_UnknownTable confirms a table absent from the
// schema registry returns ErrUnknownTable rather than guessing column names.
func TestVizierConeSearch_UnknownTable(t *testing.T) {
	prov := New()

	req := resolve.ConeRequest{
		Table:  "X/999/not_a_real_table",
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(1),
	}

	iter := prov.ConeSearch(context.Background(), req)
	iter(func(_ resolve.Target, err error) bool {
		if !errors.Is(err, ErrUnknownTable) {
			t.Fatalf("expected ErrUnknownTable, got %v", err)
		}

		return false
	})
}

// TestVizierConeSearch_CacheKeyIncludesTable is a regression test: the same
// cone queried against two different tables must not collide on one cache
// entry (the cache key previously covered only ra/dec/rad/limit).
func TestVizierConeSearch_CacheKeyIncludesTable(t *testing.T) {
	var calls int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++

		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}

		adql := r.PostFormValue("QUERY")

		w.Header().Set("Content-Type", "text/csv")

		if strings.Contains(adql, "I/239/hip_main") {
			fmt.Fprint(w, "designation,ra,dec\n1,10.68470,41.26875\n") //nolint:errcheck // test server
		} else {
			fmt.Fprint(w, "designation,ra,dec,epoch_jd\n2,10.68470,41.26875,2451545.0\n") //nolint:errcheck // test server
		}
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	center := coord.NewICRS(angle.Deg(10.684), angle.Deg(41.269))
	radius := angle.Deg(0.01)

	var (
		defaultTargets, hipTargets []resolve.Target
	)

	prov.ConeSearch(context.Background(), resolve.ConeRequest{Center: center, Radius: radius})(func(tar resolve.Target, _ error) bool {
		defaultTargets = append(defaultTargets, tar)
		return true
	})

	prov.ConeSearch(context.Background(), resolve.ConeRequest{Table: "I/239/hip_main", Center: center, Radius: radius})(func(tar resolve.Target, _ error) bool {
		hipTargets = append(hipTargets, tar)
		return true
	})

	if calls != 2 {
		t.Fatalf("expected 2 live requests (no cache collision), got %d", calls)
	}

	if len(defaultTargets) != 1 || defaultTargets[0].Designation != "2" {
		t.Fatalf("expected default-table target Designation=2, got %+v", defaultTargets)
	}

	if len(hipTargets) != 1 || hipTargets[0].Designation != "1" {
		t.Fatalf("expected hip_main target Designation=1, got %+v", hipTargets)
	}
}

func TestProviderInterface(t *testing.T) {
	p := New()
	if p.Name() != "vizier" {
		t.Errorf("expected vizier, got %s", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapConeSearch {
		t.Errorf("expected CapConeSearch, got %v", caps)
	}

	_, err := p.Resolve(context.Background(), "foo")
	if !errors.Is(err, resolve.ErrUnsupported) {
		t.Errorf("Resolve error = %v, want ErrUnsupported — vizier is cone-search only", err)
	}

	if got, serr := p.Search(context.Background(), "foo"); got != nil || !errors.Is(serr, resolve.ErrUnsupported) {
		t.Errorf("Search = %v, %v; want nil, ErrUnsupported", got, serr)
	}

	// errTransport keeps this default (non-network-tagged) test fully
	// offline — see CLAUDE.md's build-tag convention.
	redirect(t, "http://127.0.0.1:1")

	iter := p.ConeSearch(context.Background(), resolve.ConeRequest{})
	iter(func(_ resolve.Target, err error) bool {
		if err == nil {
			t.Error("expected an error with no transport reachable")
		}

		return false
	})
}

// redirect points endpoint id at a test server for the duration of one
// test. It replaces the old http.RoundTripper injection: remote/api's
// Client is opaque by design, and every request resolves its URL through
// remote.URL(id) anyway, so the registry is the natural seam.
func redirect(t *testing.T, url string) {
	t.Helper()

	id := remote.VizieR

	scope := remote.Capture(id)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(id, url); err != nil {
		t.Fatalf("SetURL(%s): %v", id, err)
	}
}

// The cone query orders by distance from the center, so a capped result is
// the nearest sources rather than whichever the service reaches first (#605).
// Its live counterpart is TestVizierConeSearchReturnsTheNearestSources.
func TestVizierConeQueryOrdersByDistance(t *testing.T) {
	var adql string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		adql = r.PostFormValue("QUERY")

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, "designation,ra,dec\n1,83.8221,-5.3911\n")
	}))
	defer server.Close()

	redirect(t, server.URL)

	req := resolve.ConeRequest{Table: "I/239/hip_main", Center: coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), Radius: angle.Deg(5), Limit: 5}
	New().ConeSearch(context.Background(), req)(func(resolve.Target, error) bool { return true })

	for _, want := range []string{
		"DISTANCE(POINT('ICRS', RAICRS, DEICRS), POINT('ICRS', 83.822100, -5.391100)) AS dist",
		"ORDER BY dist ASC",
		"TOP 5\n",
	} {
		if !strings.Contains(adql, want) {
			t.Errorf("the cone query lacks %q:\n%s", want, adql)
		}
	}
}

// Each table's epoch is the Julian epoch its comment names: J2000 for 2MASS's
// "raj2000" columns, J1991.25 for Hipparcos, J2016.0 for Gaia DR3, each that
// many years of 365.25 days from J2000 (JD 2451545.0, TT). Gaia's was JD
// 2457388.5, midnight on 2016 January 1 rather than J2016.0's noon (#611).
func TestTableEpochsAreTheirJulianEpochs(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		epoch time.Time
		years float64
	}{
		{"2MASS", epoch2MASS, 0},
		{"Hipparcos", epochHipparcos, 1991.25 - 2000},
		{"Gaia DR3", epochGaiaDR3, 2016 - 2000},
	} {
		want := 2451545.0 + c.years*365.25
		if got := c.epoch.TT().JD(); math.Abs(got-want) > 1e-9 {
			t.Errorf("%s epoch is JD %.6f (TT), want %.6f", c.name, got, want)
		}
	}
}

// TestVizierConeCenterIsMovedToTheTablesEpoch: a center given at J2000 with
// a proper motion is searched where the table has the star (#627).
// Barnard's star, as SIMBAD gives it, is moved back to Hipparcos's J1991.25
// and lands 0.35" from its own row there, HIP 87937 at (269.45402305,
// 4.66828815), where unmoved it would be 91" away.
func TestVizierConeCenterIsMovedToTheTablesEpoch(t *testing.T) {
	var adql string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		adql = r.PostFormValue("QUERY")

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, "designation,ra,dec\n")
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	barnard := coord.NewICRSWithKinematics(angle.Deg(269.4520769586), angle.Deg(4.6933649666),
		angle.Arcsec(-0.8015510), angle.Arcsec(10.3623940), angle.Arcsec(0.5469759), unit.KmPerSec(-110.11))

	req := resolve.ConeRequest{Table: "I/239/hip_main", Center: barnard, Epoch: time.J2000(), Radius: angle.Arcsec(5), Limit: 5}
	prov.ConeSearch(context.Background(), req)(func(resolve.Target, error) bool { return true })

	m := regexp.MustCompile(`CIRCLE\('ICRS', ([-0-9.]+), ([-0-9.]+),`).FindStringSubmatch(adql)
	if m == nil {
		t.Fatalf("no CIRCLE in the cone query:\n%s", adql)
	}

	ra, errRA := strconv.ParseFloat(m[1], 64)
	dec, errDec := strconv.ParseFloat(m[2], 64)

	if errRA != nil || errDec != nil {
		t.Fatalf("unparseable CIRCLE center %q, %q", m[1], m[2])
	}

	sep := coord.Separation(coord.NewICRS(angle.Deg(ra), angle.Deg(dec)),
		coord.NewICRS(angle.Deg(269.45402305), angle.Deg(4.66828815))).Arcseconds()
	t.Logf("cone centered %.3f\" from HIP 87937 at J1991.25", sep)

	if sep > 0.5 {
		t.Errorf("cone centered %.3f\" from Barnard's star's Hipparcos row, want under 0.5", sep)
	}
}

// TestTwoMASSRowsCarryTheNightTheyWereObserved: a 2MASS position is where its
// source was on the night 2MASS observed it, between 1997 and 2001, and the
// table publishes that date per row as a Julian date. Every row used to be
// stamped J2000 on the strength of the "raj2000"/"dej2000" column names, which
// state the equinox and frame, not an epoch (#628). The rows below are the
// live ones for Barnard's star and the M31 nucleus.
func TestTwoMASSRowsCarryTheNightTheyWereObserved(t *testing.T) {
	t.Parallel()

	const rows = "designation,ra,dec,epoch_jd\n" +
		`"17574849+0441405 ",269.452044,4.694597,2451692.9284` + "\n" +
		`"00424433+4116085 ",10.684737,41.269035,2450745.8589` + "\n"

	out, err := parseCSV(strings.NewReader(rows), tableSchemas[defaultTable])
	if err != nil {
		t.Fatalf("parseCSV: %v", err)
	}

	for i, jd := range []float64{2451692.9284, 2450745.8589} {
		if got := out[i].Epoch.JD(); math.Abs(got-jd) > 1e-9 {
			t.Errorf("row %d: epoch JD %.6f, want %.6f, the night it was observed", i, got, jd)
		}
	}

	// A 2MASS answer without the column is a changed schema, not a J2000 row.
	if _, err := parseCSV(strings.NewReader("designation,ra,dec\n1,10.68,41.27\n"), tableSchemas[defaultTable]); !errors.Is(err, ErrUnexpectedSchema) {
		t.Errorf("2MASS rows without epoch_jd: error = %v, want ErrUnexpectedSchema", err)
	}

	// A table at one epoch stamps every row with it.
	hip, err := parseCSV(strings.NewReader("designation,ra,dec\n87937,269.45402305,4.66828815\n"), tableSchemas["I/239/hip_main"])
	if err != nil {
		t.Fatalf("parseCSV(Hipparcos): %v", err)
	}

	if !hip[0].Epoch.Equal(epochHipparcos) {
		t.Errorf("Hipparcos row epoch %v, want J1991.25", hip[0].Epoch)
	}
}

// TestOnlyTwoMASSIsAskedForRowEpochs: the cone query asks for a row epoch
// column exactly where the table has one.
func TestOnlyTwoMASSIsAskedForRowEpochs(t *testing.T) {
	var queries []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		queries = append(queries, r.PostFormValue("QUERY"))

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, "designation,ra,dec,epoch_jd\n")
	}))
	defer server.Close()

	prov := New()

	redirect(t, server.URL)

	for _, table := range []string{"", "I/239/hip_main", "I/355/gaiadr3"} {
		req := resolve.ConeRequest{Table: table, Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)), Radius: angle.Arcsec(5)}
		prov.ConeSearch(context.Background(), req)(func(resolve.Target, error) bool { return true })
	}

	if len(queries) != 3 {
		t.Fatalf("%d queries, want 3", len(queries))
	}

	for i, want := range []bool{true, false, false} {
		if asked := strings.Contains(queries[i], "jd as epoch_jd"); asked != want {
			t.Errorf("query %d asks for row epochs = %v, want %v:\n%s", i, asked, want, queries[i])
		}
	}
}
