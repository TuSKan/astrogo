package skybrightness

import (
	"context"
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// Sentinel errors for model construction and evaluation.
var (
	// ErrNoGrid is returned when a query has an unusable spectral grid.
	ErrNoGrid = errors.New("skybrightness: query needs a valid spectral grid")

	// ErrComponentFailed wraps a component's own error, naming which one
	// failed so a caller is not left guessing across a dozen terms.
	ErrComponentFailed = errors.New("skybrightness: component failed")
)

// Fidelity selects how much computation a caller is willing to spend. All
// levels share this same API and the same physics; they differ in
// approximation, not in model.
//
// A scheduler evaluating thousands of candidate observations cannot run a
// reference calculation for each, so the levels exist to make that
// trade-off explicit and recorded rather than hidden in a configuration
// file.
type Fidelity uint8

const (
	// Fast permits lookup tables, cached natural sky, spectral basis
	// compression, surrogate kernels and reduced angular or spectral
	// resolution. It does not permit different, undocumented physics: a
	// surrogate must be generated from, and measured against, the model it
	// stands in for.
	Fast Fidelity = iota

	// Standard is the native semi-analytic model, the default for normal
	// operation.
	Standard

	// Reference is the highest available fidelity: fine spectral grids,
	// detailed atmospheric calculation, precomputed radiative transfer
	// where available, minimal approximation.
	Reference
)

// String renders the fidelity level.
func (f Fidelity) String() string {
	switch f {
	case Fast:
		return "fast"
	case Standard:
		return "standard"
	case Reference:
		return "reference"
	default:
		return fmt.Sprintf("Fidelity(%d)", uint8(f))
	}
}

// Query is one evaluation request: where to look, under what scene, at
// what fidelity and on which spectral grid.
type Query struct {
	// Scene is the physical state. Required.
	Scene *Scene

	// Direction is the viewing direction in the observer's horizontal
	// frame.
	Direction coord.AltAz

	// Grid is the spectral axis. Zero value means DefaultOpticalGrid.
	Grid unit.SpectralGrid

	// Fidelity selects the computation level.
	Fidelity Fidelity

	// Components restricts evaluation to these IDs. Empty means all
	// registered components.
	Components []ComponentID
}

// grid returns the query's grid, defaulted.
func (q Query) grid() unit.SpectralGrid {
	if q.Grid.Validate() != nil {
		return DefaultOpticalGrid()
	}

	return q.Grid
}

// Model sums components into a sky radiance. It is the assembled engine a
// caller evaluates against.
//
// A Model is immutable after construction and safe for concurrent use, so
// a scheduler can share one across goroutines.
type Model struct {
	version    string
	components []Component
}

// NewModel assembles a model from components. Duplicate component IDs are
// rejected: two terms claiming to be the same physical contribution would
// double-count it.
//
// A model with no components is legal and is the Phase 0 state. It
// evaluates to zero radiance and flags itself NoComponents rather than
// pretending to be a dark sky.
func NewModel(version string, components ...Component) (*Model, error) {
	if err := checkComponents(components); err != nil {
		return nil, err
	}

	owned := make([]Component, len(components))
	copy(owned, components)

	return &Model{version: version, components: owned}, nil
}

// Version reports the model version string.
func (m *Model) Version() string { return m.version }

// Components reports the registered component IDs, in evaluation order.
func (m *Model) Components() []ComponentID {
	out := make([]ComponentID, len(m.components))
	for i, c := range m.components {
		out[i] = c.ID()
	}

	return out
}

// Estimate evaluates the sky in one direction.
//
// It performs no I/O and makes no network calls: every input it needs is
// already in the Scene, so a given scene and dataset version always
// produce the same answer.
func (m *Model) Estimate(ctx context.Context, q Query) (*Estimate, error) {
	if err := q.Scene.Validate(); err != nil {
		return nil, err
	}

	grid := q.grid()
	if err := grid.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoGrid, err)
	}

	wanted := q.selected()

	est := &Estimate{
		grid:       grid,
		total:      NewSpectralRadiance(grid),
		components: make(map[ComponentID]SpectralRadiance),
		Reproducibility: Reproducibility{
			ModelVersion: m.version,
			Fidelity:     q.Fidelity,
			Grid:         grid.String(),
		},
	}

	var evaluated int

	for _, c := range m.components {
		if wanted != nil {
			if _, ok := wanted[c.ID()]; !ok {
				continue
			}
		}

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("skybrightness: estimate: %w", err)
		}

		buf := NewSpectralRadiance(grid)

		flags, err := c.AddRadiance(ctx, buf, grid, q.Direction, q.Scene)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %w", ErrComponentFailed, c.ID(), err)
		}

		est.Quality.Add(flags)

		// Validate per component, not just on the total: a negative term
		// cancelled by a positive one would otherwise pass unnoticed, and
		// the point of the check is to name which component is wrong.
		if err := buf.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %q: %w", ErrComponentFailed, c.ID(), err)
		}

		if err := est.total.Add(buf); err != nil {
			return nil, fmt.Errorf("%w: %q: %w", ErrComponentFailed, c.ID(), err)
		}

		est.components[c.ID()] = buf
		est.Reproducibility.Components = append(est.Reproducibility.Components, c.Provenance())
		est.Reproducibility.Datasets = append(est.Reproducibility.Datasets, c.Provenance().Datasets...)
		evaluated++
	}

	// Reference fidelity is the full scattering model.
	//
	// The components have written their direct radiance, which under a scene at
	// kappa = 1 is the L_d of Masana et al. (2024) Eq. 8. This adds the L_s
	// that completes it: the hemispheric integral of Eq. 11, evaluated per
	// component so a breakdown still attributes scattered light to whatever
	// supplied it.
	//
	// It costs one evaluation of every component in about nine hundred
	// directions, so a reference estimate is roughly three orders of magnitude
	// dearer than a standard one. That is the trade the paper describes and the
	// reason its own web service does not make it.
	if q.Fidelity == Reference && evaluated > 0 {
		if err := m.addScatteredIn(ctx, est, q, grid, 0); err != nil {
			return nil, err
		}
	}

	if evaluated == 0 {
		est.Quality.Add(NoComponents)
	}

	return est, nil
}

// selected returns the component filter, or nil for "everything".
func (q Query) selected() map[ComponentID]struct{} {
	if len(q.Components) == 0 {
		return nil
	}

	out := make(map[ComponentID]struct{}, len(q.Components))
	for _, id := range q.Components {
		out[id] = struct{}{}
	}

	return out
}
