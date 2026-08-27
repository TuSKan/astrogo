package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// fixedDepth is a SkyDepth that always reports the same limit.
//
// A stub rather than a real sky, because what is under test here is the
// constraint's logic — soft against hard, the horizon, a target with no
// photometry — and none of that is made more convincing by a radiance engine
// behind it. The engine has its own tests; this has the interface between
// them, which is one method wide precisely so it can be stubbed like this.
type fixedDepth float64

func (d fixedDepth) LimitingMagnitudeAt(_ time.Time, _, _ angle.Angle) (float64, error) {
	return float64(d), nil
}

// failingDepth reports that it cannot answer.
type failingDepth struct{}

var errDepth = errors.New("no depth")

func (failingDepth) LimitingMagnitudeAt(_ time.Time, _, _ angle.Angle) (float64, error) {
	return 0, errDepth
}

// skyConstraintFixture is a target well above the horizon at the evaluated
// instant, and the site and context to evaluate it in.
func skyConstraintFixture(t *testing.T, vmag float64) (*Star, *Site, *coord.Context) {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Paranal", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	when := time.Date(2026, time.March, 2, 5, 0, 0, 0, time.LocationUTC)

	// Declination matching the site's latitude puts the target near the
	// zenith when it transits, so the horizon branch is not what is being
	// exercised by accident.
	star := NewStar("target", angle.Hour(16.5), angle.Deg(-24.6272), WithStarMagnitude(vmag))

	ctx := coord.NewContext(when, site.Location(), site.Refraction())

	return star, site, ctx
}

// By default the constraint never rejects: it reports a margin and leaves the
// decision to a score.
//
// This is the behaviour that distinguishes sky brightness from an altitude
// limit. Half a magnitude of moonlight makes a target harder, not impossible,
// and a constraint that failed it outright would drop targets a real observer
// would happily take.
func TestLimitingMagnitudeIsSoftByDefault(t *testing.T) {
	t.Parallel()

	star, site, ctx := skyConstraintFixture(t, 12)

	// A sky three magnitudes shallower than the target needs.
	c := LimitingMagnitudeConstraint{Sky: fixedDepth(9)}

	got, err := c.CheckCtx(star, ctx.Time(), site, ctx)
	if err != nil {
		t.Fatalf("CheckCtx: %v", err)
	}

	if !got.Pass {
		t.Errorf("a soft constraint rejected a target: %v", got)
	}

	if math.Abs(got.Value-(-3)) > 1e-9 {
		t.Errorf("margin is %.4f, want -3", got.Value)
	}
}

// Boolean mode is a hard cutoff, and says by how much it failed.
func TestLimitingMagnitudeBooleanRejects(t *testing.T) {
	t.Parallel()

	star, site, ctx := skyConstraintFixture(t, 12)

	c := LimitingMagnitudeConstraint{Sky: fixedDepth(9), Boolean: true}

	got, err := c.CheckCtx(star, ctx.Time(), site, ctx)
	if err != nil {
		t.Fatalf("CheckCtx: %v", err)
	}

	if got.Pass {
		t.Error("a target three magnitudes too faint passed a hard cutoff")
	}

	if got.Reason == "" {
		t.Error("a rejection with no reason is not actionable")
	}

	// And a sky deep enough passes the same target.
	deep := LimitingMagnitudeConstraint{Sky: fixedDepth(15), Boolean: true}

	got, err = deep.CheckCtx(star, ctx.Time(), site, ctx)
	if err != nil {
		t.Fatalf("CheckCtx: %v", err)
	}

	if !got.Pass {
		t.Errorf("a sky three magnitudes deeper than needed still rejected: %v", got)
	}
}

// The score is monotonic in the margin and bounded in [0,1].
//
// Monotonicity is the property a scheduler depends on: a deeper sky must
// never lower a target's merit, or the optimiser is free to prefer a worse
// night.
func TestLimitingMagnitudeScoreIsMonotonic(t *testing.T) {
	t.Parallel()

	star, _, ctx := skyConstraintFixture(t, 12)

	prev := -1.0

	for _, depth := range []float64{6, 9, 11, 12, 13, 15, 20} {
		c := LimitingMagnitudeConstraint{Sky: fixedDepth(depth)}

		got, err := c.ScoreMultiplier(star, ctx)
		if err != nil {
			t.Fatalf("ScoreMultiplier: %v", err)
		}

		if got < 0 || got > 1 {
			t.Errorf("a sky reaching %g scores %.4f, outside [0,1]", depth, got)
		}

		if got <= prev {
			t.Errorf("a sky reaching %g scores %.4f, no better than the %.4f of the "+
				"shallower one", depth, got, prev)
		}

		prev = got
	}
}

