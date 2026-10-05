package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// errControlRejected is a TAP service refusing a control query.
var errControlRejected = errors.New("the service rejected the control query")

// SkipOnDegradedService is [SkipOnUpstreamFailure] for a service whose bad
// day can look like our bad request.
//
// A 4xx means astrogo built a request the service rejected, and stays a
// failure everywhere else. VizieR breaks that rule when degraded: for hours on
// 2026-10-05 it answered every query, including a two-column read of a table
// it has served for decades, with "400 Incorrect ADQL query: 1 unresolved
// identifiers" between bouts of 503 (#492). Its tests failed rather than
// skipped, on branches that never touched them.
//
// The same message is also what a genuinely renamed column produces, so the
// message alone cannot decide. control can: it sends a request that cannot be
// wrong, and if the service rejects that too, the 4xx was the service's. If
// the control succeeds, the service is answering correctly and the 4xx is
// ours, so this returns and the caller fails as before.
//
// control is consulted only for a 4xx, after the ordinary upstream
// classification has had its say. Build it so it cannot share a defect with
// the request under test; [TAPControl] does that by not using astrogo's client.
func SkipOnDegradedService(tb testing.TB, err error, control func() error) {
	tb.Helper()

	if err == nil {
		return
	}

	SkipOnUpstreamFailure(tb, err)

	var status interface{ HTTPStatus() int }
	if !errors.As(err, &status) {
		return
	}

	if code := status.HTTPStatus(); code < 400 || code >= 500 {
		return
	}

	if cerr := control(); cerr != nil {
		tb.Skipf("service degraded, not verified: it rejected a control request that cannot be wrong (%v), "+
			"so its rejection of this one is not evidence against astrogo: %v", cerr, err)
	}
}

// TAPControl returns a control for [SkipOnDegradedService] against a TAP
// service: a synchronous query for one row of from, verbatim as the FROM
// clause (a delimited VizieR table name keeps its quotes).
//
// It is a plain net/http request rather than one through astrogo's remote
// client, on purpose. A control sharing the request-building path would fail
// alongside the request under test when that path is what broke, and the
// helper would then skip the very defect the test exists to catch.
func TAPControl(ctx context.Context, syncURL, from string) func() error {
	return func() error {
		form := url.Values{
			"REQUEST": {"doQuery"},
			"LANG":    {"ADQL"},
			"FORMAT":  {"csv"},
			"QUERY":   {"SELECT TOP 1 * FROM " + from},
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, syncURL, strings.NewReader(form.Encode()))
		if err != nil {
			return fmt.Errorf("control query: %w", err)
		}

		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("control query: %w", err)
		}

		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil {
			return fmt.Errorf("control query: reading the response: %w", err)
		}

		// Some TAP services report a failed query in a 200 VOTable rather
		// than a status, so both are checked.
		if resp.StatusCode != http.StatusOK || strings.Contains(string(body), `QUERY_STATUS" value="ERROR"`) {
			return fmt.Errorf("%w: %q: HTTP %d: %s", errControlRejected, form.Get("QUERY"), resp.StatusCode, firstLine(body))
		}

		return nil
	}
}

// firstLine is the start of a response body, for an error message.
func firstLine(body []byte) string {
	s := strings.TrimSpace(string(body))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}

	const limit = 200
	if len(s) > limit {
		s = s[:limit] + "…"
	}

	return s
}
