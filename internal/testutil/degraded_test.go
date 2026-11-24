package testutil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

var errControlFailed = errors.New("control rejected too")

// TestSkipOnDegradedServiceAsksTheControlWhatUpstreamLeavesOpen: a degraded
// service is judged by whether it can answer a request that cannot be wrong.
// What SkipOnUpstreamFailure decides on its own is decided without a request;
// everything else, a rejection or an answer that would not parse, goes to the
// control.
func TestSkipOnDegradedServiceAsksTheControlWhatUpstreamLeavesOpen(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		err         error
		controlErr  error
		wantSkip    bool
		wantControl bool
	}{
		{"no error", nil, nil, false, false},
		{"a 503 is an outage on its own", &httpStatusError{503}, nil, true, false},
		{"a parse failure the control passes is ours", errStaticParse, nil, false, true},
		{"a parse failure the control fails too is the service's", errStaticParse, errControlFailed, true, true},
		{"a 400 the control passes is ours", &httpStatusError{400}, nil, false, true},
		{"a 400 the control fails too is the service's", &httpStatusError{400}, errControlFailed, true, true},
		{"a 404 the control fails too is the service's", &httpStatusError{404}, errControlFailed, true, true},
	} {
		fake := &fakeTB{TB: t}
		called := false

		SkipOnDegradedService(fake, tc.err, func() error {
			called = true

			return tc.controlErr
		})

		if fake.skipped != tc.wantSkip {
			t.Errorf("%s: skipped=%v, want %v", tc.name, fake.skipped, tc.wantSkip)
		}

		if called != tc.wantControl {
			t.Errorf("%s: control called=%v, want %v", tc.name, called, tc.wantControl)
		}
	}
}

// TestTAPControlAsksForOneRowAndReadsBothKindsOfFailure: the control is one
// synchronous ADQL row from the given FROM clause, and it fails on a non-200
// and on a 200 VOTable reporting a query error, since TAP services use both.
func TestTAPControlAsksForOneRowAndReadsBothKindsOfFailure(t *testing.T) {
	t.Parallel()

	// Recorded, not plain strings: a retried request can leave an earlier
	// attempt's handler writing after the test reads (#1012).
	type sent struct{ method, query string }

	var seen Recorded[sent]

	respond := func(status int, body string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen.Set(sent{method: r.Method, query: r.FormValue("QUERY")})

			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)

		return srv
	}

	ok := respond(http.StatusOK, "HIP,Vmag\n1,9.1\n")
	if err := TAPControl(context.Background(), ok.URL, `"I/239/hip_main"`, "HIP")(); err != nil {
		t.Fatalf("a 200 with a row failed the control: %v", err)
	}

	// A named column, not *: the degradation the control exists to detect is
	// a failure to resolve names, and * gives it none to fail on (#520).
	if got := seen.Get(); got.method != http.MethodPost || got.query != `SELECT TOP 1 HIP FROM "I/239/hip_main"` {
		t.Errorf("sent %s %q, want POST SELECT TOP 1 HIP FROM \"I/239/hip_main\"", got.method, got.query)
	}

	rejected := respond(http.StatusBadRequest, "<VOTABLE><INFO name=\"QUERY_STATUS\" value=\"ERROR\">Incorrect ADQL query: 1 unresolved identifiers!</INFO>")
	if err := TAPControl(context.Background(), rejected.URL, `"I/239/hip_main"`, "HIP")(); err == nil || !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("a 400 passed the control or was not named: %v", err)
	}

	errorIn200 := respond(http.StatusOK, "<VOTABLE><INFO name=\"QUERY_STATUS\" value=\"ERROR\">no such table</INFO></VOTABLE>")
	if err := TAPControl(context.Background(), errorIn200.URL, "gaiadr3.gaia_source", "source_id")(); err == nil {
		t.Error("a 200 VOTable reporting a query error passed the control")
	}
}

