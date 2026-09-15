// Package sgp4 propagates NORAD two-line element sets with the SGP4/SDP4
// analytical orbit model.
//
// # What SGP4 is, and what it is not
//
// SGP4 is not a general orbit propagator that happens to take TLEs. It is one
// half of a matched pair: Space-Track fits mean elements to observations *by
// running this same model*, and publishes the elements. Those numbers mean
// nothing on their own — they are not osculating elements, and converting them
// to a Cartesian state by two-body formulae gives a wrong answer. They mean
// "the input that makes SGP4 reproduce where we saw it", so only SGP4, built
// the same way, gets that answer back.
//
// Three consequences follow, and all three are easy to get wrong:
//
//   - The gravity model is part of the fit. TLEs are fitted with WGS-72, so
//     [WGS72] is the default here. Running them through WGS-84 constants is a
//     model mismatch, not a refinement; measured against Vallado's own
//     reference suite it cost astrogo 93 times the position error.
//   - The output frame is TEME (true equator, mean equinox), which is SGP4's
//     own frame and not one anybody else uses. Converting it to GCRS is the
//     caller's job — [github.com/TuSKan/astrogo/ephemeris/satellite] does it.
//   - The accuracy is kilometres, and it degrades with time from epoch. SGP4
//     agrees with the reference implementation to millimetres, which is a
//     statement about this code, not about where the satellite is.
//
// # Provenance
//
// Written from the algorithm as published, not ported from another
// implementation:
//
//   - Vallado, Crawford, Hujsak & Kelso (2006), "Revisiting Spacetrack Report
//     #3", AIAA 2006-6753, and the C++ released with it.
//   - Hoots & Roehrich (1980), Spacetrack Report #3, the original.
//
// Where Vallado's code deliberately departs from Spacetrack Report #3 he marks
// it `sgp4fix` and says why. Every one of those carries its reason into the Go
// comment here, because they are the parts a later reader will otherwise
// "correct" back into a bug.
//
// Correctness is measured against the reference states Vallado published with
// the paper, checked into
// [github.com/TuSKan/astrogo/ephemeris/satellite]/testdata/vallado.
//
// # Elements in, elements out
//
// The propagator takes an [Elements] value, not two lines of text. A TLE is
// one serialization of a mean element set and [ParseTLE] reads it, but an OMM
// or a catalogue row describes the same thing, and none of them should have to
// become a TLE first.
package sgp4
