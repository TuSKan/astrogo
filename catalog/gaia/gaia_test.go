package gaia

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
	"github.com/TuSKan/astrogo/internal/votable"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"

	"github.com/TuSKan/astrogo/remote"
)

func TestGaiaOfflineConeSearch(t *testing.T) {
	csvData := `source_id,ra,dec,pmra,pmdec,parallax
123456789,10.684,41.269,1.1,-2.2,5.5
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")

		if _, err := fmt.Fprint(w, csvData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(5),
	}

	iter := prov.ConeSearch(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("Expected 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "ID", targets[0].ID, "123456789")
	testutil.AssertEqual(t, "Kind", string(targets[0].Kind), string(resolve.KindStar))
	testutil.AssertEqual(t, "Catalog", targets[0].Catalog, "Gaia DR3")

	// Gaia DR3's positions are at J2016.0, the Julian epoch 16 years of 365.25
	// days after J2000: JD 2457389.0. It was 2457388.5, half a day early (#611).
	if got := targets[0].Epoch.TT().JD(); math.Abs(got-(2451545.0+16*365.25)) > 1e-9 {
		t.Errorf("Epoch is JD %.6f (TT), want J2016.0 = 2457389.0", got)
	}
}

// TestGaiaOfflineConeSearch_SkipsUnparseableRow is a regression test: a row
// with a malformed RA/Dec must be skipped entirely, never silently become a
// fake (0,0) position reported as HasCoord=true (the bug class this
// provider used to have — see catalog/catalog.go's trustworthyCoord, which
// exists as defense in depth against exactly this).
func TestGaiaOfflineConeSearch_SkipsUnparseableRow(t *testing.T) {
	csvData := `source_id,ra,dec,pmra,pmdec,parallax
111111111,not-a-number,41.269,1.1,-2.2,5.5
222222222,10.684,41.269,1.1,-2.2,5.5
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")

		if _, err := fmt.Fprint(w, csvData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)),
		Radius: angle.Deg(5),
	}

	iter := prov.ConeSearch(context.Background(), req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		targets = append(targets, tar)

		return true
	})

	if len(targets) != 1 {
		t.Fatalf("expected the unparseable row to be skipped, leaving 1 target, got %d", len(targets))
	}

	testutil.AssertEqual(t, "ID", targets[0].ID, "222222222")

	if !targets[0].HasCoord || targets[0].Coord.IsZero() {
		t.Errorf("expected a real, non-zero coordinate, got HasCoord=%v Coord=%v", targets[0].HasCoord, targets[0].Coord)
	}
}

func TestProviderInterface(t *testing.T) {
	p := newForTest(t)
	testutil.AssertEqual(t, "Name", p.Name(), "gaia")

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapConeSearch {
		t.Errorf("expected CapConeSearch, got %v", caps)
	}

	_, err := p.Resolve(context.Background(), "foo")
	if !errors.Is(err, resolve.ErrUnsupported) {
		t.Errorf("Resolve error = %v, want ErrUnsupported — gaia is cone-search only", err)
	}

	if got, serr := p.Search(context.Background(), "foo"); got != nil || !errors.Is(serr, resolve.ErrUnsupported) {
		t.Errorf("Search = %v, %v; want nil, ErrUnsupported", got, serr)
	}
}

// redirect points endpoint id at a test server for the duration of one
// test. It replaces the old http.RoundTripper injection: remote/api's
// Client is opaque by design, and every request resolves its URL through
// remote.URL(id) anyway, so the registry is the natural seam.
// newForTest builds a provider against the default archive.
//
// The tests redirect [DefaultEndpoint] to a local server, so what they
// exercise is this package's request building and CSV parsing rather than any
// archive. Naming the constant rather than an endpoint keeps the two in step:
// were the default to move, a test redirecting the old one would quietly
// exercise nothing.
func newForTest(t *testing.T) *Provider {
	t.Helper()

	p, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return p
}

func redirect(t *testing.T, url string) {
	t.Helper()

	scope := remote.Capture(DefaultEndpoint)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(DefaultEndpoint, url); err != nil {
		t.Fatalf("SetURL(%s): %v", DefaultEndpoint, err)
	}
}

// TestParseVOTableReportsAWebPageAsDowntime is #300 on the VOTable side.
//
// #301 made the message legible — "the service answered with a web page, not a
// VOTable" instead of an XML syntax error pointing at a parser bug that does
// not exist — but the sentinel carrying that distinction lives in
// internal/votable, which no program outside this module can import. So a
// caller could read the sentence and not branch on it.
//
// remote.ErrNotServingData is what it can branch on, and the two cases want
// opposite handling: an archive serving its maintenance page should be retried
// later, a malformed VOTable should not be retried at all.
func TestParseVOTableReportsAWebPageAsDowntime(t *testing.T) {
	t.Parallel()

	const page = `<!DOCTYPE html>
<html lang="en"><head><title>Gaia archive</title></head>
<body><h1>The archive is undergoing maintenance</h1></body></html>
`

	_, err := parseVOTable(strings.NewReader(page))
	if err == nil {
		t.Fatal("a web page parsed as a VOTable without error")
	}

	if !errors.Is(err, remote.ErrNotServingData) {
		t.Errorf("err = %v, which does not match remote.ErrNotServingData", err)
	}

	// The underlying sentence survives the wrap, so a log line still says what
	// was actually seen rather than only that something was not data.
	if !errors.Is(err, votable.ErrNotVOTable) {
		t.Errorf("err = %v, which no longer matches votable.ErrNotVOTable", err)
	}
}

