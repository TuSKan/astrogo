package spk

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// horizonsCIFault is the explanation JPL Horizons returned for both 433 Eros
// and Apophis on 2026-09-15, with HTTP 200 and an otherwise well-formed body,
// failing astrogo's CI. The same two requests succeeded again within the hour.
//
// Quoted verbatim, double semicolon included, because the point of this file is
// that astrogo recognizes the real string and not a tidied-up version of it.
const horizonsCIFault = "wldini(): missing required file LTKERNL; ; " +
	"ERROR in VLRDC: Var not declared: IP_ADDR"

// TestHorizonsInternalFaultSeparatesOutageFromRefusal is the discrimination
// this whole mechanism exists for, stated as a table.
//
// The two classes arrive identically — 200, well-formed JSON, an explanation in
// the "error" field — and mean opposite things. Getting it backwards in either
// direction is a real cost: a refusal misread as an outage is a permanently
// broken request that skips silently for ever, and an outage misread as a
// refusal is a red build nobody can fix.
func TestHorizonsInternalFaultSeparatesOutageFromRefusal(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name        string
		explanation string
		wantFault   bool
	}{
		{
			name:        "the fault that failed CI",
			explanation: horizonsCIFault,
			wantFault:   true,
		},
		{
			name:        "a missing file on JPL's own disk",
			explanation: "wldini(): missing required file LTKERNL",
			wantFault:   true,
		},
		{
			name:        "an undeclared variable in JPL's own code",
			explanation: "ERROR in VLRDC: Var not declared: IP_ADDR",
			wantFault:   true,
		},
		{
			// The canonical real refusal, quoted in ErrHorizonsRefused's doc
			// comment. It must stay loud: no retry will ever make it work.
			name: "a considered refusal about the request",
			explanation: "SPK creation is not available for pre-computed objects " +
				"in the major body index",
			wantFault: false,
		},
		{
			name:        "an out-of-range object number",
			explanation: "requested IOBJ=20000004 is out of bounds",
			wantFault:   false,
		},
		{
			// "ERROR in " is deliberately not a marker, precisely so that a
			// complaint about the request phrased that way is not swallowed.
			name:        "a request complaint wearing an ERROR in prefix",
			explanation: "ERROR in SPKSUB: Requested interval is outside object coverage",
			wantFault:   false,
		},
		{
			name:        "no explanation at all",
			explanation: "",
			wantFault:   false,
		},
	} {
		if got := horizonsInternalFault(tc.explanation); got != tc.wantFault {
			t.Errorf("%s: horizonsInternalFault(%q) = %v, want %v",
				tc.name, tc.explanation, got, tc.wantFault)
		}
	}
}

// TestTransientHorizonsFaultCoversBothAnomalies asserts the predicate callers
// and tests actually branch on.
//
// Both known ways for Horizons to answer successfully while being unable to do
// the work must satisfy it, and a real refusal must not — otherwise the five
// test sites that skip on it would start hiding permanent failures.
func TestTransientHorizonsFaultCoversBothAnomalies(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"empty kernel", ErrHorizonsEmptyKernel, true},
		{"server fault", ErrHorizonsInternalFault, true},
		{"wrapped server fault", errors.Join(ErrHorizonsRefused, ErrHorizonsInternalFault), true},
		{"plain refusal", ErrHorizonsRefused, false},
		{"unrelated", ErrCorruptSPK, false},
		{"nil", nil, false},
	} {
		if got := TransientHorizonsFault(tc.err); got != tc.want {
			t.Errorf("%s: TransientHorizonsFault(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

// TestCacheAPIReportsAServerFaultAsBothRefusalAndFault is the end-to-end half:
// the real response shape, through the real code path, to the two sentinels.
//
// The double wrapping is the compatibility contract — every caller written
// before ErrHorizonsInternalFault existed matched on ErrHorizonsRefused and
// must keep matching — so it is asserted rather than assumed.
func TestCacheAPIReportsAServerFaultAsBothRefusalAndFault(t *testing.T) {
	origEndpoint, _ := remote.Lookup(remote.JPLHorizonsSPK)

	t.Cleanup(func() { _ = remote.SetURL(remote.JPLHorizonsSPK, origEndpoint.URL) })

	bucket := tempBucket(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Horizons' own shape for this: 200, no spk, the fault in "error".
		_, _ = w.Write([]byte(`{"error":"` + horizonsCIFault + `"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizonsSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	_, err := CacheAPI(context.Background(), bucket, "", "433", start, end)
	if err == nil {
		t.Fatal("CacheAPI returned no error for a Horizons server fault")
	}

	if !errors.Is(err, ErrHorizonsRefused) {
		t.Errorf("error does not match ErrHorizonsRefused, which every caller "+
			"written before the fault sentinel existed branches on: %v", err)
	}

	if !errors.Is(err, ErrHorizonsInternalFault) {
		t.Errorf("error does not match ErrHorizonsInternalFault: %v", err)
	}

	if !TransientHorizonsFault(err) {
		t.Errorf("TransientHorizonsFault said no, so CI would still go red on "+
			"a JPL outage: %v", err)
	}
}

// TestCacheAPIKeepsARealRefusalFatal is the other half, and the one that would
// actually cost something if it broke: a permanent refusal must NOT look
// transient, or every test that skips on the predicate starts passing while the
// request can never work.
func TestCacheAPIKeepsARealRefusalFatal(t *testing.T) {
	origEndpoint, _ := remote.Lookup(remote.JPLHorizonsSPK)

	t.Cleanup(func() { _ = remote.SetURL(remote.JPLHorizonsSPK, origEndpoint.URL) })

	bucket := tempBucket(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"SPK creation is not available for ` +
			`pre-computed objects in the major body index"}`))
	}))
	defer srv.Close()

	if err := remote.SetURL(remote.JPLHorizonsSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	remote.EnableDownloads(0, remote.JPLHorizonsSPK)

	start := time.FromJD(2451545.0, time.UTC)
	end := time.FromJD(2451546.0, time.UTC)

	_, err := CacheAPI(context.Background(), bucket, "", "433", start, end)
	if err == nil {
		t.Fatal("CacheAPI returned no error for a Horizons refusal")
	}

	if !errors.Is(err, ErrHorizonsRefused) {
		t.Errorf("a refusal should match ErrHorizonsRefused: %v", err)
	}

	if TransientHorizonsFault(err) {
		t.Errorf("a permanent refusal was reported as transient, which would make "+
			"every skip-on-transient test hide it for ever: %v", err)
	}
}
