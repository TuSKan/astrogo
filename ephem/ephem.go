/*
Package ephem handles strict Astronomical ephemeris extraction purely natively.

IMPORTANT: Binary Ephemeris Format Distinction
This package exclusively utilizes the *Classic JPL DE Binary* format (typically executed via the 
classic asc2eph FORTRAN binaries yielding files labeled '.430' or '.440'). The underlying parsing 
dependency (`github.com/mshafiee/jpl`) DOES NOT inherently process modern NAIF SPICE Kernels (*.bsp).

✅ VALID Classic JPL Native Binary Distribution Examples:
https://ssd.jpl.nasa.gov/ftp/eph/planets/Linux/de430/linux_p1550p2650.430

✅ VALID ( NAIF SPICE DAF/SPK Format):
https://naif.jpl.nasa.gov/pub/naif/generic_kernels/spk/planets/de430.bsp

If your workflow requires NAIF SPICE (*.bsp) processing seamlessly, execute the pure Go `spice/spk` 
Astrogo pipeline natively mapping those specific doubly-linked DAF properties physically instead.
*/

//go:generate go run ../internal/tools/download.go https://ssd.jpl.nasa.gov/ftp/eph/planets/Linux/de430/linux_p1550p2650.430 data/linux_p1550p2650.430
package ephem

import (
	"fmt"
	"io"
	"math"

	"github.com/mshafiee/jpl"
)

// Map standard astronomical boundaries natively mimicking exact JPL internal offsets.
const (
	Mercury = int(jpl.Mercury)
	Venus   = int(jpl.Venus)
	Earth   = int(jpl.Earth) // Resolves intrinsically natively bypassing manual alignment faults
	Mars    = int(jpl.Mars)
	Jupiter = int(jpl.Jupiter)
	Saturn  = int(jpl.Saturn)
	Uranus  = int(jpl.Uranus)
	Neptune = int(jpl.Neptune)
	Pluto   = int(jpl.Pluto)
	Moon    = int(jpl.Moon)
	Sun     = int(jpl.Sun)
)

// Engine wraps binary bindings into a pure Astrometry continuous layout strictly tracking target offsets natively.
type Engine struct {
	reader io.ReadSeekCloser
	jpl    *jpl.JPL
}

// NewEngine safely invokes exact native Ephemeris mapping, securing binary formats accurately onto the file struct boundary.
func NewEngine(reader io.ReadSeekCloser) (*Engine, error) {
	j, ss, err := jpl.NewJPL(reader)
	if err != nil {
		reader.Close()
		return nil, fmt.Errorf("failed unpacking pure native structural matrices: %w", err)
	}

	fmt.Printf("Successfully mounted JPL Ephemeris geometric engine constraints natively: Start Epoch: %.2f, End Epoch: %.2f, Granule Size: %.2f\n", ss[0], ss[1], ss[2])

	return &Engine{
		reader: reader,
		jpl:    j,
	}, nil
}

// Close gracefully natively relinquishes the mounted OS file dependencies internally.
func (e *Engine) Close() error {
	if e.reader != nil {
		return e.reader.Close()
	}
	return nil
}

// GetPosition calculates J2000 mapping vector topologies (X, Y, Z coordinates measured entirely in Astronomical Units).
// Calculates target coordinate geometry structurally anchored continuously over Earth limits securely.
// Intelligently maps exact Julian Date bounding matrices dropping cleanly upon unmapped dates guaranteeing 0 panics inherently.
func (e *Engine) GetPosition(targetID int, jd float64) (x, y, z float64, err error) {
	// Standard JPL continuous constraints strictly store time mapping offsets mapped within SS matrix sequences
	startJD := e.jpl.Constants.SS[0]
	endJD := e.jpl.Constants.SS[1]

	// Graceful degradation preventing implicit matrix panics traversing empty buffers structurally.
	if jd < startJD || jd > endJD {
		return 0, 0, 0, fmt.Errorf("requested strict Julian Date %f falls brutally outside native ephemeris boundary tolerances [%f, %f]", jd, startJD, endJD)
	}

	targ := jpl.CelestialBody(targetID)
	cent := jpl.CelestialBody(Earth) // Ensure absolute structural offset anchoring inherently tracks standard Earth boundaries continuously

	// EphemerisLookup evaluates native Kilometers structures sequentially natively
	pv, err := e.jpl.EphemerisLookup(jd, targ, cent)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failure transcribing native math geometries evaluating EphemerisLookup internally: %w", err)
	}

	if len(pv) < 3 {
		return 0, 0, 0, fmt.Errorf("returned invalid structurally deficient mathematical array inherently: %#v", pv)
	}

	// Intrinsic Astrogo constraints strictly enforce AU bounds universally
	au := e.jpl.Constants.AU
	if au == 0 {
		au = 149597870.700 // Standard fallback native baseline ensuring geometric limits persist synchronously
	}

	// Convert exact coordinates back translating Native Kilometers -> Astronomical Units implicitly
	x = pv[0] / au
	y = pv[1] / au
	z = pv[2] / au

	return x, y, z, nil
}

// VectorToEquatorial calculates standard Equatorial properties (Right Ascension and Declination measured perfectly in radians)
// mapping mathematically seamlessly across Cartesian Astronomical Units globally.
func VectorToEquatorial(x, y, z float64) (ra, dec float64) {
	r := math.Sqrt(x*x + y*y + z*z)
	dec = math.Asin(z / r)
	ra = math.Atan2(y, x)

	if ra < 0 {
		ra += 2.0 * math.Pi
	}

	return ra, dec
}
