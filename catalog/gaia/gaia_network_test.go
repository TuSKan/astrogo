//go:build network

package gaia

import (
	"context"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
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
		t.Skipf("%s is not resolvable: %v", DefaultEndpoint, err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Skipf("%s has an unusable URL: %v", DefaultEndpoint, err)
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
