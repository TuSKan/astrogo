package jpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/remote"
)

// jsonResultPayload builds a minimal Horizons JSON envelope carrying the
// given free-text "result" body, escaping it via encoding/json to avoid
// hand-rolled string-literal escaping bugs.
func jsonResultPayload(t *testing.T, result string) string {
	t.Helper()

	b, err := json.Marshal(struct {
		Result string `json:"result"`
	}{Result: result})
	testutil.AssertNoError(t, err)

	return string(b)
}

// newMockProvider spins up an httptest.Server always returning jsonPayload
// and points the Horizons endpoint at it, bypassing real network I/O.
func newMockProvider(t *testing.T, jsonPayload string) *Provider {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jsonPayload) //nolint:errcheck // test server; a write failure surfaces through the response assertions
	}))
	t.Cleanup(server.Close)

	redirect(t, remote.JPLHorizons, server.URL)

	return New()
}

// TestJPLResolveObject_ExactMatch uses real Horizons "Target body name:"
// header lines (verified live), with the "Rec #" line a small body's
// response opens with where there is one. The ID is the last
// parenthetical, which is a NAIF/SPK ID for a major body and may be a
// provisional designation for a small one; a comet may have none, leaving
// the record number as its only identifier.
func TestJPLResolveObject_ExactMatch(t *testing.T) {
	tests := []struct {
		name      string
		result    string
		fixture   string
		wantName  string
		wantID    string
		wantSPKID string
		wantDesig string
	}{
		{
			name:      "major body numeric ID",
			result:    "Target body name: Mars (499)                      {source: mar099}\nCenter body name: Earth (399)                     {source: DE441}\n",
			wantName:  "Mars",
			wantID:    "499",
			wantSPKID: "499",
		},
		{
			name:      "small body provisional designation",
			result:    "Target body name: 1685 Toro (1948 OA)             {source: JPL#895}\n",
			wantName:  "1685 Toro",
			wantID:    "1948 OA",
			wantDesig: "1948 OA",
		},
		{
			// The first parenthetical used to be taken: ID "spacecraft".
			name:      "spacecraft",
			fixture:   "exact-voyager-1.txt",
			wantName:  "Voyager 1 (spacecraft)",
			wantID:    "-31",
			wantSPKID: "-31",
		},
		{
			name:      "small body in the major-body index",
			fixture:   "exact-bennu.txt",
			wantName:  "101955 Bennu (1999 RQ36)",
			wantID:    "2101955",
			wantSPKID: "2101955",
		},
		{
			name:      "asteroid with a record number",
			fixture:   "exact-2688.txt",
			wantName:  "2688 Halley",
			wantID:    "1982 HG1",
			wantDesig: "1982 HG1",
		},
		{
			// This and the next used to be ErrNotImplemented.
			name:     "comet with no parenthetical",
			fixture:  "exact-90000030.txt",
			wantName: "1P/Halley",
			wantID:   "90000030",
		},
		{
			name:     "numbered periodic comet with no parenthetical",
			fixture:  "exact-p-2010-a2.txt",
			wantName: "354P/LINEAR",
			wantID:   "90001346",
		},
		{
			name:      "named comet",
			fixture:   "exact-c-2020-f3.txt",
			wantName:  "NEOWISE",
			wantID:    "C/2020 F3",
			wantDesig: "C/2020 F3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.result
			if tt.fixture != "" {
				result = readFixture(t, tt.fixture)
			}

			prov := newMockProvider(t, jsonResultPayload(t, result))

			var got []resolve.Target

			prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: tt.name})(func(tg resolve.Target, err error) bool {
				testutil.AssertNoError(t, err)

				got = append(got, tg)

				return true
			})

			if len(got) != 1 {
				t.Fatalf("expected 1 target, got %d: %+v", len(got), got)
			}

			testutil.AssertEqual(t, "Name", got[0].Name, tt.wantName)
			testutil.AssertEqual(t, "ID", got[0].ID, tt.wantID)
			testutil.AssertEqual(t, "SPKID", got[0].SPKID, tt.wantSPKID)
			testutil.AssertEqual(t, "Designation", got[0].Designation, tt.wantDesig)
		})
	}
}

