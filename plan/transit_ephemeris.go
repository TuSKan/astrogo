package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// TransitEphemeris is the linear ephemeris of a periodic transit, a planet's
// across its star or one star's across another: the midpoint of cycle n
// arrives at the solar system barycenter at Epoch + n·Period. It is not a
// meridian transit, which [TransitEstimate] and an Observable's
// rise/transit/set give.
//
// Epoch is an instant at the barycenter, the BJD_TDB archives and ephemeris
// tables state: epoch.FromJD(bjd, epoch.TDB). A time measured at an observer,
// or a heliocentric one, is not an Epoch until it has been referred to the
// barycenter. [ObserveTransits] applies the barycentric correction once, from
// Epoch to the site, so an Epoch must not carry it already undone.
type TransitEphemeris struct {
	// Epoch is a reference midpoint, T0, as it arrives at the barycenter.
	Epoch epoch.Time

	// Period is the time between successive midpoints.
	Period unit.Duration

	// Duration is the time from first to fourth contact, T14.
	Duration unit.Duration

	// Uncertainty is the one-sigma uncertainty of Epoch and Period, or nil
	// for an ephemeris that states none, which every predicted transit then
	// reports through [TransitUncertaintyUnknown].
	Uncertainty *TimingUncertainty

	// TimingVariations marks transits known to depart from a constant period,
	// through orbital decay or another body's pull, so that a linear
	// prediction can miss by more than its own uncertainty says. Every
	// predicted transit reports it through [TransitTimingVariations].
	TimingVariations bool
}

// TimingUncertainty is the one-sigma uncertainty of a [TransitEphemeris].
//
// The midpoint of cycle n then has the variance
//
//	Var(Tn) = σ²(Epoch) + 2n·ρ·σ(Epoch)·σ(Period) + n²·σ²(Period)
//
// which is Var(T0) + 2n·Cov(T0, P) + n²·Var(P) with the covariance written as
// ρ·σ(Epoch)·σ(Period). A fit states the covariance; an ephemeris quoted as two
// uncertainties alone has Correlation zero, which assumes the two
// independent: an Epoch chosen near the middle of the fitted transits makes
// that nearly so, and one at an end of them does not.
type TimingUncertainty struct {
	// Epoch is the uncertainty of TransitEphemeris.Epoch.
	Epoch unit.Duration

	// Period is the uncertainty of TransitEphemeris.Period.
	Period unit.Duration

	// Correlation is the correlation of Epoch and Period, in [-1, 1].
	Correlation float64
}

// TransitWindow is how much time around a transit an observation needs.
type TransitWindow struct {
	// Before and After are the out-of-transit baseline needed before first
	// contact and after fourth.
	Before, After unit.Duration

	// Sigmas is the timing margin, in standard deviations of the predicted
	// midpoint: that much more time before first contact and after fourth,
	// so a transit early or late by up to that many sigmas is still covered.
	// It applies nothing to an ephemeris that states no uncertainty.
	Sigmas float64
}

// PredictedTransit is one predicted transit of a [TransitEphemeris], at the solar
// system barycenter.
type PredictedTransit struct {
	// Cycle is n, the number of periods from the ephemeris's Epoch, negative
	// before it.
	Cycle int

	// Start, Mid and End are first contact, the midpoint and fourth contact
	// as they arrive at the barycenter.
	Start, Mid, End epoch.Time

	// Sigma is the one-sigma uncertainty of Mid, zero when the ephemeris
	// states none.
	Sigma unit.Duration

	// Flags record what the prediction assumes or could not tell.
	Flags TransitFlags
}

// TransitFlags record what a [PredictedTransit]'s prediction assumes or could not
// tell. Test one with flags&TransitOverlapsNeighbor != 0.
type TransitFlags uint8

const (
	// TransitUncertaintyUnknown is a transit predicted from an ephemeris that
	// states no uncertainty: Sigma is zero, and no timing margin was applied,
	// though the prediction has an error all the same.
	TransitUncertaintyUnknown TransitFlags = 1 << iota

	// TransitOverlapsNeighbor is a transit whose window, the time an
	// observation of it needs, overlaps the previous or the next cycle's.
	TransitOverlapsNeighbor

	// TransitTimingVariations is a transit of an ephemeris marked as departing
	// from a constant period, which a linear prediction does not follow.
	TransitTimingVariations
)

// Transits returns the transits whose windows, the time need asks for around
// each, overlap w, in time order. A transit whose midpoint lies outside w is
// returned when its window reaches into it, and cycles before Epoch are
// negative. w and every time returned are at the barycenter; see
// [ObserveTransits] for times at a site.
//
// An ephemeris with a non-positive Period, a Duration not shorter than its
// Period, a negative uncertainty or a Correlation outside [-1, 1] is
// [ErrInvalidEphemeris]. A window with a negative baseline or margin, or whose
// margin grows by at least a period every cycle, is [ErrInvalidTransitWindow].
func (e TransitEphemeris) Transits(w Window, need TransitWindow) ([]PredictedTransit, error) {
	if err := e.validate(); err != nil {
		return nil, err
	}

	if err := need.validate(e); err != nil {
		return nil, err
	}

	if err := checkInterval(w.Start, w.End); err != nil {
		return nil, err
	}

	from, to := w.Start.Sub(e.Epoch).Seconds(), w.End.Sub(e.Epoch).Seconds()
	period := e.Period.Seconds()

	// Cycle numbers past 2^53 are not integers a float64 can step through.
	if math.Abs(from/period) > 1<<53 || math.Abs(to/period) > 1<<53 {
		return nil, fmt.Errorf("%w: the interval lies more than 2^53 periods from the epoch", ErrInvalidTransitWindow)
	}

	// Down from the cycle at or before the interval's start to the first
	// whose window ends before it. Each step down moves a window's end at
	// least Period less the margin's growth per cycle, which validate keeps
	// positive, so this ends.
	n := int(math.Floor(from / period))
	for e.windowEnd(n, need) >= from {
		n--
	}

	// From the cycle above that one every window ends at or after the
	// interval's start, since each later one ends later still, so each
	// starting by its end overlaps it.
	var out []PredictedTransit

	for n++; e.windowStart(n, need) <= to; n++ {
		out = append(out, e.transit(n, need))
	}

	return out, nil
}

