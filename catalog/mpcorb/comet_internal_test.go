package mpcorb

import (
	"errors"
	"strconv"
	"testing"
)

// TestCometPerihelionTimeRefusesAYearThatIsNotANumber holds the perihelion
// parser to its own refusal. Read never brings it such a row, because
// isCometRow turns away a row whose year columns do not parse before any field
// is read; the parser does not lean on that, and this keeps it so.
func TestCometPerihelionTimeRefusesAYearThatIsNotANumber(t *testing.T) {
	t.Parallel()

	const halleBopp = "    CJ95O010  1997 03 29.0319  0.925246  0.994897  130.7250  281.8047   89.7360  20260922  -2.0  4.0  C/1995 O1 (Hale-Bopp)"

	row := halleBopp[:cometTYearStart] + "19x7" + halleBopp[cometTYearEnd:]
	if isCometRow(row) {
		t.Fatalf("isCometRow admits a year of %q; the premise of this test is gone", row[cometTYearStart:cometTYearEnd])
	}

	at, err := cometPerihelionTime(row)
	if _, ok := errors.AsType[*strconv.NumError](err); !ok {
		t.Errorf("cometPerihelionTime(year 19x7) = %v, %v; want the year's parse error", at, err)
	}

	if _, err := cometPerihelionTime(halleBopp); err != nil {
		t.Errorf("cometPerihelionTime(Hale-Bopp) = %v, want its perihelion", err)
	}
}
