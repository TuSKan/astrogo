package api_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/remote/api"
	"github.com/TuSKan/astrogo/time"
)

// Every test here stands up an httptest server and hands its URL to the
// method under test, because that is the whole of this package's contract: a
// base URL in, a body out. There is no registry to stub and no transport to
// inject — resolving which URL an endpoint has belongs to
// github.com/TuSKan/astrogo/remote, and is tested there against the gate
// itself rather than through a client that would only be carrying the answer.

// newClient builds a client for one test, closed on cleanup.
//
// The zero timeout means [api.DefaultTimeout]; a test that cares passes
// api.WithTimeout.
func newClient(t *testing.T, opts ...api.Option) *api.Client {
	t.Helper()

	c := api.NewClient(0, opts...)

	t.Cleanup(func() { _ = c.Close() })

	return c
}

func TestGetReturnsBodyStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "q=%s", r.URL.Query().Get("q")) //nolint:errcheck // failure surfaces in the assertion below
	}))
	defer srv.Close()

	base := srv.URL

	body, err := newClient(t).Get(context.Background(), base, "", url.Values{"q": {"M31"}})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	defer func() { _ = body.Close() }()

	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if string(got) != "q=M31" {
		t.Errorf("body = %q, want %q", got, "q=M31")
	}
}

func TestGetJSONDecodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"name":"Ceres","id":1}`) //nolint:errcheck // failure surfaces in the assertion below
	}))
	defer srv.Close()

	base := srv.URL

	var out struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}

	if err := newClient(t).GetJSON(context.Background(), base, "", nil, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	if out.Name != "Ceres" || out.ID != 1 {
		t.Errorf("decoded %+v, want {Ceres 1}", out)
	}
}

func TestPostFormAndPostJSON(t *testing.T) {
	var (
		gotContentType string
		gotBody        string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")

		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)

		fmt.Fprint(w, "ok") //nolint:errcheck // failure surfaces in the assertions below
	}))
	defer srv.Close()

	base := srv.URL

	client := newClient(t)

	form, err := client.PostForm(context.Background(), base, "", url.Values{"QUERY": {"SELECT 1"}})
	if err != nil {
		t.Fatalf("PostForm: %v", err)
	}

	_ = form.Close()

	if !strings.HasPrefix(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("PostForm Content-Type = %q", gotContentType)
	}

	if gotBody != "QUERY=SELECT+1" {
		t.Errorf("PostForm body = %q", gotBody)
	}

	jsonResp, err := client.PostJSON(context.Background(), base, "", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("PostJSON: %v", err)
	}

	_ = jsonResp.Close()

	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Errorf("PostJSON Content-Type = %q", gotContentType)
	}

	if gotBody != `{"k":"v"}` {
		t.Errorf("PostJSON body = %q", gotBody)
	}
}

// A non-2xx must never reach a caller as a body: parsing an error page as
// data is exactly the failure this converts into a typed error.
func TestNon2xxBecomesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such object", http.StatusNotFound)
	}))
	defer srv.Close()

	base := srv.URL

	_, err := newClient(t).Get(context.Background(), base, "", nil)

	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("Get = %v, want *api.HTTPError", err)
	}

	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", httpErr.StatusCode)
	}

	if !strings.Contains(httpErr.Body, "no such object") {
		t.Errorf("Body = %q, want the server's own explanation", httpErr.Body)
	}
}

func TestRetriesServerErrorThenSucceeds(t *testing.T) {
	var attempts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)

			return
		}

		fmt.Fprint(w, "recovered") //nolint:errcheck // failure surfaces in the assertion below
	}))
	defer srv.Close()

	base := srv.URL

	body, err := newClient(t).Get(context.Background(), base, "", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	defer func() { _ = body.Close() }()

	if attempts != 3 {
		t.Errorf("attempts = %d, want 3 (two 503s then success)", attempts)
	}
}

// A 4xx is the caller's own request; re-sending it just wastes the
// service's budget.
func TestDoesNotRetryClientError(t *testing.T) {
	var attempts int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++

		http.Error(w, "bad query", http.StatusBadRequest)
	}))
	defer srv.Close()

	base := srv.URL

	if _, err := newClient(t).Get(context.Background(), base, "", nil); err == nil {
		t.Fatal("expected an error for a 400")
	}

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 — a 400 must not be retried", attempts)
	}
}

func TestWithTimeoutOverridesTheConstructorsTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(w, "late") //nolint:errcheck // this response is expected to be abandoned
	}))
	defer srv.Close()

	base := srv.URL

	_, err := newClient(t, api.WithTimeout(time.Millisecond), api.WithRetries(0)).
		Get(context.Background(), base, "", nil)
	if err == nil {
		t.Fatal("expected a timeout error with WithTimeout(1ms)")
	}
}

// A base URL that already carries a query must keep it when a path is
// appended — string concatenation would splice the path in after the query
// and silently address the wrong thing.
func TestRequestURLPreservesExistingQuery(t *testing.T) {
	var gotPath, gotToken string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotToken = r.URL.Query().Get("token")

		fmt.Fprint(w, "ok") //nolint:errcheck // failure surfaces in the assertions below
	}))
	defer srv.Close()

	base := srv.URL + "/base?token=secret"

	body, err := newClient(t).Get(context.Background(), base, "sync", nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	_ = body.Close()

	if gotPath != "/base/sync" {
		t.Errorf("path = %q, want /base/sync", gotPath)
	}

	if gotToken != "secret" {
		t.Errorf("token = %q, want the endpoint URL's own query preserved", gotToken)
	}
}

// serveJSON stands up a server answering every request with body, and returns
// its base URL.
func serveJSON(t *testing.T, body string) string {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))

	t.Cleanup(srv.Close)

	return srv.URL
}

// TestGetJSONRejectsDuplicateNames is the correctness this decoder moved to
// encoding/json/v2 for.
//
// v1 silently took the last occurrence of a repeated key, which is a wrong
// answer delivered with confidence and indistinguishable from a right one. A
// response that repeats a name is malformed; saying so is worth more than
// guessing which occurrence was meant.
func TestGetJSONRejectsDuplicateNames(t *testing.T) {
	base := serveJSON(t, `{"ra":10.5,"dec":41.2,"ra":359.9}`)

	var out struct {
		RA  float64 `json:"ra"`
		Dec float64 `json:"dec"`
	}

	err := newClient(t).GetJSON(context.Background(), base, "", nil, &out)
	if err == nil {
		t.Fatalf("a response repeating \"ra\" decoded without complaint, to RA %v — "+
			"under the old decoder the second occurrence silently won", out.RA)
	}

	if !strings.Contains(err.Error(), "remote/api: decode JSON") {
		t.Errorf("err = %v, want it wrapped with the decode context", err)
	}
}

// TestGetJSONStillMatchesNamesCaseInsensitively pins a v2 default that is
// deliberately turned back off.
//
// Exact matching would leave a renamed field at its zero value, and for a
// coordinate that is not a missing value: RA 0, Dec 0 is a real point in
// Pisces, and a target built on it rises, transits and sets without
// complaint. plan.ErrNoCoordinates exists because that substitution reached
// production once already — so exact matching would trade a silently wrong
// value for a silently zero one, which is not an improvement.
func TestGetJSONStillMatchesNamesCaseInsensitively(t *testing.T) {
	base := serveJSON(t, `{"RA":10.5,"Dec":41.2}`)

	var out struct {
		RA  float64 `json:"ra"`
		Dec float64 `json:"dec"`
	}

	if err := newClient(t).GetJSON(context.Background(), base, "", nil, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	if out.RA != 10.5 || out.Dec != 41.2 {
		t.Errorf("decoded %+v, want {10.5 41.2} — a service renaming \"ra\" to \"RA\" "+
			"must not silently leave the position at the origin", out)
	}
}

// TestGetJSONToleratesInvalidUTF8InAName pins the other restored default.
//
// v2 rejects invalid UTF-8 outright. The field that might carry a bad byte
// here is a label; the fields that have to be right are numbers. Losing a
// whole position because an object's name is mis-encoded is the wrong trade.
func TestGetJSONToleratesInvalidUTF8InAName(t *testing.T) {
	// 0xFF is not valid UTF-8 in any position.
	base := serveJSON(t, "{\"name\":\"Ceres\xff\",\"ra\":10.5}")

	var out struct {
		Name string  `json:"name"`
		RA   float64 `json:"ra"`
	}

	if err := newClient(t).GetJSON(context.Background(), base, "", nil, &out); err != nil {
		t.Fatalf("GetJSON: %v — a bad byte in a name must not cost the caller its position", err)
	}

	if out.RA != 10.5 {
		t.Errorf("RA = %v, want 10.5", out.RA)
	}

	if !strings.HasPrefix(out.Name, "Ceres") {
		t.Errorf("Name = %q, want it to start with Ceres", out.Name)
	}
}
