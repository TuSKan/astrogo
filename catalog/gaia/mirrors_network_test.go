//go:build network

package gaia

import (
	"context"
	"net"
	"net/url"
	"slices"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// TestGaiaArchiveMirrorsAnswerAsTheArchive holds the ESA archive's registered
// mirrors to what makes them mirrors: this package's own cone search, sent to
// each host alone, returns the same Gaia DR3 sources in the same order. The
// cone is the Trapezium's, dense enough that the five nearest depend on every
// row the host holds there. A host that cannot be reached is left out, and
// the test needs two to compare.
func TestGaiaArchiveMirrorsAnswerAsTheArchive(t *testing.T) {
	ep, ok := remote.Lookup(remote.GaiaTAP)
	if !ok {
		t.Fatalf("%s is not registered", remote.GaiaTAP)
	}

	if len(ep.Mirrors) == 0 {
		t.Fatalf("%s registers no mirrors to hold", remote.GaiaTAP)
	}

	req := resolve.ConeRequest{Center: coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911)), Radius: angle.Deg(0.2), Limit: 5}

	answers := map[string][]string{}

	for _, base := range append([]string{ep.URL}, ep.Mirrors...) {
		u, err := url.Parse(base)
		if err != nil {
			t.Fatalf("%q does not parse: %v", base, err)
		}

		if !testutil.Reachable(net.JoinHostPort(u.Hostname(), "443")) {
			t.Logf("%s is unreachable; left out", u.Hostname())
			continue
		}

		ids, err := coneIDs(t, base, req)
		if why, upstream := testutil.UpstreamFailure(err); upstream {
			t.Logf("%s is not serving (%s); left out", u.Hostname(), why)
			continue
		} else if err != nil {
			t.Fatalf("%s: %v", u.Hostname(), err)
		}

		answers[u.Hostname()] = ids
	}

	if len(answers) < 2 {
		t.Skipf("only %d of the archive's hosts answered; two are needed to compare: %v", len(answers), answers)
	}

	var (
		firstHost string
		first     []string
	)

	for host, ids := range answers {
		if first == nil {
			firstHost, first = host, ids

			continue
		}

		if !slices.Equal(ids, first) {
			t.Errorf("%s returned %q, %s returned %q: not the same answer", host, ids, firstHost, first)
		}
	}
}

// coneIDs sends req to the archive at base alone, through GaiaTAP pointed at
// it, and returns the sources' IDs in order.
func coneIDs(t *testing.T, base string, req resolve.ConeRequest) ([]string, error) {
	t.Helper()

	scope := remote.Capture(remote.GaiaTAP)
	defer scope.Restore()

	if err := remote.SetURL(remote.GaiaTAP, base); err != nil {
		t.Fatalf("SetURL(%s, %q): %v", remote.GaiaTAP, base, err)
	}

	p, err := New(remote.GaiaTAP)
	if err != nil {
		t.Fatalf("New(%s): %v", remote.GaiaTAP, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var ids []string

	for tgt, err := range p.ConeSearch(ctx, req) {
		if err != nil {
			return nil, err
		}

		ids = append(ids, tgt.ID)
	}

	return ids, nil
}
