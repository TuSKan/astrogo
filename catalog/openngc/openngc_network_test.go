//go:build network

package openngc

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
)

// The real catalogue, the two files remote.OpenNGC pins, resolved the way a
// caller would. Every other test in this package parses a fixture; this one
// is what found that 72 of OpenNGC's rows were dropped for a type code the
// parser did not know (#599). M24 and Brocchi's Cluster are two of them, and
// both are naked-eye objects.
func TestOpenNGCResolvesTheRealCatalogue(t *testing.T) {
	testutil.RequireReachable(t, "raw.githubusercontent.com:443")

	remote.EnableDownloads(16<<20, remote.OpenNGC)
	defer remote.DisableDownloads(remote.OpenNGC)

	p := New()

	cases := []struct{ query, id string }{
		{"M24", "IC4715"},
		{"Brocchi's Cluster", "CL399"},
		{"NGC1936", "NGC1936"},
		{"M42", "NGC1976"},
		{"M31", "NGC224"},
	}

	for _, c := range cases {
		got, err := p.Resolve(t.Context(), c.query)
		testutil.SkipOnUpstreamFailure(t, err)

		if err != nil {
			t.Errorf("Resolve(%q): %v", c.query, err)
			continue
		}

		if got.ID != c.id || !got.HasCoord {
			t.Errorf("Resolve(%q) = %s (has coordinates: %v), want %s", c.query, got.ID, got.HasCoord, c.id)
		}

		t.Logf("%-18s %-8s %-16s V %.2f", c.query, got.ID, got.Kind, got.VMag)
	}
}
