package coords

import (
	"fmt"
	"math"
	"time"

	"github.com/hebl/gofa"
)

type Location struct {
	Latitude  float64 // In degrees
	Longitude float64 // In degrees (East is positive)
	Elevation float64 // In meters above sea level
	Pressure  float64 // Ambient pressure in hPa (default: 1013.25)
	Temp      float64 // Temperature in Celsius (default: 15.0)
	Humidity  float64 // Relative humidity (default: 0.5)
}

// AltAz wraps structural mathematical matrices natively isolating coordinate geometry globally.
type AltAz struct {
	Alt float64 // In radians natively evaluated internally against boundaries
	Az  float64 // In radians natively mapped smoothly against physical planes
}

// ICRSToObserved converts J2000/ICRS Right Ascension and Declination to
// purely Topocentric (Observed) Altitude and Azimuth coordinates natively.
func ICRSToObserved(raRad, decRad float64, t time.Time, loc Location, dut1 float64) (altRad, azRad float64, err error) {
	utc := t.UTC()
	year, month, day := utc.Date()
	hour, min, sec := utc.Clock()

	// Extract standard float seconds encompassing nsec fraction
	secFloat := float64(sec) + float64(utc.Nanosecond())/1e9

	// Translate UTC time.Time into two-part Julian dates suitable for SOFA algorithms
	var d1, d2 float64
	status := gofa.Dtf2d("UTC", year, int(month), day, hour, min, secFloat, &d1, &d2)
	if status < 0 {
		return 0, 0, fmt.Errorf("invalid date/time provided, gofa.Dtf2d status: %d", status)
	}

	// Translate Location parameters (Degrees) to Radians intrinsically
	phi := loc.Latitude * (math.Pi / 180.0)
	elong := loc.Longitude * (math.Pi / 180.0)

	// Substitute sane default properties seamlessly to guarantee SOFA refraction logic tracks.
	phpa := loc.Pressure
	if phpa == 0 {
		phpa = 1013.25
	}
	tc := loc.Temp
	if tc == 0 && loc.Pressure == 0 {
		// Only substitute if pressure defaults indicating explicitly empty struct mapping.
		tc = 15.0
	}
	rh := loc.Humidity
	if rh == 0 && loc.Pressure == 0 {
		rh = 0.5
	}

	wl := 0.5 // Standard effective visual wavelength measured in micrometers

	var aob, zob, hob, dob, rob, eo float64

	// Translate ICRS RA/Dec dynamically spanning refraction parameters.
	// Parameters: RA, Dec, ProperMotionRA, ProperMotionDec, Parallax, Radial Velocity, UTC1, UTC2, DUT1,
	// Longitude, Latitude, Elevation, PolarMotionX, PolarMotionY, Pressure, Temp, Humidity, Wavelength
	statusAtco := gofa.Atco13(
		raRad, decRad,
		0.0, 0.0, 0.0, 0.0,
		d1, d2, dut1,
		elong, phi, loc.Elevation,
		0.0, 0.0,
		phpa, tc, rh, wl,
		&aob, &zob, &hob, &dob, &rob, &eo,
	)

	if statusAtco < 0 {
		return 0, 0, fmt.Errorf("gofa.Atco13 mathematically rejected the given inputs, status: %d", statusAtco)
	}

	// Extract Azimuth native geometry output
	azRad = aob

	// Translating Zenith Distance effectively to Horizon-centric Altitude geometry (Altitude = Pi/2 - ZenithDist)
	altRad = (math.Pi / 2.0) - zob

	return altRad, azRad, nil
}

// AngularSeparation wrappers gofa internally evaluating pure angular boundary extraction inherently.
// Transcribes separation geometries between isolated equatorial vectors returning native Radians efficiently.
func AngularSeparation(ra1, dec1, ra2, dec2 float64) float64 {
	return gofa.Seps(ra1, dec1, ra2, dec2)
}
