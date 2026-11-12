package viirs_test

import (
	"context"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/skybrightness/dataset/viirs"
)

// FuzzOpen serves the VIIRS annual archive from fuzzed bytes and opens it as a
// caller does, through viirs.Open, so what the fuzzer decodes reaches the ZIP
// reader, the extraction into the cache, the GeoTIFF decoder and then this
// package's own use of the decoded composite: sampling it, and building ground
// emitters from it around a site (#1057).
//
// raster.FuzzOpen fuzzes the GeoTIFF decoder on its own; this is the chain
// around it. The property is the one every parser of someone else's bytes here
// is held to: it returns, and never panics.
func FuzzOpen(f *testing.F) {
	good := archive(f, compositeTIFF(pixels))

	f.Add(good)
	f.Add(archive(f, compositeTIFF([4]float64{math.NaN(), math.Inf(1), -1, 1e300})))
	f.Add(archive(f, []byte(errorPage)))
	f.Add(good[:len(good)/2])
	f.Add([]byte(errorPage))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, body []byte) {
		viirsCacheAndSource(t, body)

		r, closer, err := viirs.Open(context.Background(), healYear)
		if err != nil {
			return
		}

		defer func() { _ = closer.Close() }()

		for _, lat := range []float64{-89, -45, 0, 45, 89} {
			for _, lon := range []float64{-179, -90, 0, 90, 179} {
				_, _ = r.RadianceAt(angle.Deg(lon), angle.Deg(lat))
			}
		}

		_, _ = r.Emitters(viirs.Region{
			Site:          paranal(t),
			InnerM:        1_000,
			OuterM:        50_000,
			Sectors:       8,
			RadialSamples: 4,
			Spectrum:      topHatSpectrum(),
		})
	})
}
