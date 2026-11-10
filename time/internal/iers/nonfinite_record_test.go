package iers

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The column ranges ParseFinals2000A slices, as [lo, hi).
var (
	mjdField  = [2]int{7, 15}
	xField    = [2]int{18, 27}
	yField    = [2]int{37, 46}
	dut1Field = [2]int{58, 68}
	lodField  = [2]int{79, 86}
)

// finalsTable is n copies of realRecord on consecutive days from MJD 41684,
// with row bad's field replaced by value, right-aligned as the bulletin writes
// its numbers.
func finalsTable(n, bad int, field [2]int, value string) string {
	setField := func(line string, f [2]int, v string) string {
		return line[:f[0]] + fmt.Sprintf("%*s", f[1]-f[0], v) + line[f[1]:]
	}

	var b strings.Builder

	for i := range n {
		line := setField(realRecord, mjdField, fmt.Sprintf("%.2f", 41684.0+float64(i)))
		if i == bad {
			line = setField(line, field, value)
		}

		b.WriteString(line + "\n")
	}

	return b.String()
}

// A bulletin with a field that parsed but is not a number is refused, every
// field and every spelling strconv.ParseFloat reads. Each was served before:
// a NaN MJD in the last row made Table.EOP panic past the end of the data, a
// +Inf one stretched Coverage to infinity, and a non-finite DUT1, x or y came
// back as NaN or ±Inf with a nil error (#1037).
func TestParseRefusesANonFiniteField(t *testing.T) {
	t.Parallel()

	if table, err := ParseFinals2000A(strings.NewReader(finalsTable(10, 9, mjdField, "41693.00"))); err != nil ||
		len(table.records) != 10 {
		t.Fatalf("the finite bulletin: err %v, want 10 records", err)
	}

	for _, field := range []struct {
		name string
		at   [2]int
	}{
		{"MJD", mjdField}, {"x", xField}, {"y", yField}, {"UT1-UTC", dut1Field}, {"LOD", lodField},
	} {
		for _, value := range []string{"NaN", "Inf", "+Inf", "-Inf"} {
			// The last row, where a NaN MJD did its worst, and a middle one.
			for _, row := range []int{9, 4} {
				table, err := ParseFinals2000A(strings.NewReader(finalsTable(10, row, field.at, value)))
				if !errors.Is(err, ErrMalformed) || table != nil {
					t.Errorf("%s = %s in row %d: table %v, err %v, want ErrMalformed", field.name, value, row, table, err)
				}
			}
		}
	}
}

// The error names the line and the field, since the bulletin is a row a day
// since 1973 and the operator has to find the one.
func TestParseNamesTheMalformedLine(t *testing.T) {
	t.Parallel()

	_, err := ParseFinals2000A(strings.NewReader(finalsTable(10, 6, dut1Field, "NaN")))
	if err == nil || !strings.Contains(err.Error(), "line 7") || !strings.Contains(err.Error(), "UT1-UTC") {
		t.Errorf("err %v, want it to name line 7 and UT1-UTC", err)
	}
}
