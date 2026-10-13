package starlight_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
)

// FuzzLoad holds the star-map reader to never panicking or hanging on
// whatever bytes it is handed (#746). The map is a download, and Load takes
// its band names from a header comment and its pixel indices from the rows,
// then scatters radiances into arrays sized by the pixel count.
func FuzzLoad(f *testing.F) {
	f.Add(tableFor("V"))
	f.Add(tableFor(""))
	f.Add("# bands: V\n0 1.0\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		for _, frame := range []starlight.Frame{starlight.Galactic, starlight.ICRS} {
			_, _ = starlight.Load(strings.NewReader(s), frame)
		}
	})
}
