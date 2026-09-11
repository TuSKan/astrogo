package remote

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// The point of a Client is that two of them disagree. Every test here sets
// opposite policies on two clients and checks that each one keeps its own —
// a test that configured a single client would pass just as well against the
// process-wide globals this replaced, and would prove nothing.

// TestTwoClientsHoldOppositePolicies is #114's motivating case: an HTTP
// handler serving user queries offline while a background prefetcher
// downloads, in one binary.
func TestTwoClientsHoldOppositePolicies(t *testing.T) {
	t.Parallel()

	prefetch := NewClient()
	prefetch.EnableDownloads(200<<20, NAIFSPK)

	serve := NewClient()
	serve.SetOffline(true)

	if ok, maxSize := prefetch.DownloadsEnabled(NAIFSPK); !ok || maxSize != 200<<20 {
		t.Errorf("prefetcher: DownloadsEnabled = (%v, %d), want (true, %d)", ok, maxSize, 200<<20)
	}

	if ok, _ := serve.DownloadsEnabled(NAIFSPK); ok {
		t.Error("the handler inherited the prefetcher's download consent")
	}

	if _, err := prefetch.URL(NAIFSPK); err != nil {
		t.Errorf("prefetcher: URL = %v, want it reachable", err)
	}

	if _, err := serve.URL(NAIFSPK); !errors.Is(err, ErrOffline) {
		t.Errorf("handler: URL = %v, want ErrOffline", err)
	}
}

// TestADedicatedClientDoesNotDisturbTheDefault is the other direction, and the
// one that decides whether this is safe to adopt piecemeal: a component that
// starts holding its own client must not change what everything else sees.
func TestADedicatedClientDoesNotDisturbTheDefault(t *testing.T) {
	scope := Capture(SIMBAD, NAIFSPK)
	t.Cleanup(scope.Restore)

	own := NewClient()
	own.SetOffline(true)
	own.Disable(SIMBAD)
	own.EnableDownloads(0, NAIFSPK)

	if err := own.SetURL(SIMBAD, "https://mirror.invalid/simbad"); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	own.SetDataDir("file:///somewhere/else")

	if Offline() {
		t.Error("the default went offline because another client did")
	}

	if _, err := URL(SIMBAD); err != nil {
		t.Errorf("default: URL(SIMBAD) = %v, want it still reachable", err)
	}

	if got, _ := URL(SIMBAD); strings.Contains(got, "mirror.invalid") {
		t.Errorf("default: URL(SIMBAD) = %q, want the built-in URL — another client's override leaked", got)
	}

	if ok, _ := DownloadsEnabled(NAIFSPK); ok {
		t.Error("the default gained download consent another client granted")
	}

	if strings.Contains(DataDirURL(), "somewhere/else") {
		t.Errorf("default: DataDirURL = %q, want its own — another client's cache location leaked", DataDirURL())
	}
}

// TestPackageFunctionsOperateOnDefault pins the http.DefaultClient analogue.
//
// It is what makes the change compatible: every existing caller keeps writing
// remote.SetOffline and keeps getting the same behaviour, because that call and
// Default().SetOffline are the same state rather than two that agree by
// convention.
func TestPackageFunctionsOperateOnDefault(t *testing.T) {
	scope := Capture(SIMBAD)
	t.Cleanup(scope.Restore)

	Disable(SIMBAD)

	if _, err := Default().URL(SIMBAD); !errors.Is(err, ErrEndpointDisabled) {
		t.Errorf("Default().URL after package-level Disable = %v, want ErrEndpointDisabled", err)
	}

	Default().Enable(SIMBAD)

	if _, err := URL(SIMBAD); err != nil {
		t.Errorf("package-level URL after Default().Enable = %v, want it reachable", err)
	}
}

