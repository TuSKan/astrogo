//go:build validation

package time_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// iersReply is the timescale service's JSON answer.
//
// Value is a JSON string ("37"), not a number, and is null for instants before
// UTC began accumulating whole leap seconds in 1972 — hence the pointer.
type iersReply struct {
	Datetime string  `json:"Datetime"`
	Param    string  `json:"Param"`
	Value    *string `json:"Value"`
}

// TestIERSOracleMatchesTheRecord validates the pinned leap-second record
// against IERS itself.
//
// # Why this exists when three offline checks already pass
//
// gofa's compiled table, NAIF's naif0012.tls and the discontinuities in
// finals2000A are three republications of one source. Every cross-check
// between them — including TestLeapSecondSourcesAgree — is a consistency
// check, and would still pass if all three misquoted IERS identically. The
// metrology package has a field for exactly this distinction
// ([metrology.Reference].SharedAncestor), and it applies here.
//
// This is the one reference that does not share that ancestry: it is IERS's
// own answer. It is what makes leapSecondRecord a validated table rather than
// a carefully transcribed one.
//
// # Why it is not the runtime source
//
// It answers one instant per request, so a conversion would cost a network
// round-trip and could not work offline — unacceptable for Time.TAI, which is
// on every UTC path. It also fails outright beyond EOP coverage
// ("Error in reading EOP-data!" for 2030), so it cannot answer the question
// that actually matters for freshness: whether a leap second has been
// announced. See the endpoint's own doc comment in remote.
//
// # Cost
//
// One request per record entry plus a boundary triplet at three steps. The
// service is a PHP controller with no published rate limit, so this stays in
// the validation tier that runs on a schedule, never per-PR.
func TestIERSOracleMatchesTheRecord(t *testing.T) {
	client := remote.Default()

	ctx := t.Context()

	// Probe once before the loop so a service outage skips immediately rather
	// than after 28 failed requests.
	if _, err := deltaAT(ctx, client, "2017-01-01 00:00:00"); err != nil {
		testutil.SkipOnDegradedService(t, err, iersControl(ctx))
		t.Fatalf("the IERS timescale service returned an unexpected error: %v", err)
	}

	for _, s := range leapSecondRecord {
		at := fmt.Sprintf("%04d-%02d-%02d 00:00:00", s.year, s.month, s.day)

		got, err := deltaAT(ctx, client, at)
		if err != nil {
			testutil.SkipOnDegradedService(t, err, iersControl(ctx))
			t.Errorf("%s: %v", at, err)

			continue
		}

		if math.Abs(got-s.deltaAT) > 1e-9 {
			t.Errorf("%s: IERS says ΔAT = %g, the pinned record says %g.\n"+
				"  IERS is authoritative here — correct the record, do not relax this test.",
				at, got, s.deltaAT)
		}
	}
}

// TestIERSOracleConfirmsTheBoundaryConvention checks against IERS the rule that
// TestLeapSecondBoundaryConvention asserts offline: the interval is half-open,
// and the inserted second carries the *old* ΔAT.
//
// Three steps rather than all 28, because the convention is a property of the
// definition rather than of any particular entry, and this is somebody else's
// server.
func TestIERSOracleConfirmsTheBoundaryConvention(t *testing.T) {
	client := remote.Default()

	ctx := t.Context()

	cases := []struct {
		before, leap, at string
		old, new         float64
	}{
		{"1981-06-30 23:59:59", "1981-06-30 23:59:60", "1981-07-01 00:00:00", 19, 20},
		{"2015-06-30 23:59:59", "2015-06-30 23:59:60", "2015-07-01 00:00:00", 35, 36},
		{"2016-12-31 23:59:59", "2016-12-31 23:59:60", "2017-01-01 00:00:00", 36, 37},
	}

	for _, c := range cases {
		for _, q := range []struct {
			at   string
			want float64
			why  string
		}{
			{c.before, c.old, "the last ordinary second before the step"},
			{c.leap, c.old, "the inserted second itself"},
			{c.at, c.new, "the step instant"},
		} {
			got, err := deltaAT(ctx, client, q.at)
			if err != nil {
				testutil.SkipOnDegradedService(t, err, iersControl(ctx))
				t.Errorf("%s: %v", q.at, err)

				continue
			}

			if math.Abs(got-q.want) > 1e-9 {
				t.Errorf("%s (%s): IERS says ΔAT = %g, expected %g — the half-open "+
					"convention asserted offline does not match the authority",
					q.at, q.why, got, q.want)
			}
		}
	}
}

