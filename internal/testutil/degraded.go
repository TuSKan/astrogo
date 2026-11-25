package testutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// errControlRejected is a service refusing a control request.
var errControlRejected = errors.New("the service rejected the control query")

// controlBodyLimit is how much of an answer a control reads. What it looks for
// comes at the start of every answer it has.
const controlBodyLimit = 64 << 10

// votableField captures the name of each FIELD a VOTable declares.
var votableField = regexp.MustCompile(`(?i)<FIELD\b[^>]*\bname\s*=\s*"([^"]*)"`)

// SkipOnDegradedService is [SkipOnUpstreamFailure] for a service whose bad
// day can look like our mistake.
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
// wrong, and if the service fails that too, the error was the service's. If
// the control succeeds, the service is answering correctly and the error is
// ours, so this returns and the caller fails as before.
//
// # Why every error, and not only a 4xx
//
// An archive can also answer with its own web page and a 200. catalog/gaia's
// cone search met one: the page failed to parse, which carries no status to
// classify, and a parse failure was taken as ours. [TAPControl] fails on such
// a page as well, since it names no column, so the control decides that error
// as it decides a 4xx.
//
// control is consulted for whatever the ordinary upstream classification
// leaves undecided. Build it so it cannot share a defect with the request
// under test; [TAPControl] does that by not using astrogo's client.
func SkipOnDegradedService(tb testing.TB, err error, control func() error) {
	tb.Helper()

	if err == nil {
		return
	}

	// Returning rather than relying on Skipf to end the test: a testing.TB
	// need not stop there, and the control must not be asked about an error
	// already decided.
	if _, ok := UpstreamFailure(err); ok {
		SkipOnUpstreamFailure(tb, err)

		return
	}

	if cerr := control(); cerr != nil {
		tb.Skipf("service degraded, not verified: it failed a control request that cannot be wrong (%v), "+
			"so its failure on this one is not evidence against astrogo: %v", cerr, err)
	}
}

// TAPControl returns a control for [SkipOnDegradedService] against a TAP
// service: a synchronous query for column from one row of from, both verbatim
// (a delimited VizieR table name keeps its quotes). column must be one the
// table has carried for as long as anyone has queried it — HIP in
// I/239/hip_main, source_id in gaiadr3.gaia_source — so the control cannot be
// wrong.
//
// The service passes only by answering: a 200 that reports no query error and
// whose CSV header or VOTable FIELD names column, which is what every TAP
// service's answer to this query has in common. A web page served with a 200
// names no column, so it fails.
//
// mirrors are the endpoint's mirrors, asked as remote asks them: while a host
// is not serving the API (no answer, a 404 or a 5xx) the next is asked, and
// the first that serves decides. A control asking only the primary would fail
// while a mirror answered the request under test, and so skip a defect in that
// answer.
//
// # Why it names a column
//
// It used to send SELECT TOP 1 *, which names none. VizieR's degraded answer
// is "unresolved identifiers": a failure to resolve the names in a query. A
// query with no names gave it nothing to fail on, and on 2026-10-06 the
// control passed while the bright-star query was refused as unresolved — a
// query that returned rows minutes later — so the test failed instead of
// skipping (#520). Naming a column is what makes the control exercise the
// failure it is there to detect.
//
// It is a plain net/http request rather than one through astrogo's remote
// client, on purpose. A control sharing the request-building path would fail
// alongside the request under test when that path is what broke, and the
// helper would then skip the very defect the test exists to catch.
func TAPControl(ctx context.Context, syncURL, from, column string, mirrors ...string) func() error {
	query := "SELECT TOP 1 " + column + " FROM " + from

	return func() error {
		failures := make([]error, 0, 1+len(mirrors))

		for _, base := range append([]string{syncURL}, mirrors...) {
			serving, err := tapControlAt(ctx, base, query, column)
			if serving {
				return err
			}

			failures = append(failures, err)
		}

		return errors.Join(failures...)
	}
}

// tapControlAt sends the control query to one host. It reports whether the
// host is serving the API, as remote judges it, and if it is, what its answer
// decides.
func tapControlAt(ctx context.Context, syncURL, query, column string) (bool, error) {
	form := url.Values{
		"REQUEST": {"doQuery"},
		"LANG":    {"ADQL"},
		"FORMAT":  {"csv"},
		"QUERY":   {query},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, syncURL, strings.NewReader(form.Encode()))
	if err != nil {
		return true, fmt.Errorf("control query: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("control query to %s: %w", syncURL, err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, controlBodyLimit))
	if err != nil {
		return false, fmt.Errorf("control query to %s: reading the response: %w", syncURL, err)
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode >= http.StatusInternalServerError {
		return false, fmt.Errorf("%w: %q: %s: HTTP %d: %s", errControlRejected, query, syncURL, resp.StatusCode, firstLine(body))
	}

	// Some TAP services report a failed query in a 200 VOTable rather
	// than a status, so both are checked.
	if resp.StatusCode != http.StatusOK || strings.Contains(string(body), `QUERY_STATUS" value="ERROR"`) {
		return true, fmt.Errorf("%w: %q: HTTP %d: %s", errControlRejected, query, resp.StatusCode, firstLine(body))
	}

	if !namesColumn(body, column) {
		return true, fmt.Errorf("%w: %q: HTTP 200 without the column %s: %s", errControlRejected, query, column, firstLine(body))
	}

	return true, nil
}

// HTTPControl returns a control for [SkipOnDegradedService] against a service
// answering a plain GET: rawURL is a request whose answer has held want for as
// long as anyone has asked. want is a value only that answer holds, as
// CelesTrak's elements for catalog number 25544 name the ISS "ISS (ZARYA)", or
// a key every answer of its kind carries.
//
// The service passes only by answering: a 200 whose body holds want. A web
// page served with a 200 does not hold it, so it fails, as a page naming no
// column fails [TAPControl]. It is a plain net/http request for the reason
// TAPControl is one, and it asks one host, so it is for an endpoint with no
// mirrors.
func HTTPControl(ctx context.Context, rawURL, want string) func() error {
	return func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return fmt.Errorf("control request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("control request to %s: %w", rawURL, err)
		}

		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, controlBodyLimit))
		if err != nil {
			return fmt.Errorf("control request to %s: reading the response: %w", rawURL, err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%w: %s: HTTP %d: %s", errControlRejected, rawURL, resp.StatusCode, firstLine(body))
		}

		if !strings.Contains(string(body), want) {
			return fmt.Errorf("%w: %s: HTTP 200 without %q: %s", errControlRejected, rawURL, want, firstLine(body))
		}

		return nil
	}
}

// namesColumn reports whether body is a TAP answer naming column: a VOTable
// declaring a FIELD of that name, or a CSV whose header holds it. Names are
// compared without case or quotes, as ADQL resolves an undelimited one.
func namesColumn(body []byte, column string) bool {
	want := strings.ToLower(strings.Trim(column, `"`))

	for _, m := range votableField.FindAllSubmatch(body, -1) {
		if strings.ToLower(string(m[1])) == want {
			return true
		}
	}

	header, _, _ := strings.Cut(strings.TrimSpace(string(body)), "\n")

	for name := range strings.SplitSeq(header, ",") {
		if strings.ToLower(strings.Trim(strings.TrimSpace(name), `"`)) == want {
			return true
		}
	}

	return false
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
