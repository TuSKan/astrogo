package airglow_test

import (
	"bytes"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
)

// FuzzParse holds the airglow-spectrum reader to never panicking or hanging
// on whatever bytes it is handed (#746). The FITS layer beneath it is fuzzed
// in its own right; what this adds is the column lookup by name and the row
// decoding on top, which trust the table's declared shape.
func FuzzParse(f *testing.F) {
	lam := []float64{500, 501, 502, 503}

	f.Add(bintableFITS(
		[]string{"lam", "flux_ael", "flux_arc", "trans"},
		[][]float64{lam, {1e3, 2e3, 0, 4e3}, {5e2, 5e2, 5e2, 5e2}, unity(len(lam))},
	))
	f.Add(bintableFITS([]string{"LAM", "FLUX_AEL"}, [][]float64{{500}, {1}}))
	f.Add([]byte{})

	f.Fuzz(func(_ *testing.T, b []byte) {
		_, _ = airglow.Parse(bytes.NewReader(b))
	})
}
