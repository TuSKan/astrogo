//go:build network

package starlight_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// The one test that catches a malformed query.
//
// Every other check here is a substring assertion, and substring assertions
// cannot tell valid ADQL from invalid. Two real defects got through them: a
// GROUP BY repeating the expression instead of naming the alias, and a CASE
// expression the archive rejects. Both produced an HTTP 400 and both were
// invisible until a query was actually sent.
func TestGaiaQueryIsAcceptedByTheArchive(t *testing.T) {
	t.Parallel()

	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// A few pixels only: enough to prove the archive parses and answers the
	// query this package generates, without aggregating a meaningful share of
	// 1.8 billion sources.
	build := starlight.GaiaBuild{Order: 8,
		Chunk: 4,
		Bands: []starlight.GaiaBand{{
			Name:           "V",
			ColourTerm:     []float64{0.02, 0.007, 0.17},
			FluxToRadiance: 1e-21,
		}},
	}

	adql, err := build.ADQL(100000, 100003)
	if err != nil {
		t.Fatalf("ADQL: %v", err)
	}

	t.Logf("query: %s", adql)

	m, counts, err := starlight.RunChunk(ctx, build, 100000, 100003)
	if err != nil {
		t.Fatalf("the archive rejected the generated query: %v", err)
	}

	var stars int64

	for _, c := range counts {
		stars += c
	}

	if stars == 0 {
		t.Error("four pixels of the Gaia catalogue held no sources at all")
	}

	t.Logf("four pixels held %d sources", stars)

	if m == nil {
		t.Fatal("no map returned")
	}
}

// Fetch is the path a user actually takes, so it gets run against the real
// archive: a scattered target list in one query, then the same list again to
// prove the cache answers without asking.
//
// The unit tests cover the query text and the cache bookkeeping. Neither can
// tell whether a several-hundred-pixel disjunction is something the archive
// will parse and answer in a reasonable time, which is the whole premise of
// fetching narrowly instead of building a sky.
func TestFetchAnswersATargetList(t *testing.T) {
	testutil.RequireReachable(t, "gea.esac.esa.int:443")

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	// Forty targets spread across the sky, the shape of a night's plan.
	dirs := make([]coord.ICRS, 0, 40)
	for i := range 40 {
		dirs = append(dirs, coord.NewICRS(
			angle.Deg(float64(i)*9), angle.Deg(float64(i%9)*10-40)))
	}

	spec := starlight.GaiaBuild{
		Order: 8,
		Bands: []starlight.GaiaBand{starlight.GaiaJohnsonV()},
	}

	start := time.Now()

	m, err := starlight.Fetch(ctx, spec, dirs...)
	if err != nil {
		// A congested archive is external downtime, which this repository's
		// policy says to skip on rather than fail. It answers this query in
		// three seconds when quiet and cancels its own SQL statement when not,
		// and neither says anything about the code under test.
		if archiveCongested(err) {
			t.Skipf("Gaia archive is not answering: %v", err)
		}

		t.Fatalf("Fetch: %v", err)
	}

	first := time.Since(start)

	var covered int

	for _, dir := range dirs {
		v, err := m.RadianceAt("V", dir.RA(), dir.Dec())
		if err != nil {
			t.Fatalf("RadianceAt: %v", err)
		}

		if v > 0 {
			covered++
		}
	}

	t.Logf("%d of %d directions covered in %v", covered, len(dirs), first.Round(time.Millisecond))

	if covered != len(dirs) {
		t.Errorf("only %d of %d directions came back with flux", covered, len(dirs))
	}

	// Again. Every pixel is cached now, so this must not touch the network —
	// which is what makes the second night on the same targets free.
	start = time.Now()

	if _, err := starlight.Fetch(ctx, spec, dirs...); err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	second := time.Since(start)
	t.Logf("cached repeat took %v", second.Round(time.Millisecond))

	if second > first/2 {
		t.Errorf("cached repeat took %v against a cold %v; the cache is not being used", second, first)
	}
}

// archiveCongested reports whether an error is the archive being too busy
// rather than the query being wrong.
//
// A malformed query comes back as an HTTP 400 with a parse message and must
// still fail the test; a statement timeout or a dead connection is the service
// having a bad day.
func archiveCongested(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	text := err.Error()
	for _, sign := range []string{"statement timeout", "500", "timeout", "EOF", "connection reset"} {
		if strings.Contains(text, sign) {
			return true
		}
	}

	return false
}
