package mast

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// A position MAST answers as NaN, infinite or impossible is no position
// (#1080).
//
// encoding/xml and encoding/json decode a float64 through strconv.ParseFloat,
// which accepts NaN and the infinities, so the XML shape below decodes them
// without complaint. Before #1080 the target came back with HasCoord true and
// a NaN coordinate, and every plan answer about it said "never rises".
func TestMastReadsANonFinitePositionAsAbsent(t *testing.T) {
	for _, c := range []struct {
		name, ra, dec string
		want          bool
	}{
		{"nan-ra", "NaN", "41.26875", false},
		{"inf-dec", "10.684708", "Inf", false},
		{"dec-past-the-pole", "10.684708", "95", false},
		{"real", "10.684708", "41.26875", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			body := fmt.Sprintf(`<?xml version="1.0"?><ns2:resolvedItems xmlns:ns2="santa.stsci.edu">`+
				`<resolvedCoordinate><canonicalName>%s</canonicalName><dec>%s</dec><ra>%s</ra>`+
				`</resolvedCoordinate></ns2:resolvedItems>`, c.name, c.dec, c.ra)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/xml;charset=UTF-8")
				_, _ = fmt.Fprint(w, body)
			}))
			defer server.Close()

			redirect(t, server.URL)

			var got []resolve.Target

			New().ResolveObject(context.Background(), resolve.ObjectRequest{Query: c.name})(
				func(tgt resolve.Target, err error) bool {
					if err != nil {
						t.Fatalf("ResolveObject: %v", err)
					}

					got = append(got, tgt)

					return true
				})

			if len(got) != 1 {
				t.Fatalf("got %d targets, want the one MAST named", len(got))
			}

			if got[0].HasCoord != c.want {
				t.Errorf("ra %s, dec %s: HasCoord = %t, want %t (coordinate %v)",
					c.ra, c.dec, got[0].HasCoord, c.want, got[0].Coord)
			}
		})
	}
}
