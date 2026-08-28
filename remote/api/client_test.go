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
	"time"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
)

// redirect points an endpoint at a test server for one test. Every method
// resolves its URL through remote.URL(id), so the registry is the seam —
// there is no transport to inject, by design.
func redirect(t *testing.T, id remote.EndpointID, serverURL string) {
	t.Helper()

	scope := remote.Capture(id)
	t.Cleanup(scope.Restore)

	if err := remote.SetURL(id, serverURL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}
}

func newClient(t *testing.T, id remote.EndpointID, opts ...api.Option) *api.Client {
	t.Helper()

	c, err := api.NewClient(id, opts...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	t.Cleanup(func() { _ = c.Close() })

	return c
}

func TestGetReturnsBodyStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "q=%s", r.URL.Query().Get("q")) //nolint:errcheck // failure surfaces in the assertion below
	}))
	defer srv.Close()

	redirect(t, remote.SIMBAD, srv.URL)

	body, err := newClient(t, remote.SIMBAD).Get(context.Background(), remote.SIMBAD, "", url.Values{"q": {"M31"}})
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

	redirect(t, remote.JPLSBDB, srv.URL)

	var out struct {
		Name string `json:"name"`
		ID   int    `json:"id"`
	}

	if err := newClient(t, remote.JPLSBDB).GetJSON(context.Background(), remote.JPLSBDB, "", nil, &out); err != nil {
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

	redirect(t, remote.VizieR, srv.URL)

	client := newClient(t, remote.VizieR)

	form, err := client.PostForm(context.Background(), remote.VizieR, "", url.Values{"QUERY": {"SELECT 1"}})
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

	jsonResp, err := client.PostJSON(context.Background(), remote.VizieR, "", map[string]string{"k": "v"})
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

	redirect(t, remote.SIMBAD, srv.URL)

	_, err := newClient(t, remote.SIMBAD).Get(context.Background(), remote.SIMBAD, "", nil)

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

	redirect(t, remote.SIMBAD, srv.URL)

	body, err := newClient(t, remote.SIMBAD).Get(context.Background(), remote.SIMBAD, "", nil)
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

	redirect(t, remote.SIMBAD, srv.URL)

	if _, err := newClient(t, remote.SIMBAD).Get(context.Background(), remote.SIMBAD, "", nil); err == nil {
		t.Fatal("expected an error for a 400")
	}

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 — a 400 must not be retried", attempts)
	}
}

// Every method gates on the registry, so offline mode and Disable stop an
// API call exactly as they stop a file fetch.
func TestRegistryGateApplies(t *testing.T) {
	client := newClient(t, remote.SIMBAD)

	t.Run("disabled", func(t *testing.T) {
		scope := remote.Capture(remote.SIMBAD)
		t.Cleanup(scope.Restore)
		remote.Disable(remote.SIMBAD)

		_, err := client.Get(context.Background(), remote.SIMBAD, "", nil)
		if !errors.Is(err, remote.ErrEndpointDisabled) {
			t.Errorf("Get on a disabled endpoint = %v, want ErrEndpointDisabled", err)
		}
	})

	t.Run("offline", func(t *testing.T) {
		remote.SetOffline(true)
		t.Cleanup(func() { remote.SetOffline(false) })

		_, err := client.Get(context.Background(), remote.SIMBAD, "", nil)
		if !errors.Is(err, remote.ErrOffline) {
			t.Errorf("Get while offline = %v, want ErrOffline", err)
		}
	})
}

func TestNewClientUsesEndpointTimeout(t *testing.T) {
	// FINK's registered timeout is 120s and SIMBAD's is 30s; a client must
	// take the endpoint's own value rather than one hand-copied per caller.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		fmt.Fprint(w, "late") //nolint:errcheck // this response is expected to be abandoned
	}))
	defer srv.Close()

	redirect(t, remote.SIMBAD, srv.URL)

	_, err := newClient(t, remote.SIMBAD, api.WithTimeout(time.Millisecond), api.WithRetries(0)).
		Get(context.Background(), remote.SIMBAD, "", nil)
	if err == nil {
		t.Fatal("expected a timeout error with WithTimeout(1ms)")
	}
}

func TestNewClientUnknownEndpoint(t *testing.T) {
	if _, err := api.NewClient("nope.not.registered"); !errors.Is(err, remote.ErrUnknownEndpoint) {
		t.Errorf("NewClient(unknown) = %v, want ErrUnknownEndpoint", err)
	}
}

// An endpoint URL that already carries a query must keep it when a path is
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

	redirect(t, remote.SIMBAD, srv.URL+"/base?token=secret")

	body, err := newClient(t, remote.SIMBAD).Get(context.Background(), remote.SIMBAD, "sync", nil)
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
