//go:build network

package gaia

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// requireGaia skips the test when the default archive is unreachable - per
// this project's network test policy, a reachability failure must never fail
// CI outright.
//
// The host is derived from the endpoint rather than written down, so it
// follows [DefaultEndpoint] instead of having to be remembered alongside it.
func requireGaia(t *testing.T) {
	t.Helper()

	raw, err := remote.URL(DefaultEndpoint)
	if err != nil {
		// Offline mode and a disabled endpoint are choices the caller made, so
		// there is nothing here to verify and nothing wrong. Anything else —
		// an id missing from the registry above all — is this repository's own
		// mistake, and skipping past it would hide a broken endpoint table
		// behind a message about the archive.
		if errors.Is(err, remote.ErrOffline) || errors.Is(err, remote.ErrEndpointDisabled) {
			t.Skipf("%s is not resolvable: %v", DefaultEndpoint, err)
		}

		t.Fatalf("resolving %s: %v", DefaultEndpoint, err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("%s resolves to %q, which does not parse: %v", DefaultEndpoint, raw, err)
	}

	port := u.Port()
	if port == "" {
		port = "443"
	}

	testutil.RequireReachable(t, net.JoinHostPort(u.Hostname(), port))
}

func TestGaiaNetworkConeSearch(t *testing.T) {
	requireGaia(t)

	prov, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second) // ESA TAP can be slower
	defer cancel()

	// Tap the Pleiades core
	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(56.75), angle.Deg(24.116)),
		Radius: angle.Deg(0.05),
		Limit:  5,
	}

	iter := prov.ConeSearch(ctx, req)

	var targets []resolve.Target

	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			testutil.SkipOnUpstreamFailure(t, err)
			t.Fatalf("Live network failed: %v", err)
		}

		targets = append(targets, tar)

		return true
	})

	if len(targets) == 0 {
		t.Fatalf("Expected stars from Gaia DR3 at Pleiades")
	}

	if !targets[0].HasCoord {
		t.Fatalf("Expected astremetry mapped to coordinates from Gaia")
	}
}

// A cone holding more sources than the limit returns the nearest of them,
// nearest first. Before #605 the query had no ORDER BY, and a TAP service
// returns the first rows it reaches: a capped cone came back as an arbitrary
// subset, as likely from the edge as from the center. The Trapezium sits in
// one of the densest patches of the Orion Nebula, so the five nearest of a
// 0.2° cone lie close to its center, far inside the edge.
func TestGaiaConeSearchReturnsTheNearestSources(t *testing.T) {
	requireGaia(t)

	prov, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	center := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911))
	radius := angle.Deg(0.2)

	var seps []float64

	prov.ConeSearch(ctx, resolve.ConeRequest{Center: center, Radius: radius, Limit: 5})(func(tar resolve.Target, err error) bool {
		if err != nil {
			testutil.SkipOnUpstreamFailure(t, err)
			t.Fatalf("ConeSearch: %v", err)
		}

		seps = append(seps, coord.Separation(center, tar.Coord).Degrees())

		return true
	})

	assertNearestFirst(t, seps, 5, radius.Degrees())
}

// assertNearestFirst checks a capped cone search returned want sources in
// order of distance, all well inside the cone: an arbitrary subset of a cone
// lies mostly toward its edge, where most of its area is.
func assertNearestFirst(t *testing.T, seps []float64, want int, radiusDeg float64) {
	t.Helper()

	if len(seps) != want {
		t.Fatalf("got %d sources, want %d", len(seps), want)
	}

	for i := 1; i < len(seps); i++ {
		if seps[i] < seps[i-1]-1e-9 {
			t.Errorf("source %d is %.5f° from the center, nearer than source %d at %.5f°; "+
				"the result should be nearest first", i, seps[i], i-1, seps[i-1])
		}
	}

	if far := seps[len(seps)-1]; far > radiusDeg/4 {
		t.Errorf("the farthest of the %d nearest sources is %.4f° out, in a %.2f° cone; "+
			"that is a subset of the cone, not its nearest sources", want, far, radiusDeg)
	}

	t.Logf("distances from the center: %.5f°", seps)
}