// TestJPLResolveObject_ZeroMatches uses a real Horizons "no matches found"
// response (verified live) — a recognized-but-empty small-body index
// response must yield zero targets and no error, not ErrNotImplemented.
func TestJPLResolveObject_ZeroMatches(t *testing.T) {
	result := "*******************************************************************************\n" +
		"JPL/DASTCOM            Small-body Index Search Results     2026-Jul-07 16:26:03\n\n" +
		" Comet AND asteroid index search:\n\n   NAME = ZZZNOTAREALBODYZZZ;\n\n" +
		" Matching small-bodies: \n    No matches found.\n" +
		"*******************************************************************************\n"

	prov := newMockProvider(t, jsonResultPayload(t, result))

	var (
		got    []resolve.Target
		gotErr error
	)

	prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "ZZZNOTAREALBODYZZZ"})(func(tg resolve.Target, err error) bool {
		got = append(got, tg)
		gotErr = err

		return true
	})

	testutil.AssertNoError(t, gotErr)

	if len(got) != 0 {
		t.Fatalf("expected 0 targets, got %d: %+v", len(got), got)
	}
}

// TestJPLResolveObject_UnrecognizedShape confirms a non-blank result that
// matches none of the three known shapes still surfaces ErrNotImplemented
// rather than silently returning nothing or fabricating a Target.
func TestJPLResolveObject_UnrecognizedShape(t *testing.T) {
	prov := newMockProvider(t, jsonResultPayload(t, "Some entirely novel Horizons output shape with no recognizable marker text.\n"))

	iter := prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "???"})
	iter(func(_ resolve.Target, err error) bool {
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("expected ErrNotImplemented, got %v", err)
		}

		return false
	})
}

// TestJPLResolve_ReturnsRealMatch confirms Provider.Resolve (the
// resolve.Provider-interface entry point) now surfaces a real resolved
// Target for an unambiguous query instead of always returning ok=false.
func TestJPLResolve_ReturnsRealMatch(t *testing.T) {
	result := "Target body name: Mars (499)                      {source: mar099}\n"
	prov := newMockProvider(t, jsonResultPayload(t, result))

	target, err := prov.Resolve(context.Background(), "Mars")
	testutil.AssertNoError(t, err)
	testutil.AssertEqual(t, "Resolve Name", target.Name, "Mars")
}

func TestJPLErrorResponse(t *testing.T) {
	jsonPayload := `{
  "signature": {"version": "1.2", "source": "NASA/JPL Horizons API"},
  "error": "unrecognized command"
}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if _, err := fmt.Fprint(w, jsonPayload); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	prov := New()

	redirect(t, remote.JPLHorizons, server.URL)

	req := resolve.ObjectRequest{Query: "!!!ERROR!!!"}
	iter := prov.ResolveObject(context.Background(), req)
	iter(func(_ resolve.Target, err error) bool {
		if err == nil {
			t.Fatalf("Expected explicit json payload error")
		}

		if !errors.Is(err, ErrAPIError) {
			t.Fatalf("Expected ErrAPIError, got: %v", err)
		}

		if !strings.Contains(err.Error(), "unrecognized command") {
			t.Fatalf("Unexpected error mapping: %v", err)
		}

		return false
	})
}

func TestProviderInterface(t *testing.T) {
	p := New()
	testutil.AssertEqual(t, "Name", p.Name(), "jpl")

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapObjectResolution {
		t.Errorf("expected CapObjectResolution, got %v", caps)
	}

	redirect(t, remote.JPLHorizons, "http://127.0.0.1:1")

	// Fast fail search / resolve without any real network call.
	// This hits the missing coverage lines.
	_, err := p.Resolve(context.Background(), "non_existent_body_to_trigger_miss")
	if err == nil {
		t.Error("expected Resolve to fail with no transport")
	}
}

// redirect points endpoint id at a test server for the duration of one
// test. It replaces the old http.RoundTripper injection: remote/api's
// Client is opaque by design, and every request resolves its URL through
// remote.URL(id) anyway, so the registry is the natural seam.
func redirect(t *testing.T, id remote.EndpointID, url string) {
	t.Helper()

	scope := remote.Capture(id)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(id, url); err != nil {
		t.Fatalf("SetURL(%s): %v", id, err)
	}
}
