//go:build network || validation

package gaia

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// requireArchive skips the test when the archive id names is unreachable, and
// returns a control for testutil.SkipOnDegradedService against it: one row of
// gaia_source's source_id, which it has served for as long as it has served
// DR3.
//
// Either archive being down is downtime, not wrong data, and the policy here
// is that only wrong data from a responsive endpoint fails. ESA has been
// unreachable for a whole working day. Gaia@AIP answered every query with a
// 405 on 2026-11-23 (#1252), which only the control tells from a request of
// ours it rejects.
//
// The host is derived from the endpoint rather than written down, so it
// follows [DefaultEndpoint], or a URL set with remote.SetURL, instead of
// having to be remembered alongside it.
func requireArchive(t *testing.T, id remote.EndpointID) func() error {
	t.Helper()

	raw, err := remote.URL(id)
	if err != nil {
		// Offline mode and a disabled endpoint are choices the caller made, so
		// there is nothing here to verify and nothing wrong. Anything else —
		// an id missing from the registry above all — is this repository's own
		// mistake, and skipping past it would hide a broken endpoint table
		// behind a message about the archive.
		if errors.Is(err, remote.ErrOffline) || errors.Is(err, remote.ErrEndpointDisabled) {
			t.Skipf("%s is not resolvable: %v", id, err)
		}

		t.Fatalf("resolving %s: %v", id, err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("%s resolves to %q, which does not parse: %v", id, raw, err)
	}

	port := u.Port()
	if port == "" {
		port = "443"
	}

	testutil.RequireReachable(t, net.JoinHostPort(u.Hostname(), port))

	ep, _ := remote.Lookup(id)

	// Its deadline starts when it runs, not when the test did: a query can
	// take minutes to fail, and a control whose time had run out by then
	// would fail too and skip whatever the query met.
	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		return testutil.TAPControl(ctx, raw, "gaiadr3.gaia_source", "source_id", ep.Mirrors...)()
	}
}
