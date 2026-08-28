package skybrightness

import (
	"context"
	"errors"
	"fmt"
	"math"

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

	// ErrPresetMismatch is returned when a model built by [NewPreset] is
	// asked to evaluate under a transfer that is not the one the preset
	// names.
	ErrPresetMismatch = errors.New("skybrightness: query does not match the preset")
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

	// field is an incoming field already sampled over the hemisphere, for
	// reference fidelity. Unexported because it is an optimisation SkyMap
	// applies to itself rather than a parameter of the question being asked:
	// the answer is identical with or without it, only the cost differs.
	field *hemisphereField
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

	// preset is the preset this model was built from, empty when it was
	// assembled by hand. It is kept so that [Model.Estimate] can tell whether
	// the transfer it is being asked to evaluate under is the one the preset
	// is defined by; see checkPreset.
	preset Preset
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

// Preset reports the preset this model was built from, and whether it had
// one. A model assembled by [NewModel] has none.
func (m *Model) Preset() (Preset, bool) { return m.preset, m.preset != "" }

// WithoutPreset returns a copy of this model that no longer knows which preset
// built it, so [Model.Estimate] accepts any transfer.
//
// It exists for measuring what one transfer choice is worth, which is what
// this module's own validation does: a preset evaluated with and without
// higher scattering orders, or with and without the Eq. 11 integral, is two
// runs of the same components under two transfers, and only one of the two can
// be the preset's own. Deliberately mismatching is the measurement, and
// without this there would be no way to take it except by rebuilding the
// component set by hand and hoping it stayed in step with [NewPreset].
//
// Production code reaching for this is almost certainly wrapping a mistake.
// The check it removes exists because a sky evaluated under the wrong transfer
// is smooth, positive, correctly shaped and wrong; prefer building the
// atmosphere from [Preset.DiffuseKappa] and [Preset.MultipleScattering] and
// asking at [Preset.Fidelity]. The point of a named method is that the
// exception is visible at the call site rather than absent from it.
//
// Components are shared rather than copied, which is safe because a Model is
// immutable after construction: the two evaluate identically apart from the
// check.
func (m *Model) WithoutPreset() *Model {
	return &Model{version: m.version, components: m.components}
}

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

	if err := m.checkPreset(q); err != nil {
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
		if err := m.addScatteredIn(ctx, est, q, grid, q.field, 0); err != nil {
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

// presetKappaTolerance is how far a scene's kappa may sit from the preset's
// before the two are different models.
//
// Every value a preset uses - 0.5, 0.75, 1 - is exact in binary, so a caller
// who passes [Preset.DiffuseKappa] straight through compares equal. This is
// here so that a kappa arrived at by arithmetic is not rejected over a last
// bit, and it is small enough that the differences worth catching (0.5 against
// 1 is a factor of two in the diffuse optical depth) are nowhere near it.
const presetKappaTolerance = 1e-9

// checkPreset rejects a query that would quietly evaluate a different model
// than the one the caller named.
//
// [NewPreset] builds components, and the rest of a preset lives on the
// caller's atmosphere and query: the effective-optical-depth factor kappa,
// whether higher scattering orders are on, and whether the hemispheric
// integral is run at all. Three things to remember, and forgetting any of
// them returns a plausible number rather than an error - a sky that is
// smooth, positive, correctly shaped and not the model whose name is on it.
//
// That is the failure mode this module treats as the dangerous one, and it
// has already happened once here: a placeholder star map with the right pixel
// count was served for weeks because nothing downstream could tell it from a
// real sky. A model that knows which preset built it can simply refuse, which
// is cheaper than any amount of documentation telling a caller to remember.
//
// A model from [NewModel] carries no preset and is not checked. Assembling
// components by hand is a statement that the caller owns the transfer too.
func (m *Model) checkPreset(q Query) error {
	if m.preset == "" {
		return nil
	}

	kappa, err := m.preset.DiffuseKappa()
	if err != nil {
		return err
	}

	multiple, err := m.preset.MultipleScattering()
	if err != nil {
		return err
	}

	want, err := m.preset.Fidelity()
	if err != nil {
		return err
	}

	atm := q.Scene.Atmosphere

	if math.Abs(atm.DiffuseKappa()-kappa) > presetKappaTolerance {
		return fmt.Errorf("%w: %q is defined at kappa = %g and this scene's atmosphere "+
			"carries %g; build it with atmosphere.Builder.DiffuseScattering(%g)",
			ErrPresetMismatch, m.preset, kappa, atm.DiffuseKappa(), kappa)
	}

	if atm.MultipleScattering() != multiple {
		return fmt.Errorf("%w: %q needs multiple scattering %t and this scene's atmosphere "+
			"has %t; build it with atmosphere.Builder.MultipleScattering(%t)",
			ErrPresetMismatch, m.preset, multiple, atm.MultipleScattering(), multiple)
	}

	// Fidelity is compared on what it does rather than on what it is called.
	// [Reference] runs the Eq. 11 integral and the other levels do not, so
	// that - not the label - is what has to agree. A preset needing no
	// integral has no reason to reject [Fast], which today is the same
	// evaluation as [Standard] and is a caller's own cost decision.
	//
	// Both directions are wrong and differently so. Asking a Reference preset
	// for less drops its scattering term outright. Asking a Standard preset
	// for Reference adds an integral on top of the simplified transfer already
	// inside its components, counting scattering twice.
	if (want == Reference) != (q.Fidelity == Reference) {
		if want == Reference {
			return fmt.Errorf("%w: %q is defined by the Eq. 11 scattering integral and "+
				"%s fidelity does not run it, so the scattered term would be absent; "+
				"ask for Reference",
				ErrPresetMismatch, m.preset, q.Fidelity)
		}

		return fmt.Errorf("%w: %q carries its transfer inside its components, so Reference "+
			"fidelity would add a scattering integral on top of it and count scattering "+
			"twice; ask for Standard",
			ErrPresetMismatch, m.preset)
	}

	return nil
}
