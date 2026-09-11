package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The APIClient tests below are the ones that used to live in remote/api, and
// they moved for a reason worth stating once: that package no longer knows
// what an endpoint is. It takes a base URL per call and does what it is told.
// Resolving an id into that URL — refusing it while offline, refusing it while
// disabled, honouring an override — is this package's job and is only
// observable here.

// TestAPIClientGatesEveryRequest is the property the wrapper exists for.
//
// A client is not a licence to reach a service. It resolves per call, so
// SetOffline and Disable stop an API request exactly as they stop a file
// fetch, on a client that was built while the endpoint was perfectly
// reachable. Building the client first and only then closing the gate is the
// whole point of the arrangement — a resolution done once at construction
// would pass this test's setup and fail its intent.
func TestAPIClientGatesEveryRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("reached the server"))
	}))
	defer srv.Close()

	scope := Capture(SIMBAD)
	t.Cleanup(scope.Restore)

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	client, err := NewAPIClient(SIMBAD)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	// Reachable first, so a later refusal is the gate rather than a broken
	// fixture.
	body, err := client.Get(t.Context(), SIMBAD, "", nil)
	if err != nil {
		t.Fatalf("Get before the gate closed: %v", err)
	}

	_ = body.Close()

	t.Run("offline", func(t *testing.T) {
		SetOffline(true)
		t.Cleanup(func() { SetOffline(false) })

		if _, err := client.Get(t.Context(), SIMBAD, "", nil); !errors.Is(err, ErrOffline) {
			t.Errorf("Get while offline = %v, want ErrOffline.\n"+
				"  The client was built while online; resolution has to happen per "+
				"request or offline mode means nothing to an existing client.", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		Disable(SIMBAD)
		t.Cleanup(func() { Enable(SIMBAD) })

		if _, err := client.Get(t.Context(), SIMBAD, "", nil); !errors.Is(err, ErrEndpointDisabled) {
			t.Errorf("Get on a disabled endpoint = %v, want ErrEndpointDisabled", err)
		}
	})

	t.Run("unknown endpoint", func(t *testing.T) {
		if _, err := client.Get(t.Context(), EndpointID("nope.not.registered"), "", nil); !errors.Is(err, ErrUnknownEndpoint) {
			t.Errorf("Get on an unregistered id = %v, want ErrUnknownEndpoint", err)
		}
	})
}

// TestAPIClientGatesEveryVerb keeps the three quieter methods honest.
//
// Get is the one every reader checks. PostForm carries TAP-ADQL queries and
// PostJSON carries Horizons kernel requests, and a gate that covered only the
// method with a test would let both straight past while looking correct.
func TestAPIClientGatesEveryVerb(t *testing.T) {
	scope := Capture(SIMBAD)
	t.Cleanup(scope.Restore)

	client, err := NewAPIClient(SIMBAD)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	SetOffline(true)
	t.Cleanup(func() { SetOffline(false) })

	var out struct{}

	for _, tc := range []struct {
		call func() error
		name string
	}{
		{name: "Get", call: func() error {
			_, err := client.Get(t.Context(), SIMBAD, "", nil)

			return err
		}},
		{name: "GetJSON", call: func() error {
			return client.GetJSON(t.Context(), SIMBAD, "", nil, &out)
		}},
		{name: "PostForm", call: func() error {
			_, err := client.PostForm(t.Context(), SIMBAD, "", nil)

			return err
		}},
		{name: "PostJSON", call: func() error {
			_, err := client.PostJSON(t.Context(), SIMBAD, "", map[string]string{"k": "v"})

			return err
		}},
	} {
		if err := tc.call(); !errors.Is(err, ErrOffline) {
			t.Errorf("%s while offline = %v, want ErrOffline", tc.name, err)
		}
	}
}

// TestNewAPIClientRefusesAnUnregisteredEndpoint pins the one thing the
// constructor still checks.
//
// It looks the endpoint up for its timeout, so an id that is not in the
// registry has no timeout to take and is a caller mistake rather than a
// transient refusal. Saying so at construction beats returning a client whose
// every request fails.
func TestNewAPIClientRefusesAnUnregisteredEndpoint(t *testing.T) {
	t.Parallel()

	if _, err := NewAPIClient("nope.not.registered"); !errors.Is(err, ErrUnknownEndpoint) {
		t.Errorf("NewAPIClient(unknown) = %v, want ErrUnknownEndpoint", err)
	}
}

// TestNewAPIClientSucceedsWhileOffline states the order deliberately, because
// the opposite is a defensible design and this is not it.
//
// Constructing a client is not asking to reach the network; a provider is
// built once, often at process start, and used much later. Refusing here would
// make offline mode a property of when a program happened to construct its
// providers rather than of when it tried to use them.
func TestNewAPIClientSucceedsWhileOffline(t *testing.T) {
	scope := Capture(SIMBAD)
	t.Cleanup(scope.Restore)

	SetOffline(true)
	t.Cleanup(func() { SetOffline(false) })

	client, err := NewAPIClient(SIMBAD)
	if err != nil {
		t.Fatalf("NewAPIClient while offline = %v, want a client that fails on use instead", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	if _, err := client.Get(t.Context(), SIMBAD, "", nil); !errors.Is(err, ErrOffline) {
		t.Errorf("Get = %v, want ErrOffline — the refusal belongs to the request", err)
	}
}

// TestAPIClientReachesTheResolvedURL proves the wrapper forwards to the URL the
// registry gave rather than to one it kept from construction.
//
// SetURL after the client exists is the sharpest version of that: a test
// server stood up second must still receive the request.
func TestAPIClientReachesTheResolvedURL(t *testing.T) {
	scope := Capture(JPLSBDB)
	t.Cleanup(scope.Restore)

	client, err := NewAPIClient(JPLSBDB)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Ceres"}`))
	}))
	defer srv.Close()

	if err := SetURL(JPLSBDB, srv.URL+"/api"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	var out struct {
		Name string `json:"name"`
	}

	if err := client.GetJSON(t.Context(), JPLSBDB, "lookup", nil, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}

	if out.Name != "Ceres" {
		t.Errorf("decoded name = %q, want Ceres", out.Name)
	}

	if gotPath != "/api/lookup" {
		t.Errorf("server saw path %q, want /api/lookup — the override's own path must survive the join", gotPath)
	}
}

// TestAPIErrorsAreReachableFromThisPackage keeps the front door's promise that
// a caller never learns remote/api exists.
//
// The type and the sentinel are declared down there; if either stopped being
// re-exported, a caller inspecting a failed request would have to import the
// subpackage to name what they caught, which is precisely the import this
// arrangement forbids.
func TestAPIErrorsAreReachableFromThisPackage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no such object", http.StatusNotFound)
	}))
	defer srv.Close()

	scope := Capture(SIMBAD)
	t.Cleanup(scope.Restore)

	if err := SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	client, err := NewAPIClient(SIMBAD)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	_, err = client.Get(t.Context(), SIMBAD, "", nil)

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("Get = %v, want a *remote.HTTPError", err)
	}

	if httpErr.StatusCode != http.StatusNotFound {
		t.Errorf("StatusCode = %d, want 404", httpErr.StatusCode)
	}

	if !strings.Contains(httpErr.Body, "no such object") {
		t.Errorf("Body = %q, want the service's own explanation", httpErr.Body)
	}
}
