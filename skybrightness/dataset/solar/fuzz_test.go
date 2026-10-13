package solar_test

import (
	"bytes"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/solar"
)

// FuzzParse holds the solar-spectrum reader to never panicking or hanging on
// whatever bytes it is handed (#746). The FITS layer beneath it is fuzzed in
// its own right; what this adds is the column lookup and row decoding on top,
// which trust the table's declared shape.
func FuzzParse(f *testing.F) {
	f.Add(calspecLike(f, []float64{4000, 5000, 6000}, []float64{2.5, 4.0, 1.0}))
	f.Add(calspecLike(f, []float64{4000}, []float64{-1}))
	f.Add([]byte{})

	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _ = solar.Parse(bytes.NewReader(b))
	})
}
