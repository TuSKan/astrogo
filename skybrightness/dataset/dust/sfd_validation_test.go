//go:build validation

package dust_test

import (
	"context"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/dust"
)

// The local all-sky map reproduces what IRSA answers for the same directions.
//
// This is the test the whole local path rests on. IRSA's service is
// authoritative and reads the same SFD data; the local reader adds a
// projection, an interpolation and a hemisphere choice, any of which could be
// wrong in a way that still returns plausible numbers everywhere. A mirrored
// longitude, a half-pixel offset or a swapped hemisphere all produce a map
// that looks like dust and is not this dust.
//
// It costs IRSA nothing. The comparison runs against directions already
// fetched and cached — the byproduct of an earlier all-sky run — so the
// service is not asked anything it has not already been asked.
func TestSFDMatchesIRSA(t *testing.T) {
	testutil.RequireReachable(t, "dataverse.harvard.edu:443")

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	// Two hemispheres of 64 MB, consent-gated like every other bulk fetch.
	remote.EnableDownloads(160<<20, remote.SFDDustMap)

	cached, err := dust.CachedDirections(ctx)
	if err != nil {
		t.Skipf("no cached IRSA answers to compare against: %v", err)
	}

	if len(cached) < 100 {
		t.Skipf("only %d cached IRSA answers; run an all-sky comparison first", len(cached))
	}

	sfd, err := dust.Open(ctx)
	if err != nil {
		t.Skipf("could not open the SFD map: %v", err)
	}

	var (
		ratios   []float64
		worst    float64
		worstDir string
		compared int
	)

	for _, c := range cached {
		if c.Intensity <= 0 {
			continue
		}

		got, err := sfd.IntensityAt(c.L, c.B)
		if err != nil {
			t.Fatalf("l=%.3f b=%.3f: %v", c.L.Degrees(), c.B.Degrees(), err)
		}

		ratio := got / c.Intensity
		ratios = append(ratios, ratio)
		compared++

		if d := math.Abs(ratio - 1); d > worst {
			worst = d
			worstDir = formatDirection(c.L, c.B, c.Intensity, got)
		}
	}

	if compared == 0 {
		t.Fatal("nothing to compare")
	}

	sort.Float64s(ratios)

	median := ratios[len(ratios)/2]
	p05, p95 := ratios[len(ratios)*5/100], ratios[len(ratios)*95/100]

	t.Logf("%d directions compared against IRSA", compared)
	t.Logf("  local/IRSA: median %.5f, 5th %.5f, 95th %.5f", median, p05, p95)
	t.Logf("  worst: %s", worstDir)

	// The two are not required to agree exactly and cannot. IRSA returns the
	// value of the pixel containing the position; this interpolates between
	// pixels, which is the better answer for a map consulted at arbitrary
	// directions and is a different one. Over a 2.37 arcminute pixel the
	// intensity can change by a few per cent in the plane.
	//
	// What a projection error looks like is not a few per cent. A mirrored
	// longitude or a swapped hemisphere lands on unrelated sky and gives
	// ratios scattered over orders of magnitude, which is what these bounds
	// are set to catch.
	if math.Abs(median-1) > 0.02 {
		t.Errorf("the median ratio is %.5f; the local map is systematically off, which is "+
			"a projection or a scaling error rather than interpolation", median)
	}

	if p05 < 0.8 || p95 > 1.25 {
		t.Errorf("the middle ninety per cent spans %.4f to %.4f; interpolation across a "+
			"2.4 arcminute pixel does not do that", p05, p95)
	}
}

// formatDirection renders one comparison for a failure message.
func formatDirection(l, b angle.Angle, irsa, local float64) string {
	return fmt.Sprintf("l=%.3f b=%.3f: IRSA %.5g, local %.5g, ratio %.4f",
		l.Degrees(), b.Degrees(), irsa, local, local/irsa)
}
