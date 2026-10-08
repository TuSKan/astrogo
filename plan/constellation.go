package plan

import (
	"fmt"
	"strings"

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
//
// The target is named for the constellation matched, wherever its point
// falls. Two points fall outside their constellation (see
// constellation.Centroid): Eridanus's in Fornax, which it winds around, and
// Serpens's in Ophiuchus, which splits it in two. Naming the target from
// the constellation its point fell in called those two Fornax and Ophiuchus
// (#640).
func NewConstellation(name string) (*Constellation, error) {
	pos, err := constellation.Centroid(name)
	if err != nil {
		return nil, fmt.Errorf("plan: constellation %q: %w", name, err)
	}

	want := strings.ToLower(strings.ReplaceAll(name, " ", ""))

	for _, c := range constellation.List() {
		if strings.ToLower(strings.ReplaceAll(c.Name, " ", "")) == want || strings.ToLower(c.Abbreviation) == want {
			return &Constellation{name: c.Name, abbr: c.Abbreviation, pos: pos}, nil
		}
	}

	// Centroid also answers to its catalog's keys for Serpens's two halves,
	// "SER1" and "SER2", which name no constellation.
	return nil, fmt.Errorf("plan: constellation %q: %w", name, constellation.ErrUnknownAbbreviation)
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
func (c *Constellation) GetDetails(ctx *coord.Context, over DetailOverrides) (*TargetDetails, error) {
	return computeDetails(c, ctx, over)
}