// TestClientGetFileUsesItsOwnCacheAndConsent walks the whole fetch path on a
// dedicated client: its consent gate, its cache location, its endpoint URL.
//
// Cache location is the part worth having a test for. A GetFile that consulted
// the client for consent but the process default for the cache directory would
// pass every consent assertion and quietly write into the wrong bucket, which
// is the failure this whole change would otherwise introduce.
func TestClientGetFileUsesItsOwnCacheAndConsent(t *testing.T) {
	t.Parallel()

	const (
		name    = "planets/tiny.bsp"
		payload = "kernel bytes"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	cacheURL := testutil.FileURL(t, t.TempDir())

	c := NewClient()
	c.SetDataDir(cacheURL)

	if err := c.SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	// Consent first: without it this client must refuse, whatever the process
	// default has been told.
	if _, _, err := c.GetFile(t.Context(), NAIFSPK, name); !errors.Is(err, ErrDownloadDenied) {
		t.Fatalf("GetFile without consent = %v, want ErrDownloadDenied", err)
	}

	c.EnableDownloads(0, NAIFSPK)

	bucket, key, err := c.GetFile(t.Context(), NAIFSPK, name)
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}

	got, err := bucket.ReadAll(t.Context(), key)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}

	if string(got) != payload {
		t.Errorf("cached content = %q, want %q", got, payload)
	}

	// The object landed in this client's cache, which is the assertion a
	// default-cache regression fails.
	own, err := c.DataDir(t.Context())
	if err != nil {
		t.Fatalf("DataDir: %v", err)
	}

	if exists, _ := own.Exists(t.Context(), key); !exists {
		t.Errorf("key %q is not in the client's own cache at %s", key, cacheURL)
	}
}

// TestAPIClientAnswersToTheClientThatBuiltIt covers the seam most likely to be
// got wrong, because it is the one where the policy is consulted later than it
// is chosen.
//
// An APIClient resolves per request. If it resolved through the package default
// rather than through its owner, a component that built its client from a
// scoped policy would silently use the process-wide one — and every test that
// only ever uses Default would still pass.
func TestAPIClientAnswersToTheClientThatBuiltIt(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("reached the server"))
	}))
	defer srv.Close()

	c := NewClient()

	if err := c.SetURL(SIMBAD, srv.URL); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	api, err := c.NewAPIClient(SIMBAD)
	if err != nil {
		t.Fatalf("NewAPIClient: %v", err)
	}

	t.Cleanup(func() { _ = api.Close() })

	body, err := api.Get(t.Context(), SIMBAD, "", nil)
	if err != nil {
		t.Fatalf("Get through the owning client: %v", err)
	}

	_ = body.Close()

	// Offline on the owner stops it; the default is untouched and cannot be
	// what answered above.
	c.SetOffline(true)

	if _, err := api.Get(t.Context(), SIMBAD, "", nil); !errors.Is(err, ErrOffline) {
		t.Errorf("Get after the owning client went offline = %v, want ErrOffline.\n"+
			"  The request resolved through some other policy than the client that built it.", err)
	}

	if Offline() {
		t.Error("the default went offline because a dedicated client did")
	}
}

// TestNewClientStartsFromTheDefaultPolicyNotTheDefaultClient states the choice
// deliberately, because the opposite is defensible and this is not it.
//
// A new client does not inherit whatever the program has configured on
// [Default]. The reason to build a second client is that the first one's policy
// is not yours, so inheriting it would be the more surprising of the two.
func TestNewClientStartsFromTheDefaultPolicyNotTheDefaultClient(t *testing.T) {
	scope := Capture(NAIFSPK)
	t.Cleanup(scope.Restore)

	EnableDownloads(0, NAIFSPK)
	SetOffline(true)

	t.Cleanup(func() { SetOffline(false) })

	fresh := NewClient()

	if ok, _ := fresh.DownloadsEnabled(NAIFSPK); ok {
		t.Error("a new client inherited the default's download consent")
	}

	if fresh.Offline() {
		t.Error("a new client inherited the default's offline mode")
	}
}

// TestScopeRestoresTheClientItCameFrom keeps Capture/Restore from becoming a
// way to copy one client's configuration onto another.
func TestScopeRestoresTheClientItCameFrom(t *testing.T) {
	t.Parallel()

	a := NewClient()
	b := NewClient()

	snapshot := a.Capture(SIMBAD)

	a.Disable(SIMBAD)
	b.Disable(SIMBAD)

	snapshot.Restore()

	if _, err := a.URL(SIMBAD); err != nil {
		t.Errorf("a: URL after its own Restore = %v, want it re-enabled", err)
	}

	if _, err := b.URL(SIMBAD); !errors.Is(err, ErrEndpointDisabled) {
		t.Error("b was restored by a snapshot taken from a")
	}
}

// TestZeroScopeRestoreIsHarmless covers the value a caller can trivially
// produce — var s remote.Scope — and which must not panic on a nil client.
func TestZeroScopeRestoreIsHarmless(t *testing.T) {
	t.Parallel()

	var s Scope

	s.Restore()
}
