package airglow_test

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
	"github.com/TuSKan/astrogo/time"
)

// Fetch and AlmanacAt talk to SkyCalc through remote.Default(), the client the
// whole process shares, and must leave it as they found it. Until #1090 each
// closed it on the way out. Close drops every transport the client holds, for
// every endpoint, so a SIMBAD or Gaia request made after an airglow call got a
// new transport: a new connection, and a pacing interval started from zero.
//
// The test watches a second endpoint's connections, because that endpoint is
// what the defect reached. A request before the airglow call opens one
// connection. A request after it reuses that connection while the transport
// survives, and opens a second one if the transport was dropped.

// fakeSkyCalc answers the four SkyCalc calls this package makes: the model
// run, the table it wrote, the release of its directory, and the almanac.
func fakeSkyCalc(t *testing.T) {
	t.Helper()

	table := bintableFITS(
		[]string{"lam", "flux_ael", "flux_arc", "trans"},
		[][]float64{{500, 501, 502, 503}, {1e3, 2e3, 3e3, 4e3}, {5e2, 5e2, 5e2, 5e2}, unity(4)},
	)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/skycalc", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"status":"success","tmpdir":"run1"}`)
	})
	mux.HandleFunc("GET /tmp/run1/skytable.fits", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(table)
	})
	mux.HandleFunc("GET /api/rmtmp", func(http.ResponseWriter, *http.Request) {})
	mux.HandleFunc("POST /api/skycalc_almanac", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"output":{"observation":{"season_flag":2,"time_flag":3},"sun":{"sun_aveflux":129.5}}}`)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	if err := remote.SetURL(remote.ESOSkyCalc, srv.URL); err != nil {
		t.Fatalf("SetURL(ESOSkyCalc): %v", err)
	}
}

// neighbor is another service the process talks to through the same client.
// It counts the connections it is asked to open.
type neighbor struct {
	id     remote.EndpointID
	opened atomic.Int32
}

func newNeighbor(t *testing.T) *neighbor {
	t.Helper()

	n := &neighbor{id: remote.MAST}

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	srv.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			n.opened.Add(1)
		}
	}

	srv.Start()
	t.Cleanup(srv.Close)

	if err := remote.SetURL(n.id, srv.URL); err != nil {
		t.Fatalf("SetURL(%s): %v", n.id, err)
	}

	return n
}

// ask makes one complete request to the neighbor through remote.Default().
func (n *neighbor) ask(t *testing.T) {
	t.Helper()

	body, err := remote.Get(t.Context(), n.id, "", nil)
	if err != nil {
		t.Fatalf("Get(%s): %v", n.id, err)
	}

	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func TestSkyCalcCallsLeaveTheSharedClientsOtherTransportsAlone(t *testing.T) {
	// Not parallel: it points process-wide endpoints at its own servers and
	// counts connections on the process-wide client.
	when := time.GoDate(2024, time.March, 21, 3, 0, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		name string
		call func(t *testing.T)
	}{
		{"Fetch", func(t *testing.T) {
			t.Helper()

			spec := airglow.Spec{Observatory: airglow.Paranal, SolarFluxSFU: 100, MinNM: 499, MaxNM: 504}
			if _, err := airglow.Fetch(t.Context(), spec); err != nil {
				t.Fatalf("Fetch: %v", err)
			}
		}},
		{"AlmanacAt", func(t *testing.T) {
			t.Helper()

			if _, err := airglow.AlmanacAt(t.Context(), when, airglow.Paranal); err != nil {
				t.Fatalf("AlmanacAt: %v", err)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Cleanup(remote.Capture().Restore)
			// An empty cache, so Fetch goes to the service rather than to
			// a table an earlier test left behind.
			remote.SetDataDir(testutil.FileURL(t, t.TempDir()))

			fakeSkyCalc(t)
			n := newNeighbor(t)

			n.ask(t)

			if got := n.opened.Load(); got != 1 {
				t.Fatalf("fixture: the first request opened %d connections, want 1", got)
			}

			c.call(t)

			n.ask(t)

			if got := n.opened.Load(); got != 1 {
				t.Errorf("after %s the neighbor's next request opened a new connection (%d in all): "+
					"its transport on the shared client was dropped", c.name, got)
			}
		})
	}
}
