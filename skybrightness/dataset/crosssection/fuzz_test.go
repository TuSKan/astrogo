package crosssection_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/crosssection"
)

// FuzzParse holds the cross-section reader to never panicking or hanging on
// whatever bytes it is handed (#746). They are the MPI-Mainz UV/VIS Spectral
// Atlas's, two whitespace-separated columns with comments, read in each of
// the three wavelength units the atlas uses.
func FuzzParse(f *testing.F) {
	f.Add("# O3 absorption cross section\n300 1.5e-19\n310 1.0e-19\n320 5.0e-20\n")
	f.Add("4000 1e-23\n5000 2e-23\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		for _, u := range []crosssection.WavelengthUnit{
			crosssection.Nanometer, crosssection.Angstrom, crosssection.Wavenumber,
		} {
			_, _ = crosssection.Parse(strings.NewReader(s), "O3", u)
		}
	})
}
