package lsk_test

import (
	"io"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
)

// minimalLSK is the shape of NAIF's leap-second kernel: the DELTET constants
// and a DELTA_AT table running over continuation lines, inside a data block.
const minimalLSK = `\begindata

DELTET/DELTA_T_A       =   32.184
DELTET/K               =    1.657D-3
DELTET/EB              =    1.671D-2
DELTET/M               = (  6.239996D0   1.99096871D-7 )

DELTET/DELTA_AT        = ( 10,   @1972-JAN-1
                           11,   @1972-JUL-1
                           37,   @2017-JAN-1 )

\begintext
`

// FuzzNewReader holds the leap-second kernel reader to never panicking or
// hanging on whatever bytes it is handed (#746). The kernel is NAIF's,
// downloaded, and its DELTA_AT table is read across continuation lines with
// dates split on hyphens.
func FuzzNewReader(f *testing.F) {
	f.Add(minimalLSK)
	f.Add(strings.ReplaceAll(minimalLSK, "@1972-JAN-1", "@1972-JAN"))
	f.Add("DELTET/DELTA_AT = ( 10, @")
	f.Add("")

	f.Fuzz(func(_ *testing.T, s string) {
		_, _ = lsk.NewReader(io.NopCloser(strings.NewReader(s)))
	})
}
