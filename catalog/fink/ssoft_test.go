package fink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// The SSOFT bulk table is a parquet file of some hundreds of thousands of
// asteroids, so nothing offline can exercise the load path against the real
// one. What can be exercised is every way it goes wrong, and the filter that
// decides which rows are usable — and those are the failures that happen: an
// API that answers 200 with a JSON error, a truncated body, a schema whose
// columns moved.
//
// The fixtures are built here rather than checked in, so what each test varies
// is visible where it is asserted, and a parquet blob does not become a binary
// nobody can read in a diff.

// ssoftRow is one asteroid as the fixture writer takes it. Only the fields the
// tests actually vary are named; the rest are written as plausible constants,
// since a reader looking at a test should not have to skip twenty columns to
// find the one that matters.
type ssoftRow struct {
	name        string
	number      int64
	h1          float64
	fit, status int32
}

// ssoftParquet renders rows as a parquet file with the SSOFT column names
// readParquet looks for.
//
// Written through the same Arrow parquet stack the reader uses, so a schema the
// writer accepts is one the reader can open — the alternative, a hand-rolled
// byte fixture, would test this package against a file format nobody produces.
func ssoftParquet(t *testing.T, rows []ssoftRow) []byte {
	t.Helper()

	pool := memory.NewGoAllocator()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "sso_name", Type: arrow.BinaryTypes.String},
		{Name: "sso_number", Type: arrow.PrimitiveTypes.Int64},
		{Name: "H_1", Type: arrow.PrimitiveTypes.Float64},
		{Name: "H_2", Type: arrow.PrimitiveTypes.Float64},
		{Name: "err_H_1", Type: arrow.PrimitiveTypes.Float64},
		{Name: "err_H_2", Type: arrow.PrimitiveTypes.Float64},
		{Name: "G1_1", Type: arrow.PrimitiveTypes.Float64},
		{Name: "G1_2", Type: arrow.PrimitiveTypes.Float64},
		{Name: "G2_1", Type: arrow.PrimitiveTypes.Float64},
		{Name: "G2_2", Type: arrow.PrimitiveTypes.Float64},
		{Name: "R", Type: arrow.PrimitiveTypes.Float64},
		{Name: "alpha0", Type: arrow.PrimitiveTypes.Float64},
		{Name: "delta0", Type: arrow.PrimitiveTypes.Float64},
		{Name: "a_b", Type: arrow.PrimitiveTypes.Float64},
		{Name: "a_c", Type: arrow.PrimitiveTypes.Float64},
		{Name: "fit", Type: arrow.PrimitiveTypes.Int32},
		{Name: "status", Type: arrow.PrimitiveTypes.Int32},
		{Name: "n_obs", Type: arrow.PrimitiveTypes.Int32},
		{Name: "rms", Type: arrow.PrimitiveTypes.Float64},
	}, nil)

	b := array.NewRecordBuilder(pool, schema)
	defer b.Release()

	// Checked assertions, so a schema edit that changes a column's type fails
	// here rather than panicking halfway through building a fixture.
	str := func(i int) *array.StringBuilder {
		fb, ok := b.Field(i).(*array.StringBuilder)
		if !ok {
			t.Fatalf("field %d is %T, not a string builder", i, b.Field(i))
		}

		return fb
	}

	i64 := func(i int) *array.Int64Builder {
		fb, ok := b.Field(i).(*array.Int64Builder)
		if !ok {
			t.Fatalf("field %d is %T, not an int64 builder", i, b.Field(i))
		}

		return fb
	}

	i32 := func(i int) *array.Int32Builder {
		fb, ok := b.Field(i).(*array.Int32Builder)
		if !ok {
			t.Fatalf("field %d is %T, not an int32 builder", i, b.Field(i))
		}

		return fb
	}

	f64 := func(i int) *array.Float64Builder {
		fb, ok := b.Field(i).(*array.Float64Builder)
		if !ok {
			t.Fatalf("field %d is %T, not a float64 builder", i, b.Field(i))
		}

		return fb
	}

	for _, r := range rows {
		str(0).Append(r.name)
		i64(1).Append(r.number)
		f64(2).Append(r.h1)
		f64(3).Append(r.h1 - 0.5) // H_2
		f64(4).Append(0.1)        // err_H_1
		f64(5).Append(0.1)        // err_H_2
		f64(6).Append(0.15)       // G1_1
		f64(7).Append(0.15)       // G1_2
		f64(8).Append(0.2)        // G2_1
		f64(9).Append(0.2)        // G2_2
		f64(10).Append(0.9)       // R
		f64(11).Append(120.0)     // alpha0
		f64(12).Append(-30.0)     // delta0
		f64(13).Append(1.1)       // a_b
		f64(14).Append(1.2)       // a_c
		i32(15).Append(r.fit)
		i32(16).Append(r.status)
		i32(17).Append(500) // n_obs
		f64(18).Append(0.05)
	}

	rec := b.NewRecordBatch()
	defer rec.Release()

	tbl := array.NewTableFromRecords(schema, []arrow.RecordBatch{rec})
	defer tbl.Release()

	var buf bytes.Buffer
	if err := pqarrow.WriteTable(tbl, &buf, 1024, parquet.NewWriterProperties(), pqarrow.DefaultWriterProps()); err != nil {
		t.Fatalf("writing parquet fixture: %v", err)
	}

	return buf.Bytes()
}

