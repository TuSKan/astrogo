package metrology

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// DefaultRegressionFactor is how much a percentile may grow before it counts
// as a regression.
//
// Two, rather than something tighter, because these suites measure against
// live external services and reference data that moves. A percentile drifting
// by tens of per cent between runs is ordinary; one doubling is not, and is
// worth a human look even when the contract still holds.
const DefaultRegressionFactor = 2.0

// ErrDecodeBaseline marks a baseline document that cannot be read.
var ErrDecodeBaseline = errors.New("metrology: cannot decode baseline")

// Baseline is the accuracy a set of suites achieved at some accepted point,
// kept so a later run can notice getting worse while still inside contract.
//
// # Why a contract is not enough on its own
//
// Because a contract is a floor, and code can fall a long way without
// reaching it. A suite whose p99 goes from 0.15 to 0.62 arcseconds under a
// 1 arcsecond contract has got four times worse and still passes; nothing in
// the contract can say so, because nothing in the contract remembers what the
// number used to be. That is what this file remembers.
//
// Updating a baseline is deliberately a checked-in diff. A model change that
// legitimately moves the numbers should show up in review as exactly that,
// next to the change that caused it — never as a file some test rewrote on
// its way past a failure.
type Baseline struct {
	SchemaVersion int              `json:"schema_version"`
	Suites        map[string]Stats `json:"suites"`
}

// NewBaseline collects results into a baseline document.
func NewBaseline(results ...Result) Baseline {
	b := Baseline{SchemaVersion: SchemaVersion, Suites: make(map[string]Stats, len(results))}

	for _, r := range results {
		// A suite that did not run has no statistics to record, and
		// writing its zeroes would turn "we did not measure" into "we
		// measured zero error" — the strongest possible claim, made by
		// accident.
		if r.Status == StatusNotVerified {
			continue
		}

		b.Suites[r.Suite] = r.Stats
	}

	return b
}

// JSON renders the baseline for checking in.
func (b Baseline) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("metrology: encoding baseline: %w", err)
	}

	return append(data, '\n'), nil
}

// LoadBaseline decodes a baseline, refusing one written under a different
// schema.
func LoadBaseline(data []byte) (Baseline, error) {
	var b Baseline

	if err := json.Unmarshal(data, &b); err != nil {
		return Baseline{}, fmt.Errorf("%w: %w", ErrDecodeBaseline, err)
	}

	if b.SchemaVersion != SchemaVersion {
		return Baseline{}, fmt.Errorf("%w: document is version %d, this build reads version %d",
			ErrSchemaVersion, b.SchemaVersion, SchemaVersion)
	}

	return b, nil
}

// Regression is one percentile that grew beyond the allowed factor.
type Regression struct {
	Statistic string
	Was       float64
	Now       float64
	Factor    float64
}

// Compare returns the regressions in res against the baseline, worst first.
//
// A suite absent from the baseline returns nothing: there is no evidence it
// got worse, and inventing a comparison against zero would make every new
// suite look like an infinite regression on its first run.
//
// Percentiles that were zero in the baseline are skipped for the same reason
// — a ratio against zero is not a factor, and a suite that used to agree
// exactly is better served by the contract than by a growth check.
func (b Baseline) Compare(res Result, factor float64) []Regression {
	was, ok := b.Suites[res.Suite]
	if !ok || res.Status == StatusNotVerified {
		return nil
	}

	pairs := []struct {
		name     string
		was, now float64
	}{
		{"p50", was.P50, res.Stats.P50},
		{"p95", was.P95, res.Stats.P95},
		{"p99", was.P99, res.Stats.P99},
		{"max", was.Max, res.Stats.Max},
	}

	var out []Regression

	for _, p := range pairs {
		if p.was <= 0 {
			continue
		}

		if f := p.now / p.was; f > factor {
			out = append(out, Regression{Statistic: p.name, Was: p.was, Now: p.now, Factor: f})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Factor > out[j].Factor })

	return out
}

// Check compares res against the baseline and fails tb for any regression.
//
// It reports even when the contract still holds, which is the entire reason
// it exists; the message says so explicitly, so nobody reads the failure as a
// contract violation and reaches for the contract to fix it.
func (b Baseline) Check(tb TB, res Result, factor float64) {
	tb.Helper()

	regressions := b.Compare(res, factor)
	if len(regressions) == 0 {
		return
	}

	for _, r := range regressions {
		tb.Errorf("%s: %s regressed %.2gx — was %.4g %s, now %.4g %s.\n"+
			"  The contract (%.4g %s) still holds, so this is not a correctness failure; it is the "+
			"accuracy getting worse without anything saying so.\n"+
			"  Investigate first. Update the baseline only once the change is understood and intended.",
			res.Suite, r.Statistic, r.Factor,
			r.Was, res.Contract.Units, r.Now, res.Contract.Units,
			res.Contract.Max, res.Contract.Units)
	}
}
