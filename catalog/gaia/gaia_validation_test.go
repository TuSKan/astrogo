//go:build validation

package gaia

import (
	"context"
	"errors"
	"net"
	"net/url"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// The two archives return the same DR3.
//
// # Why ESA is still registered
//
// [DefaultEndpoint] is Gaia@AIP, on measured speed and availability. That
// choice is only defensible if the two archives actually agree, because a
// faster service returning different sources is not a mirror, it is a second
// catalogue wearing the same name. DR3 is a fixed data release — neither
// archive can be more current than the other — so any disagreement here is a
// service defect rather than an update, and worth failing over.
//
// This is what ESA is for in this package now: not the path a caller takes,
// but the independent answer the default is checked against.
func TestArchivesAgree(t *testing.T) {
	requireArchive(t, remote.GaiaAIP)
	requireArchive(t, remote.GaiaTAP)

	// The north galactic pole, and a field sized against the cap rather than
	// picked round. The provider's query is TOP N with no ORDER BY, so a
	// result that reaches the limit is an arbitrary subset and two services
	// may choose different ones — a difference in truncation rather than in
	// data, which would make this test compare noise. This field holds about
	// 340 sources against a limit of 2000, measured, so neither archive is
	// near truncating; a much smaller one returned two, which agrees trivially
	// and would go on agreeing however wrong an archive became.
	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(192.859), angle.Deg(27.128)),
		Radius: angle.Deg(0.2),
		Limit:  2000,
	}

	got := make(map[remote.EndpointID]map[string]coord.ICRS, 2)

	for _, id := range []remote.EndpointID{remote.GaiaAIP, remote.GaiaTAP} {
		p, err := New(id)
		if err != nil {
			t.Fatalf("New(%s): %v", id, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)

		found := make(map[string]coord.ICRS)

		var iterErr error

		p.ConeSearch(ctx, req)(func(tar resolve.Target, err error) bool {
			if err != nil {
				iterErr = err

				return false
			}

			found[tar.ID] = tar.Coord

			return true
		})

		cancel()

		if iterErr != nil {
			// A front end that accepts the connection and then stops
			// answering is downtime, not wrong data. ESA's did this
			// routinely, which is most of why it is no longer the default.
			var netErr net.Error
			if errors.Is(iterErr, context.DeadlineExceeded) ||
				(errors.As(iterErr, &netErr) && netErr.Timeout()) {
				t.Skipf("%s accepted the connection but did not answer: %v", id, iterErr)
			}

			t.Fatalf("%s: cone search: %v", id, iterErr)
		}

		if len(found) >= req.Limit {
			t.Skipf("%s returned %d sources, at or above the %d cap; a truncated result is "+
				"an arbitrary subset and cannot be compared as a set", id, len(found), req.Limit)
		}

		t.Logf("%s: %d sources", id, len(found))

		got[id] = found
	}

	aip, esa := got[remote.GaiaAIP], got[remote.GaiaTAP]

	if len(aip) == 0 {
		t.Fatal("both archives returned nothing; this field should not be empty")
	}

	// Set equality first, because a missing source is the difference that
	// matters most: a caller gets no second chance at a source that was never
	// returned, whereas a shifted position is at least visible.
	for id := range aip {
		if _, ok := esa[id]; !ok {
			t.Errorf("Gaia DR3 %s is in AIP's answer and not ESA's", id)
		}
	}

	for id := range esa {
		if _, ok := aip[id]; !ok {
			t.Errorf("Gaia DR3 %s is in ESA's answer and not AIP's", id)
		}
	}

	// Positions next. Both read the same stored double, so this is not a
	// tolerance on astrometry — it is a check that neither service is rounding
	// or reprojecting on the way out. A milliarcsecond is far looser than that
	// and far tighter than any real difference would be.
	const tolMas = 1.0

	var worst float64

	var worstID string

	for id, a := range aip {
		e, ok := esa[id]
		if !ok {
			continue
		}

		sep := coord.Separation(a, e).Arcsec() * 1000

		if sep > worst {
			worst, worstID = sep, id
		}
	}

	if worst > tolMas {
		t.Errorf("Gaia DR3 %s sits %.3f mas apart between the archives, over the %.1f mas "+
			"they should agree to — both read the same stored position", worstID, worst, tolMas)
	}

	t.Logf("%d sources in both, worst positional difference %.4f mas", len(aip), worst)
}

// requireArchive skips when one archive is unreachable.
//
// Either being down is downtime, not wrong data, and the policy here is that
// only wrong data from a responsive endpoint fails. ESA in particular has been
// unreachable for a whole working day.
func requireArchive(t *testing.T, id remote.EndpointID) {
	t.Helper()

	raw, err := remote.URL(id)
	if err != nil {
		t.Skipf("%s is not resolvable: %v", id, err)
	}

	u, err := url.Parse(raw)
	if err != nil {
		t.Skipf("%s has an unusable URL: %v", id, err)
	}

	port := u.Port()
	if port == "" {
		port = "443"
	}

	testutil.RequireReachable(t, net.JoinHostPort(u.Hostname(), port))
}
