package testutil

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// TestFinalsTextColumns reads the rows back at the offsets time's
// ParseFinals2000A reads, so a change to either side is caught here and not by
// a benchmark that quietly stops finding its table.
func TestFinalsTextColumns(t *testing.T) {
	const (
		start = 60600.0
		days  = 5
	)

	lines := strings.Split(strings.TrimSuffix(FinalsText(start, days), "\n"), "\n")
	if len(lines) != days {
		t.Fatalf("got %d rows, want %d", len(lines), days)
	}

	field := func(line string, from, to int) float64 {
		t.Helper()

		v, err := strconv.ParseFloat(strings.TrimSpace(line[from:to]), 64)
		if err != nil {
			t.Fatalf("columns %d:%d of %q: %v", from, to, line, err)
		}

		return v
	}

	prevDUT1 := math.NaN()

	for i, line := range lines {
		if len(line) < 86 {
			t.Fatalf("row %d is %d columns, the reader needs 86", i, len(line))
		}

		if mjd := field(line, 7, 15); mjd != start+float64(i) {
			t.Errorf("row %d: MJD = %v, want %v", i, mjd, start+float64(i))
		}

		x, y, dut1, lod := field(line, 18, 27), field(line, 37, 46), field(line, 58, 68), field(line, 79, 86)

		if math.Abs(x) > 1 || math.Abs(y) > 1 || math.Abs(dut1) > 0.9 || math.Abs(lod) > 2 {
			t.Errorf("row %d: x=%v y=%v dut1=%v lod=%v are not Earth-sized", i, x, y, dut1, lod)
		}

		// A step near half a second would be read as a leap second.
		if !math.IsNaN(prevDUT1) && math.Abs(dut1-prevDUT1) > 0.01 {
			t.Errorf("row %d: DUT1 stepped by %v in a day", i, dut1-prevDUT1)
		}

		prevDUT1 = dut1
	}
}

func TestFinalsTextEmpty(t *testing.T) {
	if got := FinalsText(60600, 0); got != "" {
		t.Errorf("zero days gave %q, want nothing", got)
	}
}

// TestPutFieldRefusesAValueOfTheWrongWidth: a value too wide for its column
// would shift every column after it, so the generator panics and does not write
// a row the parser would read wrongly. FinalsText cannot reach it with any date
// it is asked for, so the guard is tested where it is.
func TestPutFieldRefusesAValueOfTheWrongWidth(t *testing.T) {
	row := []byte("..........")

	defer func() {
		if recover() == nil {
			t.Error("putField accepted a value wider than its column")
		}

		if string(row) != ".........." {
			t.Errorf("putField wrote %q before refusing", row)
		}
	}()

	putField(row, 0, 3, "1234")
}
