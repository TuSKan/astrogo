package astrogo

import (
	"time"

	"github.com/TuSKan/astrogo/coords"
)

// Target unifies diverse celestial phenomena resolving structured Topocentric Altitude and
// Azimuth orientations securely mapped natively to observer limits.
type Target interface {
	// AltAz returns the Altitude and Azimuth in radians for a given time and location.
	AltAz(t time.Time, loc coords.Location) (alt float64, az float64, err error)
	// Name returns the common name or ID of the target.
	Name() string
}
