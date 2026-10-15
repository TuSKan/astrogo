package fits_test

import (
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// TestWCSIsSharedByConcurrentTransforms: goroutines transforming through one
// WCS at once get what one goroutine gets alone. The transforms used to write
// the WCS on every call, which the race detector reported as soon as two ran
// together (#797); CI's -race job is what holds that, and the comparison here
// holds the answers.
func TestWCSIsSharedByConcurrentTransforms(t *testing.T) {
	t.Parallel()

	w := fits.NewWCS(2)

	for _, err := range []error{
		w.SetCTYPE([]string{"RA---TAN-SIP", "DEC--TAN-SIP"}),
		w.SetCRVAL([]float64{150, 2}),
		w.SetCRPIX([]float64{1024, 1024}),
		w.SetCDELT([]float64{-2.8e-4, 2.8e-4}),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	w.SetSIP(map[[2]int]float64{{2, 0}: 1.2e-5}, map[[2]int]float64{{1, 1}: 5.6e-6})

	const n = 64

	want := make([][]float64, n)

	for i := range n {
		var err error
		if want[i], err = w.PixelToWorld([]float64{float64(32 * i), 7}); err != nil {
			t.Fatalf("PixelToWorld: %v", err)
		}
	}

	var wg sync.WaitGroup

	errs := make(chan string, 8*n)

	for range 8 {
		wg.Go(func() {
			for i := range n {
				world, err := w.PixelToWorld([]float64{float64(32 * i), 7})
				if err != nil || world[0] != want[i][0] || world[1] != want[i][1] {
					errs <- "PixelToWorld disagreed with a lone call"

					return
				}

				if _, err := w.WorldToPixel(world); err != nil {
					errs <- "WorldToPixel: " + err.Error()

					return
				}
			}
		})
	}

	wg.Wait()
	close(errs)

	for e := range errs {
		t.Error(e)
	}
}