// transit is cycle n, with its flags.
func (e TransitEphemeris) transit(n int, need TransitWindow) PredictedTransit {
	mid := e.Epoch.Add(unit.Seconds(float64(n) * e.Period.Seconds()))
	half := e.Duration / 2

	tr := PredictedTransit{
		Cycle: n,
		Start: mid.Add(-half),
		Mid:   mid,
		End:   mid.Add(half),
		Sigma: e.sigma(n),
	}

	if e.Uncertainty == nil {
		tr.Flags |= TransitUncertaintyUnknown
	}

	if e.TimingVariations {
		tr.Flags |= TransitTimingVariations
	}

	if e.windowEnd(n, need) > e.windowStart(n+1, need) || e.windowStart(n, need) < e.windowEnd(n-1, need) {
		tr.Flags |= TransitOverlapsNeighbor
	}

	return tr
}

// sigma is the one-sigma uncertainty of cycle n's midpoint, from
// TimingUncertainty's variance.
func (e TransitEphemeris) sigma(n int) unit.Duration {
	u := e.Uncertainty
	if u == nil {
		return 0
	}

	s0, sp, k := u.Epoch.Seconds(), u.Period.Seconds(), float64(n)

	// Never negative with |Correlation| <= 1, since it is at least
	// (s0 - |k|·sp)²; the clamp keeps rounding from making it so.
	variance := max(0, s0*s0+2*k*u.Correlation*s0*sp+k*k*sp*sp)

	return unit.Seconds(math.Sqrt(variance))
}

// windowStart and windowEnd are where cycle n's window begins and ends, in
// seconds from Epoch: first and fourth contact, widened by the timing margin
// and the baselines.
func (e TransitEphemeris) windowStart(n int, need TransitWindow) float64 {
	return float64(n)*e.Period.Seconds() - e.Duration.Seconds()/2 -
		need.Sigmas*e.sigma(n).Seconds() - need.Before.Seconds()
}

func (e TransitEphemeris) windowEnd(n int, need TransitWindow) float64 {
	return float64(n)*e.Period.Seconds() + e.Duration.Seconds()/2 +
		need.Sigmas*e.sigma(n).Seconds() + need.After.Seconds()
}

// validate refuses an ephemeris no transit can be predicted from.
func (e TransitEphemeris) validate() error {
	switch {
	case !finiteTime(e.Epoch):
		return fmt.Errorf("%w: epoch %v is not a finite time", ErrInvalidEphemeris, e.Epoch)
	case !finite(e.Period.Seconds()) || e.Period <= 0:
		return fmt.Errorf("%w: period %v s is not positive", ErrInvalidEphemeris, e.Period.Seconds())
	case !finite(e.Duration.Seconds()) || e.Duration <= 0 || e.Duration >= e.Period:
		return fmt.Errorf("%w: duration %v s is not positive and shorter than the period",
			ErrInvalidEphemeris, e.Duration.Seconds())
	}

	u := e.Uncertainty
	if u == nil {
		return nil
	}

	switch {
	case !finite(u.Epoch.Seconds()) || u.Epoch < 0, !finite(u.Period.Seconds()) || u.Period < 0:
		return fmt.Errorf("%w: uncertainties %v s and %v s are not both non-negative",
			ErrInvalidEphemeris, u.Epoch.Seconds(), u.Period.Seconds())
	case !finite(u.Correlation) || math.Abs(u.Correlation) > 1:
		return fmt.Errorf("%w: correlation %v is outside [-1, 1]", ErrInvalidEphemeris, u.Correlation)
	}

	return nil
}

// validate refuses a window no observation can be planned with for e.
func (need TransitWindow) validate(e TransitEphemeris) error {
	switch {
	case !finite(need.Before.Seconds()) || need.Before < 0, !finite(need.After.Seconds()) || need.After < 0:
		return fmt.Errorf("%w: baselines %v s and %v s are not both non-negative",
			ErrInvalidTransitWindow, need.Before.Seconds(), need.After.Seconds())
	case !finite(need.Sigmas) || need.Sigmas < 0:
		return fmt.Errorf("%w: margin of %v sigmas is not non-negative", ErrInvalidTransitWindow, need.Sigmas)
	}

	// The margin grows by at most Sigmas·σ(Period) a cycle. At a period or
	// more, every window reaches every other, and no cycle is the first.
	if u := e.Uncertainty; u != nil && need.Sigmas*u.Period.Seconds() >= e.Period.Seconds() {
		return fmt.Errorf("%w: a margin of %v sigmas grows %v s a cycle, at least the %v s period",
			ErrInvalidTransitWindow, need.Sigmas, need.Sigmas*u.Period.Seconds(), e.Period.Seconds())
	}

	return nil
}
