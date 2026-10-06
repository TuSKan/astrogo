package testutil

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var errControlFailed = errors.New("control rejected too")

// TestSkipOnDegradedServiceAsksTheControlOnlyAboutA4xx: a degraded service is
// judged by whether it can answer a request that cannot be wrong, and only a
// rejection (4xx) raises that question. Everything else is decided as
// SkipOnUpstreamFailure decides it, without a request.
func TestSkipOnDegradedServiceAsksTheControlOnlyAboutA4xx(t *testing.T) {
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
		{"a parse failure is ours", errStaticParse, errControlFailed, false, false},
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

	var gotQuery, gotMethod string

	respond := func(status int, body string) *httptest.Server {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotQuery = r.FormValue("QUERY")

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
	if gotMethod != http.MethodPost || gotQuery != `SELECT TOP 1 HIP FROM "I/239/hip_main"` {
		t.Errorf("sent %s %q, want POST SELECT TOP 1 HIP FROM \"I/239/hip_main\"", gotMethod, gotQuery)
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
