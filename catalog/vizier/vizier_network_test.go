//go:build network

package vizier

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

// failOrSkipOnOutage settles the calling test after
// coneSearchWithRetry has already exhausted every retry — reached only
// when every attempt (each against a freshly built Provider/*http.Client)
// still failed. Live-isolated this session, all the way down through raw
// net/http with byte-identical requests bypassing astrogo's Client
// entirely: the SAME syntactically-valid, independently-verified-correct
// ADQL query returns a genuine 503 from some requests and a bogus HTTP
// 400 "Incorrect ADQL query: 1 unresolved identifiers!" from others,
// within the same few minutes, against the same query text — this TAP
// endpoint's own backend infrastructure is presently unstable and
// produces more than one distinct external-failure shape, not just a
// clean 5xx. Since coneSearchWithRetry's exhaustion already means several
// independent connection attempts all failed, any error reaching this
// point used to be treated as the same "external downtime" class this
// project's network-test policy exempts from failing CI, on the grounds that
// a human running it locally still sees every attempt's real error via
// t.Logf and can investigate a suspiciously-consistent failure by hand.
//
// # Why it no longer skips on everything
//
// Because the bogus 400 is also what a genuinely broken query returns.
// "Incorrect ADQL query: N unresolved identifiers" is VizieR's answer to a
// column or table name it does not know — which is the regression this file
// exists to catch, and which fails identically on every backend. The retries
// above absorb a flaky node; skipping on whatever survives three fresh
// connections turned the one case they cannot absorb into a permanent pass.
//
// So the 503s are classified and skip, and anything that fails three times on
// three connections fails the test. That is the outcome the paragraph above
// asks for — a human looking at a suspiciously consistent failure — and a skip
// is the one outcome nobody ever looks at.
func failOrSkipOnOutage(ctx context.Context, t *testing.T, err error) {
	t.Helper()

	testutil.SkipOnDegradedService(t, err, vizierControl(ctx, t))
	t.Fatalf("VizieR TAP failed on every fresh-connection attempt: %v", err)
}

// vizierControl is a request a working VizieR cannot reject. A degraded VizieR
// answers every query with "400 1 unresolved identifiers", which reads as a
// malformed query of ours; it fails this one too, and a renamed column of ours
// does not (#492).
//
// Its own deadline rather than the caller's: by the time a control is wanted,
// the retries have spent most of that one.
func vizierControl(ctx context.Context, t *testing.T) func() error {
	t.Helper()

	syncURL, err := remote.URL(remote.VizieR)
	if err != nil {
		t.Fatalf("VizieR endpoint: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	t.Cleanup(cancel)

	return testutil.TAPControl(ctx, syncURL, `"I/239/hip_main"`, "HIP")
}

// requireVizier skips the test when the VizieR TAP endpoint is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright.
func requireVizier(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "tapvizier.u-strasbg.fr:80")
}

// coneSearchWithRetry runs req up to 3 times, retrying on ANY error with a
// short pause — not just a 5xx. Live-confirmed this session, isolated all
// the way down to raw net/http (bypassing astrogo's Client entirely): the
// exact same syntactically-valid ADQL query, sent with byte-identical
// headers/body, succeeds consistently from separate curl processes but
// fails consistently from a single Go *http.Client reusing one pooled TCP
// connection — VizieR's TAP endpoint sits behind a load balancer with at
// least one backend node returning a bogus "unresolved identifiers" 400
// (and others returning 503), and Go's connection reuse pins every retry
// to whichever node the first request landed on. A fresh Provider (and
// therefore a fresh *http.Client/*http.Transport with its own, new TCP
// connection) is built on every attempt specifically to escape that
// pinning, giving each retry a real chance at a different, healthy
// backend node — the same outcome separate curl processes get for free.
// A query that is GENUINELY broken fails the same way regardless of which
// backend answers, so this retry doesn't mask a real regression; it only
// absorbs the proven backend-routing flakiness. Only the final attempt's
// error, if every attempt failed, is handed to
// failOrSkipOnOutage.
func coneSearchWithRetry(ctx context.Context, t *testing.T, req resolve.ConeRequest) int {
	t.Helper()

	const attempts = 3

	var (
		count   int
		lastErr error
	)

	for attempt := range attempts {
		if attempt > 0 {
			time.Sleep(2 * time.Second)
		}

		count = 0
		lastErr = nil

		New().ConeSearch(ctx, req)(func(_ resolve.Target, err error) bool {
			if err != nil {
				lastErr = err

				return false
			}

			count++

			return true
		})

		if lastErr == nil {
			return count
		}

		t.Logf("attempt %d/%d failed: %v", attempt+1, attempts, lastErr)
	}

	failOrSkipOnOutage(ctx, t, lastErr)

	return count
}

func TestVizierNetworkConeSearch(t *testing.T) {
	requireVizier(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generic ConeSearch around M31 core
	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10.684), angle.Deg(41.269)),
		Radius: angle.Deg(0.01), // Very tight 36 arcseconds
		Limit:  10,
	}

	count := coneSearchWithRetry(ctx, t, req)

	// VizieR 2MASS should return sources inside a 36-arcsecond radius of
	// Andromeda's core; parseCSV now really parses the response (see R22 fix).
	if count == 0 {
		t.Error("expected at least one 2MASS source within 36 arcseconds of Andromeda's core")
	}

	if count > 10 {
		t.Fatalf("Expected limit to be respected")
	}
}

