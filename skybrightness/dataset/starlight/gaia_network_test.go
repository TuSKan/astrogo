//go:build network

package starlight_test

import (
	"context"
	"net"
	"testing"
	"time"

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

	//nolint:noctx // a reachability pre-check, not a request that should honour a deadline
	if c, err := net.DialTimeout("tcp", "gea.esac.esa.int:443", 5*time.Second); err != nil {
		t.Skipf("Gaia archive unreachable: %v", err)
	} else {
		_ = c.Close()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// A few pixels only: enough to prove the archive parses and answers the
	// query this package generates, without aggregating a meaningful share of
	// 1.8 billion sources.
	build := starlight.GaiaBuild{
		Order: 8,
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
