package resolve_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/catalog/simbad"
	"github.com/TuSKan/astrogo/remote"
)

// TestWithClientReachesTheProvider is the end-to-end half of #114.
//
// remote/client_isolation_test.go proves two Clients hold separate policies.
// That is necessary and not sufficient: the policy has to survive the trip
// through resolve.Option, api.WithRemote and the provider's own constructor to
// the request itself. Everything in between compiles whether or not it is
// wired up, so compiling proves nothing here.
//
// The check is a redirect rather than an offline flag because a redirect is
// positive evidence: the request has to arrive somewhere this test controls,
// which an unwired client cannot fake by simply failing.
func TestWithClientReachesTheProvider(t *testing.T) {
	// Not parallel: it reads remote.Default to prove the default is untouched.
	var got string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path

		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("\n"))
	}))
	defer srv.Close()

	c := remote.NewClient(remote.WithEndpointURL(remote.SIMBAD, srv.URL))

	p := simbad.New(resolve.WithClient(c))

	// The query is expected to fail — an empty body is not a VOTable — and
	// that is fine. What is being asserted is where the request went.
	_, _ = p.Resolve(t.Context(), "Vega")

	if got == "" {
		t.Fatal("the provider never contacted the redirected endpoint.\n" +
			"  resolve.WithClient is meant to carry the policy through api.WithRemote to " +
			"the request; if the request went to the real SIMBAD instead, the option is " +
			"accepted and ignored, which is worse than not existing (#114).")
	}

	// And the default is untouched, which is what makes this a shape change
	// rather than a break for everyone else.
	if u, err := remote.URL(remote.SIMBAD); err != nil || u == srv.URL {
		t.Errorf("remote.Default's SIMBAD URL is now %q (err %v).\n"+
			"  A client-scoped override that leaks into the process default is the bug "+
			"this whole change exists to remove.", u, err)
	}
}

// TestProviderWithoutOptionsUsesTheDefault pins the compatibility direction.
//
// Every provider constructor here gained a variadic parameter. That is
// source-compatible by Go's rules, but it would be entirely possible to gain
// the parameter and change the behaviour of the niladic call at the same time
// — by resolving a client eagerly, say, and capturing the default at
// construction rather than at use.
func TestProviderWithoutOptionsUsesTheDefault(t *testing.T) {
	// Not parallel: it mutates remote.Default.
	t.Cleanup(remote.Capture().Restore)

	p := simbad.New()

	const mirror = "https://mirror.invalid/simbad/"

	// Set *after* construction: a provider that captured the default eagerly
	// would miss this, and a caller who configures remote in main() before
	// building providers elsewhere would silently get the wrong endpoint.
	if err := remote.SetURL(remote.SIMBAD, mirror); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	_, err := p.Resolve(t.Context(), "Vega")
	if err == nil {
		t.Fatal("resolving against an invalid host succeeded")
	}

	// It must have tried the mirror. A DNS failure for mirror.invalid is the
	// evidence; reaching the real SIMBAD would succeed or fail differently.
	if errors.Is(err, remote.ErrUnknownEndpoint) {
		t.Errorf("provider reported an unknown endpoint: %v", err)
	}
}
