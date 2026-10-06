// Package satellite provides SGP4-based orbit propagation for Earth-orbiting
// satellites using NORAD General Perturbations (GP) element sets.
//
// # SGP4 Propagation
//
// The model is astrogo's own,
// [github.com/TuSKan/astrogo/ephemeris/satellite/sgp4], written from Vallado,
// Crawford, Hujsak & Kelso (2006), "Revisiting Spacetrack Report #3" (AIAA
// 2006-6753). It computes position and velocity in TEME (true equator, mean
// equinox), and this package converts them to GCRS so that a [Satellite]
// answers the same contract as any other
// [github.com/TuSKan/astrogo/ephemeris.Provider].
//
// # If your timestamp came from a receiver, say so
//
// [Satellite.State] takes an astrogo time.Time, which carries its own scale, and
// a GNSS receiver does not hand out UTC. GPS and Galileo system time run 18
// seconds ahead of it today, because they were synchronised once in 1980 and
// told to ignore leap seconds since.
//
// Passing that timestamp as UTC puts the satellite that far along its track,
// which for the ISS is 138 km. The offset is ΔAT − 19, so it is not a constant
// a caller can subtract once and remember: it was 14 s in 2008 and stepped to 18
// at the 2017 leap second. Build the instant with
// [github.com/TuSKan/astrogo/time.GPST] as its scale and the arithmetic happens
// for you, at whatever epoch:
//
//	t := time.FromJD(gpsJD, time.GPST)   // not time.UTC
//	state, err := sat.State(0, t)
//
// BeiDou is a different offset again — TAI − 33 s, so BDT − UTC is 4 s today —
// and has [github.com/TuSKan/astrogo/time.BDT]. GLONASS is UTC plus three hours
// with leap seconds applied, so it needs no scale of its own.
//
// # Accuracy, and what it is a statement about
//
// Against the reference states Vallado published with the paper, the model
// agrees to 4.1e-6 km at worst, over 666 states in 31 cases, and every build
// asserts it to 1e-4 km. The suite is built to exercise the hard regimes:
// perigees below 220 km, where SGP4 switches to simplified drag, the
// deep-space SDP4 branch that 24 of the 31 cases take, and objects in their
// last stage of decay. The Go implementation this package wrapped before
// disagreed with the same suite on seven cases, by up to 3438 km
// (TuSKan/astrogo#309); all seven now agree.
//
// That is agreement with the reference implementation, which is a statement
// about this code and not about where the satellite is. SGP4's accuracy
// against the real orbit is kilometers, and it degrades with time from the
// element set's epoch; no implementation of the model improves on that.
//
// What the model does with a given element set is on [Satellite.Propagator]:
// [sgp4.Propagator.SimplifiedDrag], [sgp4.Propagator.DeepSpace] and
// [sgp4.Propagator.PerigeeAltitude] report which branches it takes and the
// perigee it branches on.
//
// # Usage
//
// Construct a [Satellite] from a name and the two lines of a TLE:
//
//	sat, err := satellite.NewFromTLE("ISS", line1, line2)
//	state, err := sat.State(0, t)  // geocentric GCRS, au and au/day
//	h, err := sat.Altitude(t)      // above the WGS84 ellipsoid
//
// # Frame Conversion
//
// SGP4 outputs TEME coordinates (km, km/s). [Satellite.State] converts them:
//   - TEME → true equator and equinox of date, by the equation of the
//     equinoxes
//   - true of date → GCRS, by the transpose of the IAU 2006/2000A
//     bias-precession-nutation matrix
//   - km → au and km/s → au/day, with the astronomical unit from
//     [github.com/TuSKan/astrogo/constants]
//
// # Pass Prediction
//
// Satellite pass prediction over an observer site is
// [github.com/TuSKan/astrogo/plan.SatellitePasses].
package satellite
