package starlight

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// cappedArchive answers gaiaBrightPage the way the Gaia archive does: the
// sources with lo <= G < hi, at most limit of them, and truncated when there
// were more. Each source's right ascension is its index, so the test can tell
// which ones came back and how often.
func cappedArchive(g []float64, limit int, asked *int) gaiaBrightPage {
	return func(_ context.Context, lo, hi float64) ([]angle.Angle, []angle.Angle, bool, error) {
		*asked++

		var ra, dec []angle.Angle

		for i, m := range g {
			if m < lo || m >= hi {
				continue
			}

			if len(ra) == limit {
				return ra, dec, true, nil
			}

			ra = append(ra, angle.Deg(float64(i)))
			dec = append(dec, angle.Zero())
		}

		return ra, dec, false, nil
	}
}

// TestCollectGaiaBrightGathersPastTheCap: an archive that caps each answer
// below the number of sources still yields every source exactly once, by
// halving the magnitude interval of every answer that comes back truncated
// (#772). One query for all of them returned the first 50,000 of 177,426, and
// the Hipparcos stars matched against that reported 10,900 of themselves
// missing from Gaia instead of 74.
func TestCollectGaiaBrightGathersPastTheCap(t *testing.T) {
	t.Parallel()

	// 1,000 sources spread over G 1 to 9, crowded toward the faint end as
	// the sky is, against a cap of 64.
	g := make([]float64, 1000)
	for i := range g {
		f := float64(i) / float64(len(g))
		g[i] = 1 + 8*(1-(1-f)*(1-f))
	}

	asked := 0

	var ra, dec []angle.Angle

	err := collectGaiaBright(context.Background(), cappedArchive(g, 64, &asked), gaiaBrightestG, 9.5, 0, &ra, &dec)
	if err != nil {
		t.Fatalf("collectGaiaBright: %v", err)
	}

	seen := make([]int, len(g))
	for _, a := range ra {
		seen[int(a.Degrees()+0.5)]++
	}

	for i, n := range seen {
		if n != 1 {
			t.Errorf("source %d at G = %.4f came back %d times, want once", i, g[i], n)
		}
	}

	t.Logf("%d sources in %d queries against a cap of 64", len(ra), asked)
}

// TestCollectGaiaBrightRefusesWhatCannotFit: sources that no magnitude
// interval can separate, more of them than the cap at one G, are an error
// rather than a partial answer.
func TestCollectGaiaBrightRefusesWhatCannotFit(t *testing.T) {
	t.Parallel()

	g := make([]float64, 100)
	for i := range g {
		g[i] = 6.5
	}

	asked := 0

	var ra, dec []angle.Angle

	err := collectGaiaBright(context.Background(), cappedArchive(g, 10, &asked), gaiaBrightestG, 9, 0, &ra, &dec)
	if !errors.Is(err, ErrBrightStar) {
		t.Fatalf("collectGaiaBright = %v, want ErrBrightStar for an answer no interval completes", err)
	}

	if asked > 2*maxGaiaBrightSplits+2 {
		t.Errorf("%d queries before giving up, want the halving to stop at its depth", asked)
	}
}
