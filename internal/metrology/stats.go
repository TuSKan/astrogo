package metrology

import (
	"math"
	"sort"
)

// Sample is one comparison against a reference.
//
// Error is signed and expressed in the contract's units. Signed rather than
// absolute because the sign is evidence: a set of residuals scattered about
// zero says something different from the same magnitudes all pointing one
// way, and only the second is a bias worth chasing. [Stats] takes absolute
// values where it needs them and keeps the signed mean separately.
//
// Label and Context are what turn a number back into a scenario. Label names
// the case ("Mars @ Paranal"); Context carries the detail needed to reproduce
// it (epoch, geometry, dataset). A report that says only "max error 2.66
// arcsec" sends the next reader looking for the point that produced it; one
// that says where it was does not.
type Sample struct {
	Error   float64
	Label   string
	Context string
}

// Stats summarises a suite's residuals.
//
// Percentiles and Max are computed on |Error|, because that is what a
// contract bounds. MeanSigned, MinSigned and MaxSigned keep the signed
// distribution, because that is what reveals bias.
type Stats struct {
	// N is the number of samples that contributed. Samples rejected as
	// non-finite are not counted here; see Rejected.
	N int

	// Mean and RMS are over |Error|. RMS exceeds Mean whenever the
	// distribution has a tail, so the gap between them is itself a signal.
	Mean float64
	RMS  float64

	// Percentiles of |Error|, by linear interpolation between order
	// statistics (the definition numpy and R's type 7 use). With fewer than
	// two samples every percentile equals the single value.
	P50 float64
	P90 float64
	P95 float64
	P99 float64

	// Max is the largest |Error|; Worst is the sample that produced it.
	Max   float64
	Worst Sample

	// The signed distribution. MeanSigned near zero with a large Max says
	// scatter; MeanSigned near Max says bias.
	MeanSigned float64
	MinSigned  float64
	MaxSigned  float64

	// Exceeding counts samples outside the contract.
	Exceeding int

	// Rejected counts samples dropped as NaN or infinite.
	//
	// Dropped rather than propagated, because one NaN turns every aggregate
	// into NaN and destroys the evidence about the samples that were fine.
	// Counted rather than ignored, because a suite quietly discarding half
	// its input would otherwise report excellent statistics over whatever
	// survived.
	Rejected int
}

// Suite accumulates samples for one validation suite.
//
// Not safe for concurrent use. Neither of astrogo's statistical suites fans
// out today, and adding a mutex would not make the result deterministic
// anyway — with concurrent Add the insertion order, and so the tie-break on
// Worst, would vary between runs. Collect per worker and Add in a fixed order
// if that changes.
type Suite struct {
	Name      string
	Reference Reference
	Contract  Contract

	samples []Sample
}

// NewSuite starts a suite. Name is the key the result is reported and
// baselined under, so it must be stable across runs — a dotted path such as
// "ephemeris.observer_precision" rather than a sentence.
func NewSuite(name string, ref Reference, contract Contract) *Suite {
	return &Suite{Name: name, Reference: ref, Contract: contract}
}

// Add records one comparison.
func (s *Suite) Add(sample Sample) {
	s.samples = append(s.samples, sample)
}

// Len reports how many samples have been added, non-finite ones included.
func (s *Suite) Len() int { return len(s.samples) }

// Stats computes the summary. It does not modify the suite, so it is safe to
// call more than once.
func (s *Suite) Stats() Stats {
	var (
		out       Stats
		abs       = make([]float64, 0, len(s.samples))
		sum       float64
		sumSigned float64
		sumSq     float64
	)

	out.MinSigned = math.Inf(1)
	out.MaxSigned = math.Inf(-1)

	for _, sample := range s.samples {
		if math.IsNaN(sample.Error) || math.IsInf(sample.Error, 0) {
			out.Rejected++

			continue
		}

		a := math.Abs(sample.Error)

		abs = append(abs, a)
		sum += a
		sumSigned += sample.Error
		sumSq += a * a

		if a > out.Max {
			out.Max = a
			out.Worst = sample
		}

		out.MinSigned = math.Min(out.MinSigned, sample.Error)
		out.MaxSigned = math.Max(out.MaxSigned, sample.Error)

		if s.Contract.Max > 0 && a > s.Contract.Max {
			out.Exceeding++
		}
	}

	out.N = len(abs)
	if out.N == 0 {
		out.MinSigned, out.MaxSigned = 0, 0

		return out
	}

	n := float64(out.N)
	out.Mean = sum / n
	out.MeanSigned = sumSigned / n
	out.RMS = math.Sqrt(sumSq / n)

	sort.Float64s(abs)

	out.P50 = quantile(abs, 0.50)
	out.P90 = quantile(abs, 0.90)
	out.P95 = quantile(abs, 0.95)
	out.P99 = quantile(abs, 0.99)

	return out
}

// quantile interpolates linearly between order statistics of an
// already-sorted slice.
//
// The definition matters enough to name: this is the one numpy uses by
// default and R calls type 7, where the p-th quantile of n points sits at
// index p*(n-1). Nearest-rank would be defensible too, but the two disagree
// on small samples, and a percentile whose definition is unstated cannot be
// compared against anyone else's.
func quantile(sorted []float64, p float64) float64 {
	switch len(sorted) {
	case 0:
		return 0
	case 1:
		return sorted[0]
	}

	pos := p * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))

	if lo == hi {
		return sorted[lo]
	}

	frac := pos - float64(lo)

	return sorted[lo]*(1-frac) + sorted[hi]*frac
}