// serveSSOFT points remote.FINK at a local server returning body, and reports
// how many times it was asked.
func serveSSOFT(t *testing.T, status int, body []byte) *int {
	t.Helper()

	t.Cleanup(remote.Reset)

	calls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++

		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
		}

		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	return &calls
}

// TestEnsureLoadedIndexesUsableRows is the load path and the filter on it.
//
// SSOFT carries rows the fit did not converge on and rows the pipeline flagged,
// and serving those as though they were measurements is the failure this filter
// exists to prevent — a phase-curve fit that did not converge still has numbers
// in every column.
func TestEnsureLoadedIndexesUsableRows(t *testing.T) {
	body := ssoftParquet(t, []ssoftRow{
		{name: "Benoitcarry", number: 8467, h1: 15.0, fit: 0, status: 1},
		{name: "Badfit", number: 111, h1: 12.0, fit: 1, status: 1},  // fit did not converge
		{name: "Flagged", number: 222, h1: 13.0, fit: 0, status: 0}, // status below 1
		{name: "", number: 333, h1: 14.0, fit: 0, status: 2},        // usable, but unnamed
	})

	serveSSOFT(t, http.StatusOK, body)

	p := New()

	if p.Loaded() {
		t.Error("Loaded() is true before anything was loaded")
	}

	if err := p.ensureLoaded(context.Background()); err != nil {
		t.Fatalf("ensureLoaded: %v", err)
	}

	if !p.Loaded() {
		t.Error("Loaded() is false after a successful load")
	}

	// Two of the four rows are usable: the converged, unflagged ones.
	if got := p.Count(); got != 2 {
		t.Errorf("Count() = %d, want 2 — rows with fit != 0 or status < 1 must not be indexed", got)
	}

	if rec := p.lookupCached("8467"); rec == nil || rec.Name != "Benoitcarry" {
		t.Errorf("lookupCached(8467) = %+v, want Benoitcarry", rec)
	}

	if rec := p.lookupCached("Benoitcarry"); rec == nil || rec.Number != 8467 {
		t.Errorf("lookupCached by name = %+v, want number 8467", rec)
	}

	for _, absent := range []string{"111", "Badfit", "222", "Flagged"} {
		if rec := p.lookupCached(absent); rec != nil {
			t.Errorf("lookupCached(%q) returned %+v; the row should have been filtered out", absent, rec)
		}
	}

	// The unnamed row is indexed by number and must not create an empty-string
	// name key, which would answer every nameless query with it.
	if rec := p.lookupCached("333"); rec == nil {
		t.Error("a usable row with no name was not indexed by number")
	}
}

// TestEnsureLoadedDownloadsOnce: SSOFT is a bulk table, and re-fetching it per
// query is the cost the index exists to avoid.
func TestEnsureLoadedDownloadsOnce(t *testing.T) {
	body := ssoftParquet(t, []ssoftRow{{name: "Benoitcarry", number: 8467, h1: 15.0, status: 1}})

	calls := serveSSOFT(t, http.StatusOK, body)

	p := New()

	for range 3 {
		if err := p.ensureLoaded(context.Background()); err != nil {
			t.Fatalf("ensureLoaded: %v", err)
		}
	}

	if *calls != 1 {
		t.Errorf("the bulk table was fetched %d times, want 1", *calls)
	}
}

