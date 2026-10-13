package starlight

import (
	"strings"
	"testing"
)

// FuzzParseHipparcos holds the bright-star reader to never panicking or
// hanging on whatever a VizieR TAP answer holds (#746). It finds its columns
// by header name and reads a position either in degrees or in sexagesimal.
func FuzzParseHipparcos(f *testing.F) {
	f.Add("HIP,RAICRS,DEICRS,Vmag,pmRA,pmDE,RAhms,DEdms\n" +
		"3,0.00500795,38.85928608,6.61,5.24,-2.91,00 00 01.20,+38 51 33.4\n" +
		"55203,,,3.79,-430.31,-587.29,11 18 10.90,+31 31 45.0\n" +
		"99999,10.0,20.0,,1.0,1.0,00 40 00.00,+20 00 00.0\n")
	f.Add("RAICRS,Vmag\n1.5,2.5\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _, _, _ = parseHipparcos(strings.NewReader(s))
	})
}

// FuzzParseCache holds the partial-map cache reader to never panicking or
// hanging (#746). The cache is a file on the user's disk, and each row names
// the pixel it writes; this is where an index from the file meets a slice.
func FuzzParseCache(f *testing.F) {
	f.Add("# bands: V\n7 1.25e-09\nbroken\n9 not-a-number\n1e9 5\n")
	f.Add("# bands: V\n3 2.5e-09 41\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		values, counts := make([]float64, 12), make([]int64, 12)
		_ = parseCache(strings.NewReader(s), values, counts)
		_ = parseCache(strings.NewReader(s), values, nil)
	})
}