// iersControl is the request [testutil.SkipOnDegradedService] sends when the
// service answers a 4xx: ΔAT at 2017-01-01 00:00:00, which it has always
// answered "37", as a plain net/http GET outside astrogo's client, so that it
// cannot share a defect with the request under test. Its deadline is the
// endpoint's registered timeout.
//
// On 2026-10-26 the service answered every request, this one included, with
// a firewall page and HTTP 400, "Request Rejected" (#853). Through
// SkipOnUpstreamFailure alone that 400 read as astrogo's own bad request, and
// both oracle tests failed instead of skipping.
func iersControl(ctx context.Context) func() error {
	return func() error {
		ep, ok := remote.Lookup(remote.IERSTimescales)
		if !ok {
			return errIERSControl
		}

		ctx, cancel := context.WithTimeout(ctx, ep.Timeout)
		defer cancel()

		q := url.Values{"param": {"leapseconds"}, "datetime": {"2017-01-01 00:00:00"}}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, ep.URL+"?"+q.Encode(), nil)
		if err != nil {
			return fmt.Errorf("control: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("control: %w", err)
		}

		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		if err != nil {
			return fmt.Errorf("control: reading the reply: %w", err)
		}

		if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"Value":"37"`) {
			return fmt.Errorf("%w: HTTP %d: %s", errIERSControl, resp.StatusCode, truncate(strings.TrimSpace(string(body))))
		}

		return nil
	}
}

// errIERSControl is the timescale service failing its control request.
var errIERSControl = errors.New("the IERS timescale service failed its control request")

// deltaAT asks the service for TAI-UTC at one instant.
//
// The service answers errors as bare text rather than JSON — "Please enter a
// valid time!" for a missing datetime, "Error in reading EOP-data!" beyond EOP
// coverage — so the body is checked before it is decoded. Feeding those to a
// JSON decoder and reporting a decode failure would hide what actually
// happened.
func deltaAT(ctx context.Context, c *remote.Client, at string) (float64, error) {
	q := url.Values{}
	q.Set("param", "leapseconds")
	q.Set("datetime", at)

	body, err := c.Get(ctx, remote.IERSTimescales, "", q)
	if err != nil {
		return 0, fmt.Errorf("query %q: %w", at, err)
	}

	defer func() { _ = body.Close() }()

	raw, err := io.ReadAll(body)
	if err != nil {
		return 0, fmt.Errorf("read the reply for %q: %w", at, err)
	}

	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "{") {
		return 0, fmt.Errorf("%w: the service answered %q for %q",
			errIERSPlainText, truncate(trimmed), at)
	}

	var reply iersReply
	if err := json.Unmarshal(raw, &reply); err != nil {
		return 0, fmt.Errorf("decode the reply for %q: %w", at, err)
	}

	if reply.Value == nil {
		return 0, fmt.Errorf("%w: no value at %q, which is expected before 1972",
			errIERSNoValue, at)
	}

	v, err := strconv.ParseFloat(*reply.Value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %q as ΔAT for %q: %w", *reply.Value, at, err)
	}

	return v, nil
}

func truncate(s string) string {
	const maxLen = 80
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "..."
}

var (
	errIERSPlainText = errors.New("iers: non-JSON reply")
	errIERSNoValue   = errors.New("iers: null value")
)