// TestEnsureLoadedRejectsWhatIsNotTheTable covers each way the load fails, and
// each is a real one. The API answers 200 with a JSON error body, which is why
// the parquet magic is checked rather than the status.
func TestEnsureLoadedRejectsWhatIsNotTheTable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   []byte
		want   error
	}{
		{
			name:   "a JSON error served as 200",
			status: http.StatusOK,
			body:   []byte(`{"error": "unknown version"}`),
			want:   ErrSSOFTError,
		},
		{
			name:   "something that is not parquet",
			status: http.StatusOK,
			body:   bytes.Repeat([]byte("not a parquet file "), 100),
			want:   ErrInvalidParquet,
		},
		{
			name:   "an empty body",
			status: http.StatusOK,
			body:   nil,
			want:   ErrInvalidParquet,
		},
		{
			name:   "a server error",
			status: http.StatusInternalServerError,
			body:   []byte("upstream is down"),
			want:   ErrHTTPStatus,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			serveSSOFT(t, tc.status, tc.body)

			p := New()

			err := p.ensureLoaded(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}

			if p.Loaded() {
				t.Error("Loaded() is true after a failed load")
			}

			if p.Count() != 0 {
				t.Errorf("Count() = %d after a failed load, want 0", p.Count())
			}
		})
	}
}

// TestFailedLoadIsNotRetried pins behaviour rather than endorsing it.
//
// A failed load is recorded and replayed for the life of the provider, so a
// transient outage or a consent gate granted a moment later disables this
// provider permanently. catalog/openngc had the same shape and #191 called it
// worse than a per-query failure, precisely because it is invisible and cannot
// recover; time/internal/iers takes the middle path with a retry cooldown.
//
// The trade-off here is not the same, since SSOFT is a bulk download rather
// than two CSVs, so this is filed as #215 rather than changed under a coverage
// pull request. If it is changed, this test is the one to delete.
func TestFailedLoadIsNotRetried(t *testing.T) {
	calls := serveSSOFT(t, http.StatusInternalServerError, []byte("down"))

	p := New()

	first := p.ensureLoaded(context.Background())
	if first == nil {
		t.Fatal("ensureLoaded against a failing server returned no error")
	}

	second := p.ensureLoaded(context.Background())
	if second == nil {
		t.Fatal("the second ensureLoaded returned no error")
	}

	if *calls != 1 {
		t.Errorf("the server was asked %d times; the recorded failure was expected to be replayed", *calls)
	}

	if first.Error() != second.Error() {
		t.Errorf("the replayed error differs from the first:\n  %v\n  %v", first, second)
	}
}

// TestResolveFallsBackToTheBulkTable: Resolve tries the single-object API
// first, and the bulk index is what answers when that has nothing. Without
// this, the whole SSOFT path is unreachable from the public API in tests.
func TestResolveFallsBackToTheBulkTable(t *testing.T) {
	t.Cleanup(remote.Reset)

	body := ssoftParquet(t, []ssoftRow{{name: "Benoitcarry", number: 8467, h1: 15.0, status: 1}})

	// The single-object endpoint answers with an exception; the bulk one serves
	// the table. Both are the same registered endpoint, so one handler decides
	// by looking at what was asked for.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer

		_, _ = buf.ReadFrom(r.Body)

		if bytes.Contains(buf.Bytes(), []byte("parquet")) {
			_, _ = w.Write(body)
			return
		}

		_, _ = w.Write([]byte(`{"RemoteException": "sso_number not found"}`))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	p := New()

	tgt, err := p.Resolve(context.Background(), "8467")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if tgt.Name != "Benoitcarry" {
		t.Errorf("Resolve(8467).Name = %q, want Benoitcarry", tgt.Name)
	}

	// H_2, not H_1: recordToTarget prefers r-band over g-band because r is
	// closer to V, and the fixture writes H_2 half a magnitude below H_1 so the
	// two cannot be confused. Asserting 15.0 here is the mistake this catches —
	// I made it, and the failure said 14.5.
	if math.Abs(tgt.H-14.5) > 1e-9 {
		t.Errorf("Resolve(8467).H = %v, want the r-band H_2 of 14.5", tgt.H)
	}

	if !tgt.HasH {
		t.Error("HasH is false on a target built from a row carrying both H columns")
	}
}

