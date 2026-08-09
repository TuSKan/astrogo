package skybrightness

import (
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
)

// ClimatologyDefaultAtmosphere returns a deterministic, offline,
// site-elevation-aware default atmosphere.Atmosphere: no aerosol, no clouds,
// pressure/temperature from the ICAO ISA barometric profile at the
// site's height, zero surface albedo. This is ModeClimatology's baseline
// — a future phase replaces the aerosol/cloud fields with a real
// climatological dataset behind the same constructor shape. Which default
// atmosphere to use for a Request that supplied none is a skybrightness
// policy decision, not general atmosphere-package behavior, which is why
// this lives here rather than in package atmosphere alongside
// atmosphere.StandardDefault (the plain ISA-profile constructor this
// wraps).
func ClimatologyDefaultAtmosphere(site *coord.Geodetic) *atmosphere.Atmosphere {
	h := 0.0
	if site != nil {
		h = site.Height()
	}

	return atmosphere.StandardDefault(h)
}