// TestParseVOTableDoesNotBlameTheServiceForABadDocument keeps the distinction
// pointing both ways: a genuinely malformed VOTable must not claim the archive
// is down, or a caller reads it as transient and retries forever.
func TestParseVOTableDoesNotBlameTheServiceForABadDocument(t *testing.T) {
	t.Parallel()

	// Well-formed XML, recognizably a VOTable, and truncated mid-table.
	const broken = `<?xml version="1.0"?><VOTABLE><RESOURCE><TABLE><DATA><TABLEDATA><TR><TD>1`

	_, err := parseVOTable(strings.NewReader(broken))
	if err == nil {
		t.Skip("this document parsed cleanly; it is not a useful negative case")
	}

	if errors.Is(err, remote.ErrNotServingData) {
		t.Errorf("a malformed VOTable was reported as the service not serving data: %v", err)
	}
}

// TestVMagOnlyWhereTheColorRelationHolds: V comes from Riello et al.'s (2021)
// G − V relation inside the colors it was fitted over, from nothing outside
// them, and from G alone when there is no color at all.
//
// It used to apply its own copy of the cubic at any color and mark the result
// as a real magnitude, so an L dwarf at BP−RP = 6 came back with a V the
// relation does not support (#530). A known extreme color is evidence that G
// is a poor stand-in for V; an absent one is not, so the two cases differ on
// purpose.
func TestVMagOnlyWhereTheColorRelationHolds(t *testing.T) {
	t.Parallel()

	col := map[string]int{"source_id": 0, "ra": 1, "dec": 2, "phot_g_mean_mag": 3, "bp_rp": 4}

	for _, tc := range []struct {
		name    string
		bpRp    string
		wantHas bool
		wantV   float64
	}{
		{"solar color", "0.82", true, 12.15247},
		{"just inside the red edge", "4.99", true, 0},
		{"an L dwarf past the fitted range", "6.0", false, 0},
		{"bluer than anything fitted", "-0.8", false, 0},
		{"no color", "", true, 12.0},
	} {
		target, ok := targetFromRow([]string{"1", "10.0", "20.0", "12.0", tc.bpRp}, col)
		if !ok {
			t.Fatalf("%s: row rejected", tc.name)
		}

		if target.HasVMag != tc.wantHas {
			t.Errorf("%s: HasVMag = %v, want %v (V = %.3f)", tc.name, target.HasVMag, tc.wantHas, target.VMag)

			continue
		}

		if tc.wantV != 0 && math.Abs(target.VMag-tc.wantV) > 1e-3 {
			t.Errorf("%s: V = %.5f, want %.5f", tc.name, target.VMag, tc.wantV)
		}
	}
}

// The cone query orders by distance from the center, so a capped result is
// the nearest sources rather than whichever the archive reaches first (#605).
// Its live counterpart is TestGaiaConeSearchReturnsTheNearestSources.
func TestGaiaConeQueryOrdersByDistance(t *testing.T) {
	var adql string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		adql = r.PostFormValue("QUERY")

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, "source_id,ra,dec\n1,83.8221,-5.3911\n")
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	req := resolve.ConeRequest{Center: coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), Radius: angle.Deg(0.2), Limit: 5}
	prov.ConeSearch(context.Background(), req)(func(resolve.Target, error) bool { return true })

	for _, want := range []string{
		"DISTANCE(POINT('ICRS', ra, dec), POINT('ICRS', 83.822100, -5.391100)) AS dist",
		"ORDER BY dist ASC",
		"TOP 5 ",
	} {
		if !strings.Contains(adql, want) {
			t.Errorf("the cone query lacks %q:\n%s", want, adql)
		}
	}
}

// TestGaiaConeCenterIsMovedToJ2016: a center given at J2000 with a proper
// motion is searched where Gaia DR3 has the star, at J2016.0 (#627).
// Barnard's star, as SIMBAD gives it, lands within 0.2" of its own Gaia
// row, 4472832130942575872 at (269.4485025254, 4.7394200511); unmoved, the
// search would be centered 166" away.
func TestGaiaConeCenterIsMovedToJ2016(t *testing.T) {
	var adql string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		adql = r.PostFormValue("QUERY")

		w.Header().Set("Content-Type", "text/csv")
		_, _ = fmt.Fprint(w, "source_id,ra,dec\n")
	}))
	defer server.Close()

	prov := newForTest(t)

	redirect(t, server.URL)

	barnard := coord.NewICRSWithKinematics(angle.Deg(269.4520769586), angle.Deg(4.6933649666),
		angle.Arcsec(-0.8015510), angle.Arcsec(10.3623940), angle.Arcsec(0.5469759), unit.KmPerSec(-110.11))

	req := resolve.ConeRequest{Center: barnard, Epoch: time.J2000(), Radius: angle.Arcsec(5), Limit: 5}
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

	center := coord.NewICRS(angle.Deg(ra), angle.Deg(dec))
	gaiaRow := coord.NewICRS(angle.Deg(269.4485025254), angle.Deg(4.7394200511))

	if sep := coord.Separation(center, gaiaRow).Arcseconds(); sep > 0.2 {
		t.Errorf("cone centered %.3f\" from Barnard's star's Gaia row, want under 0.2", sep)
	}
}
