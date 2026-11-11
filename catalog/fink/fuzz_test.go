package fink

import (
	"bytes"
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// FuzzResolve serves FINK's two answers from fuzzed bytes: the single-object
// JSON and the SSOFT Parquet table, which arrive from the same endpoint and are
// told apart by what was asked for. Resolve tries the first and falls back to
// the second, so what the fuzzer decodes reaches the JSON field readers, the
// staged Parquet file, Arrow's decoder, the column readers and the record
// filter (#1057).
//
// The property is the one every parser of someone else's bytes here is held
// to: it returns, and never panics. A target it resolves always has an ID, and
// every value it claims to have is finite (#1080): FINK reads an absent field
// as NaN, and a JSON string or a Parquet float can carry an infinity.
func FuzzResolve(f *testing.F) {
	table := ssoftParquet(f, []ssoftRow{
		{name: "Benoitcarry", number: 8467, h1: 15.0, status: 1},
		{name: "Unfitted", number: 2, h1: 9.0, fit: 1, status: 1},
		{name: "Designated", designation: "2010 AB", h1: 18.0, status: 1},
	})
	record := []byte(`{"sso_name":"Benoitcarry","sso_number":8467,"H_1":15.0,"G1_1":0.2,"G2_1":0.3,` +
		`"R":0.9,"alpha0":120.5,"delta0":-30.2,"fit":0,"status":1}`)
	notFound := []byte(`{"RemoteException": "sso_number not found"}`)

	f.Add(record, table)
	f.Add(notFound, table)
	f.Add([]byte(`[`+string(record)+`]`), table[:len(table)/2])
	f.Add(notFound, append([]byte("PAR1"), bytes.Repeat([]byte{0xff}, 64)...))
	f.Add([]byte(`{}`), []byte{})
	f.Add([]byte(`not json`), []byte(`{"error":"too many requests"}`))
	// Every value non-finite, as FINK's string fields can carry them.
	f.Add([]byte(`{"sso_name":"Benoitcarry","sso_number":8467,"H_1":"Infinity","H_2":"NaN",`+
		`"G1_1":"Infinity","G2_1":0.3,"R":"Infinity","alpha0":"-Infinity","delta0":10,"status":1}`), table)

	var jsonAnswer, tableAnswer atomic.Pointer[[]byte]

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req bytes.Buffer

		_, _ = req.ReadFrom(r.Body)

		answer := jsonAnswer.Load()
		if bytes.Contains(req.Bytes(), []byte("parquet")) {
			answer = tableAnswer.Load()
		}

		if answer != nil {
			_, _ = w.Write(*answer)
		}
	}))
	f.Cleanup(server.Close)
	f.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.FINK, server.URL); err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, single, bulk []byte) {
		jsonAnswer.Store(&single)
		tableAnswer.Store(&bulk)

		for _, query := range []string{"8467", "Benoitcarry"} {
			// An error is an answer the fuzzer is allowed to produce; only a
			// target that resolved is held to anything.
			tgt, err := New().Resolve(context.Background(), query)
			if err == nil && tgt.ID == "" {
				t.Errorf("Resolve(%q) resolved a target with no ID: %+v", query, tgt.Name)
			}

			if field := nonFinite(tgt); err == nil && field != "" {
				t.Errorf("Resolve(%q) resolved a target whose %s is not finite", query, field)
			}
		}
	})
}

// nonFinite names the first value a target claims to have that is NaN or
// infinite, or returns "".
func nonFinite(tgt resolve.Target) string {
	bad := func(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) }

	switch {
	case tgt.HasH && bad(tgt.H):
		return "H"
	case tgt.HasG1G2 && (bad(tgt.G1) || bad(tgt.G2)):
		return "G1/G2"
	case tgt.HasSpin && (bad(tgt.SpinRA.Radians()) || bad(tgt.SpinDec.Radians())):
		return "spin axis"
	case tgt.HasOblateness && bad(tgt.Oblateness):
		return "oblateness"
	}

	return ""
}
