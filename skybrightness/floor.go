package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/coord"
)

// ErrBortleClass is returned when a Bortle class is outside the 1–9 scale.
var ErrBortleClass = errors.New("skybrightness: Bortle class must be in [1,9]")

// ErrUninitializedFloor is returned when a zero-value Floor (created without a
// constructor) is evaluated.
var ErrUninitializedFloor = errors.New("skybrightness: Floor has no SQM source; use NewFloorSQM/NewFloorGrid/FloorFromBortle")

// SQMGrid samples a light-pollution floor (sky-quality, mag/arcsec²) by local
// horizontal direction, modelling directional light domes such as Falchi-style
// artificial-skyglow maps projected onto the local hemisphere.
type SQMGrid interface {
	SQMAt(altaz coord.AltAz) SurfaceBrightnessV
}

// GridFunc adapts a plain function to the [SQMGrid] interface, letting callers
// supply light-pollution data sampled however they like. This package imposes
// no file format and performs no runtime downloads; ingesting a Falchi/VIIRS
// grid is the caller's responsibility.
type GridFunc func(altaz coord.AltAz) SurfaceBrightnessV

// SQMAt implements [SQMGrid].
func (g GridFunc) SQMAt(altaz coord.AltAz) SurfaceBrightnessV { return g(altaz) }

// Floor is the artificial + ambient light-pollution baseline component. It is
// static in time and may vary with direction when built from an [SQMGrid].
type Floor struct {
	grid SQMGrid
}

// NewFloorSQM creates a Floor from a single scalar sky-quality value
// (mag/arcsec²), uniform over the sky. SQM is the physical, first-class input.
func NewFloorSQM(sqm SurfaceBrightnessV) Floor {
	return Floor{grid: GridFunc(func(coord.AltAz) SurfaceBrightnessV { return sqm })}
}

// NewFloorGrid creates a Floor whose sky-quality varies with direction, sampled
// from the supplied grid.
func NewFloorGrid(grid SQMGrid) Floor { return Floor{grid: grid} }

// bortleSQM maps Bortle classes 1–9 to representative zenith sky-quality values
// (mag/arcsec²). The Bortle↔SQM correspondence is NOT standardized and varies
// substantially between sources; these are approximate midpoints provided for
// convenience only.
var bortleSQM = [10]SurfaceBrightnessV{
	1: 21.99, 2: 21.85, 3: 21.6, 4: 21.3,
	5: 20.5, 6: 19.25, 7: 18.5, 8: 18.0, 9: 17.5,
}

// FloorFromBortle creates a Floor from a Bortle dark-sky class (1–9).
//
// The mapping is LOSSY and approximate: Bortle is a qualitative naked-eye scale
// and published Bortle↔SQM tables disagree. SQM is the physical, first-class
// input — prefer [NewFloorSQM] when a measurement exists, and never round-trip
// SQM → Bortle → SQM.
func FloorFromBortle(class int) (Floor, error) {
	if class < 1 || class > 9 {
		return Floor{}, fmt.Errorf("%w: got %d", ErrBortleClass, class)
	}

	return NewFloorSQM(bortleSQM[class]), nil
}

// bortleNames are the descriptive names of the Bortle dark-sky classes (1–9),
// after Bortle (2001), Sky & Telescope.
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

// BortleClass classifies a V-band sky surface brightness onto the Bortle
// dark-sky scale, returning the nearest class (1 = excellent dark sky,
// 9 = inner-city) and its descriptive name.
//
// This is a lossy, approximate convenience LABEL for a computed brightness —
// the Bortle↔SQM correspondence is not standardized. Unlike a measured SQM, the
// returned class must NOT be fed back through [FloorFromBortle].
func BortleClass(sb SurfaceBrightnessV) (class int, name string) {
	best := 1
	bestDiff := math.Abs(float64(sb) - float64(bortleSQM[1]))

	for c := 2; c <= 9; c++ {
		if d := math.Abs(float64(sb) - float64(bortleSQM[c])); d < bestDiff {
			best, bestDiff = c, d
		}
	}

	return best, bortleNames[best]
}

// Radiance returns the floor's linear radiance toward altaz. The floor is
// time-independent, so ctx is ignored.
func (f Floor) Radiance(altaz coord.AltAz, _ *coord.Context) (Nanolambert, error) {
	if f.grid == nil {
		return 0, ErrUninitializedFloor
	}

	return f.grid.SQMAt(altaz).Nanolamberts(), nil
}
