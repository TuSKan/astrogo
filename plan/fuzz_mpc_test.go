package plan

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// FuzzParseMPCObsCodes holds the MPC observatory-code reader to never
// panicking or hanging on whatever the Minor Planet Center serves (#746). The
// file is fixed-column, and some rows run their fields together with no
// separator, so every field is sliced by column.
func FuzzParseMPCObsCodes(f *testing.F) {
	f.Add(mpcFixture)
	f.Add("<pre>\nCode  Long.   cos      sin    Name\n005   2.231000.659891\n</pre>")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = parseMPCObsCodes(strings.NewReader(s))
	})
}

// FuzzParseHorizonProfile holds the horizon-table reader to never panicking
// or hanging on whatever file a caller hands it, and the profile it returns
// to never panicking at any azimuth.
func FuzzParseHorizonProfile(f *testing.F) {
	f.Add("350 10\n10,20\n90\t20\n90 40\n180 5\n")
	f.Add("# comment\n; comment\n\n123 7.5\n")
	f.Add("90 91\n")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		p, err := ParseHorizonProfile(strings.NewReader(s))
		if err != nil {
			return
		}

		for _, az := range []float64{-720, -1, 0, 0.5, 90, 359.999, 360, 1e9} {
			_ = p(angle.Deg(az))
		}
	})
}
