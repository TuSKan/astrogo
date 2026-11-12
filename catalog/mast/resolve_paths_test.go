package mast

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// serveMAST answers every Name.Lookup request with body and counts the
// requests it was asked.
func serveMAST(t *testing.T, body string) *atomic.Int32 {
	t.Helper()

	var asked atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	redirect(t, srv.URL)

	return &asked
}

// twoMatches is a Name.Lookup answer that resolved one name to two objects,
// in the order MAST ranks them.
const twoMatches = `{"status":"COMPLETE","resolvedCoordinate":[
	{"canonicalName":"MESSIER 031","ra":10.684,"decl":41.269},
	{"canonicalName":"MESSIER 032","ra":10.674,"decl":40.865}]}`

// TestResolveReturnsTheFirstOfSeveralMatches: Resolve answers with MAST's
// first match, and Search with all of them in MAST's order.
func TestResolveReturnsTheFirstOfSeveralMatches(t *testing.T) {
	serveMAST(t, twoMatches)

	got, err := New().Resolve(t.Context(), "M31")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if got.ID != "MESSIER 031" {
		t.Errorf("Resolve = %q, want MAST's first match", got.ID)
	}

	all, err := New().Search(t.Context(), "M31")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(all) != 2 || all[0].ID != "MESSIER 031" || all[1].ID != "MESSIER 032" {
		t.Errorf("Search = %v, want both matches in MAST's order", ids(all))
	}
}

// TestResolveReportsANameMASTDoesNotKnow: an answer with no match is
// resolve.ErrNotFound from Resolve, and an empty result, not an error, from
// Search. The distinction is what lets catalog.Resolver tell "nobody knows
// it" from "somebody could not be asked".
func TestResolveReportsANameMASTDoesNotKnow(t *testing.T) {
	serveMAST(t, `{"status":"COMPLETE","resolvedCoordinate":[]}`)

	if _, err := New().Resolve(t.Context(), "no such object"); !errors.Is(err, resolve.ErrNotFound) {
		t.Errorf("Resolve = %v, want resolve.ErrNotFound", err)
	}

	all, err := New().Search(t.Context(), "no such object")
	if err != nil || len(all) != 0 {
		t.Errorf("Search = %v, %v; want no targets and no error", ids(all), err)
	}
}

// TestResolveAndSearchReportAServiceError: MAST's own error status reaches
// both callers as ErrAPIError, with its message, never as "not found".
func TestResolveAndSearchReportAServiceError(t *testing.T) {
	serveMAST(t, `{"status":"ERROR","msg":"service unavailable"}`)

	if _, err := New().Resolve(t.Context(), "M31"); !errors.Is(err, ErrAPIError) {
		t.Errorf("Resolve = %v, want ErrAPIError", err)
	}

	if _, err := New().Search(t.Context(), "M31"); !errors.Is(err, ErrAPIError) {
		t.Errorf("Search = %v, want ErrAPIError", err)
	}
}

// TestAResolvedNameIsAskedOnce: a provider keeps what MAST answered, so the
// same name asked again, however it is spelled, is served without a second
// request, and with the same answer.
func TestAResolvedNameIsAskedOnce(t *testing.T) {
	asked := serveMAST(t, twoMatches)
	p := New()

	first, err := p.Search(t.Context(), "M31")
	if err != nil {
		t.Fatalf("first Search: %v", err)
	}

	// resolve.Normalize folds case, spaces and "Messier" into one key.
	again, err := p.Search(t.Context(), "  messier 31 ")
	if err != nil {
		t.Fatalf("second Search: %v", err)
	}

	if n := asked.Load(); n != 1 {
		t.Errorf("MAST was asked %d times for one name, want once", n)
	}

	if len(again) != len(first) || again[0].ID != first[0].ID {
		t.Errorf("the repeat answered %v, the first %v", ids(again), ids(first))
	}
}

// TestResolveObjectStopsWhenTheCallerDoes: a caller that stops after the
// first target is given no more, and what MAST answered is still kept whole
// for the next caller.
func TestResolveObjectStopsWhenTheCallerDoes(t *testing.T) {
	asked := serveMAST(t, twoMatches)
	p := New()

	var seen int

	for _, err := range p.ResolveObject(t.Context(), resolve.ObjectRequest{Query: "M31"}) {
		if err != nil {
			t.Fatalf("ResolveObject: %v", err)
		}

		seen++

		break
	}

	if seen != 1 {
		t.Errorf("the iterator yielded %d targets after the caller stopped at one", seen)
	}

	all, err := p.Search(t.Context(), "M31")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(all) != 2 || asked.Load() != 1 {
		t.Errorf("after an early stop the kept answer has %d targets and MAST was asked %d times; want 2 and once",
			len(all), asked.Load())
	}
}

// TestResolveReportsATruncatedAnswer: a body cut short of its declared length
// is a read error, not a shorter document to decode.
func TestResolveReportsATruncatedAnswer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(twoMatches)))
		_, _ = io.WriteString(w, twoMatches[:len(twoMatches)/2])
	}))
	t.Cleanup(srv.Close)

	redirect(t, srv.URL)

	if _, err := New().Resolve(t.Context(), "M31"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Resolve = %v, want the truncation reported as io.ErrUnexpectedEOF", err)
	}
}

// ids lists the targets' IDs for a failure message.
func ids(ts []resolve.Target) []string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}

	return out
}