// The single-object endpoint hands back untyped JSON, and its numbers do not
// always arrive as numbers: the API has served magnitudes as strings, and
// encoding/json produces json.Number when a decoder asks for it. These helpers
// absorb all three shapes, and the branch that matters most is the one that
// gives up — a value that cannot be read has to become NaN rather than zero,
// because zero is a magnitude and NaN is an absence.

func TestJSONF64ReadsEveryShapeTheAPIHasSent(t *testing.T) {
	m := map[string]any{
		"float":      15.25,
		"number":     json.Number("15.25"),
		"string":     "15.25",
		"bad string": "not a number",
		"null":       nil,
		"wrong type": []any{1, 2},
	}

	for _, tc := range []struct {
		key  string
		want float64
	}{
		{"float", 15.25},
		{"number", 15.25},
		{"string", 15.25},
	} {
		if got := jsonF64(m, tc.key); math.Abs(got-tc.want) > 1e-12 {
			t.Errorf("jsonF64(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}

	// Every unreadable shape is an absence, not a zero. A zero here is a
	// magnitude of zero — a brighter object than any asteroid — and it would
	// propagate into recordToTarget as a measurement.
	for _, key := range []string{"bad string", "null", "wrong type", "absent"} {
		if got := jsonF64(m, key); !math.IsNaN(got) {
			t.Errorf("jsonF64(%q) = %v, want NaN", key, got)
		}
	}
}

func TestJSONIntReadsEveryShapeTheAPIHasSent(t *testing.T) {
	m := map[string]any{
		"float":      8467.0,
		"number":     json.Number("8467"),
		"string":     "8467",
		"bad string": "eight thousand",
		"null":       nil,
		"wrong type": map[string]any{},
	}

	for _, key := range []string{"float", "number", "string"} {
		if got := jsonInt(m, key); got != 8467 {
			t.Errorf("jsonInt(%q) = %d, want 8467", key, got)
		}
	}

	// Zero is the only answer an integer field can give for "unreadable", which
	// is why ssoRecord keeps the magnitudes in float64 and only the identifiers
	// here: an asteroid numbered 0 does not exist, so the confusion is
	// detectable where a magnitude of 0 would not be.
	for _, key := range []string{"bad string", "null", "wrong type", "absent"} {
		if got := jsonInt(m, key); got != 0 {
			t.Errorf("jsonInt(%q) = %d, want 0", key, got)
		}
	}
}

func TestJSONStrFormatsWhatIsNotAString(t *testing.T) {
	m := map[string]any{
		"string": "Benoitcarry",
		"number": 8467.0,
		"null":   nil,
	}

	if got := jsonStr(m, "string"); got != "Benoitcarry" {
		t.Errorf("jsonStr(string) = %q, want Benoitcarry", got)
	}

	// A number where a name was expected is rendered rather than dropped: the
	// value is wrong for the field but it is what the service sent, and showing
	// it is what lets somebody see that.
	if got := jsonStr(m, "number"); got != "8467" {
		t.Errorf("jsonStr(number) = %q, want 8467", got)
	}

	for _, key := range []string{"null", "absent"} {
		if got := jsonStr(m, key); got != "" {
			t.Errorf("jsonStr(%q) = %q, want empty", key, got)
		}
	}
}

// Search and ResolveObject are the same question asked three ways, and the
// distinction they have to keep is the one #102 is about: "no such asteroid" is
// an answer, "FINK could not be reached" is not. Search must return an empty
// slice for the first and an error for the second; ResolveObject must yield
// nothing for the first and the error for the second.

// TestSearchAndResolveObjectSeparateAbsenceFromFailure covers both directions
// against one server, since the two differ only in what carries the answer.
func TestSearchAndResolveObjectSeparateAbsenceFromFailure(t *testing.T) {
	t.Run("absence", func(t *testing.T) {
		// The single-object endpoint says it has nothing, and the bulk table
		// loads cleanly without the object in it. Nothing failed.
		body := ssoftParquet(t, []ssoftRow{{name: "Somethingelse", number: 1, h1: 10, status: 1}})

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var buf bytes.Buffer

			_, _ = buf.ReadFrom(r.Body)

			if bytes.Contains(buf.Bytes(), []byte("parquet")) {
				_, _ = w.Write(body)
				return
			}

			_, _ = w.Write([]byte(`{"RemoteException": "sso_number not found"}`))
		}))
		t.Cleanup(srv.Close)
		t.Cleanup(remote.Reset)

		if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
			t.Fatal(err)
		}

		p := New()

		// FINK answers an unknown identifier with a RemoteException, which
		// reads as a failure. The bulk table loading and not having the object
		// is what settles that it is an absence — before that was distinguished,
		// every asteroid FINK had never heard of came back as an outage.
		got, err := p.Search(context.Background(), "999999")
		if err != nil {
			t.Errorf("Search for an absent object returned an error: %v", err)
		}

		if len(got) != 0 {
			t.Errorf("Search returned %d targets for an absent object", len(got))
		}

		n := 0

		for _, err := range p.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "999999"}) {
			n++

			if err != nil {
				t.Errorf("ResolveObject yielded an error for an absent object: %v", err)
			}
		}

		if n != 0 {
			t.Errorf("ResolveObject yielded %d results for an absent object", n)
		}
	})

	t.Run("failure", func(t *testing.T) {
		// Every route fails: the single-object endpoint and the bulk table
		// alike. Reporting that as "not found" is the defect.
		serveSSOFT(t, http.StatusInternalServerError, []byte("down"))

		p := New()

		if _, err := p.Search(context.Background(), "8467"); err == nil {
			t.Error("Search reported an unreachable service as no matches")
		}

		var yielded error

		for _, err := range p.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "8467"}) {
			if err != nil {
				yielded = err
			}
		}

		if yielded == nil {
			t.Error("ResolveObject yielded no error for an unreachable service")
		}
	})
}

