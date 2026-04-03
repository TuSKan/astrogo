package frame

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
)

// Frame is the interface for all coordinate reference systems.
// It provides a human-readable name for the system.
type Frame interface {
	fmt.Stringer
	Name() string
}

// ── Frame Identities ─────────────────────────────────────────────────────────

// ICRS represents the International Celestial Reference System.
// It is a space-fixed, barycentric frame that is essentially independent
// of epoch for most applications.
type ICRS struct{}

func (f ICRS) Name() string   { return "ICRS" }
func (f ICRS) String() string { return "ICRS" }

// Galactic represents the IAU 1958 Galactic coordinate system.
type Galactic struct{}

func (f Galactic) Name() string   { return "Galactic" }
func (f Galactic) String() string { return "Galactic" }

// Ecliptic represents a geocentric ecliptic coordinate system.
type Ecliptic struct {
	Equinox time.Time // The epoch of the mean equinox and ecliptic.
}

func (f Ecliptic) Name() string { return "Ecliptic" }
func (f Ecliptic) String() string {
	return fmt.Sprintf("Ecliptic(Equinox=%s)", f.Equinox)
}

// AltAz represents a local horizontal coordinate system.
// It is defined by the observer's location and the time of observation.
type AltAz struct {
	Time     time.Time
	Location ObserversLocation // Simplified location metadata
}

func (f AltAz) Name() string { return "AltAz" }
func (f AltAz) String() string {
	return fmt.Sprintf("AltAz(Time=%s, Lat=%s, Lon=%s)",
		f.Time, f.Location.Lat.DMSString(0), f.Location.Lon.DMSString(0))
}

// ── Metadata Structures ───────────────────────────────────────────────────────

// ObserversLocation carries the minimal terrestrial metadata needed for
// topocentric frames without depending on the full `earth` package.
type ObserversLocation struct {
	Lon    angle.Angle
	Lat    angle.Angle
	Height float64 // Meters
}

// ── Frame Sentinels ───────────────────────────────────────────────────────────

var (
	ICRSFrame     = ICRS{}
	GalacticFrame = Galactic{}
	EclipticFrame = Ecliptic{}
	AltAzFrame    = AltAz{}
)

// ── Equality Helpers ─────────────────────────────────────────────────────────

// Equals reports whether two frames represent the same coordinate system
// with identical metadata.
func Equals(a, b Frame) bool {
	if a == b {
		return true
	}
	// Use type-specific comparisons for metadata-heavy frames.
	switch fa := a.(type) {
	case Ecliptic:
		fb, ok := b.(Ecliptic)
		return ok && fa.Equinox.Equal(fb.Equinox)
	case AltAz:
		fb, ok := b.(AltAz)
		return ok && fa.Time.Equal(fb.Time) &&
			fa.Location.Lat == fb.Location.Lat &&
			fa.Location.Lon == fb.Location.Lon &&
			fa.Location.Height == fb.Location.Height
	}
	return a.Name() == b.Name()
}