// TestVizierNetworkConeSearch_RegisteredTable confirms ConeSearch works
// live against a second registered table (Hipparcos), not just the default
// 2MASS one.
func TestVizierNetworkConeSearch_RegisteredTable(t *testing.T) {
	requireVizier(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A 2-degree cone around M31's core comfortably contains a Hipparcos star.
	req := resolve.ConeRequest{
		Table:  "I/239/hip_main",
		Center: coord.NewICRS(angle.Deg(10.684), angle.Deg(41.269)),
		Radius: angle.Deg(2),
		Limit:  5,
	}

	count := coneSearchWithRetry(ctx, t, req)

	if count == 0 {
		t.Error("expected at least one Hipparcos star within 2 degrees of Andromeda's core")
	}
}

// A cone holding more sources than the limit returns the nearest of them,
// nearest first. Before #605 the query had no ORDER BY: TOP 5 of a 5° cone
// around the Trapezium came back from 2° to 4.4° out, missing the Trapezium
// itself. Hipparcos holds the θ¹ and θ² Orionis stars a few arcminutes from
// the center.
func TestVizierConeSearchReturnsTheNearestSources(t *testing.T) {
	requireVizier(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	center := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911))
	radius := angle.Deg(5)

	var (
		seps    []float64
		lastErr error
	)

	New().ConeSearch(ctx, resolve.ConeRequest{Table: "I/239/hip_main", Center: center, Radius: radius, Limit: 5})(
		func(tar resolve.Target, err error) bool {
			if err != nil {
				lastErr = err
				return false
			}

			seps = append(seps, coord.Separation(center, tar.Coord).Degrees())

			return true
		})

	if lastErr != nil {
		failOrSkipOnOutage(ctx, t, lastErr)
	}

	if len(seps) != 5 {
		t.Fatalf("got %d sources, want 5", len(seps))
	}

	for i := 1; i < len(seps); i++ {
		if seps[i] < seps[i-1]-1e-9 {
			t.Errorf("source %d is %.4f° from the center, nearer than source %d at %.4f°", i, seps[i], i-1, seps[i-1])
		}
	}

	if far := seps[len(seps)-1]; far > 0.5 {
		t.Errorf("the farthest of the 5 nearest Hipparcos stars is %.3f° out, in a 5° cone; "+
			"that is a subset of the cone, not its nearest stars", far)
	}

	t.Logf("distances from the center: %.4f°", seps)
}
