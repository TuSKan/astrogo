package iers

import (
	"errors"
	"math"
	"testing"
)

// A NaN MJD passes the range check on both sides and used to index one past
// the last record; a Julian Date of +/-inf reaches the table as a NaN MJD too.
func TestTableEOPRefusesNonFiniteMJD(t *testing.T) {
	table := &Table{records: []Record{{MJD: 60000, DUT1: 0.1, XP: 0.2, YP: 0.3, LOD: 1}}}
	for _, mjd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		eop, err := table.EOP(mjd)
		if !errors.Is(err, ErrOutOfRange) {
			t.Errorf("EOP(%v) returned err = %v, want ErrOutOfRange", mjd, err)
		}
		if eop != (EOP{}) {
			t.Errorf("EOP(%v) returned a value with an error", mjd)
		}
	}
}
