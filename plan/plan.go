package plan

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"

	"github.com/TuSKan/astrogo/time"
)

// Observation pairs a Target with a specific observing time requirement.
type Observation struct {
	Target   Observable
	Duration time.Duration
}

// Slot pairs a coord.Object with an observing Window.
type Slot struct {
	Object Observable
	Window Window
}

// Planner evaluates coord.Objects against a set of Constraints at a given Site.
type Planner struct {
	Site        *Site
	Constraints []Constraint
}

// NewPlanner creates a new Planner for the given site and constraints.
func NewPlanner(site *Site, constraints []Constraint) (*Planner, error) {
	return &Planner{
		Site:        site,
		Constraints: constraints,
	}, nil
}

// Observable returns true if all constraints are satisfied for obj at time t.
func (p *Planner) Observable(obj Observable, t time.Time) (bool, error) {
	eval, err := IsObservable(obj, t, p.Site, p.Constraints...)
	if err != nil {
		return false, err
	}
	return eval.Observable, nil
}

// FilterObservable returns the subset of objects that satisfy all constraints
// at the given time. Objects are evaluated concurrently.
func (p *Planner) FilterObservable(objects []Observable, t time.Time) ([]Observable, error) {
	type indexedResult struct {
		idx int
		ok  bool
	}

	results := make([]indexedResult, len(objects))

	g := new(errgroup.Group)
	g.SetLimit(runtime.GOMAXPROCS(0))

	for i, obj := range objects {
		g.Go(func() error {
			ok, err := p.Observable(obj, t)
			if err != nil {
				return err
			}
			results[i] = indexedResult{idx: i, ok: ok}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var filtered []Observable
	for i, r := range results {
		if r.ok {
			filtered = append(filtered, objects[i])
		}
	}
	return filtered, nil
}

// RankedObject pairs an object with its observability score.
type RankedObject struct {
	Object Observable
	Score  float64 // e.g., peak altitude in degrees
}

// RankObservable ranks objects by their maximum altitude within the given
// time window. Only objects that satisfy constraints at least once in the
// window are included. Objects are evaluated concurrently.
func (p *Planner) RankObservable(objects []Observable, start, end time.Time) ([]RankedObject, error) {
	type indexedResult struct {
		obj   Observable
		score float64
		ok    bool
	}

	results := make([]indexedResult, len(objects))

	g := new(errgroup.Group)
	g.SetLimit(runtime.GOMAXPROCS(0))

	for i, obj := range objects {
		g.Go(func() error {
			// TransitEstimate expects coord.Object for now.
			skyObj, ok := obj.(coord.Object)
			if !ok {
				return fmt.Errorf("object %T does not implement coord.Object required for ranking", obj)
			}

			transitTime, peakAlt, err := TransitEstimate(skyObj, p.Site, start, end)
			if err != nil {
				return err
			}

			observable, err := p.Observable(obj, transitTime)
			if err != nil {
				return err
			}

			if observable {
				results[i] = indexedResult{obj: obj, score: peakAlt.Degrees(), ok: true}
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var ranked []RankedObject
	for _, r := range results {
		if r.ok {
			ranked = append(ranked, RankedObject{
				Object: r.obj,
				Score:  r.score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	return ranked, nil
}

// Evaluation represents the aggregated result of multiple constraint checks.
type Evaluation struct {
	// Observable is true if all evaluated constraints passed.
	Observable bool
	// Results contains the individual results for each
	Results []Result
	// Position is the ICRS position of the object at evaluation time.
	Position *coord.ICRS
	// AltAz is the locally observed horizontal coordinates at evaluation time.
	AltAz *coord.AltAz
}

// IsObservable evaluates all provided constraints against a target at a specific
// time and site. It returns an Evaluation containing the outcome and individual
// constraint results.
//
// When a constraint implements ConstraintCtx, the pre-built coord.Context is
// shared, avoiding redundant SOFA matrix computations (~91 µs each). This
// reduces cost from O(N) Context allocations to O(1) for N constraints.
//
// It only returns an error if a constraint check fails due to a technical error
// (e.g., ephemeris lookup failure), not if a constraint is simply not satisfied.
func IsObservable(
	obj Observable,
	t time.Time,
	site *Site,
	constraints ...Constraint,
) (Evaluation, error) {
	pos, err := obj.Position(t)
	if err != nil {
		return Evaluation{}, err
	}
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
	altAz, err := ctx.ICRSToAltAz(pos)
	if err != nil {
		return Evaluation{}, err
	}

	eval := Evaluation{
		Observable: true,
		Results:    make([]Result, 0, len(constraints)),
		Position:   pos,
		AltAz:      altAz,
	}

	for _, c := range constraints {
		var res Result
		if cc, ok := c.(ConstraintCtx); ok {
			res, err = cc.CheckCtx(obj, t, site, ctx)
		} else {
			res, err = c.Check(obj, t, site)
		}
		if err != nil {
			return Evaluation{}, err
		}
		eval.Results = append(eval.Results, res)
		if !res.Pass {
			eval.Observable = false
		}
	}

	return eval, nil
}

// ── Scoring ──────────────────────────────────────────────────────────────────

// ScoredTarget pairs an Observable with its calculated desirability score.
type ScoredTarget struct {
	Object Observable
	Score  float64
}

// Prioritized is an optional interface that targets can implement to provide
// a base priority for scoring.
type Prioritized interface {
	Priority() float64
}

// ScoreConfig controls the weights of the composite merit function used by
// ScoreObservable. All weights are normalized internally; they do not need
// to sum to 1.0.
type ScoreConfig struct {
	// AltitudeWeight controls the contribution of the target's current
	// altitude (normalized to 0–1 via alt/90°). Higher altitude means
	// lower airmass and better photometric conditions.
	// Default: 0.5
	AltitudeWeight float64

	// UrgencyWeight controls the contribution of time pressure.
	// Targets that are about to set receive a higher urgency score.
	// The urgency term is 1/max(hours_until_set, 0.5), capped at 1.0.
	// Default: 0.3
	UrgencyWeight float64

	// MoonWeight controls the contribution of lunar separation.
	// Targets far from the Moon score higher. The term ramps linearly
	// from 0 at 0° separation to 1.0 at MoonFullPenaltyDeg.
	// Default: 0.2
	MoonWeight float64

	// MoonFullPenaltyDeg is the separation (degrees) at which the Moon
	// penalty reaches zero. Below this angle, the penalty is proportional.
	// Default: 30.0
	MoonFullPenaltyDeg float64
}

// DefaultScoreConfig returns the default scoring weights.
func DefaultScoreConfig() ScoreConfig {
	return ScoreConfig{
		AltitudeWeight:     0.5,
		UrgencyWeight:      0.3,
		MoonWeight:         0.2,
		MoonFullPenaltyDeg: 30.0,
	}
}

// normalize ensures all weights are non-negative and computes the total
// for normalization. Returns (wAlt, wUrg, wMoon) normalized to sum to 1.0.
func (sc ScoreConfig) normalize() (wAlt, wUrg, wMoon float64) {
	a := math.Max(sc.AltitudeWeight, 0)
	u := math.Max(sc.UrgencyWeight, 0)
	m := math.Max(sc.MoonWeight, 0)

	total := a + u + m
	if total == 0 {
		// All zero — fall back to altitude-only
		return 1, 0, 0
	}
	return a / total, u / total, m / total
}

// moonSepCache stores the Moon's ICRS position for a given epoch to avoid
// redundant ephemeris lookups when scoring many targets at the same time.
var moonSepCache struct {
	mu   sync.Mutex
	time time.Time
	pos  *coord.ICRS
}

// getMoonPosition returns the Moon's ICRS coordinates, caching per-epoch.
func getMoonPosition(t time.Time) (*coord.ICRS, error) {
	moonSepCache.mu.Lock()
	defer moonSepCache.mu.Unlock()

	if moonSepCache.pos != nil && moonSepCache.time.Equal(t) {
		return moonSepCache.pos, nil
	}

	moon := NewTarget(catalog.Target{ID: "10", Name: "Moon", Kind: resolve.KindMoon}, eph.Default())
	pos, err := moon.Position(t)
	if err != nil {
		return nil, err
	}
	moonSepCache.time = t
	moonSepCache.pos = pos
	return pos, nil
}

// estimateHoursUntilSet computes a lightweight estimate of how many hours
// remain before the target sets below minAlt. It evaluates altitude at a
// small set of future offsets to avoid the cost of a full EventSolver call.
//
// Returns math.Inf(1) if the target is still above threshold at all probe
// points (circumpolar or very long visibility window).
func estimateHoursUntilSet(obj Observable, t time.Time, site *Site, ctx *coord.Context, currentAlt float64) float64 {
	// If already below horizon, urgency is maximum.
	if currentAlt <= 0 {
		return 0
	}

	// Probe at +30m, +1h, +2h, +4h, +8h — 5 evaluations total.
	probeOffsets := [5]time.Duration{
		30 * time.Minute,
		1 * time.Hour,
		2 * time.Hour,
		4 * time.Hour,
		8 * time.Hour,
	}

	_ = ctx // Not reused — each probe is a different epoch.

	for _, offset := range probeOffsets {
		ft := t.Add(offset)
		pos, err := obj.Position(ft)
		if err != nil {
			continue
		}
		fctx := coord.NewContext(ft, site.Location(), site.Atmosphere())
		aa, err := fctx.ICRSToAltAz(pos)
		if err != nil {
			continue
		}
		if aa.Alt().Degrees() <= 0 {
			// Target sets between previous probe and this one.
			// Linear interpolation for a rough estimate.
			hours := offset.Hours()
			return hours * (currentAlt / (currentAlt - aa.Alt().Degrees()))
		}
	}

	// Still up at +8h — not urgent at all.
	return math.Inf(1)
}

// ScoreObservable calculates a composite desirability score for a target at a
// given time and site using a configurable merit function.
//
// The composite score combines three factors:
//
//  1. Altitude merit:  alt / 90° (0–1), rewarding lower airmass.
//  2. Urgency merit:   1 / max(hours_until_set, 0.5), capped at 1.0.
//     Targets about to set are prioritized over those with hours of visibility.
//  3. Moon separation: min(separation / threshold, 1.0).
//     Targets far from the Moon score higher.
//
// The weighted composite is multiplied by the target's priority if it
// implements the Prioritized interface.
//
// If cfg is nil, DefaultScoreConfig() is used.
func ScoreObservable(
	obj Observable,
	t time.Time,
	site *Site,
	cfg *ScoreConfig,
	constraints ...Constraint,
) (float64, error) {
	eval, err := IsObservable(obj, t, site, constraints...)
	if err != nil {
		return 0, err
	}

	if !eval.Observable {
		return 0, nil
	}

	sc := DefaultScoreConfig()
	if cfg != nil {
		sc = *cfg
	}
	wAlt, wUrg, wMoon := sc.normalize()

	// ── Altitude merit (0–1) ────────────────────────────────────────────
	altDeg := eval.AltAz.Alt().Degrees()
	altMerit := math.Max(altDeg/90.0, 0)

	// ── Urgency merit (0–1) ─────────────────────────────────────────────
	var urgMerit float64
	if wUrg > 0 {
		ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
		hoursLeft := estimateHoursUntilSet(obj, t, site, ctx, altDeg)
		urgMerit = math.Min(1.0/(math.Max(hoursLeft, 0.5)), 1.0)
	}

	// ── Moon separation merit (0–1) ──────────────────────────────────────
	var moonMerit float64 = 1.0 // default: no penalty if Moon lookup fails
	if wMoon > 0 {
		moonPos, err := getMoonPosition(t)
		if err == nil {
			sep := coord.Separation(eval.Position, moonPos).Degrees()
			threshold := sc.MoonFullPenaltyDeg
			if threshold <= 0 {
				threshold = 30.0
			}
			moonMerit = math.Min(sep/threshold, 1.0)
		}
	}

	// ── Composite ────────────────────────────────────────────────────────
	score := wAlt*altMerit + wUrg*urgMerit + wMoon*moonMerit

	// Scale to a familiar 0–90 range (like the old altitude-based score)
	// so that priority multipliers remain interpretable.
	score *= 90.0

	// Apply priority if available.
	if p, ok := obj.(Prioritized); ok {
		score *= p.Priority()
	}

	return score, nil
}

// RankObservables evaluates and ranks a list of targets based on their
// composite observability score at a specific time and site.
// Objects are evaluated concurrently.
func RankObservables(
	objs []Observable,
	t time.Time,
	site *Site,
	constraints ...Constraint,
) ([]ScoredTarget, error) {
	type indexedScore struct {
		score float64
	}

	scores := make([]indexedScore, len(objs))

	g := new(errgroup.Group)
	g.SetLimit(runtime.GOMAXPROCS(0))

	for i, obj := range objs {
		g.Go(func() error {
			s, err := ScoreObservable(obj, t, site, nil, constraints...)
			if err != nil {
				return err
			}
			scores[i] = indexedScore{score: s}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var scored []ScoredTarget
	for i, s := range scores {
		if s.score > 0 {
			scored = append(scored, ScoredTarget{
				Object: objs[i],
				Score:  s.score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored, nil
}

// Window represents a contiguous time interval.
type Window struct {
	Start time.Time
	End   time.Time
}

// Duration returns the duration of the window as a standard time.Duration.
func (w Window) Duration() time.Duration {
	return w.End.Sub(w.Start)
}

// maxObservableStep is the maximum step size allowed for sampled observability
// searches. Steps larger than this risk silently missing short visibility
// windows and produce unreliable results.
const maxObservableStep = 15 * time.Minute

// ObservableWindows computes the time intervals where the target satisfies all
// provided constraints by sampling the range [start, end] at the given cadence.
//
// Transition boundaries are refined using binary search (sub-second precision),
// eliminating the ±step quantization error of pure grid search.
//
// The step must be positive and at most 15 minutes. Larger steps risk missing
// short visibility windows entirely and will return an error.
func ObservableWindows(
	obj Observable,
	start, end time.Time,
	step time.Duration,
	site *Site,
	constraints ...Constraint,
) ([]Window, error) {
	if step <= 0 {
		return nil, fmt.Errorf("step must be positive, got %v", step)
	}
	if step > maxObservableStep {
		return nil, fmt.Errorf("step %v exceeds maximum %v: large steps risk missing short visibility windows", step, maxObservableStep)
	}

	// Observability check function for bisection refinement.
	checkObs := func(t time.Time) bool {
		eval, err := IsObservable(obj, t, site, constraints...)
		if err != nil {
			return false
		}
		return eval.Observable
	}

	var windows []Window
	inWindow := false
	var windowStart time.Time
	var prevT time.Time
	hasPrev := false
	prevOK := false

	t := start
	for t.Before(end) || t.Equal(end) {
		eval, err := IsObservable(obj, t, site, constraints...)
		if err != nil {
			return nil, err
		}

		if eval.Observable && !inWindow {
			if hasPrev {
				windowStart = refineBisect(prevT, t, prevOK, checkObs)
			} else {
				windowStart = t
			}
			inWindow = true
		} else if !eval.Observable && inWindow {
			windowEnd := refineBisect(prevT, t, prevOK, checkObs)
			windows = append(windows, Window{
				Start: windowStart,
				End:   windowEnd,
			})
			inWindow = false
		}

		prevT = t
		prevOK = eval.Observable
		hasPrev = true
		t = t.Add(step)
	}

	// Close the final window if the target was observable at the end of the range.
	if inWindow {
		windows = append(windows, Window{
			Start: windowStart,
			End:   end,
		})
	}

	return windows, nil
}
