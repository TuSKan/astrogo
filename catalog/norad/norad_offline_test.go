package norad

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
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

	targets, err := p.Search(context.Background(), "ISS")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(targets) != 1 || targets[0].Name != "ISS (ZARYA)" {
		t.Fatalf("Search(%q) = %+v, want a single ISS (ZARYA) target", "ISS", targets)
	}

	target, err := p.Resolve(context.Background(), "ISS")
	if err != nil || target.Name != "ISS (ZARYA)" {
		t.Fatalf("Resolve(%q) = %+v, %v, want ISS (ZARYA), true", "ISS", target, err)
	}

	// Regression: Kind must be the canonical resolve.KindSatellite constant,
	// not an ad hoc resolve.Kind("Satellite") string built outside the enum.
	if target.Kind != resolve.KindSatellite {
		t.Errorf("Kind = %q, want %q", target.Kind, resolve.KindSatellite)
	}

	gp, err := p.FetchByID(context.Background(), 25544)
	if err != nil {
		t.Fatalf("FetchByID: %v", err)
	}

	if gp.NoradCatID != 25544 {
		t.Errorf("FetchByID NoradCatID = %d, want 25544", gp.NoradCatID)
	}
}

// serveCelesTrak points CelesTrak at a server answering every request with
// status, content type and body.
func serveCelesTrak(t *testing.T, status int, contentType, body string) {
	t.Helper()
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.CelesTrak, srv.URL); err != nil {
		t.Fatal(err)
	}
}

// noGPData is CelesTrak's answer to a query nothing matches, checked live
// for NAME=QZXVNONEXISTENT and CATNR=999999999: HTTP 404, text/plain. This
// test used to serve "[]", which CelesTrak does not send for it, and accept
// any error, which the 404 also was.
func serveNoGPData(t *testing.T) {
	t.Helper()
	serveCelesTrak(t, http.StatusNotFound, "text/plain; charset=UTF-8", "No GP data found")
}

func TestFetchByIDNoData(t *testing.T) {
	serveNoGPData(t)

	_, err := New().FetchByID(context.Background(), 999999999)
	if !errors.Is(err, ErrNoData) {
		t.Errorf("FetchByID = %v, want ErrNoData", err)
	}
}

// TestResolveUnknownIsNotFound: CelesTrak's "no data" is an answer, so a
// catalog.Resolver asking NORAD about a name it lacks hears "no", not a
// failure.
func TestResolveUnknownIsNotFound(t *testing.T) {
	for _, q := range []string{"QZXVNONEXISTENT", "999999999"} {
		t.Run(q, func(t *testing.T) {
			serveNoGPData(t)

			_, err := New().Resolve(context.Background(), q)
			if !errors.Is(err, resolve.ErrNotFound) {
				t.Errorf("Resolve(%q) = %v, want ErrNotFound", q, err)
			}
		})
	}
}

// TestOtherHTTPErrorsStayFailures: only CelesTrak's own "no data" answer is
// read as empty.
func TestOtherHTTPErrorsStayFailures(t *testing.T) {
	tests := []struct {
		name, contentType, body string
		status                  int
	}{
		{"another 404", "text/html", "<html>Not Found</html>", http.StatusNotFound},
		{"server error", "text/plain", "No GP data found", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serveCelesTrak(t, tt.status, tt.contentType, tt.body)

			_, err := New().Resolve(context.Background(), "ISS")
			if err == nil || errors.Is(err, resolve.ErrNotFound) {
				t.Errorf("Resolve = %v, want a failure that is not ErrNotFound", err)
			}
		})
	}
}
