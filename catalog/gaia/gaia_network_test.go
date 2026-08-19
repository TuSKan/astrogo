//go:build network

package gaia

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// requireGaia skips the test when the Gaia TAP endpoint is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright.
func requireGaia(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "gea.esac.esa.int:443")
}

// skipIfUnresponsive turns a timed-out query into a skip. The TCP
// pre-check above is not sufficient on its own: ESA's TAP front end
// routinely accepts the connection and then never answers, which is
// downtime rather than wrong data, and the policy is that only wrong data
// from a responsive endpoint fails.
func skipIfUnresponsive(t *testing.T, err error) {
	t.Helper()

	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		t.Skipf("Gaia TAP accepted the connection but did not answer, skipping live test: %v", err)
	}
}

func TestGaiaNetworkConeSearch(t *testing.T) {
	requireGaia(t)

	prov := New()

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
			skipIfUnresponsive(t, err)
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
