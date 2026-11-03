//go:build network

package skybrightness_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
	"github.com/TuSKan/astrogo/time"
)

// TestScatteredMoonlightAgainstKS91WithCALSPEC is TestScatteredMoonlightAgainstKS91
// on the CALSPEC solar spectrum instead of the blackbody fixture, which is
// what makes it a measurement of the component rather than of the fixture.
// It is the comparison docs/VALIDATION.md quotes.
//
// On the ten rows inside ROLO's phase range the component reads 0.30 mag
// faint on average, with a scatter of 0.35 in the log of the ratio. The one
// row reading bright, 1990 December 28 with the Moon high, depends on that
// night's extinction, which was not measured; at the 0.116 measured on 1991
// January 4 it reads faint like the rest, and the mean is 0.41 mag. #938 is
// finding the cause. Until it lands, this asserts the same two-magnitude bound
// as the offline test, and the budget's 16-18% is asserted with the fix.
func TestScatteredMoonlightAgainstKS91WithCALSPEC(t *testing.T) {
	testutil.RequireReachable(t, "archive.stsci.edu:443")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// The file is a few megabytes; consent is still required.
	remote.EnableDownloads(64<<20, remote.CALSPEC)
	defer remote.EnableDownloads(0)

	m, err := solar.NewScatteredMoonlight(ctx)
	if err != nil {
		testutil.SkipOnUpstreamFailure(t, err)
		t.Fatalf("NewScatteredMoonlight: %v", err)
	}

	var sum, sumSq float64

	n := 0

	for _, o := range ks91Table1 {
		if !ks91InROLODomain(o) {
			continue
		}

		moon, _ := ks91Moonlight(t, m, o)

		dmag := -2.5 * math.Log10(moon/o.bMoon)

		t.Logf("%v: rho %v, Moon Z %v: %.0f nL against %.0f measured, ratio %.3f (%+.2f mag)",
			o.when, o.rho, o.moonZ, moon, o.bMoon, moon/o.bMoon, dmag)

		if math.Abs(dmag) > 2 {
			t.Errorf("%v: %.0f nL against %.0f measured, %+.2f mag", o.when, moon, o.bMoon, dmag)
		}

		l := math.Log(moon / o.bMoon)
		sum += l
		sumSq += l * l
		n++
	}

	mean := sum / float64(n)
	scatter := math.Sqrt((sumSq - float64(n)*mean*mean) / float64(n-1))

	t.Logf("%d rows: mean ratio %.3f (%+.2f mag), scatter %.2f in ln ratio",
		n, math.Exp(mean), -2.5*mean/math.Ln10, scatter)
}
