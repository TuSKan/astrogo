package skybrightness

import "github.com/TuSKan/astrogo/coord"

// defaultAirglowV is the default dark-sky airglow + diffuse-background floor in
// V mag/arcsec². ~21.9 is a representative true-night, high-altitude dark-sky
// value consistent with the Cerro Paranal sky model (Noll et al. 2012) and
// Patat (2008). Airglow varies with solar activity, zenith distance, and time
// of night; this component models only a constant floor.
const defaultAirglowV = 21.9

// Airglow is the airglow + diffuse-starlight floor component. It is modelled as
// a constant V-band surface brightness, independent of direction and time.
type Airglow struct {
	sb SurfaceBrightnessV
}

// NewAirglow creates an Airglow component using the default dark-sky floor
// (~21.9 V mag/arcsec²).
func NewAirglow() Airglow { return Airglow{sb: defaultAirglowV} }

// NewAirglowSB creates an Airglow component with a caller-specified constant
// V-band surface brightness (mag/arcsec²).
func NewAirglowSB(sb SurfaceBrightnessV) Airglow { return Airglow{sb: sb} }

// Radiance returns the airglow floor radiance. It is independent of direction
// and time, so altaz and ctx are ignored.
func (a Airglow) Radiance(_ coord.AltAz, _ *coord.Context) (Nanolambert, error) {
	sb := a.sb
	if sb == 0 {
		sb = defaultAirglowV // zero-value Airglow falls back to the default floor
	}

	return sb.Nanolamberts(), nil
}
