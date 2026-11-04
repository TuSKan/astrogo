package time_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// TestEOPPublicPathRefusesNonFiniteJD walks the public API — DeltaT, Time.EOP
// and Time.UT1 — for a non-finite Julian Date with a real table registered, as
// the benchmarks pin one. The direct Table.EOP test beside it
// (time/internal/iers) covers the model; this one covers the routes a caller
// actually takes, where a NaN comparison used to slip past every range check
// into the past-bulletin hold: no panic, and no finite answer on any of the
// three (#963).
func TestEOPPublicPathRefusesNonFiniteJD(t *testing.T) {
	t.Cleanup(time.ResetEOP)

	table, err := time.ParseFinals2000A(strings.NewReader(sampleFinals2000AForGateway))
	if err != nil {
		t.Fatalf("ParseFinals2000A: %v", err)
	}

	time.RegisterModel(table)

	for label, jd := range map[string]float64{
		"NaN":  math.NaN(),
		"+Inf": math.Inf(1),
		"-Inf": math.Inf(-1),
	} {
		t.Run(label, func(t *testing.T) {
			tm := time.FromJD(jd, time.UTC)

			if dt := time.DeltaT(tm).Seconds(); !math.IsNaN(dt) {
				t.Errorf("DeltaT(FromJD(%v)) = %.9f s, want a non-finite value", jd, dt)
			}

			eop := tm.EOP()
			if !math.IsNaN(eop.DUT1) {
				t.Errorf("FromJD(%v).EOP().DUT1 = %.9f s, want a non-finite value", jd, eop.DUT1)
			}

			if ut1, err := tm.UT1(); err == nil {
				t.Errorf("FromJD(%v).UT1() = %v, nil error; want an error, not a finite conversion", jd, ut1)
			}
		})
	}
}
