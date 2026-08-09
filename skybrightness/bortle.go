package skybrightness

import (
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// bortleZeroPointCdM2 relates a V mag/arcsec^2 value to photopic luminance:
// m = -2.5*log10(L_cdm2 / bortleZeroPointCdM2). Carried from astrogo v1's
// own SurfaceBrightnessFromMcdM2 zero point (1.08e8 mcd/m^2), converted to
// cd/m^2 (divide by 1000), so this stays the same physical anchor v1 used
// rather than a freshly invented one.
const bortleZeroPointCdM2 = 1.08e5

// bortleAnchorMag maps Bortle classes 1-9 to representative zenith V
// mag/arcsec^2 values, carried verbatim from astrogo v1. The Bortle<->SQM
// correspondence is NOT standardized and varies substantially between
// sources; these are approximate midpoints, for reporting only.
var bortleAnchorMag = [10]float64{
	1: 21.99, 2: 21.85, 3: 21.6, 4: 21.3,
	5: 20.5, 6: 19.25, 7: 18.5, 8: 18.0, 9: 17.5,
}

// bortleNames are the descriptive names of the Bortle dark-sky classes
// (1-9), after Bortle (2001), Sky & Telescope.
var bortleNames = [10]string{
	1: "excellent dark-sky site",
	2: "typical truly dark site",
	3: "rural sky",
	4: "rural/suburban transition",
	5: "suburban sky",
	6: "bright suburban sky",
	7: "suburban/urban transition",
	8: "city sky",
	9: "inner-city sky",
}

// BortleFromLuminance classifies a photopic zenith luminance onto the
// Bortle dark-sky scale, returning the nearest class (1 = excellent dark
// sky, 9 = inner-city) and its descriptive name.
//
// This is deliberately one-way and output-only — there is no
// BortleToLuminance in this package. Bortle is a lossy, human-authored,
// non-invertible qualitative descriptor (v1 already documented this;
// docs/skybrightness.md §15 explains why the reverse direction is removed
// entirely rather than merely discouraged): using a Bortle class as a
// model *input* would silently discard precision the rest of the engine
// worked to preserve. Report a class for a human reader; never feed one
// back into an atmosphere.Atmosphere or emission model.
func BortleFromLuminance(l unit.LuminanceCdM2) (class int, name string) {
	mag := math.Inf(1)
	if l > 0 {
		mag = -2.5 * math.Log10(float64(l)/bortleZeroPointCdM2)
	}

	best := 1
	bestDiff := math.Abs(mag - bortleAnchorMag[1])

	for c := 2; c <= 9; c++ {
		if d := math.Abs(mag - bortleAnchorMag[c]); d < bestDiff {
			best, bestDiff = c, d
		}
	}

	return best, bortleNames[best]
}