// A target with no photometry imposes no requirement.
//
// The alternative would be a constraint that silently empties a target list:
// most deep-sky catalogues carry magnitudes, most do not carry them for every
// row, and rejecting the gaps would look like a very selective sky.
func TestLimitingMagnitudeIgnoresTargetsWithNoMagnitude(t *testing.T) {
	t.Parallel()

	_, site, ctx := skyConstraintFixture(t, 0)

	star := NewStar("unmeasured", angle.Hour(16.5), angle.Deg(-24.6272))

	c := LimitingMagnitudeConstraint{Sky: fixedDepth(6), Boolean: true}

	got, err := c.CheckCtx(star, ctx.Time(), site, ctx)
	if err != nil {
		t.Fatalf("CheckCtx: %v", err)
	}

	if !got.Pass {
		t.Errorf("a target with no magnitude was rejected by a magnitude cutoff: %v", got)
	}

	score, err := c.ScoreMultiplier(star, ctx)
	if err != nil {
		t.Fatalf("ScoreMultiplier: %v", err)
	}

	if score != 1 {
		t.Errorf("score is %.4f, want 1 — an unmeasured target is not evidence of a bad sky",
			score)
	}
}

// A below-horizon target scores zero rather than winning.
//
// The sky is not consulted at all there, and the limit is -Inf rather than
// some finite depth. Returning a finite number would let a target under the
// ground out-score a visible one whenever the visible one sat under moonlight.
func TestLimitingMagnitudeBelowHorizonScoresZero(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4045), angle.Deg(-24.6272), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Paranal", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	// The north celestial pole, permanently below the horizon at -24.6.
	star := NewStar("north pole", angle.Hour(0), angle.Deg(89.9), WithStarMagnitude(2))

	ctx := coord.NewContext(time.Date(2026, time.March, 2, 5, 0, 0, 0, time.LocationUTC),
		site.Location(), site.Refraction())

	c := LimitingMagnitudeConstraint{Sky: fixedDepth(20)}

	score, err := c.ScoreMultiplier(star, ctx)
	if err != nil {
		t.Fatalf("ScoreMultiplier: %v", err)
	}

	if score != 0 {
		t.Errorf("a target below the horizon scored %.4f under a very deep sky; there is no "+
			"sky in that direction to be deep", score)
	}
}

// A constraint with no SkyDepth says so rather than passing everything.
func TestLimitingMagnitudeRefusesWithoutASky(t *testing.T) {
	t.Parallel()

	star, site, ctx := skyConstraintFixture(t, 12)

	var c LimitingMagnitudeConstraint

	if _, err := c.CheckCtx(star, ctx.Time(), site, ctx); !errors.Is(err, ErrNoSkyDepth) {
		t.Errorf("got %v, want ErrNoSkyDepth — a zero-value constraint that silently passed "+
			"would be indistinguishable from a perfect sky", err)
	}
}

// A failing depth model surfaces its error rather than being swallowed.
func TestLimitingMagnitudePropagatesDepthErrors(t *testing.T) {
	t.Parallel()

	star, site, ctx := skyConstraintFixture(t, 12)

	c := LimitingMagnitudeConstraint{Sky: failingDepth{}}

	if _, err := c.CheckCtx(star, ctx.Time(), site, ctx); !errors.Is(err, errDepth) {
		t.Errorf("got %v, want the depth model's own error", err)
	}
}

// The custom Required hook overrides the target's own photometry.
func TestLimitingMagnitudeRequiredOverridesCatalogMagnitude(t *testing.T) {
	t.Parallel()

	star, site, ctx := skyConstraintFixture(t, 2)

	// The star is magnitude 2, but this caller wants 18 out of it.
	c := LimitingMagnitudeConstraint{
		Sky:      fixedDepth(10),
		Boolean:  true,
		Required: func(Observable) (float64, bool) { return 18, true },
	}

	got, err := c.CheckCtx(star, ctx.Time(), site, ctx)
	if err != nil {
		t.Fatalf("CheckCtx: %v", err)
	}

	if got.Pass {
		t.Error("Required was ignored: a magnitude-2 star passed a requirement of 18")
	}
}
