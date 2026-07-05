package skybrightness

import "fmt"

// SQMProvider resolves the ARTIFICIAL-only zenith sky surface brightness
// (mag/arcsec²) at a ground location, identified by geodetic latitude and
// longitude in degrees. It is the geographic counterpart of the in-sky [Floor]:
// a provider answers "how light-polluted is this place?", a Floor answers "how
// bright is the floor toward this direction?".
//
// Implementations MUST return the artificial component only — the natural
// background (airglow, zodiacal light, moonlight) is supplied by the model's
// other [Component] values, so folding a fixed natural term into the provider
// would double-count it (see the package docs). Providers must not perform IO
// at call time beyond what their constructor already arranged.
type SQMProvider interface {
	ZenithBrightness(latDeg, lonDeg float64) (SurfaceBrightnessV, error)
}

// scalarProvider returns one constant zenith brightness everywhere.
type scalarProvider struct {
	sqm SurfaceBrightnessV
}

// NewScalarProvider returns an [SQMProvider] that reports a single, uniform
// artificial zenith brightness regardless of location. This is the cheap,
// dependency-free path — the equivalent of a planetarium's single
// light-pollution slider — when no atlas is available.
func NewScalarProvider(sqm SurfaceBrightnessV) SQMProvider {
	return scalarProvider{sqm: sqm}
}

// ZenithBrightness implements [SQMProvider].
func (p scalarProvider) ZenithBrightness(_, _ float64) (SurfaceBrightnessV, error) {
	return p.sqm, nil
}

// NewFloorFromProvider resolves the artificial zenith brightness at the given
// ground location and returns a uniform (direction-independent) [Floor] for it.
// This bridges a geographic [SQMProvider] (e.g. an atlas loader) to the in-sky
// [Component] model. Off-zenith variation is not modelled — see [NewFloorGrid]
// for a directional floor.
func NewFloorFromProvider(p SQMProvider, latDeg, lonDeg float64) (Floor, error) {
	sqm, err := p.ZenithBrightness(latDeg, lonDeg)
	if err != nil {
		return Floor{}, fmt.Errorf("skybrightness: resolve floor: %w", err)
	}

	return NewFloorSQM(sqm), nil
}
