package testutil

import (
	"fmt"
	"math"
	"strings"
)

// finalsWidth is the width of a finals2000A row. The reader skips anything
// shorter than 86 columns; the real format runs to 188.
const finalsWidth = 100

// FinalsText is days daily rows of IERS finals2000A text, the first at
// startMJD, with Earth orientation that is synthetic but smooth and of the
// right size: polar motion of a few tenths of an arcsecond, UT1−UTC well inside
// a second, length of day under a millisecond, and no leap-second step.
//
// It exists so a benchmark can measure the code that runs when Earth
// orientation is present — the table lookup, the interpolation, the DUT1
// applied to every UT1 — without reading a cache that one machine has and
// another does not, and without the network. The values are not the Earth's:
// nothing that asserts an accuracy may use them.
//
// Hand the text to time.ParseFinals2000A and the result to time.RegisterModel.
// The columns are the ones that parser reads, and time's own tests parse this
// output to keep the two in step.
func FinalsText(startMJD float64, days int) string {
	var sb strings.Builder

	for d := range days {
		day := float64(d)
		chandler := 2 * math.Pi * day / 433
		annual := 2 * math.Pi * day / 365.25

		row := []byte(strings.Repeat(" ", finalsWidth))
		putField(row, 7, 15, fmt.Sprintf("%8.2f", startMJD+day))
		putField(row, 18, 27, fmt.Sprintf("%9.6f", 0.10+0.05*math.Sin(chandler)))
		putField(row, 37, 46, fmt.Sprintf("%9.6f", 0.30+0.05*math.Cos(chandler)))
		putField(row, 58, 68, fmt.Sprintf("%10.7f", 0.05+0.10*math.Sin(annual)))
		putField(row, 79, 86, fmt.Sprintf("%7.4f", 0.5+0.3*math.Sin(annual)))

		sb.Write(row)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// putField writes s over row[start:end]. A value too wide for its column would
// shift every column after it, so that is a bug in the generator, not data.
func putField(row []byte, start, end int, s string) {
	if len(s) != end-start {
		panic(fmt.Sprintf("testutil: finals2000A field %q is not %d columns wide", s, end-start))
	}

	copy(row[start:end], s)
}
