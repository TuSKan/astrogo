// Package satellite provides SGP4-based orbit propagation for Earth-orbiting
// satellites using NORAD General Perturbations (GP) element sets.
//
// # SGP4 Propagation
//
// The package wraps the go-satellite SGP4 implementation (a Go port of David
// Vallado's reference code from Spacetrack Report #3) to compute satellite
// position and velocity in the TEME (True Equator Mean Equinox) frame, then
// converts to GCRS for consistency with astrogo's [ephemeris.Provider] contract.
//
// # Usage
//
// Construct a [Satellite] from a NORAD GP element set or raw TLE lines:
//
//	sat, err := satellite.NewFromTLE(line1, line2)
//	state, err := sat.State(ephemeris.ID(0), t)
//
// For sub-satellite ground track:
//
//	geo, err := sat.SubSatellitePoint(t)
//
// # Frame Conversion
//
// SGP4 outputs TEME coordinates (km, km/s). This package converts internally:
//   - TEME → GCRS via the IAU 2006/2000A bias-precession-nutation matrix (BPN)
//   - km → AU (1 AU = 149597870.7 km)
//   - km/s → AU/day
//
// # Pass Prediction
//
// Satellite pass prediction over an observer site is provided in the
// [plan] package via [plan.SatellitePasses].
package satellite
