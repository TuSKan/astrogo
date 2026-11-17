package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
)

var errOutage = errors.New("simulated service outage")

// m31 is the single target the mock providers below answer with. A fixed
// object rather than a parameter: these tests are about which error comes
// back, never about which object.
func m31() map[string]Target {
	return map[string]Target{
		resolve.Normalize("M31"): {ID: "M31", Name: "M31", Coord: coord.NewICRS(0, 0)},
	}
}

// TestResolveDoesNotReportAFailureAsNotFound is the defect this whole change
// exists to remove.
//
// Under the previous (Target, bool) interface a provider had nowhere to put a
// failure, so an outage, a canceled context and a genuinely absent object all
// arrived as ErrNotFound. A scheduler asking "does NGC 5139 exist?" during a
// CDS outage got a confident no.
func TestResolveDoesNotReportAFailureAsNotFound(t *testing.T) {
	t.Parallel()

	r := &Resolver{
		providers: []Provider{&mockProvider{name: "broken", failWith: errOutage}},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	_, err := r.Resolve(context.Background(), "NGC 5139")
	if err == nil {
		t.Fatal("expected an error when the only provider failed")
	}

	if errors.Is(err, ErrNotFound) {
		t.Errorf("a provider failure was reported as ErrNotFound: %v", err)
	}

	if !errors.Is(err, errOutage) {
		t.Errorf("the underlying reason was lost: %v", err)
	}
}

// TestResolveReportsNotFoundOnlyWhenEveryProviderAnswered is the other half:
// ErrNotFound must still mean what it says.
func TestResolveReportsNotFoundOnlyWhenEveryProviderAnswered(t *testing.T) {
	t.Parallel()

	r := &Resolver{
		providers: []Provider{&mockProvider{name: "empty", targets: map[string]Target{}}},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	if _, err := r.Resolve(context.Background(), "nothing here"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

// TestResolveChecksTheContextBeforeConsultingProviders pins that a canceled
// caller learns it was canceled, rather than receiving whatever the first
// provider makes of a dead context.
func TestResolveChecksTheContextBeforeConsultingProviders(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// The provider would succeed if asked, so reaching it at all is the bug.
	r := &Resolver{
		providers: []Provider{&mockProvider{name: "would-succeed", targets: m31()}},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	if _, err := r.Resolve(ctx, "M31"); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}

	if _, err := r.Search(ctx, "M31"); !errors.Is(err, context.Canceled) {
		t.Errorf("Search err = %v, want context.Canceled", err)
	}
}

// TestUnsupportedIsAnAnswerNotAnIncident covers the state ErrUnsupported was
// added for.
//
// Gaia and VizieR are cone-search only. Under the bool API their "I do not do
// name resolution" was indistinguishable from "that object does not exist", so
// a real target could be reported absent because the provider asked happened
// to be one that never answers names. A provider declining an operation must
// not turn a successful lookup into a failure, nor a miss into an outage.
func TestUnsupportedIsAnAnswerNotAnIncident(t *testing.T) {
	t.Parallel()

	r := &Resolver{
		providers: []Provider{
			&mockProvider{name: "cone-only", failWith: resolve.ErrUnsupported},
			&mockProvider{name: "names", targets: m31()},
		},
		cfg: resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "M31")
	if err != nil {
		t.Fatalf("a cone-search-only provider broke an otherwise successful resolve: %v", err)
	}

	if got.ID != "M31" {
		t.Errorf("ID = %q, want M31", got.ID)
	}

	// And with nothing else to answer, it is a miss rather than a failure.
	only := &Resolver{
		providers: []Provider{&mockProvider{name: "cone-only", failWith: resolve.ErrUnsupported}},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	if _, err := only.Resolve(context.Background(), "M31"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound — declining an operation is not an outage", err)
	}
}

// TestASuccessfulProviderWinsOverAFailedOne pins that a partial outage does
// not deny an answer somebody actually gave.
func TestASuccessfulProviderWinsOverAFailedOne(t *testing.T) {
	t.Parallel()

	r := &Resolver{
		providers: []Provider{
			&mockProvider{name: "broken", failWith: errOutage},
			&mockProvider{name: "working", targets: m31()},
		},
		cfg: resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := r.Resolve(context.Background(), "M31")
	if err != nil {
		t.Fatalf("one broken provider denied an answer another gave: %v", err)
	}

	if got.ID != "M31" {
		t.Errorf("ID = %q, want M31", got.ID)
	}
}

// TestSearchKeepsPartialResultsAndReportsTotalFailure covers both of Search's
// arms. Matches from working providers survive one failing, and come back with
// an error wrapping ErrIncomplete that names the failed catalog, counts it, and
// carries its reason. When every provider fails there is nothing usable, and
// the error does not claim a partial result.
//
// The partial arm returned a nil error until #1143: the failure was dropped
// whenever anything was found, so a caller could not tell a complete answer
// from one missing a catalog.
func TestSearchKeepsPartialResultsAndReportsTotalFailure(t *testing.T) {
	t.Parallel()

	partial := &Resolver{
		providers: []Provider{
			&mockProvider{name: "broken", failWith: errOutage},
			&mockProvider{name: "working", targets: m31()},
		},
		cfg: resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	got, err := partial.Search(context.Background(), "M31")
	if len(got) != 1 {
		t.Errorf("got %d results, want 1 from the working provider", len(got))
	}

	if !errors.Is(err, ErrIncomplete) || !errors.Is(err, errOutage) {
		t.Fatalf("err = %v, want ErrIncomplete wrapping the outage", err)
	}

	for _, want := range []string{"broken", "1 of 2 catalogs"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to say %q", err, want)
		}
	}

	total := &Resolver{
		providers: []Provider{&mockProvider{name: "broken", failWith: errOutage}},
		cfg:       resolverConfig{positionMatchThreshold: defaultPositionMatchThreshold, cap: defaultCap},
	}

	results, err := total.Search(context.Background(), "M31")
	if len(results) != 0 || !errors.Is(err, errOutage) {
		t.Errorf("total outage: %d results, err = %v; want none and the underlying outage", len(results), err)
	}

	if errors.Is(err, ErrIncomplete) {
		t.Errorf("total outage: err = %v claims a partial result where there is none", err)
	}
}

// TestResolveHearsNoFromSBDBAndNORAD runs the real SBDB and NORAD providers
// against each service's real answer for a name it lacks: SBDB's HTTP 200
// "specified object was not found" and CelesTrak's HTTP 404 "No GP data
// found". Both used to arrive here as failures, so a Resolver with either
// registered could not report ErrNotFound for anything (#620).
func TestResolveHearsNoFromSBDBAndNORAD(t *testing.T) {
	t.Cleanup(remote.Reset)

	serve := func(id remote.EndpointID, status int, contentType, body string) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", contentType)
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)

		if err := remote.SetURL(id, srv.URL); err != nil {
			t.Fatal(err)
		}
	}

	serve(remote.JPLSBDB, http.StatusOK, "application/json",
		`{"message":"specified object was not found","code":"200","moreInfo":"https://ssd-api.jpl.nasa.gov/doc/sbdb.html"}`)
	serve(remote.CelesTrak, http.StatusNotFound, "text/plain; charset=UTF-8", "No GP data found")

	_, err := NewResolver(SBDB, NORAD).Resolve(context.Background(), "Qzxv Nonexistent 7")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Resolve = %v, want ErrNotFound", err)
	}
}
