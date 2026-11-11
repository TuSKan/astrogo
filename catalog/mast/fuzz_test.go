package mast

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// FuzzResolveObject serves MAST's name-resolution answer from fuzzed bytes and
// resolves a name against it, so what the fuzzer decodes reaches everything
// that reads it: the content sniffing that picks XML or JSON, both decoders,
// the error status and the target built from each match (#1057).
//
// The property is the one every parser of someone else's bytes here is held
// to: it returns, and never panics. It also never yields a target alongside an
// error, and every target carries the provider's own catalog name.
func FuzzResolveObject(f *testing.F) {
	f.Add(`{"resolvedCoordinate":[{"canonicalName":"M  31","ra":10.684708,"decl":41.26875}],"status":""}`)
	f.Add(`{"status":"ERROR","msg":"unresolvable"}`)
	f.Add(`{"resolvedCoordinate":[{"canonicalName":"no position"}]}`)
	f.Add(`<?xml version="1.0" encoding="UTF-8"?><ns2:resolvedItems xmlns:ns2="santa.stsci.edu">` +
		`<resolvedCoordinate><canonicalName>M  31</canonicalName><dec>41.26875</dec>` +
		`<ra>10.684708</ra></resolvedCoordinate><status></status></ns2:resolvedItems>`)
	f.Add(`<resolvedItems><resolvedCoordinate><ra>NaN</ra><dec>Inf</dec></resolvedCoordinate></resolvedItems>`)
	f.Add(`<`)
	f.Add(``)

	var answer atomic.Pointer[string]

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if a := answer.Load(); a != nil {
			_, _ = w.Write([]byte(*a))
		}
	}))
	f.Cleanup(server.Close)

	scope := remote.Capture(remote.MAST)
	f.Cleanup(scope.Restore)

	if err := remote.SetURL(remote.MAST, server.URL); err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, body string) {
		answer.Store(&body)

		New().ResolveObject(context.Background(), resolve.ObjectRequest{Query: "M31", Limit: 10})(
			func(tgt resolve.Target, err error) bool {
				if err != nil && (tgt.ID != "" || tgt.Name != "" || tgt.HasCoord) {
					t.Errorf("ResolveObject yielded the target %q alongside %v", tgt.Name, err)
				}

				if err == nil && tgt.Catalog != "mast" {
					t.Errorf("ResolveObject yielded a target from catalog %q", tgt.Catalog)
				}

				return true
			})
	})
}
