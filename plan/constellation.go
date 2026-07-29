package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Constellation is a fixed target at one of the 88 IAU constellations'
// boundary centroid (see constellation.Centroid for what that means and
// its documented limitations) — a coarse "point roughly this way" target,
// not a precise catalog position. Implements Observable only, like Star/
// DeepSkyObject: a constellation has no ephemeris and no magnitude.
type Constellation struct {
	name string
	abbr string
	pos  coord.ICRS
}

// NewConstellation looks up name (its full IAU name or 3-letter
// abbreviation, case/space-insensitive — e.g. "Orion" or "Ori") and
// builds a *Constellation at its boundary centroid, or returns
// constellation.ErrUnknownAbbreviation.
func NewConstellation(name string) (*Constellation, error) {
	pos, err := constellation.Centroid(name)
	if err != nil {
		return nil, fmt.Errorf("plan: constellation %q: %w", name, err)
	}

	full, abbr, err := constellation.Lookup(pos)
	if err != nil {
		// Only reachable for one of Centroid's own documented exceptions
		// (see constellation.Centroid) where the vertex-average centroid
		// falls outside the constellation's own boundary — fall back to
		// the caller-supplied name/no abbreviation rather than fail
		// outright, since the position itself is still a reasonable
		// "point roughly this way" answer.
		return &Constellation{name: name, pos: pos}, nil //nolint:nilerr // documented fallback, not a swallowed error
	}

	return &Constellation{name: full, abbr: abbr, pos: pos}, nil
}

// Name returns the constellation's full IAU name.
func (c *Constellation) Name() string { return c.name }

// Abbreviation returns the constellation's standard 3-letter IAU
// abbreviation.
func (c *Constellation) Abbreviation() string { return c.abbr }

// Position returns the fixed boundary-centroid ICRS position
// (time-independent).
func (c *Constellation) Position(_ time.Time) (coord.ICRS, error) {
	return c.pos, nil
}

// GetDetails computes observational details using the given coordinate context.
func (c *Constellation) GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error) {
	return computeDetails(c, ctx, props...)
}
