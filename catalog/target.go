package catalog

import (
	"time"

	"github.com/TuSKan/astrogo/coords"
)

// AltAz dynamically maps target Right Ascension and Declination internally evaluating Topocentric alignments natively.
func (d DeepSkyTarget) AltAz(t time.Time, loc coords.Location) (float64, float64, error) {
	// 0.0 dut1 as standard baseline internally handled natively
	return coords.ICRSToObserved(d.RA, d.Dec, t, loc, 0.0)
}

// Name natively retrieves internal ID bindings serving the Target interface cleanly.
func (d DeepSkyTarget) Name() string {
	return d.ID
}