// TestResolveJoinsWhatEachRouteSaid: Resolve tries three routes and, when they
// all fail, the caller gets every reason rather than the last one. A single
// error here would say "bulk table: 500" and hide that the numeric lookup was
// never going to work either.
func TestResolveJoinsWhatEachRouteSaid(t *testing.T) {
	serveSSOFT(t, http.StatusInternalServerError, []byte("down"))

	p := New()

	_, err := p.Resolve(context.Background(), "8467")
	if err == nil {
		t.Fatal("Resolve against a failing service returned no error")
	}

	// Every route, not just the last one. A single error here would say
	// "bulk table: 500" and hide that the two single-object lookups failed for
	// the same reason.
	for _, want := range []string{"numeric lookup", "name lookup", "bulk table"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the joined error does not mention the %s route:\n%v", want, err)
		}
	}
}

// TestRemoteExceptionCarriesItsMessage: FINK says "sso_number not found" for an
// object it does not have and something else for a genuine error, both as a
// RemoteException. The sentinel alone cannot tell them apart, so the message
// has to survive — the same discipline #203 applied to the tests, here in the
// library.
func TestRemoteExceptionCarriesItsMessage(t *testing.T) {
	t.Cleanup(remote.Reset)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"RemoteException": "sso_number not found"}`))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	_, err := New().querySingle(context.Background(), 999999999, "")
	if !errors.Is(err, ErrRemoteException) {
		t.Fatalf("err = %v, want ErrRemoteException", err)
	}

	if !strings.Contains(err.Error(), "sso_number not found") {
		t.Errorf("the error dropped what the service said:\n  %v", err)
	}
}

// TestAbsenceNeedsTheBulkTableToHaveAnswered is the other half of the rule, and
// the one that keeps it honest: when SSOFT could not be loaded, nobody has
// asked the complete catalogue, so "not found" is an assertion the provider is
// not entitled to make.
func TestAbsenceNeedsTheBulkTableToHaveAnswered(t *testing.T) {
	t.Cleanup(remote.Reset)

	// The single-object endpoint says "not found"; the bulk download fails.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer

		_, _ = buf.ReadFrom(r.Body)

		if bytes.Contains(buf.Bytes(), []byte("parquet")) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("down"))

			return
		}

		_, _ = w.Write([]byte(`{"RemoteException": "sso_number not found"}`))
	}))
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.FINK, srv.URL); err != nil {
		t.Fatal(err)
	}

	_, err := New().Resolve(context.Background(), "999999")
	if err == nil {
		t.Fatal("Resolve returned no error although the catalogue was never consulted")
	}

	if errors.Is(err, resolve.ErrNotFound) {
		t.Errorf("Resolve claimed the object is absent without the bulk table answering:\n  %v", err)
	}
}
