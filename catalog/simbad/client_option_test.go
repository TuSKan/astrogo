package simbad_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TuSKan/astrogo/catalog/simbad"
	"github.com/TuSKan/astrogo/remote"
)

// TestWithClientReachesTheRequest is the end-to-end half of #114 for a catalog
// provider.
//
// remote's own tests prove two Clients hold separate policies. That is
// necessary and not sufficient: the policy has to survive the trip through this
// package's Option, api.WithRemote and the constructor to the request itself,
// and every step of that compiles whether or not it is wired up.
//
// The check is a redirect rather than an offline flag because a redirect is
// positive evidence — the request has to arrive somewhere this test controls,
// which an unwired client cannot fake by simply failing.
//
// # Why this test is here and not in catalog/resolve
//
// Because the option is here now. It used to be resolve.WithClient, which made
// configuring a provider require importing catalog/resolve — a package catalog
// re-exports precisely so a user never has to. A caller of simbad.New already
// imports simbad and should need nothing else.
func TestWithClientReachesTheRequest(t *testing.T) {
	// Not parallel: it reads remote.Default to prove the default is untouched.
	var got string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path

		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("\n"))
	}))
	defer srv.Close()

	c := remote.NewClient(remote.WithEndpointURL(remote.SIMBAD, srv.URL))

	p := simbad.New(simbad.WithClient(c))

	// Expected to fail — an empty body is not a VOTable — and that is fine.
	// What is asserted is where the request went.
	_, _ = p.Resolve(t.Context(), "Vega")

	if got == "" {
		t.Fatal("the provider never contacted the redirected endpoint.\n" +
			"  simbad.WithClient is meant to carry the policy through api.WithRemote to " +
			"the request; if it went to the real SIMBAD instead, the option is accepted " +
			"and ignored, which is worse than not existing (#114).")
	}

	if u, err := remote.URL(remote.SIMBAD); err != nil || u == srv.URL {
		t.Errorf("remote.Default's SIMBAD URL is now %q (err %v).\n"+
			"  A client-scoped override that leaks into the process default is the bug "+
			"this whole change exists to remove.", u, err)
	}
}

// TestNewWithoutOptionsUsesTheDefault pins the compatibility direction.
//
// New gained a variadic parameter, which leaves every existing simbad.New()
// call compiling. It would still be possible to change what that call *does* —
// by resolving a client eagerly and capturing the default at construction
// rather than at use, so a caller who configures remote in main() before
// building providers elsewhere silently gets the wrong endpoint.
func TestNewWithoutOptionsUsesTheDefault(t *testing.T) {
	// Not parallel: it mutates remote.Default.
	t.Cleanup(remote.Capture().Restore)

	p := simbad.New()

	const mirror = "https://mirror.invalid/simbad/"

	// Set after construction, deliberately.
	if err := remote.SetURL(remote.SIMBAD, mirror); err != nil {
		t.Fatalf("SetURL: %v", err)
	}

	if _, err := p.Resolve(t.Context(), "Vega"); err == nil {
		t.Fatal("resolving against an invalid host succeeded")
	}
}
