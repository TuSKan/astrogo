// Package satellite provides SGP4-based orbit propagation for Earth-orbiting
// satellites using NORAD General Perturbations (GP) element sets.
//
// # SGP4 Propagation
//
// The package wraps the go-satellite SGP4 implementation (a Go port of David
// Vallado's reference code from Spacetrack Report #3) to compute satellite
// position and velocity in the TEME (True Equator Mean Equinox) frame, then
// converts to GCRS for consistency with astrogo's [eph.Provider] contract.
//
// # Accuracy, and where it does not hold
//
// Measured against Vallado's own verification suite (AIAA 2006-6753), 588
// states across 30 element sets: twenty-two of those cases agree to a median
// of 35 m and a maximum of 289 m. That covers ordinary orbits — the ISS,
// Sun-synchronous imaging satellites, navigation constellations.
//
// Eight do not agree, by between 0.6 km and 3440 km. They are not scattered:
// they are the cases Vallado built the suite to exercise, namely satellites
// whose perigee falls below about 220 km (where SGP4 switches to a modified
// drag formulation), those propagated through the deep-space SDP4 branch, and
// those in the last stage of decay. Each is exact at the element epoch and
// diverges quadratically with time, which is the signature of a wrong secular
// drag term in the propagator rather than of anything this package does to it.
//
// The backend's own test suite covers six of the thirty-three published cases
// and none of the eight that fail, which is how an implementation with this
// defect passes its own verification.
//
// Nothing here reports which regime a given element set falls in — a position
// for a decaying rocket body comes back looking exactly like one for the ISS.
// Until that changes, treat a result for a low-perigee, deep-space or decaying
// object as unvalidated. See ephemeris/satellite/sgp4_vallado_validation_test.go,
// which measures all of the above on every run of the validation tier, and
// https://github.com/TuSKan/astrogo/issues/120.
//
// # Usage
//
// Construct a [Satellite] from a NORAD GP element set or raw TLE lines:
//
//	sat, err := satellite.NewFromTLE(line1, line2)
//	state, err := sat.State(eph.ID(0), t)
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
