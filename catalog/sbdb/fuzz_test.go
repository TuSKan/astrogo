package sbdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// FuzzSearchBright serves SBDB's bulk query answer from fuzzed bytes and asks
// SearchBright for it, undated and dated, so what the fuzzer decodes reaches
// everything that reads it: the column lookup, the element forms, the comet
// bound around a date and the brightness cut (#1057).
//
// The property is the one every parser of someone else's bytes here is held
// to: it returns, and never panics. A short row in this answer used to index
// past its end (#1058). It also never yields a target that names no body.
func FuzzSearchBright(f *testing.F) {
	f.Add(`{"fields":["full_name","spkid","H","G","class","e","a","i","om","w","ma","q","tp","epoch"],` +
		`"data":[["  1 Ceres (A801 AA)","20000001","3.34","0.12","MBA","0.0789","2.7656","10.587",` +
		`"80.25","73.29","291.4","2.548","2459000.5","2460000.5"]]}`)
	f.Add(`{"fields":["full_name","spkid","M1","K1","class","e","q","tp","i","om","w","epoch"],` +
		`"data":[["C/2023 A3 (Tsuchinshan-ATLAS)","1003876","5.4","9.6","HYP","1.0001","0.391",` +
		`"2460581.6","139.1","21.56","308.5","2460600.5"]]}`)
	// #1058's short rows, and a column the query asked for that is missing.
	f.Add(`{"fields":["full_name","spkid","H","G","class"],"data":[["  1 Ceres","20000001"]]}`)
	f.Add(`{"fields":["full_name","spkid","H","G","class"],"data":[[]]}`)
	f.Add(`{"fields":["H"],"data":[["NaN"]]}`)
	f.Add(`{}`)
	f.Add(`not json`)

	var answer atomic.Pointer[string]

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if a := answer.Load(); a != nil {
			_, _ = w.Write([]byte(*a))
		}
	}))
	f.Cleanup(server.Close)
	f.Cleanup(remote.Reset)

	if err := remote.SetURL(remote.JPLSBDBQuery, server.URL); err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, body string) {
		answer.Store(&body)

		for _, at := range []time.Time{{}, time.FromJD(2460600.5, time.TT)} {
			New().SearchBright(context.Background(), resolve.BrightRequest{MaxVMag: 12, Limit: 50, At: at})(
				func(tgt resolve.Target, err error) bool {
					if err == nil && tgt.SPKID == "" {
						t.Errorf("SearchBright yielded a target with no SPK ID: %+v", tgt)
					}

					return true
				})
		}
	})
}
