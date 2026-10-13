package raster

import (
	"bytes"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// FuzzOpen holds the GeoTIFF reader to never panicking or hanging, whatever
// bytes it is handed. They come from someone else's server — the VIIRS
// composites are mirrored by www2.lightpollutionmap.info — and Open is
// exported, so a caller can hand it any file.
//
// The reader was not fuzzed until #744, and its first sixty seconds of fuzzing
// found a panic: a TileWidth with no TileLength indexed an empty value list.
// That input is checked in under testdata/fuzz/FuzzOpen.
//
// The seeds are the package's own synthetic GeoTIFFs, one per layout the reader
// decodes, so the fuzzer starts from files that reach every branch rather than
// from bytes that fail the header check.
func FuzzOpen(f *testing.F) {
	for _, s := range []synthTIFF{
		{width: 4, height: 3, pixels: rampPixels(4, 3, 5), originLon: -10, originLat: 40, pxSize: 0.5},
		{width: 4, height: 6, pixels: rampPixels(4, 6, 5), originLon: -10, originLat: 40, pxSize: 0.5, rowsPerStrip: 2},
		{
			width: 4, height: 4, pixels: rampPixels(4, 4, 5), originLon: -10, originLat: 40, pxSize: 0.5,
			tiled: true, tileW: 16, tileH: 16, deflate: true,
		},
		{width: 4, height: 3, pixels: rampPixels(4, 3, 5), originLon: -10, originLat: 40, pxSize: 0.5, lzw: true},
		{
			width: 4, height: 3, pixels: rampPixels(4, 3, 5), originLon: -10, originLat: 40, pxSize: 0.5,
			floatPredictor: true, deflate: true,
		},
	} {
		f.Add(s.build(f))
	}

	f.Add([]byte("II*\x00\x08\x00\x00\x00"))
	f.Add([]byte{})

	f.Fuzz(func(_ *testing.T, b []byte) {
		r, err := Open(bytes.NewReader(b), nil)
		if err != nil {
			return
		}

		// Sampling decodes a block, which is where offsets and byte counts
		// read from the file are trusted; the three points fall inside,
		// on the edge of and outside the seeds' extent.
		for _, ll := range [][2]float64{{-9.9, 39.9}, {-9, 39}, {170, -80}} {
			_, _ = r.SampleBilinear(angle.Deg(ll[0]), angle.Deg(ll[1]))
		}
	})
}
