// Package constants provides a small set of physical and astronomical constants
// for use across astrogo.
//
// # Scope
//
// Only stable, canonical constants live here — values that are defined by
// international standards bodies (IAU, BIPM, NIST) or by exact SI definition
// and are unlikely to change between astrogo releases.
//
// Constants that are model-dependent, epoch-dependent, or subject to periodic
// IERS/IAU revisions (such as TT-TDB coefficients, nutation series terms, or
// planetary masses) belong in the packages that use them, not here.
//
// # Units
//
// All constants use SI base units (metres, kilograms, seconds) unless the name
// explicitly states otherwise. Angular constants are dimensionless scale factors
// between radians and other angle units.
package constants
