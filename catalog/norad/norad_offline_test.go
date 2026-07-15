package norad

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

func TestNewFetchSearchResolve(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("FORMAT") != "JSON" {
			t.Errorf("expected FORMAT=JSON, got %q", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(issFixture))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.CelesTrak, srv.URL); err != nil {
		t.Fatal(err)
	}

	p := New()

	gps, err := p.Fetch(context.Background(), QueryCatNr, "25544")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if len(gps) != 1 || gps[0].ObjectName != "ISS (ZARYA)" {
		t.Fatalf("Fetch = %+v, want a single ISS (ZARYA) record", gps)
	}

	if p.Name() != "norad" {
		t.Errorf("Name() = %q, want %q", p.Name(), "norad")
	}

	if caps := p.Capabilities(); len(caps) != 1 {
		t.Errorf("Capabilities() = %v, want exactly one capability", caps)
	}

	targets := p.Search("ISS")
	if len(targets) != 1 || targets[0].Name != "ISS (ZARYA)" {
		t.Fatalf("Search(%q) = %+v, want a single ISS (ZARYA) target", "ISS", targets)
	}

	target, ok := p.Resolve("ISS")
	if !ok || target.Name != "ISS (ZARYA)" {
		t.Fatalf("Resolve(%q) = %+v, %v, want ISS (ZARYA), true", "ISS", target, ok)
	}

	gp, err := p.FetchByID(context.Background(), 25544)
	if err != nil {
		t.Fatalf("FetchByID: %v", err)
	}

	if gp.NoradCatID != 25544 {
		t.Errorf("FetchByID NoradCatID = %d, want 25544", gp.NoradCatID)
	}
}

func TestFetchByIDNoData(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.CelesTrak, srv.URL); err != nil {
		t.Fatal(err)
	}

	p := New()

	if _, err := p.FetchByID(context.Background(), 99999); err == nil {
		t.Error("expected an error for an empty GP result")
	}
}
