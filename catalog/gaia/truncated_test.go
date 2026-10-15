package gaia

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
)

// ConeSearch and the archive's row cap (#786).
//
// ESA's synchronous endpoint caps an answer at 50,000 rows, and as CSV the
// cut answer is just 50,000 rows: a Limit above that came back short, looking
// complete, and was cached as the answer. A VOTable says so in its
// QUERY_STATUS.

// coneVOTable is a two-source answer with the given status after its table,
// where the archives write it.
func coneVOTable(status string) string {
	return `<?xml version="1.0"?>
<VOTABLE version="1.4"><RESOURCE type="results">
<INFO name="QUERY_STATUS" value="OK"/>
<TABLE><FIELD name="source_id" datatype="long"/><FIELD name="ra" datatype="double"/><FIELD name="dec" datatype="double"/>
<DATA><TABLEDATA>
<TR><TD>1</TD><TD>10.1</TD><TD>40.1</TD></TR>
<TR><TD>2</TD><TD>10.2</TD><TD>40.2</TD></TR>
</TABLEDATA></DATA></TABLE>
<INFO name="QUERY_STATUS" value="` + status + `"/>
</RESOURCE></VOTABLE>`
}

// serveArchive points endpoint at a server answering body, and returns the
// FORMAT of the last request and the number of requests.
func serveArchive(t *testing.T, endpoint remote.EndpointID, body string) (format *string, requests *atomic.Int32) {
	t.Helper()

	var (
		last  string
		count atomic.Int32
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		count.Add(1)

		last = r.PostFormValue("FORMAT")

		_, _ = fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)

	scope := remote.Capture(endpoint)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(endpoint, server.URL); err != nil {
		t.Fatalf("SetURL(%s): %v", endpoint, err)
	}

	return &last, &count
}

// drainCone runs one cone search, returning the sources and the last error.
func drainCone(p *Provider, limit int) (got int, lastErr error) {
	req := resolve.ConeRequest{Center: coord.NewICRS(angle.Deg(10), angle.Deg(40)), Radius: angle.Deg(1), Limit: limit}

	for _, err := range p.ConeSearch(context.Background(), req) {
		if err == nil {
			got++
		} else {
			lastErr = err
		}
	}

	return got, lastErr
}

func TestConeSearchSaysWhenTheArchiveCutIt(t *testing.T) {
	_, requests := serveArchive(t, DefaultEndpoint, coneVOTable("OVERFLOW"))

	p := newForTest(t)

	for pass := 1; pass <= 2; pass++ {
		got, err := drainCone(p, 5)
		if got != 2 {
			t.Errorf("pass %d: %d sources, want the 2 the archive sent", pass, got)
		}

		if !errors.Is(err, resolve.ErrTruncated) {
			t.Errorf("pass %d: error %v, want ErrTruncated", pass, err)
		}
	}

	// A cut list is not cached, so the second search asks again.
	if n := requests.Load(); n != 2 {
		t.Errorf("the archive saw %d requests, want 2", n)
	}
}

func TestConeSearchKeepsACompleteAnswer(t *testing.T) {
	_, requests := serveArchive(t, DefaultEndpoint, coneVOTable("OK"))

	p := newForTest(t)

	for pass := 1; pass <= 2; pass++ {
		got, err := drainCone(p, 5)
		if got != 2 || err != nil {
			t.Errorf("pass %d: %d sources and error %v, want 2 and none", pass, got, err)
		}
	}

	if n := requests.Load(); n != 1 {
		t.Errorf("the archive saw %d requests, want 1: a complete answer is cached", n)
	}
}

// Each archive is asked for the format it answers with a TABLEDATA VOTable,
// the serialization whose status the reader can see.
func TestConeSearchAsksEachArchiveForAVOTableItCanRead(t *testing.T) {
	for _, tt := range []struct {
		endpoint remote.EndpointID
		want     string
	}{
		{remote.GaiaTAP, "votable_plain"},
		{remote.GaiaAIP, "csv"},
	} {
		t.Run(string(tt.endpoint), func(t *testing.T) {
			format, _ := serveArchive(t, tt.endpoint, coneVOTable("OK"))

			p, err := New(tt.endpoint)
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			if _, err := drainCone(p, 5); err != nil {
				t.Fatalf("ConeSearch: %v", err)
			}

			if *format != tt.want {
				t.Errorf("FORMAT = %q, want %q", *format, tt.want)
			}
		})
	}
}

// A CSV answer cannot say it was cut, and is never taken to have been.
func TestParseResultNeverCallsACSVTruncated(t *testing.T) {
	targets, truncated, err := parseResult(strings.NewReader("source_id,ra,dec\n1,10.1,40.1\n"))
	if err != nil || truncated || len(targets) != 1 {
		t.Errorf("parseResult = %d targets, truncated %v, error %v; want 1, false, nil", len(targets), truncated, err)
	}
}