// TestTAPControlPassesOnlyAnAnswerNamingTheColumn: a 200 is not an answer
// unless it is one. An archive under maintenance can serve its own web page
// with a 200, and a control that passed it would fail the test on the outage.
// Gaia@AIP answers in a VOTable whatever FORMAT asks for, and ESA in CSV, so
// both are an answer.
func TestTAPControlPassesOnlyAnAnswerNamingTheColumn(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name, body string
		pass       bool
	}{
		{"a CSV header naming it", "source_id\n1373388291955864320\n", true},
		{"a quoted CSV header in another case", "\"Source_ID\"\r\n1\r\n", true},
		{"a VOTable FIELD naming it", `<?xml version="1.0"?><VOTABLE><RESOURCE type="results">` +
			`<INFO name="QUERY_STATUS" value="OK" /><TABLE><FIELD name="source_id" datatype="long"/>` +
			`<DATA><TABLEDATA><TR><TD>458508482075716864</TD></TR></TABLEDATA></DATA></TABLE></RESOURCE></VOTABLE>`, true},
		{"a maintenance page", "<!doctype html>\n<html><head><title>Page unavailable</title></head>" +
			"<body>The archive is down for maintenance.</body></html>\n", false},
		{"a CSV naming another column", "main_id\nM   4\n", false},
		{"an empty body", "", false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(tc.body))
		}))

		err := TAPControl(context.Background(), srv.URL, "gaiadr3.gaia_source", "source_id")()

		srv.Close()

		if (err == nil) != tc.pass {
			t.Errorf("%s: control error %v, want pass=%v", tc.name, err, tc.pass)
		}
	}
}

// TestTAPControlAsksMirrorsAsRemoteDoes: remote asks an endpoint's mirrors in
// turn while a host is not serving the API, and the control has to ask the
// host that answered. Asking only the primary, it failed while SIMBAD's CDS
// host answered every request with a 404 page and a mirror served them, so a
// defect in the mirror's answer would have been skipped as an outage.
func TestTAPControlAsksMirrorsAsRemoteDoes(t *testing.T) {
	t.Parallel()

	const answer = "main_id\nM   4\n"

	type host struct {
		status int
		body   string
	}

	closed := httptest.NewServer(http.NotFoundHandler())
	unreachable := closed.URL
	closed.Close()

	for _, tc := range []struct {
		name        string
		unreachable bool
		hosts       []host
		pass        bool
		asked       []int32
	}{
		{"a primary answering decides alone", false,
			[]host{{http.StatusOK, answer}, {http.StatusOK, answer}}, true, []int32{1, 0}},
		{"a 404 page passes to the mirror", false,
			[]host{{http.StatusNotFound, "<html>Page unavailable</html>"}, {http.StatusOK, answer}}, true, []int32{1, 1}},
		{"a 5xx passes to the mirror", false,
			[]host{{http.StatusServiceUnavailable, ""}, {http.StatusOK, answer}}, true, []int32{1, 1}},
		{"a serving mirror's rejection decides", false,
			[]host{{http.StatusBadGateway, ""}, {http.StatusBadRequest, "unresolved identifiers"}}, false, []int32{1, 1}},
		{"a primary's rejection is the answer", false,
			[]host{{http.StatusBadRequest, "unresolved identifiers"}, {http.StatusOK, answer}}, false, []int32{1, 0}},
		{"a primary's 200 page is the answer", false,
			[]host{{http.StatusOK, "<html>maintenance</html>"}, {http.StatusOK, answer}}, false, []int32{1, 0}},
		{"no host serving fails", false,
			[]host{{http.StatusNotFound, ""}, {http.StatusInternalServerError, ""}}, false, []int32{1, 1}},
		{"an unreachable primary passes to the mirror", true,
			[]host{{http.StatusOK, answer}}, true, []int32{1}},
	} {
		asked := make([]atomic.Int32, len(tc.hosts))
		urls := make([]string, 0, len(tc.hosts)+1)

		if tc.unreachable {
			urls = append(urls, unreachable)
		}

		for i, h := range tc.hosts {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				asked[i].Add(1)
				w.WriteHeader(h.status)
				_, _ = w.Write([]byte(h.body))
			}))
			t.Cleanup(srv.Close)

			urls = append(urls, srv.URL)
		}

		err := TAPControl(context.Background(), urls[0], "basic", "main_id", urls[1:]...)()
		if (err == nil) != tc.pass {
			t.Errorf("%s: control error %v, want pass=%v", tc.name, err, tc.pass)
		}

		for i := range asked {
			if got := asked[i].Load(); got != tc.asked[i] {
				t.Errorf("%s: host %d asked %d times, want %d", tc.name, i, got, tc.asked[i])
			}
		}
	}
}
