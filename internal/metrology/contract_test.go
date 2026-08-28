package metrology_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

// Every field is mandatory, and each has its own sentinel so a test — or a
// person — can tell which rule was broken.
//
// The prose fields are the point. A tolerance with no stated reason cannot be
// reviewed and cannot be changed responsibly, so the safe move is always to
// leave it alone; that is how a bound demanding twice the accuracy its own
// reference documents survived in this repository until something printed the
// measurement beside it.
func TestNewContractRefusesAnUnjustifiedBound(t *testing.T) {
	t.Parallel()

	const (
		rationale = "2x the reference routine's published worst case"
		source    = "SOFA iauMoon98 note 3"
	)

	for _, tc := range []struct {
		name                     string
		max                      float64
		units, rationale, source string
		want                     error
	}{
		{"zero maximum", 0, "AU", rationale, source, metrology.ErrContractMax},
		{"negative maximum", -1, "AU", rationale, source, metrology.ErrContractMax},
		{"infinite maximum", math.Inf(1), "AU", rationale, source, metrology.ErrContractMax},
		{"NaN maximum", math.NaN(), "AU", rationale, source, metrology.ErrContractMax},
		{"no units", 1, "", rationale, source, metrology.ErrContractUnits},
		{"no rationale", 1, "AU", "", source, metrology.ErrContractRationale},
		{"no source", 1, "AU", rationale, "", metrology.ErrContractSource},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := metrology.NewContract(tc.max, tc.units, tc.rationale, tc.source)
			if !errors.Is(err, tc.want) {
				t.Errorf("NewContract error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestNewContractAcceptsAJustifiedBound(t *testing.T) {
	t.Parallel()

	c, err := metrology.NewContract(2.12e-7, "AU",
		"2x Moon98's published worst case against ELP/MPP02",
		"SOFA iauMoon98 documentation, note 3 (31.7 km, 1950-2100)")
	if err != nil {
		t.Fatalf("NewContract: %v", err)
	}

	// String is what lands in a report and in a failure message, so both the
	// number and the reason have to be in it — a bound whose justification
	// stays in the source file is not much better than one with none.
	got := c.String()
	for _, want := range []string{"2.12e-07", "AU", "Moon98", "ELP/MPP02", "31.7 km"} {
		if !strings.Contains(got, want) {
			t.Errorf("Contract.String() = %q, missing %q", got, want)
		}
	}
}

func TestMustContractPanicsOnAnInvalidBound(t *testing.T) {
	t.Parallel()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("MustContract accepted a contract with no rationale")
		}

		err, ok := r.(error)
		if !ok || !errors.Is(err, metrology.ErrContractRationale) {
			t.Errorf("panicked with %v, want %v", r, metrology.ErrContractRationale)
		}
	}()

	metrology.MustContract(1, "AU", "", "somewhere")
}
