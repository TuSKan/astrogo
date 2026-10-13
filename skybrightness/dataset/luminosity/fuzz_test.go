package luminosity_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/luminosity"
)

// FuzzParse holds the luminous-efficiency reader to never panicking or
// hanging on whatever bytes it is handed (#746): CIE's CSVs since #740, or a
// tabulation a caller holds from elsewhere.
func FuzzParse(f *testing.F) {
	f.Add("360,0.0000039170000\n361,0.0000043935810\n362,0.0000049296040\n")
	f.Add("380, 0.0005890000\n381, 0.0006650000\n\n")
	f.Add("500, 0.9\n499, 0.8\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = luminosity.Parse(strings.NewReader(s))
	})
}
