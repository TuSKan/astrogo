package skybrightness

import (
	"fmt"

	"github.com/TuSKan/astrogo/coord"
)

// Component is one additive contributor to the sky brightness toward a pointing
// (e.g. the light-pollution floor, scattered moonlight, zodiacal light, or
// airglow). Implementations return a LINEAR radiance; a [CompositeModel] sums
// the components in linear flux space (see [Nanolambert]).
type Component interface {
	// Radiance returns the component's linear sky radiance toward altaz at the
	// epoch carried by ctx. Components that do not depend on direction or time
	// may ignore those arguments.
	Radiance(altaz coord.AltAz, ctx *coord.Context) (Nanolambert, error)
}

// Model returns the total sky surface brightness toward a horizontal pointing
// at the epoch carried by ctx. Implementations that combine multiple
// [Component] values MUST sum them in linear flux space.
type Model interface {
	SurfaceBrightness(altaz coord.AltAz, ctx *coord.Context) (SurfaceBrightnessV, error)
}

// CompositeModel is a [Model] that sums an ordered set of
// [Component] radiances in linear flux space (nanolamberts) and converts the
// total to a V-band surface brightness only at the end. Summing is allocation
// free.
type CompositeModel struct {
	components []Component
}

// NewCompositeModel creates a composite model from the given components. Running
// a cheap "floor only" model versus a full "floor + moonlight + zodiacal +
// airglow" model is purely a matter of which components are supplied.
func NewCompositeModel(components ...Component) *CompositeModel {
	return &CompositeModel{components: components}
}

// Add appends a component and returns the model for chaining.
func (m *CompositeModel) Add(c Component) *CompositeModel {
	m.components = append(m.components, c)

	return m
}

// SurfaceBrightness returns the total sky surface brightness toward altaz by
// summing the component radiances in linear flux space. An empty model returns
// an infinitely faint sky (+Inf mag).
func (m *CompositeModel) SurfaceBrightness(altaz coord.AltAz, ctx *coord.Context) (SurfaceBrightnessV, error) {
	var total Nanolambert

	for _, c := range m.components {
		r, err := c.Radiance(altaz, ctx)
		if err != nil {
			return 0, fmt.Errorf("skybrightness: component radiance: %w", err)
		}

		total += r
	}

	return total.SurfaceBrightnessV(), nil
}

// AsModel adapts a single [Component] to a [Model] (e.g. to use a bare [Floor]
// as a cheap model).
func AsModel(c Component) Model { return NewCompositeModel(c) }
