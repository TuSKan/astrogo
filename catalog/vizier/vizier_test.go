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
	if _, err := parseCSV(strings.NewReader("ra,dec\n1,2\n")); !errors.Is(err, ErrUnexpectedSchema) {
		t.Errorf("expected ErrUnexpectedSchema, got %v", err)
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

	_, ok := p.Resolve("foo")
	if ok {
		t.Error("expected Resolve to return false")
	}

	if p.Search("foo") != nil {
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
