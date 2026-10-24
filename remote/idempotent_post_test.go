package remote

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

// TestRegisteredIdempotentPostReachesTheTransport: an endpoint registered
// IdempotentPost has its POSTs retried, and GaiaAIPAsync, whose POST creates a
// UWS job, has its POST sent once (#845). Before the field existed SIMBAD's
// query went out once whatever the policy said, which is how one slow answer
// failed examples/20_whats_visible_tonight's bright-star search.
func TestRegisteredIdempotentPostReachesTheTransport(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		id       EndpointID
		attempts int32
	}{
		{SIMBAD, 2},
		{GaiaAIPAsync, 1},
	} {
		// One 503, then an answer.
		var attempts atomic.Int32

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if attempts.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)

				return
			}

			_, _ = io.WriteString(w, "answer")
		}))

		client := NewClient()
		if err := client.SetURL(tc.id, srv.URL); err != nil {
			t.Fatalf("%s: SetURL: %v", tc.id, err)
		}

		body, err := client.PostForm(t.Context(), tc.id, "", url.Values{"QUERY": {"SELECT 1"}})
		if body != nil {
			_ = body.Close()
		}

		_ = client.Close()

		srv.Close()

		if got := attempts.Load(); got != tc.attempts {
			t.Errorf("%s: %d requests, want %d (err %v)", tc.id, got, tc.attempts, err)
		}
	}
}

// TestPostedEndpointsDeclareIdempotentPost: every endpoint a library call site
// POSTs a query to is registered IdempotentPost, and the one whose POST creates
// a job is not. A new query endpoint that is POSTed to without the field is
// sent once, which is the defect #845 fixed.
func TestPostedEndpointsDeclareIdempotentPost(t *testing.T) {
	t.Parallel()

	for _, id := range []EndpointID{SIMBAD, VizieR, GaiaTAP, GaiaAIP, MAST, FINK, ESOSkyCalc} {
		if ep, ok := Lookup(id); !ok || !ep.IdempotentPost {
			t.Errorf("%s answers POSTed queries but is not registered IdempotentPost", id)
		}
	}

	if ep, ok := Lookup(GaiaAIPAsync); !ok || ep.IdempotentPost {
		t.Errorf("%s: a POST there creates a UWS job, so it must not be retried", GaiaAIPAsync)
	}
}
