package vizier

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestVizierOfflineConeSearch(t *testing.T) {
	csvData := "designation,ra,dec\n" +
		`"18375080-4835411 ",279.461678,-48.594772` + "\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")

		if _, err := fmt.Fprint(w, csvData); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

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
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

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
			fmt.Fprint(w, "designation,ra,dec\n2,10.68470,41.26875\n") //nolint:errcheck // test server
		}
	}))
	defer server.Close()

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

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

type mockTransport struct {
	Handler http.Handler
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.Handler.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req

	return resp, nil
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

	_, ok := p.Resolve(context.Background(), "foo")
	if ok {
		t.Error("expected Resolve to return false")
	}

	if p.Search(context.Background(), "foo") != nil {
		t.Error("expected Search to return nil")
	}

	// errTransport keeps this default (non-network-tagged) test fully
	// offline — see CLAUDE.md's build-tag convention.
	p.client.HTTPClient.Transport = errTransport{}

	iter := p.ConeSearch(context.Background(), resolve.ConeRequest{})
	iter(func(_ resolve.Target, err error) bool {
		if err == nil {
			t.Error("expected an error with no transport reachable")
		}

		return false
	})
}

type errTransport struct{}

var errNoTransport = errors.New("errTransport: no network access in this test")

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errNoTransport
}
