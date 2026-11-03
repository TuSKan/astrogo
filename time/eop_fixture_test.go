package time_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestSyntheticFinalsParses keeps testutil.FinalsText and ParseFinals2000A in
// step: every row the generator writes is a record the parser keeps, over the
// range asked for, and a lookup inside it interpolates rather than failing.
func TestSyntheticFinalsParses(t *testing.T) {
	const (
		start = 60600.0
		days  = 400
	)

	table, err := time.ParseFinals2000A(strings.NewReader(testutil.FinalsText(start, days)))
	if err != nil {
		t.Fatal(err)
	}

	lo, hi := table.Coverage()
	if lo != start || hi != start+days-1 {
		t.Fatalf("coverage = [%v, %v], want [%v, %v]", lo, hi, start, start+days-1)
	}

	eop, err := table.EOP(start + 100.5)
	if err != nil {
		t.Fatal(err)
	}

	if eop.DUT1 == 0 || eop.XP == 0 || eop.YP == 0 {
		t.Errorf("EOP = %+v: a synthetic table with zero orientation would benchmark nothing", eop)
	}
}
