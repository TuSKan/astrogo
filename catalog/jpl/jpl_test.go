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
// and wires it into a fresh Provider's transport, bypassing real network I/O.
func newMockProvider(jsonPayload string) *Provider {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, jsonPayload) //nolint:errcheck // test server, write failure would fail the test via response assertions
	}))

	prov := New()
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

	return prov
}

// TestJPLResolveObject_AmbiguousMajorBody uses a real Horizons "Multiple
// major-bodies match string" response (verified live for query "Mars",
// trimmed to 3 of the 10 real matches) — including a body whose name
// overflows into the designation column, the exact case that motivated
// cosparDesignationRe over fixed-offset slicing.
func TestJPLResolveObject_AmbiguousMajorBody(t *testing.T) {
	result := "*******************************************************************************\n" +
		" Multiple major-bodies match string \"MARS*\"\n\n" +
		"  ID#      Name                               Designation  IAU/aliases/other   \n" +
		"  -------  ---------------------------------- -----------  ------------------- \n" +
		"        4  Mars Barycenter                                                      \n" +
		"      499  Mars                                                                 \n" +
		"      -74  Mars Reconnaissance Orbiter (spacec2005-029A    MRO                  \n" +
		" \n" +
		"   Number of matches =  3. Use ID# to make unique selection.\n" +
		"*******************************************************************************\n"

	prov := newMockProvider(jsonResultPayload(t, result))

	var got []resolve.Target

	prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "Mars"})(func(tg resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		got = append(got, tg)

		return true
	})

	if len(got) != 3 {
		t.Fatalf("expected 3 targets, got %d: %+v", len(got), got)
	}

	testutil.AssertEqual(t, "target[0] ID", got[0].ID, "4")
	testutil.AssertEqual(t, "target[0] Name", got[0].Name, "Mars Barycenter")
	testutil.AssertEqual(t, "target[1] ID", got[1].ID, "499")
	testutil.AssertEqual(t, "target[1] Name", got[1].Name, "Mars")
	testutil.AssertEqual(t, "target[2] Designation", got[2].Designation, "2005-029A")
	testutil.AssertEqual(t, "target[2] Aliases[0]", got[2].Aliases[0], "MRO")
}

// TestJPLResolveObject_AmbiguousSmallBody uses a real Horizons "Small-body
// Index Search Results" response (verified live for query "73P", trimmed
// to 2 of the 84 real matches) — a structurally different table from the
// major-body one, keyed on Primary Desig / Name columns instead of
// ID# / Designation.
func TestJPLResolveObject_AmbiguousSmallBody(t *testing.T) {
	result := "*******************************************************************************\n" +
		"JPL/DASTCOM            Small-body Index Search Results     2026-Jul-07 16:26:47\n\n" +
		" Comet AND asteroid index search:\n\n    DES = 73P;\n\n Matching small-bodies: \n\n" +
		"    Record #  Epoch-yr  >MATCH DESIG<  Primary Desig  Name  \n" +
		"    --------  --------  -------------  -------------  -------------------------\n" +
		"    90000733    1930    73P            73P             Schwassmann-Wachmann 3\n" +
		"    90000740    1995    73P-A          73P-A           Schwassmann-Wachmann 3\n"

	prov := newMockProvider(jsonResultPayload(t, result))

	var got []resolve.Target

	prov.ResolveObject(context.Background(), resolve.ObjectRequest{Query: "73P"})(func(tg resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		got = append(got, tg)

		return true
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 targets, got %d: %+v", len(got), got)
	}

	testutil.AssertEqual(t, "target[0] ID", got[0].ID, "90000733")
	testutil.AssertEqual(t, "target[0] Name", got[0].Name, "Schwassmann-Wachmann 3")
	testutil.AssertEqual(t, "target[0] Designation", got[0].Designation, "73P")
	testutil.AssertEqual(t, "target[1] Designation", got[1].Designation, "73P-A")
}

// TestJPLResolveObject_ExactMatch uses real Horizons "Target body name:"
// header lines (verified live) — a major body with a purely numeric ID and
// a small body whose parenthetical is a non-numeric provisional
// designation instead.
func TestJPLResolveObject_ExactMatch(t *testing.T) {
	tests := []struct {
		name      string
		result    string
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov := newMockProvider(jsonResultPayload(t, tt.result))

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

	prov := newMockProvider(jsonResultPayload(t, result))

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
	prov := newMockProvider(jsonResultPayload(t, "Some entirely novel Horizons output shape with no recognizable marker text.\n"))

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
	prov := newMockProvider(jsonResultPayload(t, result))

	target, ok := prov.Resolve(context.Background(), "Mars")
	testutil.AssertEqual(t, "Resolve Ok", ok, true)
	testutil.AssertEqual(t, "Resolve Name", target.Name, "Mars")
}

type mockTransport struct {
	Handler http.Handler
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.Handler.ServeHTTP(rec, req)
	resp := rec.Result()
	resp.Request = req

	return resp, nil
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
	prov.client.HTTPClient.Transport = &mockTransport{Handler: server.Config.Handler}

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

// errTransport fails every request locally, so exercising the miss path
// below never reaches the real network (this is a default, non-network-tagged
// test — see CLAUDE.md's build-tag convention).
type errTransport struct{}

var errNoTransport = errors.New("errTransport: no network access in this test")

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errNoTransport
}

func TestProviderInterface(t *testing.T) {
	p := New()
	testutil.AssertEqual(t, "Name", p.Name(), "jpl")

	caps := p.Capabilities()
	if len(caps) != 1 || caps[0] != resolve.CapObjectResolution {
		t.Errorf("expected CapObjectResolution, got %v", caps)
	}

	p.client.HTTPClient.Transport = errTransport{}

	// Fast fail search / resolve without any real network call.
	// This hits the missing coverage lines.
	_, ok := p.Resolve(context.Background(), "non_existent_body_to_trigger_miss")
	if ok {
		t.Error("expected Resolve to fail with no transport")
	}
}
