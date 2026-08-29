package metrology

import (
	"errors"
	"fmt"
	"math"
)

// Errors returned by [NewContract]. They are sentinels so a test can assert
// which rule was broken rather than matching on a message.
var (
	// ErrContractMax marks a bound that is not a usable positive number.
	ErrContractMax = errors.New("metrology: contract maximum must be positive and finite")

	// ErrContractUnits marks a missing unit. A bare float64 in a report is
	// not a measurement.
	ErrContractUnits = errors.New("metrology: contract needs units")

	// ErrContractRationale marks a bound with no stated reason.
	ErrContractRationale = errors.New("metrology: contract needs a rationale")

	// ErrContractSource marks a bound with no citable origin.
	ErrContractSource = errors.New("metrology: contract needs a source")
)

// Contract is the accuracy bound a suite claims to satisfy, together with the
// reason it has the value it has.
//
// # Why the prose fields are mandatory
//
// Because a tolerance without a reason cannot be reviewed, and cannot be
// changed responsibly either. Faced with `Tolerance: 1e-6` a reader has no
// way to tell a physical limit from a number someone raised until the test
// went green, so the safe move is always to leave it alone — which is how
// unsound bounds survive for years.
//
// Rationale answers "why this number", Source answers "according to whom".
// Both are enforced at construction rather than checked in review, because
// review is exactly where they get skipped.
//
// A good pair reads like:
//
//	Max:       2.12e-7,
//	Units:     "AU",
//	Rationale: "2x Moon98's published worst case against ELP/MPP02",
//	Source:    "SOFA iauMoon98 documentation, note 3 (31.7 km, 1950-2100)",
//
// A bad one restates the number in words, or cites this repository's own last
// measurement — see the package doc for why that is circular.
type Contract struct {
	Max       float64
	Units     string
	Rationale string
	Source    string
}

// NewContract validates and returns a contract.
func NewContract(maximum float64, units, rationale, source string) (Contract, error) {
	switch {
	case !(maximum > 0) || math.IsInf(maximum, 0):
		return Contract{}, fmt.Errorf("%w: got %v", ErrContractMax, maximum)
	case units == "":
		return Contract{}, ErrContractUnits
	case rationale == "":
		return Contract{}, ErrContractRationale
	case source == "":
		return Contract{}, ErrContractSource
	}

	return Contract{Max: maximum, Units: units, Rationale: rationale, Source: source}, nil
}

// MustContract is [NewContract] for package-level declarations in tests,
// where an invalid contract is a programming error rather than a condition to
// handle. Follows the same convention as coord.MustGeodetic and
// time.MustLocation.
func MustContract(maximum float64, units, rationale, source string) Contract {
	c, err := NewContract(maximum, units, rationale, source)
	if err != nil {
		panic(err)
	}

	return c
}

// String renders the bound and its justification on one line.
func (c Contract) String() string {
	return fmt.Sprintf("%.4g %s (%s; %s)", c.Max, c.Units, c.Rationale, c.Source)
}
