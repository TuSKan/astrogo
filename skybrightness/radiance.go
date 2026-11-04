package skybrightness

import (
	"context"
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// ErrDuplicateComponent is returned when two components claim the same ID.
var ErrDuplicateComponent = errors.New("skybrightness: duplicate component ID")

// ComponentID names a physically distinct contribution to the sky. The
// separation is physical, not presentational: each of these has its own
// literature, its own data, its own validity domain and its own
// uncertainty, so collapsing any two of them loses information a caller
// may need.
type ComponentID string

// The contributions the model distinguishes. Not all are implemented in
// any given phase; a Model reports which it actually has.
const (
	// Starlight is integrated light from resolved and unresolved stars.
	Starlight ComponentID = "starlight"

	// DiffuseGalactic is starlight scattered by interstellar dust.
	DiffuseGalactic ComponentID = "diffuse-galactic"

	// Extragalactic is the integrated light of unresolved galaxies.
	Extragalactic ComponentID = "extragalactic"

	// Zodiacal is sunlight scattered by interplanetary dust.
	Zodiacal ComponentID = "zodiacal"

	// AirglowContinuum is the chemiluminescent continuum of the upper
	// atmosphere.
	AirglowContinuum ComponentID = "airglow-continuum"

	// AirglowLines is the discrete emission-line component of airglow —
	// separate from the continuum because its variability, its spectral
	// structure and its solar-activity dependence all differ.
	AirglowLines ComponentID = "airglow-lines"

	// Moonlight is moonlight scattered into the line of sight.
	Moonlight ComponentID = "moonlight"

	// Twilight is scattered sunlight while the Sun is below the horizon
	// but still illuminating the atmosphere.
	Twilight ComponentID = "twilight"

	// Artificial is anthropogenic light scattered into the line of sight.
	Artificial ComponentID = "artificial"
)

// Component contributes one physically distinct term to the sky radiance.
//
// A component accumulates into dst rather than returning a new spectrum:
// a full-sky evaluation runs this on the order of 10^4 directions, and
// allocating a spectrum per component per direction would dominate the
// cost. dst is caller-owned and already sized for grid.
//
// A component must add *observer-level* radiance — the light arriving at
// the observer after atmospheric propagation — not a top-of-atmosphere
// emission. Summing unpropagated source terms is physically wrong, and the
// contract is stated here because the interface cannot enforce it.
//
// Implementations must be safe for concurrent use with distinct dst
// buffers, so a caller can evaluate directions in parallel.
type Component interface {
	// ID identifies the contribution.
	ID() ComponentID

	// AddRadiance accumulates this component's observer-level spectral
	// radiance for one viewing direction into dst, and reports what applies
	// to what it just computed: its quality flags and its uncertainty.
	//
	// Both are returned per call rather than fixed per component because
	// they depend on the scene: the same model is an interpolation in one
	// geometry and an extrapolation in another, and zodiacal light known to
	// 10 per cent in the bright ecliptic is known to 25 near the pole. The
	// uncertainty comes from the same call because what it depends on - a
	// dust column, a zodiacal brightness, an optical depth - exists only
	// while the radiance is being computed.
	AddRadiance(ctx context.Context, dst SpectralRadiance, grid unit.SpectralGrid, dir coord.AltAz, scene *Scene) (Contribution, error)

	// Provenance describes the model, its references and its validity
	// domain, so a result can be traced to the science behind it.
	Provenance() Provenance
}

// Contribution is what one [Component.AddRadiance] call reports about the
// radiance it added.
//
// The zero value is no flags and an unknown uncertainty. That default is the
// point: a component that says nothing about its uncertainty reports an
// unknown one, never a perfectly certain one, which is what the budget held
// before #911.
type Contribution struct {
	// Flags are the quality flags that apply to what was just computed.
	Flags Flag

	// Relative is the 1-sigma relative standard uncertainty on the radiance
	// just added, integrated over the grid. It is meaningful only when Known.
	Relative float64

	// Source names what dominates Relative, as [Uncertainty.Source] does.
	Source string

	// Known reports whether the component could establish Relative at all.
	Known bool
}

// checkComponents reports duplicate IDs in a component set.
func checkComponents(components []Component) error {
	seen := make(map[ComponentID]struct{}, len(components))

	for _, c := range components {
		id := c.ID()
		if _, dup := seen[id]; dup {
			return fmt.Errorf("%w: %q", ErrDuplicateComponent, id)
		}

		seen[id] = struct{}{}
	}

	return nil
}
